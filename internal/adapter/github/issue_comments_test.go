package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_CreateIssueComment(t *testing.T) {
	tests := []struct {
		name           string
		owner          string
		repo           string
		prNumber       int
		body           string
		responseStatus int
		responseBody   string
		wantID         int64
		wantErr        bool
		errContains    string
	}{
		{
			name:           "successful creation",
			owner:          "testowner",
			repo:           "testrepo",
			prNumber:       42,
			body:           "Test comment with <!-- CR_FP:abc123 CR_OOD:true -->",
			responseStatus: http.StatusCreated,
			responseBody:   `{"id": 12345, "body": "Test comment", "user": {"login": "github-actions[bot]"}}`,
			wantID:         12345,
			wantErr:        false,
		},
		{
			name:           "invalid owner",
			owner:          "test/owner",
			repo:           "testrepo",
			prNumber:       42,
			body:           "Test",
			responseStatus: 0, // Won't be called
			wantErr:        true,
			errContains:    "invalid owner",
		},
		{
			name:           "invalid repo",
			owner:          "testowner",
			repo:           "test..repo",
			prNumber:       42,
			body:           "Test",
			responseStatus: 0, // Won't be called
			wantErr:        true,
			errContains:    "invalid repo",
		},
		{
			name:           "invalid PR number",
			owner:          "testowner",
			repo:           "testrepo",
			prNumber:       0,
			body:           "Test",
			responseStatus: 0, // Won't be called
			wantErr:        true,
			errContains:    "invalid PR number",
		},
		{
			name:           "empty body",
			owner:          "testowner",
			repo:           "testrepo",
			prNumber:       42,
			body:           "",
			responseStatus: 0, // Won't be called
			wantErr:        true,
			errContains:    "empty body",
		},
		{
			name:           "server error",
			owner:          "testowner",
			repo:           "testrepo",
			prNumber:       42,
			body:           "Test comment",
			responseStatus: http.StatusInternalServerError,
			responseBody:   `{"message": "Internal server error"}`,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and path
				assert.Equal(t, "POST", r.Method)
				expectedPath := "/repos/" + tt.owner + "/" + tt.repo + "/issues/" + "42" + "/comments"
				assert.Equal(t, expectedPath, r.URL.Path)

				// Verify request body
				var reqBody map[string]string
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.Equal(t, tt.body, reqBody["body"])

				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.SetBaseURL(server.URL)
			client.SetMaxRetries(0) // Disable retries for tests

			id, err := client.CreateIssueComment(context.Background(), tt.owner, tt.repo, tt.prNumber, tt.body)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantID, id)
			}
		})
	}
}

func TestClient_ListIssueComments(t *testing.T) {
	tests := []struct {
		name           string
		owner          string
		repo           string
		prNumber       int
		responseStatus int
		responseBody   string
		wantCount      int
		wantErr        bool
		errContains    string
	}{
		{
			name:           "successful list with multiple comments",
			owner:          "testowner",
			repo:           "testrepo",
			prNumber:       42,
			responseStatus: http.StatusOK,
			responseBody: `[
				{"id": 1, "body": "Regular comment", "user": {"login": "user1", "type": "User"}, "created_at": "2024-01-01T10:00:00Z"},
				{"id": 2, "body": "<!-- CR_FP:abc123 CR_OOD:true --> Finding", "user": {"login": "github-actions[bot]", "type": "Bot"}, "created_at": "2024-01-01T11:00:00Z"},
				{"id": 3, "body": "Another comment", "user": {"login": "user2", "type": "User"}, "created_at": "2024-01-01T12:00:00Z"}
			]`,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:           "empty list",
			owner:          "testowner",
			repo:           "testrepo",
			prNumber:       42,
			responseStatus: http.StatusOK,
			responseBody:   `[]`,
			wantCount:      0,
			wantErr:        false,
		},
		{
			name:           "invalid owner",
			owner:          "../evil",
			repo:           "testrepo",
			prNumber:       42,
			responseStatus: 0, // Won't be called
			wantErr:        true,
			errContains:    "invalid owner",
		},
		{
			name:           "server error",
			owner:          "testowner",
			repo:           "testrepo",
			prNumber:       42,
			responseStatus: http.StatusInternalServerError,
			responseBody:   `{"message": "Internal server error"}`,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method
				assert.Equal(t, "GET", r.Method)

				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.SetBaseURL(server.URL)
			client.SetMaxRetries(0)

			comments, err := client.ListIssueComments(context.Background(), tt.owner, tt.repo, tt.prNumber)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Len(t, comments, tt.wantCount)
			}
		})
	}
}

func TestClient_GetIssueComment(t *testing.T) {
	tests := []struct {
		name           string
		owner          string
		repo           string
		commentID      int64
		responseStatus int
		responseBody   string
		wantErr        bool
		errContains    string
	}{
		{
			name:           "successful get",
			owner:          "testowner",
			repo:           "testrepo",
			commentID:      12345,
			responseStatus: http.StatusOK,
			responseBody:   `{"id": 12345, "body": "Test comment", "user": {"login": "user1", "type": "User"}, "created_at": "2024-01-01T10:00:00Z"}`,
			wantErr:        false,
		},
		{
			name:           "not found",
			owner:          "testowner",
			repo:           "testrepo",
			commentID:      99999,
			responseStatus: http.StatusNotFound,
			responseBody:   `{"message": "Not Found"}`,
			wantErr:        true,
			errContains:    "not found",
		},
		{
			name:           "invalid comment ID",
			owner:          "testowner",
			repo:           "testrepo",
			commentID:      0,
			responseStatus: 0, // Won't be called
			wantErr:        true,
			errContains:    "invalid comment ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.SetBaseURL(server.URL)
			client.SetMaxRetries(0)

			comment, err := client.GetIssueComment(context.Background(), tt.owner, tt.repo, tt.commentID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.commentID, comment.ID)
			}
		})
	}
}

func TestIssueComment_HasFingerprint(t *testing.T) {
	tests := []struct {
		name    string
		comment IssueComment
		want    bool
	}{
		{
			name:    "has fingerprint",
			comment: IssueComment{Body: "<!-- CR_FP:abc123 CR_OOD:true --> Description"},
			want:    true,
		},
		{
			name:    "no fingerprint",
			comment: IssueComment{Body: "Just a regular comment"},
			want:    false,
		},
		{
			name:    "empty body",
			comment: IssueComment{Body: ""},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.comment.HasFingerprint()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIssueComment_IsOutOfDiffFinding(t *testing.T) {
	tests := []struct {
		name    string
		comment IssueComment
		want    bool
	}{
		{
			name:    "has OOD marker",
			comment: IssueComment{Body: "<!-- CR_FP:abc123 CR_OOD:true --> Description"},
			want:    true,
		},
		{
			name:    "fingerprint only, no OOD",
			comment: IssueComment{Body: "<!-- CR_FP:abc123 --> Description"},
			want:    false,
		},
		{
			name:    "no markers",
			comment: IssueComment{Body: "Just a regular comment"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.comment.IsOutOfDiffFinding()
			assert.Equal(t, tt.want, got)
		})
	}
}
