package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/delightfulhammers/bop/internal/usecase/triage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetPRMetadata(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name           string
		owner          string
		repo           string
		prNumber       int
		serverResponse interface{}
		statusCode     int
		wantTitle      string
		wantHeadSHA    string
		wantState      string
		wantErr        bool
		wantErrType    error
		wantErrContain string
	}{
		{
			name:     "returns PR metadata",
			owner:    "testowner",
			repo:     "testrepo",
			prNumber: 123,
			serverResponse: prAPIResponseTest{
				Number:    123,
				Title:     "Add feature X",
				Body:      "This PR adds feature X",
				State:     "open",
				CreatedAt: now.Format(time.RFC3339),
				UpdatedAt: now.Format(time.RFC3339),
				User:      userTest{Login: "author"},
				Head:      refTest{Ref: "feature-branch", SHA: "abc123"},
				Base:      refTest{Ref: "main", SHA: "def456"},
				Merged:    false,
			},
			statusCode:  http.StatusOK,
			wantTitle:   "Add feature X",
			wantHeadSHA: "abc123",
			wantState:   "open",
			wantErr:     false,
		},
		{
			name:     "returns merged state for merged PR",
			owner:    "testowner",
			repo:     "testrepo",
			prNumber: 456,
			serverResponse: prAPIResponseTest{
				Number:    456,
				Title:     "Merged PR",
				State:     "closed",
				CreatedAt: now.Format(time.RFC3339),
				UpdatedAt: now.Format(time.RFC3339),
				User:      userTest{Login: "author"},
				Head:      refTest{Ref: "feature", SHA: "aaa"},
				Base:      refTest{Ref: "main", SHA: "bbb"},
				Merged:    true,
			},
			statusCode: http.StatusOK,
			wantTitle:  "Merged PR",
			wantState:  "merged",
			wantErr:    false,
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
		{
			name:           "returns error for negative PR number",
			owner:          "testowner",
			repo:           "testrepo",
			prNumber:       -1,
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

			metadata, err := client.GetPRMetadata(context.Background(), tt.owner, tt.repo, tt.prNumber)

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
			require.NotNil(t, metadata)
			assert.Equal(t, tt.wantTitle, metadata.Title)
			if tt.wantHeadSHA != "" {
				assert.Equal(t, tt.wantHeadSHA, metadata.HeadSHA)
			}
			assert.Equal(t, tt.wantState, metadata.State)
			assert.Equal(t, tt.owner, metadata.Owner)
			assert.Equal(t, tt.repo, metadata.Repo)
		})
	}
}

func TestPRToDomain(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	pr := prAPIResponse{
		Number:    42,
		Title:     "Test PR",
		Body:      "PR description",
		State:     "open",
		CreatedAt: now,
		UpdatedAt: now,
		User: struct {
			Login string `json:"login"`
		}{Login: "testuser"},
		Head: struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		}{Ref: "feature-branch", SHA: "abc123"},
		Base: struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				Private bool `json:"private"`
				Owner   struct {
					Type string `json:"type"`
				} `json:"owner"`
			} `json:"repo"`
		}{
			Ref: "main",
			SHA: "def456",
			Repo: struct {
				Private bool `json:"private"`
				Owner   struct {
					Type string `json:"type"`
				} `json:"owner"`
			}{
				Private: true,
				Owner: struct {
					Type string `json:"type"`
				}{Type: "Organization"},
			},
		},
		Merged: false,
	}

	result := prToDomain(pr, "owner", "repo")

	assert.Equal(t, "owner", result.Owner)
	assert.Equal(t, "repo", result.Repo)
	assert.Equal(t, 42, result.Number)
	assert.Equal(t, "Test PR", result.Title)
	assert.Equal(t, "PR description", result.Description)
	assert.Equal(t, "testuser", result.Author)
	assert.Equal(t, "open", result.State)
	assert.Equal(t, "feature-branch", result.HeadRef)
	assert.Equal(t, "abc123", result.HeadSHA)
	assert.Equal(t, "main", result.BaseRef)
	assert.Equal(t, "def456", result.BaseSHA)
	assert.True(t, result.IsPrivate)
	assert.Equal(t, "Organization", result.OwnerType)
}

func TestPRToDomain_MergedState(t *testing.T) {
	pr := prAPIResponse{
		Number: 42,
		State:  "closed",
		Merged: true,
	}

	result := prToDomain(pr, "owner", "repo")

	assert.Equal(t, "merged", result.State)
}

// Test helper types that match the API response format
type prAPIResponseTest struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	State     string   `json:"state"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	User      userTest `json:"user"`
	Head      refTest  `json:"head"`
	Base      refTest  `json:"base"`
	Merged    bool     `json:"merged"`
}

type userTest struct {
	Login string `json:"login"`
}

type refTest struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}
