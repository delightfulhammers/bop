package github

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bkyoung/code-reviewer/internal/adapter/github"
	"github.com/bkyoung/code-reviewer/internal/domain"
)

// mockReviewClientForTriage implements ReviewClient for testing triage fetcher.
type mockReviewClientForTriage struct {
	comments []github.PullRequestComment
	err      error
}

func (m *mockReviewClientForTriage) CreateReview(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
	return nil, nil
}

func (m *mockReviewClientForTriage) ListReviews(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
	return nil, nil
}

func (m *mockReviewClientForTriage) DismissReview(ctx context.Context, owner, repo string, pullNumber int, reviewID int64, message string) (*github.DismissReviewResponse, error) {
	return nil, nil
}

func (m *mockReviewClientForTriage) ListPullRequestComments(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.comments, nil
}

func TestTriageContextFetcher_NoBotUsername(t *testing.T) {
	client := &mockReviewClientForTriage{}
	fetcher := NewTriageContextFetcher(client, "")

	result := fetcher.FetchTriagedFindings(context.Background(), "owner", "repo", 123)
	assert.Nil(t, result, "should return nil when bot username is empty")
}

func TestTriageContextFetcher_NoComments(t *testing.T) {
	client := &mockReviewClientForTriage{
		comments: []github.PullRequestComment{},
	}
	fetcher := NewTriageContextFetcher(client, "github-actions[bot]")

	result := fetcher.FetchTriagedFindings(context.Background(), "owner", "repo", 123)
	assert.Nil(t, result, "should return nil when no comments")
}

func TestTriageContextFetcher_OpenFinding(t *testing.T) {
	// Issue #165: Open findings (no reply yet) should be included to prevent LLM from re-raising them
	fingerprint := domain.NewFindingFingerprint("main.go", "bug", "high", "Test issue")
	line := 10
	client := &mockReviewClientForTriage{
		comments: []github.PullRequestComment{
			{
				ID:   1,
				Path: "main.go",
				Body: buildFindingComment("bug", "high", "Test issue", 10, 10, fingerprint),
				User: github.User{Login: "github-actions[bot]", Type: "Bot"},
				Line: &line,
			},
		},
	}
	fetcher := NewTriageContextFetcher(client, "github-actions[bot]")

	result := fetcher.FetchTriagedFindings(context.Background(), "owner", "repo", 123)

	require.NotNil(t, result, "should return context with open finding")
	require.Len(t, result.Findings, 1)

	f := result.Findings[0]
	assert.Equal(t, "main.go", f.File)
	assert.Equal(t, domain.StatusOpen, f.Status)
	assert.Equal(t, "bug", f.Category)
	assert.Contains(t, f.StatusReason, "awaiting")
}

func TestTriageContextFetcher_AcknowledgedFinding(t *testing.T) {
	fingerprint := domain.NewFindingFingerprint("auth.go", "security", "high", "Missing validation")
	parentComment := github.PullRequestComment{
		ID:   1,
		Path: "auth.go",
		Body: buildFindingComment("security", "high", "Missing validation", 20, 25, fingerprint),
		User: github.User{Login: "github-actions[bot]", Type: "Bot"},
		Line: intPtr(20),
	}
	replyComment := github.PullRequestComment{
		ID:          2,
		Path:        "auth.go",
		Body:        "acknowledged - we're tracking this in a separate issue",
		User:        github.User{Login: "developer", Type: "User"},
		InReplyToID: 1,
	}

	client := &mockReviewClientForTriage{
		comments: []github.PullRequestComment{parentComment, replyComment},
	}
	fetcher := NewTriageContextFetcher(client, "github-actions[bot]")

	result := fetcher.FetchTriagedFindings(context.Background(), "owner", "repo", 123)

	require.NotNil(t, result)
	require.Len(t, result.Findings, 1)

	f := result.Findings[0]
	assert.Equal(t, "auth.go", f.File)
	assert.Equal(t, domain.StatusAcknowledged, f.Status)
	assert.Equal(t, "security", f.Category)
	assert.Contains(t, f.Description, "Missing validation")
}

func TestTriageContextFetcher_DisputedFinding(t *testing.T) {
	fingerprint := domain.NewFindingFingerprint("config.go", "performance", "medium", "Inefficient loop")
	parentComment := github.PullRequestComment{
		ID:   1,
		Path: "config.go",
		Body: buildFindingComment("performance", "medium", "Inefficient loop", 5, 10, fingerprint),
		User: github.User{Login: "github-actions[bot]", Type: "Bot"},
		Line: intPtr(5),
	}
	replyComment := github.PullRequestComment{
		ID:          2,
		Path:        "config.go",
		Body:        "false positive - this is actually O(1) amortized",
		User:        github.User{Login: "developer", Type: "User"},
		InReplyToID: 1,
	}

	client := &mockReviewClientForTriage{
		comments: []github.PullRequestComment{parentComment, replyComment},
	}
	fetcher := NewTriageContextFetcher(client, "github-actions[bot]")

	result := fetcher.FetchTriagedFindings(context.Background(), "owner", "repo", 123)

	require.NotNil(t, result)
	require.Len(t, result.Findings, 1)

	f := result.Findings[0]
	assert.Equal(t, "config.go", f.File)
	assert.Equal(t, domain.StatusDisputed, f.Status)
	assert.Equal(t, "performance", f.Category)
}

func TestTriageContextFetcher_MixedFindings(t *testing.T) {
	// Issue #165: All findings should be included (acknowledged, disputed, AND open)
	fp1 := domain.NewFindingFingerprint("a.go", "security", "high", "Issue 1")
	fp2 := domain.NewFindingFingerprint("b.go", "bug", "medium", "Issue 2")
	fp3 := domain.NewFindingFingerprint("c.go", "performance", "low", "Issue 3")

	comments := []github.PullRequestComment{
		// Finding 1: acknowledged
		{
			ID:   1,
			Path: "a.go",
			Body: buildFindingComment("security", "high", "Issue 1", 10, 10, fp1),
			User: github.User{Login: "github-actions[bot]", Type: "Bot"},
			Line: intPtr(10),
		},
		{
			ID:          2,
			Path:        "a.go",
			Body:        "acknowledged",
			User:        github.User{Login: "dev", Type: "User"},
			InReplyToID: 1,
		},
		// Finding 2: disputed
		{
			ID:   3,
			Path: "b.go",
			Body: buildFindingComment("bug", "medium", "Issue 2", 20, 20, fp2),
			User: github.User{Login: "github-actions[bot]", Type: "Bot"},
			Line: intPtr(20),
		},
		{
			ID:          4,
			Path:        "b.go",
			Body:        "not an issue - false positive",
			User:        github.User{Login: "dev", Type: "User"},
			InReplyToID: 3,
		},
		// Finding 3: open (no reply)
		{
			ID:   5,
			Path: "c.go",
			Body: buildFindingComment("performance", "low", "Issue 3", 30, 30, fp3),
			User: github.User{Login: "github-actions[bot]", Type: "Bot"},
			Line: intPtr(30),
		},
	}

	client := &mockReviewClientForTriage{comments: comments}
	fetcher := NewTriageContextFetcher(client, "github-actions[bot]")

	result := fetcher.FetchTriagedFindings(context.Background(), "owner", "repo", 123)

	require.NotNil(t, result)
	// Issue #165: Should include ALL findings (acknowledged, disputed, AND open)
	assert.Len(t, result.Findings, 3)

	// Check we got all three findings with correct statuses
	var hasAcknowledged, hasDisputed, hasOpen bool
	for _, f := range result.Findings {
		switch f.Status {
		case domain.StatusAcknowledged:
			hasAcknowledged = true
			assert.Equal(t, "a.go", f.File)
		case domain.StatusDisputed:
			hasDisputed = true
			assert.Equal(t, "b.go", f.File)
		case domain.StatusOpen:
			hasOpen = true
			assert.Equal(t, "c.go", f.File)
			assert.Contains(t, f.StatusReason, "awaiting")
		}
	}
	assert.True(t, hasAcknowledged, "should have acknowledged finding")
	assert.True(t, hasDisputed, "should have disputed finding")
	assert.True(t, hasOpen, "should have open finding")
}

func TestTriageContextFetcher_MatchesBotUsername(t *testing.T) {
	// Note: GroupCommentsByParent in adapter/github uses exact case matching.
	// This test verifies that the bot username matching works correctly when
	// the usernames match exactly (as they will in production from GitHub API).
	fingerprint := domain.NewFindingFingerprint("test.go", "bug", "high", "Test")
	comments := []github.PullRequestComment{
		{
			ID:   1,
			Path: "test.go",
			Body: buildFindingComment("bug", "high", "Test", 1, 1, fingerprint),
			User: github.User{Login: "github-actions[bot]", Type: "Bot"},
			Line: intPtr(1),
		},
		{
			ID:          2,
			Path:        "test.go",
			Body:        "acknowledged",
			User:        github.User{Login: "dev", Type: "User"},
			InReplyToID: 1,
		},
	}

	client := &mockReviewClientForTriage{comments: comments}
	fetcher := NewTriageContextFetcher(client, "github-actions[bot]")

	result := fetcher.FetchTriagedFindings(context.Background(), "owner", "repo", 123)

	require.NotNil(t, result)
	assert.Len(t, result.Findings, 1, "should find triaged finding when bot username matches")
}

// Helper to build a finding comment body with fingerprint in the expected format.
// Format matches what github.ExtractCommentDetails expects.
func buildFindingComment(category, severity, description string, lineStart, lineEnd int, fp domain.FindingFingerprint) string {
	lineInfo := ""
	if lineStart == lineEnd {
		lineInfo = "📍 Line " + itoa(lineStart)
	} else {
		lineInfo = "📍 Lines " + itoa(lineStart) + "-" + itoa(lineEnd)
	}

	return "**Severity:** " + severity + " | **Category:** " + category + "\n\n" +
		lineInfo + "\n\n" +
		description + "\n\n" +
		"<!-- CR_FINGERPRINT:" + string(fp) + " -->"
}

// Simple int to string conversion for test helper
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func intPtr(i int) *int {
	return &i
}
