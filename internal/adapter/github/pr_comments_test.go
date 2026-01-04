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

func TestClient_ListPRComments(t *testing.T) {
	comments := []PullRequestComment{
		{
			ID:          1001,
			Body:        "**Severity: high**\n**Category: security**\nCR_FP:abc123\nSecurity issue found",
			Path:        "main.go",
			Line:        intPtr(10),
			User:        User{Login: "bop", Type: "User"},
			CreatedAt:   "2024-01-15T10:00:00Z",
			InReplyToID: 0,
		},
		{
			ID:          1002,
			Body:        "This is a reply",
			Path:        "main.go",
			Line:        intPtr(10),
			User:        User{Login: "developer", Type: "User"},
			CreatedAt:   "2024-01-15T11:00:00Z",
			InReplyToID: 1001,
		},
		{
			ID:          1003,
			Body:        "Regular comment without fingerprint",
			Path:        "util.go",
			Line:        intPtr(25),
			User:        User{Login: "reviewer", Type: "User"},
			CreatedAt:   "2024-01-15T12:00:00Z",
			InReplyToID: 0,
		},
		{
			ID:          1004,
			Body:        "**Severity: medium**\nBot comment without fingerprint",
			Path:        "api.go",
			Line:        intPtr(50),
			User:        User{Login: "github-actions[bot]", Type: "Bot"},
			CreatedAt:   "2024-01-15T13:00:00Z",
			InReplyToID: 0,
		},
		{
			ID:          1005,
			Body:        "Security scan result from bot",
			Path:        "security.go",
			Line:        intPtr(100),
			User:        User{Login: "github-advanced-security[bot]", Type: "Bot"},
			CreatedAt:   "2024-01-15T14:00:00Z",
			InReplyToID: 0,
		},
	}

	tests := []struct {
		name                string
		filterByFingerprint bool
		wantCount           int
		wantFingerprint     string
		wantSeverity        string
		wantCategory        string
		wantBotComments     bool // expect bot comments without fingerprints to be included
	}{
		{
			name:                "returns all top-level comments without filter",
			filterByFingerprint: false,
			wantCount:           4, // Four top-level comments (1001, 1003, 1004, 1005)
		},
		{
			name:                "filters by fingerprint but includes bot comments",
			filterByFingerprint: true,
			wantCount:           3, // Comment 1001 (fingerprint) + 1004 and 1005 (bots)
			wantFingerprint:     "abc123",
			wantSeverity:        "high",
			wantCategory:        "security",
			wantBotComments:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(comments)
			}))
			defer server.Close()

			client := NewClient("test-token")
			client.SetBaseURL(server.URL)
			client.SetMaxRetries(0)

			findings, err := client.ListPRComments(context.Background(), "owner", "repo", 123, tt.filterByFingerprint)

			require.NoError(t, err)
			assert.Len(t, findings, tt.wantCount)

			if tt.wantCount > 0 && tt.wantFingerprint != "" {
				assert.Equal(t, tt.wantFingerprint, findings[0].Fingerprint)
				assert.Equal(t, tt.wantSeverity, findings[0].Severity)
				assert.Equal(t, tt.wantCategory, findings[0].Category)
			}

			// Verify bot comments are included when filtering
			if tt.wantBotComments {
				// Should have 3 findings: 1 with fingerprint + 2 from bots
				var botFindings []domain.PRFinding
				for _, f := range findings {
					if f.Fingerprint == "" {
						botFindings = append(botFindings, f)
					}
				}
				assert.Len(t, botFindings, 2, "expected 2 bot comments without fingerprints")

				// Verify bot authors are included
				authors := make(map[string]bool)
				for _, f := range botFindings {
					authors[f.Author] = true
				}
				assert.True(t, authors["github-actions[bot]"], "should include github-actions[bot]")
				assert.True(t, authors["github-advanced-security[bot]"], "should include github-advanced-security[bot]")
			}
		})
	}
}

func TestClient_GetPRComment(t *testing.T) {
	comment := commentAPIResponse{
		ID:             1001,
		Body:           "CR_FP:abc123\nSome finding",
		Path:           "main.go",
		Line:           intPtr(10),
		User:           User{Login: "reviewer"},
		CreatedAt:      "2024-01-15T10:00:00Z",
		PullRequestURL: "https://api.github.com/repos/owner/repo/pulls/123",
	}

	tests := []struct {
		name           string
		prNumber       int
		commentID      int64
		serverResponse interface{}
		statusCode     int
		wantErr        bool
		wantErrType    error
		wantErrMsg     string
	}{
		{
			name:           "returns comment that belongs to PR",
			prNumber:       123,
			commentID:      1001,
			serverResponse: comment,
			statusCode:     http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "returns error for comment not on expected PR",
			prNumber:       456, // Different PR
			commentID:      1001,
			serverResponse: comment, // This comment is on PR 123
			statusCode:     http.StatusOK,
			wantErr:        true,
			wantErrType:    triage.ErrCommentNotFound,
		},
		{
			name:           "returns error for 404",
			prNumber:       123,
			commentID:      9999,
			serverResponse: errorResponse{Message: "Not Found"},
			statusCode:     http.StatusNotFound,
			wantErr:        true,
			wantErrType:    triage.ErrCommentNotFound,
		},
		{
			name:           "returns error for invalid comment ID",
			prNumber:       123,
			commentID:      0,
			serverResponse: nil,
			statusCode:     0,
			wantErr:        true,
			wantErrMsg:     "invalid comment ID: 0 (must be positive)",
		},
		{
			name:           "returns error for negative comment ID",
			prNumber:       123,
			commentID:      -1,
			serverResponse: nil,
			statusCode:     0,
			wantErr:        true,
			wantErrMsg:     "invalid comment ID: -1 (must be positive)",
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

			finding, err := client.GetPRComment(context.Background(), "owner", "repo", tt.prNumber, tt.commentID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrType != nil {
					assert.ErrorIs(t, err, tt.wantErrType)
				}
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, finding)
			assert.Equal(t, tt.commentID, finding.CommentID)
		})
	}
}

func TestClient_GetPRCommentByFingerprint(t *testing.T) {
	comments := []PullRequestComment{
		{
			ID:          1001,
			Body:        "CR_FP:abc123\nFirst finding",
			Path:        "main.go",
			Line:        intPtr(10),
			User:        User{Login: "reviewer"},
			CreatedAt:   "2024-01-15T10:00:00Z",
			InReplyToID: 0,
		},
		{
			ID:          1002,
			Body:        "CR_FP:def456\nSecond finding",
			Path:        "util.go",
			Line:        intPtr(20),
			User:        User{Login: "reviewer"},
			CreatedAt:   "2024-01-15T11:00:00Z",
			InReplyToID: 0,
		},
	}

	tests := []struct {
		name        string
		fingerprint string
		wantID      int64
		wantErr     bool
		wantErrType error
	}{
		{
			name:        "finds comment by fingerprint",
			fingerprint: "abc123",
			wantID:      1001,
			wantErr:     false,
		},
		{
			name:        "finds second comment by fingerprint",
			fingerprint: "def456",
			wantID:      1002,
			wantErr:     false,
		},
		{
			name:        "strips CR_FP prefix",
			fingerprint: "CR_FP:abc123",
			wantID:      1001,
			wantErr:     false,
		},
		{
			name:        "returns error for non-existent fingerprint",
			fingerprint: "nonexistent",
			wantErr:     true,
			wantErrType: triage.ErrCommentNotFound,
		},
		{
			name:        "returns error for empty fingerprint",
			fingerprint: "",
			wantErr:     true,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(comments)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finding, err := client.GetPRCommentByFingerprint(context.Background(), "owner", "repo", 123, tt.fingerprint)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrType != nil {
					assert.ErrorIs(t, err, tt.wantErrType)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, finding)
			assert.Equal(t, tt.wantID, finding.CommentID)
		})
	}
}

func TestClient_GetThreadHistory(t *testing.T) {
	rootComment := commentAPIResponse{
		ID:             1001,
		Body:           "Root comment",
		Path:           "main.go",
		Line:           intPtr(10),
		User:           User{Login: "reviewer"},
		CreatedAt:      "2024-01-15T10:00:00Z",
		PullRequestURL: "https://api.github.com/repos/owner/repo/pulls/123",
		InReplyToID:    0,
	}

	allComments := []PullRequestComment{
		{
			ID:          1001,
			Body:        "Root comment",
			Path:        "main.go",
			Line:        intPtr(10),
			User:        User{Login: "reviewer"},
			CreatedAt:   "2024-01-15T10:00:00Z",
			InReplyToID: 0,
		},
		{
			ID:          1002,
			Body:        "First reply",
			Path:        "main.go",
			Line:        intPtr(10),
			User:        User{Login: "developer"},
			CreatedAt:   "2024-01-15T11:00:00Z",
			InReplyToID: 1001,
		},
		{
			ID:          1003,
			Body:        "Second reply",
			Path:        "main.go",
			Line:        intPtr(10),
			User:        User{Login: "reviewer"},
			CreatedAt:   "2024-01-15T12:00:00Z",
			InReplyToID: 1001,
		},
		{
			ID:          1004,
			Body:        "Unrelated comment",
			Path:        "other.go",
			Line:        intPtr(5),
			User:        User{Login: "someone"},
			CreatedAt:   "2024-01-15T13:00:00Z",
			InReplyToID: 0,
		},
	}

	t.Run("returns thread with root and replies", func(t *testing.T) {
		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			w.WriteHeader(http.StatusOK)
			if requestCount == 1 {
				// First request: get root comment
				_ = json.NewEncoder(w).Encode(rootComment)
			} else {
				// Second request: get all comments for PR
				_ = json.NewEncoder(w).Encode(allComments)
			}
		}))
		defer server.Close()

		client := NewClient("test-token")
		client.SetBaseURL(server.URL)
		client.SetMaxRetries(0)

		thread, err := client.GetThreadHistory(context.Background(), "owner", "repo", 1001)

		require.NoError(t, err)
		require.Len(t, thread, 3) // Root + 2 replies

		// Verify order is chronological
		assert.Equal(t, "Root comment", thread[0].Body)
		assert.False(t, thread[0].IsReply)
		assert.Equal(t, "First reply", thread[1].Body)
		assert.True(t, thread[1].IsReply)
		assert.Equal(t, "Second reply", thread[2].Body)
		assert.True(t, thread[2].IsReply)
	})

	t.Run("returns error for non-existent comment", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(errorResponse{Message: "Not Found"})
		}))
		defer server.Close()

		client := NewClient("test-token")
		client.SetBaseURL(server.URL)
		client.SetMaxRetries(0)

		_, err := client.GetThreadHistory(context.Background(), "owner", "repo", 9999)

		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrCommentNotFound)
	})
}

func TestParseFingerprint(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{"CR_FP:abc123\nSome text", "abc123"},
		{"Some text CR_FP:def456 more text", "def456"},
		{"No fingerprint here", ""},
		{"CR_FP:ABCDEF0123456789", "ABCDEF0123456789"},
		{"Multiple CR_FP:aaa111 CR_FP:bbb222", "aaa111"}, // Takes first match
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.body[:min(20, len(tt.body))], func(t *testing.T) {
			got := parseFingerprint(tt.body)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{"**Severity: high**\nText", "high"},
		{"**Severity: CRITICAL**\nText", "critical"},
		{"**Severity: Medium**", "medium"},
		{"[high] severity", "high"},
		{"No severity here", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.body[:min(20, len(tt.body))], func(t *testing.T) {
			got := parseSeverity(tt.body)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseCategory(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{"**Category: security**\nText", "security"},
		{"**Category: Bug**", "bug"},
		{"**Category: PERFORMANCE**", "performance"},
		{"No category here", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.body[:min(20, len(tt.body))], func(t *testing.T) {
			got := parseCategory(tt.body)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractPRNumber(t *testing.T) {
	tests := []struct {
		url  string
		want int
	}{
		{"https://api.github.com/repos/owner/repo/pulls/123", 123},
		{"https://api.github.com/repos/owner/repo/pulls/1", 1},
		{"https://api.github.com/repos/owner/repo/pulls/99999", 99999},
		{"invalid", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := extractPRNumber(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildReplyCountMap(t *testing.T) {
	comments := []PullRequestComment{
		{ID: 1, InReplyToID: 0},
		{ID: 2, InReplyToID: 1},
		{ID: 3, InReplyToID: 1},
		{ID: 4, InReplyToID: 0},
		{ID: 5, InReplyToID: 4},
	}

	metadata := buildReplyMetadataMap(comments)

	assert.Equal(t, 2, metadata[1].count)
	assert.Equal(t, 1, metadata[4].count)
	assert.Equal(t, 0, metadata[2].count) // No replies to comment 2
}

func TestCommentToFinding(t *testing.T) {
	comment := PullRequestComment{
		ID:        1001,
		Body:      "**Severity: high**\n**Category: bug**\nDescription",
		Path:      "main.go",
		Line:      intPtr(42),
		User:      User{Login: "reviewer"},
		CreatedAt: "2024-01-15T10:00:00Z",
	}

	meta := replyMetadata{count: 3}
	finding := commentToFinding(comment, "abc123", meta)

	assert.Equal(t, int64(1001), finding.CommentID)
	assert.Equal(t, "abc123", finding.Fingerprint)
	assert.Equal(t, "main.go", finding.Path)
	assert.Equal(t, 42, finding.Line)
	assert.Equal(t, "high", finding.Severity)
	assert.Equal(t, "bug", finding.Category)
	assert.Equal(t, "reviewer", finding.Author)
	assert.Equal(t, 3, finding.ReplyCount)
	assert.True(t, finding.HasReply)
}

func TestCommentToFinding_NilLine(t *testing.T) {
	comment := PullRequestComment{
		ID:        1001,
		Body:      "Description",
		Path:      "main.go",
		Line:      nil, // No line number
		User:      User{Login: "reviewer"},
		CreatedAt: "2024-01-15T10:00:00Z",
	}

	finding := commentToFinding(comment, "", replyMetadata{})

	assert.Equal(t, 0, finding.Line)
	assert.False(t, finding.HasReply)
}

func TestCommentToFinding_WithReviewer(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantReviewer string
	}{
		{
			name:         "extracts reviewer from new format",
			body:         "**Severity: high**\nDescription\n\n<!-- CR_FP:abc123def456abc123def456abc12345 CR_REVIEWER:security -->",
			wantReviewer: "security",
		},
		{
			name:         "extracts reviewer with hyphen",
			body:         "**Severity: medium**\nText\n\n<!-- CR_FP:abc123def456abc123def456abc12345 CR_REVIEWER:security-expert -->",
			wantReviewer: "security-expert",
		},
		{
			name:         "no reviewer in legacy format",
			body:         "**Severity: low**\nText\n\n<!-- CR_FINGERPRINT:abc123def456abc123def456abc12345 -->",
			wantReviewer: "",
		},
		{
			name:         "no reviewer marker",
			body:         "**Severity: high**\nPlain comment with fingerprint\n\n<!-- CR_FP:abc123def456abc123def456abc12345 -->",
			wantReviewer: "",
		},
		{
			name:         "empty body",
			body:         "",
			wantReviewer: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := PullRequestComment{
				ID:        1001,
				Body:      tt.body,
				Path:      "main.go",
				Line:      intPtr(10),
				User:      User{Login: "bot"},
				CreatedAt: "2024-01-15T10:00:00Z",
			}

			finding := commentToFinding(comment, "abc123", replyMetadata{})

			assert.Equal(t, tt.wantReviewer, finding.Reviewer)
		})
	}
}

func TestAPICommentToFinding_WithReviewer(t *testing.T) {
	comment := commentAPIResponse{
		ID:             1001,
		Body:           "**Severity: high**\nDescription\n\n<!-- CR_FP:abc123def456abc123def456abc12345 CR_REVIEWER:architecture -->",
		Path:           "main.go",
		Line:           intPtr(10),
		User:           User{Login: "bot"},
		CreatedAt:      "2024-01-15T10:00:00Z",
		PullRequestURL: "https://api.github.com/repos/owner/repo/pulls/123",
	}

	finding := apiCommentToFinding(comment, "abc123", replyMetadata{})

	assert.Equal(t, "architecture", finding.Reviewer)
}

func TestResolveFindingID(t *testing.T) {
	tests := []struct {
		id                string
		wantFingerprint   string
		wantCommentID     int64
		wantIsFingerprint bool
	}{
		{"CR_FP:abc123", "abc123", 0, true},
		{"12345", "", 12345, false},
		{"abc123", "abc123", 0, true}, // Bare fingerprint
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			fp, cid, isFP := domain.ResolveFindingID(tt.id)
			assert.Equal(t, tt.wantFingerprint, fp)
			assert.Equal(t, tt.wantCommentID, cid)
			assert.Equal(t, tt.wantIsFingerprint, isFP)
		})
	}
}

// Helper function
func intPtr(i int) *int {
	return &i
}
