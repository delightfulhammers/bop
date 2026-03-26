package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/delightfulhammers/bop/internal/usecase/triage"
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

// --- Cache tests ---

func TestClient_ListIssueComments_CacheHit(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id": 1, "body": "comment1"}]`))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)

	// First call — cache miss, should hit server
	comments1, err := client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	assert.Len(t, comments1, 1)
	assert.Equal(t, int32(1), callCount.Load())

	// Second call — cache hit, should NOT hit server
	comments2, err := client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	assert.Len(t, comments2, 1)
	assert.Equal(t, int32(1), callCount.Load(), "expected cache hit, but server was called again")
}

func TestClient_ListIssueComments_CacheSeparateKeys(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id": 1, "body": "comment1"}]`))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)

	// Different PR numbers should be separate cache entries
	_, err := client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	_, err = client.ListIssueComments(context.Background(), "owner", "repo", 2)
	require.NoError(t, err)
	assert.Equal(t, int32(2), callCount.Load())
}

func TestClient_ListIssueComments_CacheTTLExpiry(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id": 1, "body": "comment1"}]`))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)

	// First call — populates cache
	_, err := client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	assert.Equal(t, int32(1), callCount.Load())

	// Manually expire the cache entry
	client.issueCommentsCache.mu.Lock()
	key := issueCommentsCacheKey{Owner: "owner", Repo: "repo", PRNumber: 1}
	if entry, ok := client.issueCommentsCache.entries[key]; ok {
		entry.fetchedAt = time.Now().Add(-issueCommentsCacheTTL - time.Second)
		client.issueCommentsCache.entries[key] = entry
	}
	client.issueCommentsCache.mu.Unlock()

	// Second call — cache expired, should hit server
	_, err = client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	assert.Equal(t, int32(2), callCount.Load(), "expected cache miss after TTL expiry")
}

func TestClient_CreateIssueComment_InvalidatesCache(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			callCount.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id": 1, "body": "comment1"}]`))
		} else {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": 99, "body": "new comment"}`))
		}
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)

	// Populate cache
	_, err := client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	assert.Equal(t, int32(1), callCount.Load())

	// Post a comment — should invalidate cache
	_, err = client.CreateIssueComment(context.Background(), "owner", "repo", 1, "new comment")
	require.NoError(t, err)

	// Next list call should hit server again
	_, err = client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	assert.Equal(t, int32(2), callCount.Load(), "expected cache invalidated after post")
}

func TestClient_ClearIssueCommentsCache(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id": 1, "body": "comment1"}]`))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)

	// Populate cache
	_, err := client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)

	// Clear cache
	client.ClearIssueCommentsCache()

	// Next call should hit server
	_, err = client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	assert.Equal(t, int32(2), callCount.Load())
}

func TestClient_ListIssueComments_CacheNotPopulatedOnError(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		if callCount.Load() == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"error"}`))
		} else {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id": 1, "body": "comment1"}]`))
		}
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)

	// First call fails — should not cache
	_, err := client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.Error(t, err)

	// Second call should still hit server (no stale cache)
	_, err = client.ListIssueComments(context.Background(), "owner", "repo", 1)
	require.NoError(t, err)
	assert.Equal(t, int32(2), callCount.Load())
}

// --- MaxPages option tests ---

func TestClient_ListIssueComments_MaxPages(t *testing.T) {
	var pageCount atomic.Int32
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := pageCount.Add(1)
		comment := fmt.Sprintf(`[{"id": %d, "body": "page %d"}]`, page, page)
		// Always return a next link to simulate many pages
		if page < 50 {
			nextURL := fmt.Sprintf("%s%s?page=%d&per_page=100", server.URL, r.URL.Path, page+1)
			w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"next\"", nextURL))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(comment))
	})
	server.Start()
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)
	client.ClearIssueCommentsCache() // Ensure no cache interference

	// With MaxPages=3, should only fetch 3 pages
	comments, err := client.ListIssueComments(
		context.Background(), "owner", "repo", 1,
		triage.ListIssueCommentsOptions{MaxPages: 3},
	)
	require.NoError(t, err)
	assert.Len(t, comments, 3)
	assert.Equal(t, int32(3), pageCount.Load())
}

func TestClient_ListIssueComments_MaxPagesZeroUnlimited(t *testing.T) {
	// MaxPages=0 means unlimited (up to hard cap)
	var pageCount atomic.Int32
	totalPages := 5
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := int(pageCount.Add(1))
		comment := fmt.Sprintf(`[{"id": %d, "body": "page %d"}]`, page, page)
		if page < totalPages {
			nextURL := fmt.Sprintf("%s%s?page=%d&per_page=100", server.URL, r.URL.Path, page+1)
			w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"next\"", nextURL))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(comment))
	})
	server.Start()
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)
	client.ClearIssueCommentsCache()

	comments, err := client.ListIssueComments(
		context.Background(), "owner", "repo", 1,
		triage.ListIssueCommentsOptions{MaxPages: 0},
	)
	require.NoError(t, err)
	assert.Len(t, comments, totalPages)
}
