package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_ListCheckRuns(t *testing.T) {
	tests := []struct {
		name           string
		ref            string
		checkName      *string
		serverResponse interface{}
		statusCode     int
		wantCount      int
		wantErr        bool
		wantErrContain string
	}{
		{
			name:      "returns check runs for commit",
			ref:       "abc123",
			checkName: nil,
			serverResponse: checkRunsResponse{
				TotalCount: 2,
				CheckRuns: []checkRunAPI{
					{
						ID:          1001,
						Name:        "code-reviewer",
						Status:      "completed",
						Conclusion:  strPtr("success"),
						HeadSHA:     "abc123",
						StartedAt:   timePtr(time.Now().Add(-10 * time.Minute)),
						CompletedAt: timePtr(time.Now()),
						Output: checkRunOutputAPI{
							AnnotationsCount: 5,
						},
					},
					{
						ID:          1002,
						Name:        "lint",
						Status:      "completed",
						Conclusion:  strPtr("failure"),
						HeadSHA:     "abc123",
						StartedAt:   timePtr(time.Now().Add(-5 * time.Minute)),
						CompletedAt: timePtr(time.Now()),
						Output: checkRunOutputAPI{
							AnnotationsCount: 3,
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantCount:  2,
			wantErr:    false,
		},
		{
			name:      "filters by check name",
			ref:       "abc123",
			checkName: strPtr("code-reviewer"),
			serverResponse: checkRunsResponse{
				TotalCount: 1,
				CheckRuns: []checkRunAPI{
					{
						ID:     1001,
						Name:   "code-reviewer",
						Status: "completed",
						Output: checkRunOutputAPI{
							AnnotationsCount: 5,
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantCount:  1,
			wantErr:    false,
		},
		{
			name:           "empty ref returns error",
			ref:            "",
			checkName:      nil,
			serverResponse: nil,
			statusCode:     0,
			wantCount:      0,
			wantErr:        true,
			wantErrContain: "ref",
		},
		{
			name:           "handles 404",
			ref:            "nonexistent",
			checkName:      nil,
			serverResponse: errorResponse{Message: "Not Found"},
			statusCode:     http.StatusNotFound,
			wantCount:      0,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify check_name query param if provided
				if tt.checkName != nil {
					assert.Equal(t, *tt.checkName, r.URL.Query().Get("check_name"))
				}

				w.WriteHeader(tt.statusCode)
				if tt.serverResponse != nil {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.SetBaseURL(server.URL)
			client.SetMaxRetries(0)

			checkRuns, err := client.ListCheckRuns(context.Background(), "owner", "repo", tt.ref, tt.checkName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, checkRuns, tt.wantCount)

			if tt.wantCount > 0 {
				// Verify first check run was parsed correctly
				assert.NotZero(t, checkRuns[0].ID)
				assert.NotEmpty(t, checkRuns[0].Name)
			}
		})
	}
}

func TestClient_GetAnnotations(t *testing.T) {
	tests := []struct {
		name           string
		checkRunID     int64
		serverResponse interface{}
		statusCode     int
		wantCount      int
		wantErr        bool
	}{
		{
			name:       "returns annotations for check run",
			checkRunID: 1001,
			serverResponse: []annotationAPI{
				{
					Path:            "main.go",
					StartLine:       10,
					EndLine:         15,
					AnnotationLevel: "warning",
					Message:         "Unused variable",
					Title:           "unused-var",
				},
				{
					Path:            "util.go",
					StartLine:       25,
					EndLine:         25,
					AnnotationLevel: "failure",
					Message:         "Nil pointer dereference",
					Title:           "nil-deref",
				},
			},
			statusCode: http.StatusOK,
			wantCount:  2,
			wantErr:    false,
		},
		{
			name:           "returns empty for no annotations",
			checkRunID:     1001,
			serverResponse: []annotationAPI{},
			statusCode:     http.StatusOK,
			wantCount:      0,
			wantErr:        false,
		},
		{
			name:           "handles 404",
			checkRunID:     9999,
			serverResponse: errorResponse{Message: "Not Found"},
			statusCode:     http.StatusNotFound,
			wantCount:      0,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(tt.serverResponse)
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.SetBaseURL(server.URL)
			client.SetMaxRetries(0)

			annotations, err := client.GetAnnotations(context.Background(), "owner", "repo", tt.checkRunID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, annotations, tt.wantCount)

			if tt.wantCount > 0 {
				// Verify first annotation was parsed correctly
				assert.Equal(t, tt.checkRunID, annotations[0].CheckRunID)
				assert.Equal(t, 0, annotations[0].Index)
				assert.Equal(t, "main.go", annotations[0].Path)
				assert.Equal(t, domain.AnnotationLevelWarning, annotations[0].Level)
			}
		})
	}
}

func TestClient_GetAnnotation(t *testing.T) {
	annotations := []annotationAPI{
		{
			Path:            "main.go",
			StartLine:       10,
			EndLine:         15,
			AnnotationLevel: "warning",
			Message:         "First annotation",
		},
		{
			Path:            "util.go",
			StartLine:       25,
			EndLine:         25,
			AnnotationLevel: "failure",
			Message:         "Second annotation",
		},
	}

	tests := []struct {
		name      string
		index     int
		wantErr   error
		wantPath  string
		wantLevel domain.AnnotationLevel
	}{
		{
			name:      "returns first annotation",
			index:     0,
			wantErr:   nil,
			wantPath:  "main.go",
			wantLevel: domain.AnnotationLevelWarning,
		},
		{
			name:      "returns second annotation",
			index:     1,
			wantErr:   nil,
			wantPath:  "util.go",
			wantLevel: domain.AnnotationLevelFailure,
		},
		{
			name:    "returns error for out of range index",
			index:   5,
			wantErr: triage.ErrAnnotationNotFound,
		},
		{
			name:    "returns error for negative index",
			index:   -1,
			wantErr: triage.ErrAnnotationNotFound,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(annotations)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotation, err := client.GetAnnotation(context.Background(), "owner", "repo", 1001, tt.index)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, annotation)
			assert.Equal(t, tt.wantPath, annotation.Path)
			assert.Equal(t, tt.wantLevel, annotation.Level)
			assert.Equal(t, tt.index, annotation.Index)
		})
	}
}

func TestAnnotationLevelFromAPI(t *testing.T) {
	tests := []struct {
		input string
		want  domain.AnnotationLevel
	}{
		{"notice", domain.AnnotationLevelNotice},
		{"warning", domain.AnnotationLevelWarning},
		{"failure", domain.AnnotationLevelFailure},
		{"unknown", domain.AnnotationLevelNotice}, // defaults to notice
		{"", domain.AnnotationLevelNotice},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := annotationLevelFromAPI(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Helper to create pointer to time.Time
func timePtr(t time.Time) *time.Time {
	return &t
}

// Helper to create pointer to string
func strPtr(s string) *string {
	return &s
}

// API response types for tests
type checkRunsResponse struct {
	TotalCount int           `json:"total_count"`
	CheckRuns  []checkRunAPI `json:"check_runs"`
}

type checkRunAPI struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Conclusion  *string           `json:"conclusion"`
	HeadSHA     string            `json:"head_sha"`
	StartedAt   *time.Time        `json:"started_at"`
	CompletedAt *time.Time        `json:"completed_at"`
	Output      checkRunOutputAPI `json:"output"`
}

type checkRunOutputAPI struct {
	AnnotationsCount int `json:"annotations_count"`
}

type annotationAPI struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	AnnotationLevel string `json:"annotation_level"`
	Message         string `json:"message"`
	Title           string `json:"title"`
	RawDetails      string `json:"raw_details"`
}

type errorResponse struct {
	Message string `json:"message"`
}
