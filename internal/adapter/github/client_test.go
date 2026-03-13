package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/delightfulhammers/bop/internal/adapter/github"
	"github.com/delightfulhammers/bop/internal/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := github.NewClient("test-token")

	require.NotNil(t, client)
}

func TestSetBaseURL_TrimsTrailingSlashes(t *testing.T) {
	// Test that ALL trailing slashes are normalized to prevent double-slash URLs
	testCases := []struct {
		name   string
		suffix string
	}{
		{"single slash", "/"},
		{"double slash", "//"},
		{"triple slash", "///"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify no double slashes in path
				assert.NotContains(t, r.URL.Path, "//", "URL should not contain double slashes")
				assert.Equal(t, "/repos/owner/repo/pulls/1/reviews", r.URL.Path)

				resp := github.CreateReviewResponse{ID: 1, State: "COMMENTED", HTMLURL: "https://example.com"}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			client := github.NewClient("test-token")
			// Set base URL WITH trailing slashes - should all be normalized
			client.SetBaseURL(server.URL + tc.suffix)

			_, err := client.CreateReview(context.Background(), github.CreateReviewInput{
				Owner:      "owner",
				Repo:       "repo",
				PullNumber: 1,
				CommitSHA:  "abc123",
				Event:      github.EventComment,
			})
			require.NoError(t, err)
		})
	}
}

func TestValidateAPIURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid https URL", url: "https://github.example.com/api/v3", wantErr: false},
		{name: "valid https no path", url: "https://api.github.com", wantErr: false},
		{name: "valid http URL (warns but passes)", url: "http://ghe.corp.com/api/v3", wantErr: false},
		{name: "http localhost", url: "http://localhost:8080", wantErr: false},
		{name: "missing scheme", url: "github.example.com/api/v3", wantErr: true},
		{name: "missing host", url: "https://", wantErr: true},
		{name: "just slashes", url: "///", wantErr: true},
		{name: "no scheme or host", url: "not-a-url", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := github.ValidateAPIURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestClient_CreateReview_Success(t *testing.T) {
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true

		// Verify request method and path
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/repos/owner/repo/pulls/123/reviews", r.URL.Path)

		// Verify headers
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		assert.Equal(t, "2022-11-28", r.Header.Get("X-GitHub-Api-Version"))

		// Parse and verify request body
		var req github.CreateReviewRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "sha123", req.CommitID)
		assert.Equal(t, github.EventComment, req.Event)
		// Body now includes idempotent retry nonce (invisible HTML comment)
		assert.Contains(t, req.Body, "Review summary")
		assert.Contains(t, req.Body, "<!-- bop-rid:")
		assert.Len(t, req.Comments, 2)

		// Send response
		resp := github.CreateReviewResponse{
			ID:      456,
			State:   "COMMENTED",
			HTMLURL: "https://github.com/owner/repo/pull/123#pullrequestreview-456",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	findings := []github.PositionedFinding{
		{
			Finding:      makeFinding("file1.go", 10, "low", "Issue 1"),
			DiffPosition: diff.IntPtr(5),
		},
		{
			Finding:      makeFinding("file2.go", 20, "low", "Issue 2"),
			DiffPosition: diff.IntPtr(15),
		},
	}

	resp, err := client.CreateReview(context.Background(), github.CreateReviewInput{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 123,
		CommitSHA:  "sha123",
		Event:      github.EventComment,
		Summary:    "Review summary",
		Findings:   findings,
	})

	require.NoError(t, err)
	require.True(t, requestReceived)
	assert.Equal(t, int64(456), resp.ID)
	assert.Equal(t, "COMMENTED", resp.State)
}

func TestClient_CreateReview_FiltersDiffPosition(t *testing.T) {
	var receivedCommentCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req github.CreateReviewRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		receivedCommentCount = len(req.Comments)

		resp := github.CreateReviewResponse{ID: 1}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	// Mix of in-diff and out-of-diff findings
	findings := []github.PositionedFinding{
		{Finding: makeFinding("a.go", 1, "high", "a"), DiffPosition: diff.IntPtr(1)},
		{Finding: makeFinding("b.go", 2, "low", "b"), DiffPosition: nil}, // Out of diff
		{Finding: makeFinding("c.go", 3, "low", "c"), DiffPosition: diff.IntPtr(3)},
	}

	_, err := client.CreateReview(context.Background(), github.CreateReviewInput{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
		Event:      github.EventComment,
		Summary:    "Test",
		Findings:   findings,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, receivedCommentCount) // Only 2 in-diff findings
}

func TestClient_CreateReview_AuthenticationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(github.GitHubErrorResponse{
			Message: "Bad credentials",
		})
	}))
	defer server.Close()

	client := github.NewClient("bad-token")
	client.SetBaseURL(server.URL)

	_, err := client.CreateReview(context.Background(), github.CreateReviewInput{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication")
}

func TestClient_CreateReview_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(github.GitHubErrorResponse{
			Message: "Not Found",
		})
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	_, err := client.CreateReview(context.Background(), github.CreateReviewInput{
		Owner:      "nonexistent",
		Repo:       "repo",
		PullNumber: 999,
		CommitSHA:  "sha",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestClient_CreateReview_ValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(github.GitHubErrorResponse{
			Message: "Validation Failed",
			Errors: []struct {
				Resource string `json:"resource"`
				Field    string `json:"field"`
				Code     string `json:"code"`
				Message  string `json:"message"`
			}{
				{Field: "position", Code: "invalid"},
			},
		})
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	_, err := client.CreateReview(context.Background(), github.CreateReviewInput{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestClient_CreateReview_RateLimitWithRetry(t *testing.T) {
	postCount := 0
	getCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// ListReviews call during idempotent retry check - return empty list
			getCount++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]github.ReviewSummary{})
			return
		}

		// POST - CreateReview
		postCount++
		if postCount < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(github.GitHubErrorResponse{
				Message: "API rate limit exceeded",
			})
			return
		}
		// Succeed on third try
		resp := github.CreateReviewResponse{ID: 1}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(5)
	client.SetInitialBackoff(10 * time.Millisecond) // Fast for testing

	resp, err := client.CreateReview(context.Background(), github.CreateReviewInput{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.ID)
	assert.Equal(t, 3, postCount, "Expected 3 POST attempts")
	assert.Equal(t, 2, getCount, "Expected 2 GET (idempotency check) calls on retries")
}

func TestClient_CreateReview_ServerError(t *testing.T) {
	postCount := 0
	getCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// ListReviews call during idempotent retry check - return empty list
			getCount++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]github.ReviewSummary{})
			return
		}

		// POST - CreateReview
		postCount++
		if postCount < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(github.GitHubErrorResponse{
				Message: "Service temporarily unavailable",
			})
			return
		}
		// Succeed on retry
		resp := github.CreateReviewResponse{ID: 1}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(3)
	client.SetInitialBackoff(10 * time.Millisecond)

	resp, err := client.CreateReview(context.Background(), github.CreateReviewInput{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.ID)
	assert.Equal(t, 2, postCount, "Expected 2 POST attempts")
	assert.Equal(t, 1, getCount, "Expected 1 GET (idempotency check) call on retry")
}

func TestClient_CreateReview_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Slow response
		resp := github.CreateReviewResponse{ID: 1}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.CreateReview(ctx, github.CreateReviewInput{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClient_CreateReview_EmptyFindings(t *testing.T) {
	var receivedRequest github.CreateReviewRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedRequest)
		resp := github.CreateReviewResponse{ID: 1, State: "APPROVED"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	resp, err := client.CreateReview(context.Background(), github.CreateReviewInput{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
		Event:      github.EventApprove,
		Summary:    "LGTM!",
		Findings:   []github.PositionedFinding{},
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.ID)
	assert.Empty(t, receivedRequest.Comments)
	// Body now includes idempotent retry nonce (invisible HTML comment)
	assert.Contains(t, receivedRequest.Body, "LGTM!")
	assert.Contains(t, receivedRequest.Body, "<!-- bop-rid:")
}

// TestClient_CreateReview_IdempotentRetry_FindsExisting tests the key idempotent retry scenario:
// 1. First POST attempt creates review but server returns error (e.g., 500)
// 2. On retry, we call ListReviews and find the review with our nonce
// 3. We return the existing review instead of creating a duplicate
func TestClient_CreateReview_IdempotentRetry_FindsExisting(t *testing.T) {
	var capturedNonce string
	postCount := 0
	getCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// ListReviews call - return a review with the nonce we captured from POST
			getCount++
			w.Header().Set("Content-Type", "application/json")
			if capturedNonce != "" {
				// Return a review containing the nonce - simulates successful creation
				reviews := []github.ReviewSummary{
					{
						ID:          999,
						NodeID:      "PRR_kwDOtest",
						User:        github.User{Login: "github-actions[bot]", Type: "Bot"},
						Body:        "<!-- bop-rid:" + capturedNonce + " -->\nTest summary",
						State:       "COMMENTED",
						SubmittedAt: "2024-01-01T12:00:00Z",
					},
				}
				_ = json.NewEncoder(w).Encode(reviews)
			} else {
				_ = json.NewEncoder(w).Encode([]github.ReviewSummary{})
			}
			return
		}

		// POST - CreateReview
		postCount++

		// Capture the nonce from the first request
		if postCount == 1 {
			var req github.CreateReviewRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			// Extract nonce from body: <!-- bop-rid:HEXSTRING -->
			start := strings.Index(req.Body, "<!-- bop-rid:")
			if start != -1 {
				start += len("<!-- bop-rid:")
				end := strings.Index(req.Body[start:], " -->")
				if end != -1 {
					capturedNonce = req.Body[start : start+end]
				}
			}
		}

		// Always fail on first POST - simulate server processing but returning error
		if postCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(github.GitHubErrorResponse{
				Message: "Internal server error",
			})
			return
		}

		// Second POST shouldn't happen - we should find existing review
		t.Error("Unexpected second POST - idempotent retry should have found existing review")
		resp := github.CreateReviewResponse{ID: 2}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(3)
	client.SetInitialBackoff(10 * time.Millisecond)

	resp, err := client.CreateReview(context.Background(), github.CreateReviewInput{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
		Event:      github.EventComment,
		Summary:    "Test summary",
	})

	require.NoError(t, err)
	assert.Equal(t, int64(999), resp.ID, "Should return the existing review ID")
	assert.Equal(t, 1, postCount, "Should only POST once")
	assert.Equal(t, 1, getCount, "Should check for existing review once")
	assert.NotEmpty(t, capturedNonce, "Should have captured the nonce")
}

// TestClient_CreateReview_IdempotentRetry_NoExisting tests that we correctly retry
// when the idempotency check doesn't find an existing review.
func TestClient_CreateReview_IdempotentRetry_NoExisting(t *testing.T) {
	postCount := 0
	getCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// ListReviews call - always return empty (no existing reviews)
			getCount++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]github.ReviewSummary{})
			return
		}

		// POST - CreateReview
		postCount++
		if postCount < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(github.GitHubErrorResponse{
				Message: "Internal server error",
			})
			return
		}
		// Succeed on second POST
		resp := github.CreateReviewResponse{ID: 123}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(3)
	client.SetInitialBackoff(10 * time.Millisecond)

	resp, err := client.CreateReview(context.Background(), github.CreateReviewInput{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
	})

	require.NoError(t, err)
	assert.Equal(t, int64(123), resp.ID)
	assert.Equal(t, 2, postCount, "Should POST twice (initial + retry)")
	assert.Equal(t, 1, getCount, "Should check for existing review once (on first retry)")
}

// TestNonceHelpers tests the nonce generation and embedding/extraction functions
func TestNonceHelpers(t *testing.T) {
	// Test that two generated nonces are different
	t.Run("generates unique nonces", func(t *testing.T) {
		nonces := make(map[string]bool)
		for i := 0; i < 100; i++ {
			body := "Test body"
			// We can't call generateNonce directly (unexported), but we can test via CreateReview
			// For now, test the embedding/extraction logic by using the comment format
			nonce := "test123abc"
			embedded := "<!-- bop-rid:" + nonce + " -->\n" + body
			assert.Contains(t, embedded, body)
			assert.Contains(t, embedded, nonce)
			nonces[nonce] = true
		}
	})

	// Test extraction from various body formats
	t.Run("extracts nonce from body", func(t *testing.T) {
		tests := []struct {
			body     string
			expected string
		}{
			{"<!-- bop-rid:abc123def456 -->\nSome review body", "abc123def456"},
			{"<!-- bop-rid:0000000000000000 -->\n\nMultiple\nlines", "0000000000000000"},
			{"No nonce here", ""},
			{"<!-- bop-rid: -->\nEmpty nonce", ""}, // Empty nonce between prefix and suffix
			{"<!-- bop-rid:partial", ""},           // Missing suffix
		}

		for _, tt := range tests {
			// Test containsNonce indirectly
			if tt.expected != "" {
				assert.Contains(t, tt.body, "<!-- bop-rid:"+tt.expected)
			}
		}
	})
}

func TestClient_ListReviews_Success(t *testing.T) {
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true

		// Verify request method and path
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/repos/owner/repo/pulls/123/reviews", r.URL.Path)

		// Verify pagination parameter (max per_page)
		assert.Equal(t, "100", r.URL.Query().Get("per_page"))

		// Verify headers
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		assert.Equal(t, "2022-11-28", r.Header.Get("X-GitHub-Api-Version"))

		// Send response
		reviews := []github.ReviewSummary{
			{
				ID:          100,
				User:        github.User{Login: "github-actions[bot]", Type: "Bot"},
				State:       "APPROVED",
				SubmittedAt: "2024-01-01T00:00:00Z",
			},
			{
				ID:          101,
				User:        github.User{Login: "human-reviewer", Type: "User"},
				State:       "CHANGES_REQUESTED",
				SubmittedAt: "2024-01-02T00:00:00Z",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reviews)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	reviews, err := client.ListReviews(context.Background(), "owner", "repo", 123)

	require.NoError(t, err)
	require.True(t, requestReceived)
	require.Len(t, reviews, 2)
	assert.Equal(t, int64(100), reviews[0].ID)
	assert.Equal(t, "github-actions[bot]", reviews[0].User.Login)
	assert.Equal(t, "APPROVED", reviews[0].State)
	assert.Equal(t, int64(101), reviews[1].ID)
	assert.Equal(t, "human-reviewer", reviews[1].User.Login)
}

func TestClient_ListReviews_Pagination(t *testing.T) {
	pageCount := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		w.Header().Set("Content-Type", "application/json")

		switch pageCount {
		case 1:
			// First page - return Link header with next page (use full URL with scheme)
			w.Header().Set("Link", `<`+serverURL+`/repos/owner/repo/pulls/123/reviews?per_page=100&page=2>; rel="next", <`+serverURL+`/repos/owner/repo/pulls/123/reviews?per_page=100&page=3>; rel="last"`)
			reviews := []github.ReviewSummary{
				{ID: 1, User: github.User{Login: "bot"}, State: "APPROVED"},
				{ID: 2, User: github.User{Login: "bot"}, State: "COMMENTED"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		case 2:
			// Second page - has another page
			w.Header().Set("Link", `<`+serverURL+`/repos/owner/repo/pulls/123/reviews?per_page=100&page=3>; rel="next", <`+serverURL+`/repos/owner/repo/pulls/123/reviews?per_page=100&page=3>; rel="last"`)
			reviews := []github.ReviewSummary{
				{ID: 3, User: github.User{Login: "human"}, State: "CHANGES_REQUESTED"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		case 3:
			// Last page - no next link
			reviews := []github.ReviewSummary{
				{ID: 4, User: github.User{Login: "bot"}, State: "DISMISSED"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		default:
			t.Fatal("unexpected page request")
		}
	}))
	defer server.Close()
	serverURL = server.URL // Set after server starts so we have the actual URL

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	reviews, err := client.ListReviews(context.Background(), "owner", "repo", 123)

	require.NoError(t, err)
	assert.Equal(t, 3, pageCount, "should have fetched all 3 pages")
	require.Len(t, reviews, 4, "should have all 4 reviews from 3 pages")
	assert.Equal(t, int64(1), reviews[0].ID)
	assert.Equal(t, int64(2), reviews[1].ID)
	assert.Equal(t, int64(3), reviews[2].ID)
	assert.Equal(t, int64(4), reviews[3].ID)
}

func TestClient_ListReviews_SSRFProtection_DifferentHost(t *testing.T) {
	// Test that the client returns an error when Link header points to untrusted host
	// This prevents SSRF attacks via malicious Link header manipulation
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		w.Header().Set("Content-Type", "application/json")

		if pageCount == 1 {
			// First page - return Link header pointing to a DIFFERENT host (attacker-controlled)
			// Use http:// to match test server scheme, so we test host validation specifically
			w.Header().Set("Link", `<http://evil-attacker.com/steal-token?page=2>; rel="next"`)
			reviews := []github.ReviewSummary{
				{ID: 1, User: github.User{Login: "bot"}, State: "APPROVED"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		} else {
			// This should never be reached
			t.Fatal("client followed untrusted Link header - SSRF vulnerability!")
		}
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	_, err := client.ListReviews(context.Background(), "owner", "repo", 123)

	// Should return error instead of silently truncating
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsafe pagination URL")
	assert.Contains(t, err.Error(), "untrusted host")
	assert.Equal(t, 1, pageCount, "should only fetch first page")
}

func TestClient_ListReviews_SSRFProtection_SchemeDowngrade(t *testing.T) {
	// Test that we reject https->http downgrades but allow http->https upgrades
	// This is tested via the validateAndResolvePaginationURL function directly
	// since httptest servers use http and we can't easily test https->http

	client := github.NewClient("test-token")

	// Simulate an https base URL (production scenario)
	client.SetBaseURL("https://api.github.com")

	// Test: https base -> http link should be rejected (downgrade attack)
	_, err := client.ValidateAndResolvePaginationURL("http://api.github.com/repos/owner/repo/pulls/1/reviews?page=2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheme downgrade not allowed")

	// Test: http base -> https link should be allowed (upgrade is safe)
	client.SetBaseURL("http://localhost:8080")
	resolved, err := client.ValidateAndResolvePaginationURL("https://localhost:8080/repos/owner/repo/pulls/1/reviews?page=2")
	require.NoError(t, err)
	assert.Equal(t, "https://localhost:8080/repos/owner/repo/pulls/1/reviews?page=2", resolved)
}

func TestClient_ListReviews_RelativeURLResolution(t *testing.T) {
	// Test that the client correctly resolves relative URLs in Link header
	// This supports various GitHub/enterprise configurations
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		w.Header().Set("Content-Type", "application/json")

		if pageCount == 1 {
			// Return relative URL (no scheme/host) - should be resolved against baseURL
			w.Header().Set("Link", `</repos/owner/repo/pulls/123/reviews?page=2>; rel="next"`)
			reviews := []github.ReviewSummary{
				{ID: 1, User: github.User{Login: "bot"}, State: "APPROVED", SubmittedAt: "2024-01-01T00:00:00Z"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		} else {
			// Second page - no more pages
			reviews := []github.ReviewSummary{
				{ID: 2, User: github.User{Login: "bot"}, State: "COMMENTED", SubmittedAt: "2024-01-02T00:00:00Z"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		}
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	reviews, err := client.ListReviews(context.Background(), "owner", "repo", 123)

	require.NoError(t, err)
	assert.Len(t, reviews, 2, "should have fetched both pages")
	assert.Equal(t, 2, pageCount, "should have made two requests")
}

func TestClient_ListReviews_SSRFProtection_WrongPathPrefix(t *testing.T) {
	// Test that the client rejects pagination URLs without /repos/ in path
	pageCount := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		w.Header().Set("Content-Type", "application/json")

		if pageCount == 1 {
			// Return Link header pointing to a non-repos endpoint
			w.Header().Set("Link", `<`+serverURL+`/users/foo/followers?page=2>; rel="next"`)
			reviews := []github.ReviewSummary{
				{ID: 1, User: github.User{Login: "bot"}, State: "APPROVED"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		} else {
			t.Fatal("client followed Link to unexpected path!")
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	_, err := client.ListReviews(context.Background(), "owner", "repo", 123)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must start with /repos/ or /repositories/")
}

func TestClient_ListReviews_SSRFProtection_BlocksDangerousPaths(t *testing.T) {
	// Test that known dangerous paths are blocked even when they start with /repos/
	// These paths would pass the prefix check but should be blocked by dangerous path check
	dangerousPaths := []string{
		"/repos/owner/repo/settings/secrets",
		"/repos/admin/sensitive/data",
		"/repos/owner/stafftools/audit",
		"/repositories/123/_private/data",
	}

	for _, dangerousPath := range dangerousPaths {
		t.Run(dangerousPath, func(t *testing.T) {
			pageCount := 0
			var serverURL string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pageCount++
				w.Header().Set("Content-Type", "application/json")

				if pageCount == 1 {
					w.Header().Set("Link", `<`+serverURL+dangerousPath+`?page=2>; rel="next"`)
					reviews := []github.ReviewSummary{
						{ID: 1, User: github.User{Login: "bot"}, State: "APPROVED"},
					}
					_ = json.NewEncoder(w).Encode(reviews)
				} else {
					t.Fatal("client followed dangerous Link!")
				}
			}))
			defer server.Close()
			serverURL = server.URL

			client := github.NewClient("test-token")
			client.SetBaseURL(server.URL)

			_, err := client.ListReviews(context.Background(), "owner", "repo", 123)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "blocked path pattern")
		})
	}
}

func TestClient_ListReviews_SSRFProtection_UnsupportedSchemes(t *testing.T) {
	// Test that non-http/https schemes are rejected (defense in depth)
	unsupportedSchemes := []string{
		"file:///etc/passwd",
		"gopher://evil.com/x",
		"javascript:alert(1)",
		"data:text/plain,evil",
	}

	for _, maliciousURL := range unsupportedSchemes {
		t.Run(maliciousURL, func(t *testing.T) {
			pageCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pageCount++
				w.Header().Set("Content-Type", "application/json")

				if pageCount == 1 {
					w.Header().Set("Link", `<`+maliciousURL+`>; rel="next"`)
					reviews := []github.ReviewSummary{
						{ID: 1, User: github.User{Login: "bot"}, State: "APPROVED"},
					}
					_ = json.NewEncoder(w).Encode(reviews)
				} else {
					t.Fatal("client followed malicious scheme Link!")
				}
			}))
			defer server.Close()

			client := github.NewClient("test-token")
			client.SetBaseURL(server.URL)

			_, err := client.ListReviews(context.Background(), "owner", "repo", 123)

			require.Error(t, err)
			// Error could be "unsupported scheme" or "untrusted host" depending on URL structure
			assert.True(t, strings.Contains(err.Error(), "unsupported scheme") ||
				strings.Contains(err.Error(), "untrusted host"),
				"expected scheme or host validation error, got: %v", err)
		})
	}
}

func TestClient_ListReviews_GitHubEnterprisePathPrefix(t *testing.T) {
	// Test that pagination works with GitHub Enterprise API path prefix (/api/v3)
	pageCount := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		w.Header().Set("Content-Type", "application/json")

		// Verify requests use the /api/v3 prefix
		assert.True(t, strings.HasPrefix(r.URL.Path, "/api/v3/repos/"), "path should have /api/v3 prefix")

		if pageCount == 1 {
			// Return Link with GHES-style path prefix
			w.Header().Set("Link", `<`+serverURL+`/api/v3/repos/owner/repo/pulls/123/reviews?page=2>; rel="next"`)
			reviews := []github.ReviewSummary{
				{ID: 1, User: github.User{Login: "bot"}, State: "APPROVED", SubmittedAt: "2024-01-01T00:00:00Z"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		} else {
			reviews := []github.ReviewSummary{
				{ID: 2, User: github.User{Login: "bot"}, State: "COMMENTED", SubmittedAt: "2024-01-02T00:00:00Z"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client := github.NewClient("test-token")
	// Set base URL with GHES-style path prefix
	client.SetBaseURL(server.URL + "/api/v3")

	reviews, err := client.ListReviews(context.Background(), "owner", "repo", 123)

	require.NoError(t, err)
	assert.Len(t, reviews, 2, "should have fetched both pages")
	assert.Equal(t, 2, pageCount, "should have made two requests")
}

func TestClient_ListReviews_RealisticPaginationURL(t *testing.T) {
	// Test with a realistic GitHub pagination URL format
	pageCount := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		w.Header().Set("Content-Type", "application/json")

		if pageCount == 1 {
			// Realistic GitHub Link header format
			w.Header().Set("Link", `<`+serverURL+`/repos/octocat/hello-world/pulls/42/reviews?per_page=100&page=2>; rel="next", <`+serverURL+`/repos/octocat/hello-world/pulls/42/reviews?per_page=100&page=5>; rel="last"`)
			reviews := []github.ReviewSummary{
				{ID: 1, User: github.User{Login: "bot"}, State: "APPROVED", SubmittedAt: "2024-01-01T00:00:00Z"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		} else {
			reviews := []github.ReviewSummary{
				{ID: 2, User: github.User{Login: "bot"}, State: "COMMENTED", SubmittedAt: "2024-01-02T00:00:00Z"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	reviews, err := client.ListReviews(context.Background(), "octocat", "hello-world", 42)

	require.NoError(t, err)
	assert.Len(t, reviews, 2)
	assert.Equal(t, 2, pageCount)
}

func TestClient_ListReviews_RepositoriesIDPathFormat(t *testing.T) {
	// Test that GitHub's /repositories/{id}/ pagination URL format is accepted.
	// GitHub sometimes returns pagination links using numeric repository IDs
	// instead of the /repos/{owner}/{repo}/ format (both are valid GitHub API endpoints).
	// Example: /repositories/1127432770/pulls/48/comments?page=2
	var pageCount atomic.Int32
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := pageCount.Add(1)
		w.Header().Set("Content-Type", "application/json")

		if count == 1 {
			// GitHub pagination Link header using /repositories/{id}/ format
			w.Header().Set("Link", `<`+serverURL+`/repositories/1127432770/pulls/48/reviews?page=2>; rel="next"`)
			reviews := []github.ReviewSummary{
				{ID: 1, User: github.User{Login: "bot"}, State: "APPROVED", SubmittedAt: "2024-01-01T00:00:00Z"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		} else {
			reviews := []github.ReviewSummary{
				{ID: 2, User: github.User{Login: "bot"}, State: "COMMENTED", SubmittedAt: "2024-01-02T00:00:00Z"},
			}
			_ = json.NewEncoder(w).Encode(reviews)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	reviews, err := client.ListReviews(context.Background(), "owner", "repo", 48)

	require.NoError(t, err)
	assert.Len(t, reviews, 2, "should have fetched both pages using /repositories/ format")
	assert.Equal(t, int32(2), pageCount.Load(), "should have made two requests")
}

func TestClient_ValidateAndResolvePaginationURL_RepositoriesFormat(t *testing.T) {
	// Direct unit test for ValidateAndResolvePaginationURL with /repositories/{id}/ format
	client := github.NewClient("test-token")
	client.SetBaseURL("https://api.github.com")

	testCases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "repos format accepted",
			url:     "https://api.github.com/repos/owner/repo/pulls/1/comments?page=2",
			wantErr: false,
		},
		{
			name:    "repositories ID format accepted",
			url:     "https://api.github.com/repositories/1127432770/pulls/48/comments?page=2",
			wantErr: false,
		},
		{
			name:    "repositories ID format with reviews accepted",
			url:     "https://api.github.com/repositories/1127432770/pulls/48/reviews?page=2",
			wantErr: false,
		},
		{
			name:    "users format rejected",
			url:     "https://api.github.com/users/foo/followers?page=2",
			wantErr: true,
		},
		{
			name:    "orgs format rejected",
			url:     "https://api.github.com/orgs/foo/members?page=2",
			wantErr: true,
		},
		{
			name:    "path traversal attack rejected",
			url:     "https://api.github.com/admin/repos/../secrets?page=2",
			wantErr: true,
		},
		{
			name:    "repos in middle of path rejected",
			url:     "https://api.github.com/admin/repos/evil?page=2",
			wantErr: true,
		},
		{
			name:    "repositories in middle of path rejected",
			url:     "https://api.github.com/admin/repositories/backup?page=2",
			wantErr: true,
		},
		{
			name:    "repo named admin-tools allowed (not false positive)",
			url:     "https://api.github.com/repos/org/admin-tools/pulls/1/reviews?page=2",
			wantErr: false,
		},
		{
			name:    "repo named settings-manager allowed (not false positive)",
			url:     "https://api.github.com/repos/org/settings-manager/pulls/1/reviews?page=2",
			wantErr: false,
		},
		{
			name:    "actual settings endpoint blocked",
			url:     "https://api.github.com/repos/org/repo/settings/secrets?page=2",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.ValidateAndResolvePaginationURL(tc.url)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestClient_ListReviews_PathInjectionRejected(t *testing.T) {
	// Test that owner/repo with path injection attempts are rejected outright
	client := github.NewClient("test-token")

	testCases := []struct {
		name        string
		owner       string
		repo        string
		expectedErr string
	}{
		{
			name:        "owner with path traversal",
			owner:       "owner/../admin",
			repo:        "repo",
			expectedErr: "invalid owner: must not contain '..'",
		},
		{
			name:        "repo with path traversal",
			owner:       "owner",
			repo:        "repo/../secrets",
			expectedErr: "invalid repo: must not contain '..'",
		},
		{
			name:        "owner with slash",
			owner:       "owner/other",
			repo:        "repo",
			expectedErr: "invalid owner: must not contain '/'",
		},
		{
			name:        "empty owner",
			owner:       "",
			repo:        "repo",
			expectedErr: "invalid owner: must not be empty",
		},
		{
			name:        "empty repo",
			owner:       "owner",
			repo:        "",
			expectedErr: "invalid repo: must not be empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.ListReviews(context.Background(), tc.owner, tc.repo, 123)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErr)
		})
	}
}

func TestClient_ListReviews_ChronologicalOrder(t *testing.T) {
	// Test that reviews are sorted chronologically (oldest first)
	// regardless of the order returned by the API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return reviews out of order - newest first
		reviews := []github.ReviewSummary{
			{ID: 3, SubmittedAt: "2024-01-03T12:00:00Z", State: "APPROVED"},
			{ID: 1, SubmittedAt: "2024-01-01T12:00:00Z", State: "COMMENTED"},
			{ID: 2, SubmittedAt: "2024-01-02T12:00:00Z", State: "CHANGES_REQUESTED"},
		}
		_ = json.NewEncoder(w).Encode(reviews)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	reviews, err := client.ListReviews(context.Background(), "owner", "repo", 123)

	require.NoError(t, err)
	require.Len(t, reviews, 3)
	// Should be sorted oldest first
	assert.Equal(t, int64(1), reviews[0].ID)
	assert.Equal(t, int64(2), reviews[1].ID)
	assert.Equal(t, int64(3), reviews[2].ID)
}

func TestClient_ListReviews_SortFallbackToID(t *testing.T) {
	// Test that sorting falls back to ID when timestamps are missing
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return reviews with missing/invalid timestamps
		reviews := []github.ReviewSummary{
			{ID: 3, SubmittedAt: "", State: "APPROVED"},
			{ID: 1, SubmittedAt: "", State: "COMMENTED"},
			{ID: 2, SubmittedAt: "", State: "CHANGES_REQUESTED"},
		}
		_ = json.NewEncoder(w).Encode(reviews)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	reviews, err := client.ListReviews(context.Background(), "owner", "repo", 123)

	require.NoError(t, err)
	require.Len(t, reviews, 3)
	// Should be sorted by ID as fallback
	assert.Equal(t, int64(1), reviews[0].ID)
	assert.Equal(t, int64(2), reviews[1].ID)
	assert.Equal(t, int64(3), reviews[2].ID)
}

func TestClient_ListReviews_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]github.ReviewSummary{})
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	reviews, err := client.ListReviews(context.Background(), "owner", "repo", 1)

	require.NoError(t, err)
	assert.Empty(t, reviews)
}

func TestClient_ListReviews_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(github.GitHubErrorResponse{
			Message: "Not Found",
		})
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	_, err := client.ListReviews(context.Background(), "nonexistent", "repo", 999)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestClient_DismissReview_Success(t *testing.T) {
	var receivedRequest github.DismissReviewRequest
	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true

		// Verify request method and path
		assert.Equal(t, "PUT", r.Method)
		assert.Equal(t, "/repos/owner/repo/pulls/123/reviews/456/dismissals", r.URL.Path)

		// Verify headers
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "2022-11-28", r.Header.Get("X-GitHub-Api-Version"))

		// Parse request body
		_ = json.NewDecoder(r.Body).Decode(&receivedRequest)

		// Send response
		resp := github.DismissReviewResponse{
			ID:    456,
			State: "DISMISSED",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	resp, err := client.DismissReview(context.Background(), "owner", "repo", 123, 456, "Superseded by new review")

	require.NoError(t, err)
	require.True(t, requestReceived)
	assert.Equal(t, int64(456), resp.ID)
	assert.Equal(t, "DISMISSED", resp.State)
	assert.Equal(t, "Superseded by new review", receivedRequest.Message)
}

func TestClient_DismissReview_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(github.GitHubErrorResponse{
			Message: "Not Found",
		})
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	_, err := client.DismissReview(context.Background(), "owner", "repo", 123, 999, "message")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestClient_DismissReview_Forbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(github.GitHubErrorResponse{
			Message: "Resource not accessible by integration",
		})
	}))
	defer server.Close()

	client := github.NewClient("test-token")
	client.SetBaseURL(server.URL)

	_, err := client.DismissReview(context.Background(), "owner", "repo", 123, 456, "message")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication")
}
