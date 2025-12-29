package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_ReplyToComment(t *testing.T) {
	tests := []struct {
		name         string
		owner        string
		repo         string
		prNumber     int
		replyToID    int64
		body         string
		statusCode   int
		responseBody string
		wantID       int64
		wantErr      bool
		errContains  string
	}{
		{
			name:       "successful reply",
			owner:      "owner",
			repo:       "repo",
			prNumber:   123,
			replyToID:  456,
			body:       "This is a reply",
			statusCode: http.StatusCreated,
			responseBody: `{
				"id": 789,
				"node_id": "IC_kwDOtest",
				"body": "This is a reply",
				"user": {"login": "bot", "id": 1, "type": "Bot"}
			}`,
			wantID:  789,
			wantErr: false,
		},
		{
			name:         "comment not found",
			owner:        "owner",
			repo:         "repo",
			prNumber:     123,
			replyToID:    999,
			body:         "Reply to missing comment",
			statusCode:   http.StatusNotFound,
			responseBody: `{"message": "Not Found"}`,
			wantErr:      true,
			errContains:  "not found",
		},
		{
			name:        "invalid owner",
			owner:       "../etc",
			repo:        "repo",
			prNumber:    123,
			replyToID:   456,
			body:        "Bad path",
			wantErr:     true,
			errContains: "invalid owner",
		},
		{
			name:        "empty body",
			owner:       "owner",
			repo:        "repo",
			prNumber:    123,
			replyToID:   456,
			body:        "",
			wantErr:     true,
			errContains: "body cannot be empty",
		},
		{
			name:         "server error retries",
			owner:        "owner",
			repo:         "repo",
			prNumber:     123,
			replyToID:    456,
			body:         "Server error test",
			statusCode:   http.StatusInternalServerError,
			responseBody: `{"message": "Internal Server Error"}`,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.statusCode != 0 {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Validate request method and path
					if r.Method != http.MethodPost {
						t.Errorf("expected POST, got %s", r.Method)
					}

					// Validate request body contains in_reply_to
					var reqBody map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
						t.Errorf("failed to decode request body: %v", err)
					}
					if reqBody["in_reply_to"] == nil {
						t.Error("expected in_reply_to in request body")
					}

					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.responseBody))
				}))
				defer server.Close()
			}

			client := NewClient("test-token")
			if server != nil {
				client.SetBaseURL(server.URL)
			}
			// Reduce retries for faster tests
			client.SetMaxRetries(1)

			gotID, err := client.ReplyToComment(context.Background(), tt.owner, tt.repo, tt.prNumber, tt.replyToID, tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReplyToComment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			}
			if !tt.wantErr && gotID != tt.wantID {
				t.Errorf("ReplyToComment() = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}

func TestClient_CreateComment(t *testing.T) {
	tests := []struct {
		name         string
		owner        string
		repo         string
		prNumber     int
		commitSHA    string
		path         string
		line         int
		body         string
		statusCode   int
		responseBody string
		wantID       int64
		wantErr      bool
		errContains  string
	}{
		{
			name:       "successful creation",
			owner:      "owner",
			repo:       "repo",
			prNumber:   123,
			commitSHA:  "abc123",
			path:       "main.go",
			line:       42,
			body:       "This is a new comment",
			statusCode: http.StatusCreated,
			responseBody: `{
				"id": 999,
				"node_id": "IC_kwDOtest",
				"body": "This is a new comment",
				"path": "main.go",
				"line": 42
			}`,
			wantID:  999,
			wantErr: false,
		},
		{
			name:        "invalid line",
			owner:       "owner",
			repo:        "repo",
			prNumber:    123,
			commitSHA:   "abc123",
			path:        "main.go",
			line:        0,
			body:        "Bad line",
			wantErr:     true,
			errContains: "line must be positive",
		},
		{
			name:        "empty path",
			owner:       "owner",
			repo:        "repo",
			prNumber:    123,
			commitSHA:   "abc123",
			path:        "",
			line:        42,
			body:        "No path",
			wantErr:     true,
			errContains: "path cannot be empty",
		},
		{
			name:        "empty commit SHA",
			owner:       "owner",
			repo:        "repo",
			prNumber:    123,
			commitSHA:   "",
			path:        "main.go",
			line:        42,
			body:        "No commit",
			wantErr:     true,
			errContains: "commit SHA cannot be empty",
		},
		{
			name:         "422 line not in diff",
			owner:        "owner",
			repo:         "repo",
			prNumber:     123,
			commitSHA:    "abc123",
			path:         "main.go",
			line:         999,
			body:         "Line not in diff",
			statusCode:   http.StatusUnprocessableEntity,
			responseBody: `{"message": "Validation Failed", "errors": [{"code": "invalid", "field": "line"}]}`,
			wantErr:      true,
			errContains:  "Validation Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.statusCode != 0 {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost {
						t.Errorf("expected POST, got %s", r.Method)
					}

					// Validate request body
					var reqBody map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
						t.Errorf("failed to decode request body: %v", err)
					}
					if reqBody["commit_id"] == nil {
						t.Error("expected commit_id in request body")
					}
					if reqBody["path"] == nil {
						t.Error("expected path in request body")
					}
					if reqBody["line"] == nil {
						t.Error("expected line in request body")
					}

					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.responseBody))
				}))
				defer server.Close()
			}

			client := NewClient("test-token")
			if server != nil {
				client.SetBaseURL(server.URL)
			}
			client.SetMaxRetries(1)

			gotID, err := client.CreateComment(context.Background(), tt.owner, tt.repo, tt.prNumber, tt.commitSHA, tt.path, tt.line, tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateComment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			}
			if !tt.wantErr && gotID != tt.wantID {
				t.Errorf("CreateComment() = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}

func TestClient_RequestReviewers(t *testing.T) {
	tests := []struct {
		name          string
		owner         string
		repo          string
		prNumber      int
		reviewers     []string
		teamReviewers []string
		statusCode    int
		responseBody  string
		wantErr       bool
		errContains   string
	}{
		{
			name:          "successful request with users",
			owner:         "owner",
			repo:          "repo",
			prNumber:      123,
			reviewers:     []string{"user1", "user2"},
			teamReviewers: nil,
			statusCode:    http.StatusCreated,
			responseBody:  `{"url": "https://api.github.com/repos/owner/repo/pulls/123"}`,
			wantErr:       false,
		},
		{
			name:          "successful request with teams",
			owner:         "owner",
			repo:          "repo",
			prNumber:      123,
			reviewers:     nil,
			teamReviewers: []string{"team-a"},
			statusCode:    http.StatusCreated,
			responseBody:  `{"url": "https://api.github.com/repos/owner/repo/pulls/123"}`,
			wantErr:       false,
		},
		{
			name:          "no reviewers specified",
			owner:         "owner",
			repo:          "repo",
			prNumber:      123,
			reviewers:     nil,
			teamReviewers: nil,
			wantErr:       true,
			errContains:   "at least one reviewer",
		},
		{
			name:          "user not found",
			owner:         "owner",
			repo:          "repo",
			prNumber:      123,
			reviewers:     []string{"nonexistent"},
			teamReviewers: nil,
			statusCode:    http.StatusUnprocessableEntity,
			responseBody:  `{"message": "Reviews may only be requested from collaborators"}`,
			wantErr:       true,
			errContains:   "collaborators",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.statusCode != 0 {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost {
						t.Errorf("expected POST, got %s", r.Method)
					}

					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.responseBody))
				}))
				defer server.Close()
			}

			client := NewClient("test-token")
			if server != nil {
				client.SetBaseURL(server.URL)
			}
			client.SetMaxRetries(1)

			err := client.RequestReviewers(context.Background(), tt.owner, tt.repo, tt.prNumber, tt.reviewers, tt.teamReviewers)
			if (err != nil) != tt.wantErr {
				t.Errorf("RequestReviewers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// contains checks if s contains substr (case-insensitive).
// Uses standard library for proper UTF-8 handling.
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
