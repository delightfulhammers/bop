package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/triage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetPRDiff(t *testing.T) {
	tests := []struct {
		name           string
		owner          string
		repo           string
		prNumber       int
		serverResponse interface{}
		statusCode     int
		wantFileCount  int
		wantFirstFile  string
		wantFirstPatch string
		wantErr        bool
		wantErrType    error
		wantErrContain string
	}{
		{
			name:     "returns diff with single file",
			owner:    "testowner",
			repo:     "testrepo",
			prNumber: 123,
			serverResponse: []prFileAPIResponse{
				{
					Filename:  "main.go",
					Status:    "modified",
					Additions: 10,
					Deletions: 5,
					Patch:     "@@ -1,5 +1,10 @@\n+package main",
				},
			},
			statusCode:     http.StatusOK,
			wantFileCount:  1,
			wantFirstFile:  "main.go",
			wantFirstPatch: "@@ -1,5 +1,10 @@\n+package main",
			wantErr:        false,
		},
		{
			name:     "returns diff with multiple files",
			owner:    "testowner",
			repo:     "testrepo",
			prNumber: 123,
			serverResponse: []prFileAPIResponse{
				{
					Filename:  "main.go",
					Status:    "modified",
					Additions: 10,
					Deletions: 5,
					Patch:     "@@ -1,5 +1,10 @@\n+package main",
				},
				{
					Filename:  "README.md",
					Status:    "added",
					Additions: 20,
					Deletions: 0,
					Patch:     "@@ -0,0 +1,20 @@\n+# Project",
				},
			},
			statusCode:    http.StatusOK,
			wantFileCount: 2,
			wantFirstFile: "main.go",
			wantErr:       false,
		},
		{
			name:     "handles renamed file",
			owner:    "testowner",
			repo:     "testrepo",
			prNumber: 123,
			serverResponse: []prFileAPIResponse{
				{
					Filename:         "new_name.go",
					PreviousFilename: "old_name.go",
					Status:           "renamed",
					Additions:        0,
					Deletions:        0,
					Patch:            "",
				},
			},
			statusCode:    http.StatusOK,
			wantFileCount: 1,
			wantFirstFile: "new_name.go",
			wantErr:       false,
		},
		{
			name:     "detects binary file (no patch)",
			owner:    "testowner",
			repo:     "testrepo",
			prNumber: 123,
			serverResponse: []prFileAPIResponse{
				{
					Filename:  "image.png",
					Status:    "added",
					Additions: 0,
					Deletions: 0,
					Patch:     "", // Binary files have no patch
				},
			},
			statusCode:    http.StatusOK,
			wantFileCount: 1,
			wantFirstFile: "image.png",
			wantErr:       false,
		},
		{
			name:           "returns error for 404",
			owner:          "testowner",
			repo:           "testrepo",
			prNumber:       999,
			serverResponse: errorResponse{Message: "Not Found"},
			statusCode:     http.StatusNotFound,
			wantErr:        true,
			wantErrType:    triage.ErrPRNotFound,
		},
		{
			name:           "returns error for empty owner",
			owner:          "",
			repo:           "testrepo",
			prNumber:       123,
			serverResponse: nil,
			statusCode:     0,
			wantErr:        true,
			wantErrContain: "owner",
		},
		{
			name:           "returns error for empty repo",
			owner:          "testowner",
			repo:           "",
			prNumber:       123,
			serverResponse: nil,
			statusCode:     0,
			wantErr:        true,
			wantErrContain: "repo",
		},
		{
			name:           "returns error for invalid PR number",
			owner:          "testowner",
			repo:           "testrepo",
			prNumber:       0,
			serverResponse: nil,
			statusCode:     0,
			wantErr:        true,
			wantErrContain: "PR number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.serverResponse != nil {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tt.statusCode)
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}))
				defer server.Close()
			}

			client := NewClient("test-token")
			if server != nil {
				client.SetBaseURL(server.URL)
			}
			client.SetMaxRetries(0)

			diff, err := client.GetPRDiff(context.Background(), tt.owner, tt.repo, tt.prNumber)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrType != nil {
					assert.ErrorIs(t, err, tt.wantErrType)
				}
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, diff.Files, tt.wantFileCount)
			if tt.wantFileCount > 0 {
				assert.Equal(t, tt.wantFirstFile, diff.Files[0].Path)
				if tt.wantFirstPatch != "" {
					assert.Equal(t, tt.wantFirstPatch, diff.Files[0].Patch)
				}
			}
		})
	}
}

func TestClient_GetPRDiff_Pagination(t *testing.T) {
	page1Called := false
	page2Called := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			page2Called = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]prFileAPIResponse{
				{Filename: "file3.go", Status: "added", Patch: "@@ -0,0 +1 @@\n+third"},
			})
			return
		}

		page1Called = true
		// Set Link header for pagination
		w.Header().Set("Link", `<`+r.URL.Path+`?page=2>; rel="next"`)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]prFileAPIResponse{
			{Filename: "file1.go", Status: "added", Patch: "@@ -0,0 +1 @@\n+first"},
			{Filename: "file2.go", Status: "added", Patch: "@@ -0,0 +1 @@\n+second"},
		})
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)

	diff, err := client.GetPRDiff(context.Background(), "owner", "repo", 123)

	require.NoError(t, err)
	assert.True(t, page1Called, "first page should be called")
	assert.True(t, page2Called, "second page should be called")
	assert.Len(t, diff.Files, 3, "should have files from both pages")
}

func TestClient_GetPRDiff_StatusMapping(t *testing.T) {
	tests := []struct {
		name         string
		apiStatus    string
		wantStatus   string
		wantOldPath  string
		previousFile string
	}{
		{
			name:       "added maps to added",
			apiStatus:  "added",
			wantStatus: domain.FileStatusAdded,
		},
		{
			name:       "modified maps to modified",
			apiStatus:  "modified",
			wantStatus: domain.FileStatusModified,
		},
		{
			name:       "removed maps to deleted",
			apiStatus:  "removed",
			wantStatus: domain.FileStatusDeleted,
		},
		{
			name:         "renamed maps to renamed with old path",
			apiStatus:    "renamed",
			wantStatus:   domain.FileStatusRenamed,
			previousFile: "old_name.go",
			wantOldPath:  "old_name.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := []prFileAPIResponse{
				{
					Filename:         "new_name.go",
					PreviousFilename: tt.previousFile,
					Status:           tt.apiStatus,
					Patch:            "@@ test",
				},
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.SetBaseURL(server.URL)
			client.SetMaxRetries(0)

			diff, err := client.GetPRDiff(context.Background(), "owner", "repo", 123)

			require.NoError(t, err)
			require.Len(t, diff.Files, 1)
			assert.Equal(t, tt.wantStatus, diff.Files[0].Status)
			assert.Equal(t, tt.wantOldPath, diff.Files[0].OldPath)
		})
	}
}

func TestClient_GetPRDiff_BinaryDetection(t *testing.T) {
	tests := []struct {
		name       string
		patch      string
		additions  int
		deletions  int
		wantBinary bool
	}{
		{
			name:       "text file with patch is not binary",
			patch:      "@@ -1,5 +1,10 @@\n+package main",
			additions:  10,
			deletions:  5,
			wantBinary: false,
		},
		{
			name:       "file with no patch but changes is binary",
			patch:      "",
			additions:  100,
			deletions:  0,
			wantBinary: true,
		},
		{
			name:       "file with empty patch and no changes is not binary",
			patch:      "",
			additions:  0,
			deletions:  0,
			wantBinary: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := []prFileAPIResponse{
				{
					Filename:  "test.file",
					Status:    "modified",
					Patch:     tt.patch,
					Additions: tt.additions,
					Deletions: tt.deletions,
				},
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.SetBaseURL(server.URL)
			client.SetMaxRetries(0)

			diff, err := client.GetPRDiff(context.Background(), "owner", "repo", 123)

			require.NoError(t, err)
			require.Len(t, diff.Files, 1)
			assert.Equal(t, tt.wantBinary, diff.Files[0].IsBinary, "binary detection mismatch")
		})
	}
}

// prFileAPIResponse matches GitHub API response for PR files
type prFileAPIResponse struct {
	Filename         string `json:"filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Patch            string `json:"patch,omitempty"`
	PreviousFilename string `json:"previous_filename,omitempty"`
}
