package github_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bkyoung/code-reviewer/internal/adapter/github"
	"github.com/bkyoung/code-reviewer/internal/diff"
	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/dedup"
	usecasegithub "github.com/bkyoung/code-reviewer/internal/usecase/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSemanticComparer is a test double for semantic comparison.
type mockSemanticComparer struct {
	compareFunc func(ctx context.Context, candidates []dedup.CandidatePair) (*dedup.ComparisonResult, error)
}

func (m *mockSemanticComparer) Compare(ctx context.Context, candidates []dedup.CandidatePair) (*dedup.ComparisonResult, error) {
	if m.compareFunc != nil {
		return m.compareFunc(ctx, candidates)
	}
	return &dedup.ComparisonResult{}, nil
}

// MockReviewClient is a mock implementation of the ReviewClient interface.
// It uses a mutex to protect shared state for thread safety in concurrent scenarios.
type MockReviewClient struct {
	mu                          sync.Mutex
	CreateReviewFunc            func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error)
	ListReviewsFunc             func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error)
	DismissReviewFunc           func(ctx context.Context, owner, repo string, pullNumber int, reviewID int64, message string) (*github.DismissReviewResponse, error)
	ListPullRequestCommentsFunc func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error)
	LastInput                   *github.CreateReviewInput
	DismissedIDs                []int64
}

func (m *MockReviewClient) CreateReview(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
	m.mu.Lock()
	m.LastInput = &input
	m.mu.Unlock()
	if m.CreateReviewFunc != nil {
		return m.CreateReviewFunc(ctx, input)
	}
	return &github.CreateReviewResponse{ID: 1}, nil
}

func (m *MockReviewClient) ListReviews(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
	if m.ListReviewsFunc != nil {
		return m.ListReviewsFunc(ctx, owner, repo, pullNumber)
	}
	return []github.ReviewSummary{}, nil
}

func (m *MockReviewClient) DismissReview(ctx context.Context, owner, repo string, pullNumber int, reviewID int64, message string) (*github.DismissReviewResponse, error) {
	m.mu.Lock()
	m.DismissedIDs = append(m.DismissedIDs, reviewID)
	m.mu.Unlock()
	if m.DismissReviewFunc != nil {
		return m.DismissReviewFunc(ctx, owner, repo, pullNumber, reviewID, message)
	}
	return &github.DismissReviewResponse{ID: reviewID, State: "DISMISSED"}, nil
}

func (m *MockReviewClient) ListPullRequestComments(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
	if m.ListPullRequestCommentsFunc != nil {
		return m.ListPullRequestCommentsFunc(ctx, owner, repo, pullNumber)
	}
	return []github.PullRequestComment{}, nil
}

// GetDismissedIDs returns a copy of dismissed IDs in a thread-safe manner.
func (m *MockReviewClient) GetDismissedIDs() []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]int64, len(m.DismissedIDs))
	copy(result, m.DismissedIDs)
	return result
}

func TestNewReviewPoster(t *testing.T) {
	client := &MockReviewClient{}
	poster := usecasegithub.NewReviewPoster(client)

	require.NotNil(t, poster)
}

func TestReviewPoster_PostReview_Success(t *testing.T) {
	client := &MockReviewClient{
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{
				ID:      123,
				State:   "APPROVED",
				HTMLURL: "https://github.com/owner/repo/pull/1#review-123",
			}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	// Low and medium findings don't block by default, so review should APPROVE
	findings := []github.PositionedFinding{
		{
			Finding:      makeFinding("file1.go", 10, "low", "Issue 1"),
			DiffPosition: diff.IntPtr(5),
		},
		{
			Finding:      makeFinding("file2.go", 20, "medium", "Issue 2"),
			DiffPosition: diff.IntPtr(15),
		},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha123",
		Review: domain.Review{
			Summary: "Found 2 issues",
		},
		Findings: findings,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(123), result.ReviewID)
	assert.Equal(t, 2, result.CommentsPosted)
	assert.Equal(t, 0, result.CommentsSkipped)
	assert.Equal(t, github.EventApprove, result.Event) // Non-blocking findings → APPROVE
	assert.Equal(t, "https://github.com/owner/repo/pull/1#review-123", result.HTMLURL)
}

func TestReviewPoster_PostReview_DeterminesEventFromSeverity(t *testing.T) {
	tests := []struct {
		name          string
		severity      string
		expectedEvent github.ReviewEvent
	}{
		{
			name:          "high severity triggers REQUEST_CHANGES",
			severity:      "high",
			expectedEvent: github.EventRequestChanges,
		},
		{
			name:          "critical severity triggers REQUEST_CHANGES",
			severity:      "critical",
			expectedEvent: github.EventRequestChanges,
		},
		{
			name:          "medium severity triggers APPROVE (non-blocking)",
			severity:      "medium",
			expectedEvent: github.EventApprove,
		},
		{
			name:          "low severity triggers APPROVE (non-blocking)",
			severity:      "low",
			expectedEvent: github.EventApprove,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &MockReviewClient{}
			poster := usecasegithub.NewReviewPoster(client)

			findings := []github.PositionedFinding{
				{
					Finding:      makeFinding("file.go", 1, tt.severity, "issue"),
					DiffPosition: diff.IntPtr(1),
				},
			}

			result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
				Owner:      "owner",
				Repo:       "repo",
				PullNumber: 1,
				CommitSHA:  "sha",
				Findings:   findings,
			})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedEvent, result.Event)
			assert.Equal(t, tt.expectedEvent, client.LastInput.Event)
		})
	}
}

func TestReviewPoster_PostReview_ApprovesOnNoFindings(t *testing.T) {
	client := &MockReviewClient{}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
		Review: domain.Review{
			Summary: "Looks good!",
		},
		Findings: []github.PositionedFinding{},
	})

	require.NoError(t, err)
	assert.Equal(t, github.EventApprove, result.Event)
	assert.Equal(t, 0, result.CommentsPosted)
}

func TestReviewPoster_PostReview_SkipsOutOfDiffFindings(t *testing.T) {
	client := &MockReviewClient{}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("a.go", 1, "high", "a"), DiffPosition: diff.IntPtr(1)},
		{Finding: makeFinding("b.go", 2, "high", "b"), DiffPosition: nil}, // Out of diff
		{Finding: makeFinding("c.go", 3, "low", "c"), DiffPosition: nil},  // Out of diff
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
		Findings:   findings,
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.CommentsPosted)
	assert.Equal(t, 2, result.CommentsSkipped)
}

func TestReviewPoster_PostReview_ClientError(t *testing.T) {
	expectedErr := errors.New("API error")
	client := &MockReviewClient{
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return nil, expectedErr
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	_, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}

func TestReviewPoster_PostReview_NilResponse(t *testing.T) {
	// Issue #35: When CreateReview returns (nil, nil), we should get an error
	// rather than a nil pointer panic.
	client := &MockReviewClient{
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return nil, nil // Pathological case: no response, no error
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	_, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil response")
}

func TestReviewPoster_PostReview_UsesSummaryFromReview(t *testing.T) {
	client := &MockReviewClient{}
	poster := usecasegithub.NewReviewPoster(client)

	_, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:      "owner",
		Repo:       "repo",
		PullNumber: 1,
		CommitSHA:  "sha",
		Review: domain.Review{
			Summary: "This is the review summary",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "This is the review summary", client.LastInput.Summary)
}

func TestReviewPoster_PostReview_OverrideEvent(t *testing.T) {
	client := &MockReviewClient{}
	poster := usecasegithub.NewReviewPoster(client)

	// Even with high severity findings, override to COMMENT
	findings := []github.PositionedFinding{
		{Finding: makeFinding("a.go", 1, "high", "critical issue"), DiffPosition: diff.IntPtr(1)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:         "owner",
		Repo:          "repo",
		PullNumber:    1,
		CommitSHA:     "sha",
		Findings:      findings,
		OverrideEvent: github.EventComment, // Force COMMENT instead of REQUEST_CHANGES
	})

	require.NoError(t, err)
	assert.Equal(t, github.EventComment, result.Event)
}

func TestReviewPoster_PostReview_OverrideEventNormalized(t *testing.T) {
	// Verify that lowercase event values are normalized to uppercase for the GitHub API
	client := &MockReviewClient{}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:         "owner",
		Repo:          "repo",
		PullNumber:    1,
		CommitSHA:     "sha",
		OverrideEvent: "approve", // lowercase
	})

	require.NoError(t, err)
	// The result should have the uppercase canonical value
	assert.Equal(t, github.EventApprove, result.Event)
	// The API should have received the uppercase value
	require.NotNil(t, client.LastInput)
	assert.Equal(t, github.EventApprove, client.LastInput.Event)
}

func TestReviewPoster_PostReview_OverrideEventValidation(t *testing.T) {
	tests := []struct {
		name          string
		overrideEvent github.ReviewEvent
		wantErr       bool
		errContains   string
	}{
		{
			name:          "empty is allowed",
			overrideEvent: "",
			wantErr:       false,
		},
		{
			name:          "APPROVE is valid",
			overrideEvent: github.EventApprove,
			wantErr:       false,
		},
		{
			name:          "REQUEST_CHANGES is valid",
			overrideEvent: github.EventRequestChanges,
			wantErr:       false,
		},
		{
			name:          "COMMENT is valid",
			overrideEvent: github.EventComment,
			wantErr:       false,
		},
		{
			name:          "lowercase approve is valid and normalized",
			overrideEvent: "approve",
			wantErr:       false,
		},
		{
			name:          "invalid event returns error",
			overrideEvent: "INVALID_EVENT",
			wantErr:       true,
			errContains:   "invalid OverrideEvent",
		},
		{
			name:          "typo returns error",
			overrideEvent: "APROVE",
			wantErr:       true,
			errContains:   "must be APPROVE, REQUEST_CHANGES, or COMMENT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &MockReviewClient{}
			poster := usecasegithub.NewReviewPoster(client)

			_, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
				Owner:         "owner",
				Repo:          "repo",
				PullNumber:    1,
				CommitSHA:     "sha",
				OverrideEvent: tt.overrideEvent,
			})

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReviewPoster_PostReview_WithCustomReviewActions(t *testing.T) {
	client := &MockReviewClient{}
	poster := usecasegithub.NewReviewPoster(client)

	// With default actions, high severity would trigger REQUEST_CHANGES
	// But with custom actions, we configure high to NOT block (just comment)
	// Since no findings trigger REQUEST_CHANGES, we use OnNonBlocking
	findings := []github.PositionedFinding{
		{Finding: makeFinding("a.go", 1, "high", "bug"), DiffPosition: diff.IntPtr(1)},
	}

	customActions := github.ReviewActions{
		OnCritical:    "request_changes",
		OnHigh:        "comment", // Override high to NOT block
		OnMedium:      "comment",
		OnLow:         "comment",
		OnClean:       "approve",
		OnNonBlocking: "comment", // When no findings block, use COMMENT
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:         "owner",
		Repo:          "repo",
		PullNumber:    1,
		CommitSHA:     "sha",
		Findings:      findings,
		ReviewActions: customActions,
	})

	require.NoError(t, err)
	// High doesn't block (OnHigh=comment), so uses OnNonBlocking=comment
	assert.Equal(t, github.EventComment, result.Event)
}

func TestReviewPoster_PostReview_OverrideEventTakesPrecedenceOverReviewActions(t *testing.T) {
	client := &MockReviewClient{}
	poster := usecasegithub.NewReviewPoster(client)

	// Both OverrideEvent and ReviewActions are set
	// OverrideEvent should take precedence
	findings := []github.PositionedFinding{
		{Finding: makeFinding("a.go", 1, "high", "bug"), DiffPosition: diff.IntPtr(1)},
	}

	customActions := github.ReviewActions{
		OnHigh: "comment", // Would return COMMENT
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:         "owner",
		Repo:          "repo",
		PullNumber:    1,
		CommitSHA:     "sha",
		Findings:      findings,
		ReviewActions: customActions,
		OverrideEvent: github.EventApprove, // This should win
	})

	require.NoError(t, err)
	// OverrideEvent takes precedence
	assert.Equal(t, github.EventApprove, result.Event)
}

func TestReviewPoster_PostReview_ReviewActionsOnClean(t *testing.T) {
	client := &MockReviewClient{}
	poster := usecasegithub.NewReviewPoster(client)

	// No findings = clean code
	// With custom OnClean = "comment", should return COMMENT instead of APPROVE
	customActions := github.ReviewActions{
		OnClean: "comment", // Override clean to comment
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:         "owner",
		Repo:          "repo",
		PullNumber:    1,
		CommitSHA:     "sha",
		Findings:      []github.PositionedFinding{},
		ReviewActions: customActions,
	})

	require.NoError(t, err)
	assert.Equal(t, github.EventComment, result.Event)
}

// Helper to create a finding for tests
func makeFinding(file string, line int, severity, description string) domain.Finding {
	return domain.Finding{
		ID:          "test-id",
		File:        file,
		LineStart:   line,
		LineEnd:     line,
		Severity:    severity,
		Category:    "test",
		Description: description,
	}
}

func TestReviewPoster_PostReview_DismissesBotReviews(t *testing.T) {
	client := &MockReviewClient{
		ListReviewsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
			return []github.ReviewSummary{
				{ID: 100, User: github.User{Login: "github-actions[bot]"}, State: "APPROVED"},
				{ID: 101, User: github.User{Login: "github-actions[bot]"}, State: "CHANGES_REQUESTED"},
				{ID: 102, User: github.User{Login: "human-user"}, State: "COMMENTED"},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 200, HTMLURL: "https://example.com/review"}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		BotUsername: "github-actions[bot]",
	})

	require.NoError(t, err)
	assert.Equal(t, int64(200), result.ReviewID)
	assert.Equal(t, 2, result.DismissedCount)
	assert.Equal(t, []int64{100, 101}, client.GetDismissedIDs())
}

func TestReviewPoster_PostReview_CaseInsensitiveBotUsername(t *testing.T) {
	// GitHub usernames are case-insensitive, so "GitHub-Actions[bot]" should match "github-actions[bot]"
	client := &MockReviewClient{
		ListReviewsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
			return []github.ReviewSummary{
				// Different case than what's configured
				{ID: 100, User: github.User{Login: "GitHub-Actions[bot]"}, State: "APPROVED"},
				{ID: 101, User: github.User{Login: "GITHUB-ACTIONS[BOT]"}, State: "CHANGES_REQUESTED"},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 200, HTMLURL: "https://example.com/review"}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		BotUsername: "github-actions[bot]", // lowercase
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.DismissedCount, "should dismiss both reviews despite case difference")
	assert.Equal(t, []int64{100, 101}, client.GetDismissedIDs())
}

func TestReviewPoster_PostReview_NoDismissWhenBotUsernameEmpty(t *testing.T) {
	listCalled := false
	client := &MockReviewClient{
		ListReviewsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
			listCalled = true
			return []github.ReviewSummary{}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		BotUsername: "", // Empty - no dismiss
	})

	require.NoError(t, err)
	assert.False(t, listCalled)
	assert.Equal(t, 0, result.DismissedCount)
}

func TestReviewPoster_PostReview_SkipsAlreadyDismissedReviews(t *testing.T) {
	client := &MockReviewClient{
		ListReviewsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
			return []github.ReviewSummary{
				{ID: 100, User: github.User{Login: "bot[bot]"}, State: "DISMISSED"},
				{ID: 101, User: github.User{Login: "bot[bot]"}, State: "APPROVED"},
			}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// Only the APPROVED one should be dismissed, not the already DISMISSED one
	assert.Equal(t, 1, result.DismissedCount)
	assert.Equal(t, []int64{101}, client.GetDismissedIDs())
}

func TestReviewPoster_PostReview_SkipsPendingReviews(t *testing.T) {
	client := &MockReviewClient{
		ListReviewsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
			return []github.ReviewSummary{
				{ID: 100, User: github.User{Login: "bot[bot]"}, State: "PENDING"},
				{ID: 101, User: github.User{Login: "bot[bot]"}, State: "COMMENTED"},
			}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// Only COMMENTED should be dismissed, not PENDING
	assert.Equal(t, 1, result.DismissedCount)
	assert.Equal(t, []int64{101}, client.GetDismissedIDs())
}

func TestReviewPoster_PostReview_ListFailureContinues(t *testing.T) {
	client := &MockReviewClient{
		ListReviewsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
			return nil, errors.New("list failed")
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 200}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		BotUsername: "bot[bot]",
	})

	// Should succeed despite list failure
	require.NoError(t, err)
	assert.Equal(t, int64(200), result.ReviewID)
	assert.Equal(t, 0, result.DismissedCount)
}

func TestReviewPoster_PostReview_DismissFailureContinues(t *testing.T) {
	dismissCalls := 0
	client := &MockReviewClient{
		ListReviewsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
			return []github.ReviewSummary{
				{ID: 100, User: github.User{Login: "bot[bot]"}, State: "APPROVED"},
				{ID: 101, User: github.User{Login: "bot[bot]"}, State: "CHANGES_REQUESTED"},
			}, nil
		},
		DismissReviewFunc: func(ctx context.Context, owner, repo string, pullNumber int, reviewID int64, message string) (*github.DismissReviewResponse, error) {
			dismissCalls++
			if reviewID == 100 {
				return nil, errors.New("dismiss failed")
			}
			return &github.DismissReviewResponse{ID: reviewID, State: "DISMISSED"}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 200}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		BotUsername: "bot[bot]",
	})

	// Should succeed despite partial dismiss failure
	require.NoError(t, err)
	assert.Equal(t, int64(200), result.ReviewID)
	assert.Equal(t, 2, dismissCalls) // Both were attempted
	assert.Equal(t, 1, result.DismissedCount)
}

func TestReviewPoster_PostReview_NoBotReviewsToDissmiss(t *testing.T) {
	client := &MockReviewClient{
		ListReviewsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
			return []github.ReviewSummary{
				{ID: 100, User: github.User{Login: "human-user"}, State: "APPROVED"},
			}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.DismissedCount)
	assert.Empty(t, client.GetDismissedIDs())
}

func TestReviewPoster_PostReview_NoDismissalOnCreateFailure(t *testing.T) {
	// Verify that if CreateReview fails, no reviews are dismissed.
	// This ensures the PR always maintains review signal.
	listCalled := false
	client := &MockReviewClient{
		ListReviewsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
			listCalled = true
			return []github.ReviewSummary{
				{ID: 100, User: github.User{Login: "bot[bot]"}, State: "APPROVED"},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return nil, errors.New("create review failed")
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	_, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		BotUsername: "bot[bot]",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create review failed")
	// ListReviews should NOT have been called since dismissal happens after CreateReview
	assert.False(t, listCalled, "ListReviews should not be called when CreateReview fails")
	// No reviews should have been dismissed
	assert.Empty(t, client.GetDismissedIDs())
}

func TestReviewPoster_PostReview_SkipsNewlyCreatedReview(t *testing.T) {
	// Verify that the newly created review is not dismissed.
	// This prevents the bot from dismissing its own fresh review.
	const newReviewID = int64(200)
	client := &MockReviewClient{
		ListReviewsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error) {
			return []github.ReviewSummary{
				{ID: 100, User: github.User{Login: "bot[bot]"}, State: "APPROVED"},          // Old review - should be dismissed
				{ID: newReviewID, User: github.User{Login: "bot[bot]"}, State: "COMMENTED"}, // New review - should NOT be dismissed
				{ID: 101, User: github.User{Login: "bot[bot]"}, State: "CHANGES_REQUESTED"}, // Old review - should be dismissed
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: newReviewID, State: "COMMENTED"}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	assert.Equal(t, newReviewID, result.ReviewID)
	// Only old reviews should be dismissed, not the newly created one
	assert.Equal(t, 2, result.DismissedCount)
	assert.Contains(t, client.GetDismissedIDs(), int64(100))
	assert.Contains(t, client.GetDismissedIDs(), int64(101))
	assert.NotContains(t, client.GetDismissedIDs(), newReviewID, "newly created review should not be dismissed")
}

// ==== Deduplication Tests (Issue #107) ====

func TestReviewPoster_PostReview_DeduplicatesFindings(t *testing.T) {
	// Simulate a previous bot comment with an embedded fingerprint.
	// The fingerprint matches one of the findings we're about to post.
	finding1 := makeFinding("file1.go", 10, "high", "Issue already posted")
	finding2 := makeFinding("file2.go", 20, "medium", "New issue")

	// Create a comment body with embedded fingerprint for finding1
	fp1 := domain.FingerprintFromFinding(finding1)
	existingCommentBody := "**Severity:** high\n\n<!-- CR_FINGERPRINT:" + string(fp1) + " -->"

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				{
					ID:   1,
					Body: existingCommentBody,
					User: github.User{Login: "github-actions[bot]"},
				},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123, HTMLURL: "https://example.com/review"}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: finding1, DiffPosition: diff.IntPtr(5)},  // Already posted - should be skipped
		{Finding: finding2, DiffPosition: diff.IntPtr(15)}, // New - should be posted
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "github-actions[bot]",
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.CommentsPosted, "only new finding should be posted")
	assert.Equal(t, 1, result.DuplicatesSkipped, "one duplicate should be skipped")
	// Verify the client only received 1 finding (the non-duplicate)
	require.NotNil(t, client.LastInput)
	assert.Len(t, client.LastInput.Findings, 1)
}

func TestReviewPoster_PostReview_NoDuplicatesWhenNoExistingComments(t *testing.T) {
	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{}, nil // No existing comments
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "Issue 1"), DiffPosition: diff.IntPtr(5)},
		{Finding: makeFinding("file2.go", 20, "medium", "Issue 2"), DiffPosition: diff.IntPtr(15)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "github-actions[bot]",
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.CommentsPosted)
	assert.Equal(t, 0, result.DuplicatesSkipped)
}

func TestReviewPoster_PostReview_IgnoresNonBotComments(t *testing.T) {
	finding := makeFinding("file1.go", 10, "high", "Issue from human")
	fp := domain.FingerprintFromFinding(finding)
	humanCommentBody := "**Severity:** high\n\n<!-- CR_FINGERPRINT:" + string(fp) + " -->"

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				{
					ID:   1,
					Body: humanCommentBody,
					User: github.User{Login: "human-user"}, // Not the bot
				},
			}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: finding, DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "github-actions[bot]",
	})

	require.NoError(t, err)
	// The finding should NOT be deduplicated because the comment is from a human
	assert.Equal(t, 1, result.CommentsPosted)
	assert.Equal(t, 0, result.DuplicatesSkipped)
}

func TestReviewPoster_PostReview_IgnoresCommentsWithoutFingerprint(t *testing.T) {
	// Legacy comments without fingerprints should be ignored
	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				{
					ID:   1,
					Body: "This is an old comment without a fingerprint",
					User: github.User{Login: "github-actions[bot]"},
				},
			}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "New issue"), DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "github-actions[bot]",
	})

	require.NoError(t, err)
	// The finding should be posted because no valid fingerprint was found
	assert.Equal(t, 1, result.CommentsPosted)
	assert.Equal(t, 0, result.DuplicatesSkipped)
}

func TestReviewPoster_PostReview_DeduplicationDisabledWithoutBotUsername(t *testing.T) {
	// When BotUsername is empty, deduplication is disabled
	listCalled := false
	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			listCalled = true
			return []github.PullRequestComment{}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "Issue"), DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "", // No bot username
	})

	require.NoError(t, err)
	assert.False(t, listCalled, "ListPullRequestComments should not be called when BotUsername is empty")
	assert.Equal(t, 1, result.CommentsPosted)
	assert.Equal(t, 0, result.DuplicatesSkipped)
}

func TestReviewPoster_PostReview_CommentFetchErrorContinues(t *testing.T) {
	// If fetching comments fails, continue without deduplication
	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return nil, errors.New("API error")
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "Issue"), DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	// Should succeed despite fetch error
	require.NoError(t, err)
	assert.Equal(t, 1, result.CommentsPosted)
	assert.Equal(t, 0, result.DuplicatesSkipped)
}

func TestReviewPoster_PostReview_AllFindingsDeduplicated(t *testing.T) {
	// When all findings are duplicates, an empty review should still be posted.
	// Critically, the review Event should still reflect the original findings'
	// severity to keep the PR blocked if there are unresolved high-severity issues.
	finding := makeFinding("file1.go", 10, "high", "Already posted")
	fp := domain.FingerprintFromFinding(finding)
	existingCommentBody := "<!-- CR_FINGERPRINT:" + string(fp) + " -->"

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				{
					ID:   1,
					Body: existingCommentBody,
					User: github.User{Login: "bot[bot]"},
				},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: finding, DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.CommentsPosted)
	assert.Equal(t, 1, result.DuplicatesSkipped)
	// Event should still be REQUEST_CHANGES because the original finding is high severity,
	// even though no new comments are being posted (the finding was already posted).
	assert.Equal(t, github.EventRequestChanges, result.Event, "should still block PR for unresolved high-severity findings")
	// Verify the review was still created (with no comments)
	require.NotNil(t, client.LastInput)
	assert.Len(t, client.LastInput.Findings, 0)
}

func TestReviewPoster_PostReview_CaseInsensitiveBotUsernameForDedup(t *testing.T) {
	// Bot username matching should be case-insensitive
	finding := makeFinding("file1.go", 10, "high", "Issue")
	fp := domain.FingerprintFromFinding(finding)
	existingCommentBody := "<!-- CR_FINGERPRINT:" + string(fp) + " -->"

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				{
					ID:   1,
					Body: existingCommentBody,
					User: github.User{Login: "GitHub-Actions[BOT]"}, // Different case
				},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: finding, DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "github-actions[bot]", // lowercase
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.CommentsPosted, "finding should be deduplicated despite case difference")
	assert.Equal(t, 1, result.DuplicatesSkipped)
}

// ==== Status-Aware Deduplication Tests (Issue #108) ====

func TestReviewPoster_PostReview_ReturnsStatusCounts(t *testing.T) {
	// Set up findings with existing bot comments that have replies
	finding1 := makeFinding("file1.go", 10, "high", "Issue with ack reply")
	finding2 := makeFinding("file2.go", 20, "high", "Issue with dispute reply")
	finding3 := makeFinding("file3.go", 30, "high", "Issue with no reply")

	fp1 := domain.FingerprintFromFinding(finding1)
	fp2 := domain.FingerprintFromFinding(finding2)
	fp3 := domain.FingerprintFromFinding(finding3)

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				// Bot comment for finding1
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(fp1) + " -->", User: github.User{Login: "bot[bot]"}},
				// Reply with acknowledgment
				{ID: 2, Body: "Acknowledged, will fix later", User: github.User{Login: "author"}, InReplyToID: 1},
				// Bot comment for finding2
				{ID: 3, Body: "<!-- CR_FINGERPRINT:" + string(fp2) + " -->", User: github.User{Login: "bot[bot]"}},
				// Reply with dispute
				{ID: 4, Body: "This is a false positive", User: github.User{Login: "author"}, InReplyToID: 3},
				// Bot comment for finding3 (no reply)
				{ID: 5, Body: "<!-- CR_FINGERPRINT:" + string(fp3) + " -->", User: github.User{Login: "bot[bot]"}},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	// All findings are duplicates (already exist in comments)
	findings := []github.PositionedFinding{
		{Finding: finding1, DiffPosition: diff.IntPtr(5)},
		{Finding: finding2, DiffPosition: diff.IntPtr(15)},
		{Finding: finding3, DiffPosition: diff.IntPtr(25)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// Verify status counts
	assert.Equal(t, 1, result.AcknowledgedCount, "should have 1 acknowledged finding")
	assert.Equal(t, 1, result.DisputedCount, "should have 1 disputed finding")
	assert.Equal(t, 1, result.OpenCount, "should have 1 open finding")
}

func TestReviewPoster_PostReview_AcknowledgedFindingsDontBlock(t *testing.T) {
	// A high-severity finding that has been acknowledged should NOT block the PR
	finding := makeFinding("file1.go", 10, "high", "Acknowledged issue")
	fp := domain.FingerprintFromFinding(finding)

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				// Bot comment for the finding
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(fp) + " -->", User: github.User{Login: "bot[bot]"}},
				// Author acknowledged
				{ID: 2, Body: "Acknowledged, won't fix for now", User: github.User{Login: "author"}, InReplyToID: 1},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: finding, DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// Should APPROVE because the finding is acknowledged (doesn't count toward blocking)
	assert.Equal(t, github.EventApprove, result.Event, "acknowledged finding should not block PR")
	assert.Equal(t, 1, result.AcknowledgedCount)
}

func TestReviewPoster_PostReview_DisputedFindingsDontBlock(t *testing.T) {
	// A high-severity finding that has been disputed should NOT block the PR
	finding := makeFinding("file1.go", 10, "high", "Disputed issue")
	fp := domain.FingerprintFromFinding(finding)

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				// Bot comment for the finding
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(fp) + " -->", User: github.User{Login: "bot[bot]"}},
				// Author disputed
				{ID: 2, Body: "This is a false positive", User: github.User{Login: "author"}, InReplyToID: 1},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: finding, DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// Should APPROVE because the finding is disputed (doesn't count toward blocking)
	assert.Equal(t, github.EventApprove, result.Event, "disputed finding should not block PR")
	assert.Equal(t, 1, result.DisputedCount)
}

func TestReviewPoster_PostReview_OpenFindingsStillBlock(t *testing.T) {
	// A high-severity finding with no replies should still block
	finding := makeFinding("file1.go", 10, "high", "Open issue")
	fp := domain.FingerprintFromFinding(finding)

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				// Bot comment for the finding (no replies)
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(fp) + " -->", User: github.User{Login: "bot[bot]"}},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: finding, DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// Should REQUEST_CHANGES because the finding is open (no acknowledgment/dispute)
	assert.Equal(t, github.EventRequestChanges, result.Event, "open high-severity finding should block PR")
	assert.Equal(t, 1, result.OpenCount)
}

func TestReviewPoster_PostReview_MixedStatusFindings(t *testing.T) {
	// Mix of acknowledged, disputed, and open findings.
	// Only open findings count toward blocking.
	findingAck := makeFinding("file1.go", 10, "high", "Acknowledged")
	findingDisputed := makeFinding("file2.go", 20, "high", "Disputed")
	findingOpen := makeFinding("file3.go", 30, "low", "Open but low severity")

	fpAck := domain.FingerprintFromFinding(findingAck)
	fpDisputed := domain.FingerprintFromFinding(findingDisputed)
	fpOpen := domain.FingerprintFromFinding(findingOpen)

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(fpAck) + " -->", User: github.User{Login: "bot[bot]"}},
				{ID: 2, Body: "Acknowledged", User: github.User{Login: "author"}, InReplyToID: 1},
				{ID: 3, Body: "<!-- CR_FINGERPRINT:" + string(fpDisputed) + " -->", User: github.User{Login: "bot[bot]"}},
				{ID: 4, Body: "False positive", User: github.User{Login: "author"}, InReplyToID: 3},
				{ID: 5, Body: "<!-- CR_FINGERPRINT:" + string(fpOpen) + " -->", User: github.User{Login: "bot[bot]"}},
				// No reply for fpOpen
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: findingAck, DiffPosition: diff.IntPtr(5)},
		{Finding: findingDisputed, DiffPosition: diff.IntPtr(15)},
		{Finding: findingOpen, DiffPosition: diff.IntPtr(25)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// Only the open finding (low severity) counts → APPROVE
	// High severity ones are acknowledged/disputed so don't block
	assert.Equal(t, github.EventApprove, result.Event, "only open low-severity finding should not block")
	assert.Equal(t, 1, result.AcknowledgedCount)
	assert.Equal(t, 1, result.DisputedCount)
	assert.Equal(t, 1, result.OpenCount)
}

func TestReviewPoster_PostReview_NewFindingsStillBlock(t *testing.T) {
	// New findings (not yet commented on) should still block regardless of existing statuses
	existingFinding := makeFinding("file1.go", 10, "high", "Existing acknowledged")
	newFinding := makeFinding("file2.go", 20, "high", "Brand new finding")

	fpExisting := domain.FingerprintFromFinding(existingFinding)

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(fpExisting) + " -->", User: github.User{Login: "bot[bot]"}},
				{ID: 2, Body: "Acknowledged", User: github.User{Login: "author"}, InReplyToID: 1},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: existingFinding, DiffPosition: diff.IntPtr(5)}, // Acknowledged - won't block
		{Finding: newFinding, DiffPosition: diff.IntPtr(15)},     // New - will block
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// New high-severity finding blocks even though existing one is acknowledged
	assert.Equal(t, github.EventRequestChanges, result.Event, "new high-severity finding should block")
	// Only 1 comment posted (new finding), existing is deduplicated
	assert.Equal(t, 1, result.CommentsPosted)
	assert.Equal(t, 1, result.DuplicatesSkipped)
}

func TestReviewPoster_PostReview_StatusCountsZeroWithoutBotUsername(t *testing.T) {
	// When BotUsername is empty, status counts should be zero
	client := &MockReviewClient{}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "Issue"), DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "", // No bot username
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.AcknowledgedCount)
	assert.Equal(t, 0, result.DisputedCount)
	assert.Equal(t, 0, result.OpenCount)
}

func TestReviewPoster_PostReview_OverrideEventIgnoresStatuses(t *testing.T) {
	// When OverrideEvent is set, status-based calculation is bypassed
	finding := makeFinding("file1.go", 10, "high", "Issue")
	fp := domain.FingerprintFromFinding(finding)

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(fp) + " -->", User: github.User{Login: "bot[bot]"}},
				// No acknowledgment/dispute - would normally block
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: finding, DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:         "owner",
		Repo:          "repo",
		PullNumber:    1,
		CommitSHA:     "sha",
		Findings:      findings,
		BotUsername:   "bot[bot]",
		OverrideEvent: github.EventComment, // Force COMMENT regardless of status
	})

	require.NoError(t, err)
	assert.Equal(t, github.EventComment, result.Event, "override should take precedence")
	// Status counts should still be calculated
	assert.Equal(t, 1, result.OpenCount)
}

func TestReviewPoster_PostReview_SummaryIncludesStatusSection(t *testing.T) {
	// When there are existing findings with statuses, the summary should include
	// a status breakdown section.
	finding := makeFinding("file1.go", 10, "high", "Acknowledged issue")
	fp := domain.FingerprintFromFinding(finding)

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(fp) + " -->", User: github.User{Login: "bot[bot]"}},
				{ID: 2, Body: "Acknowledged", User: github.User{Login: "author"}, InReplyToID: 1},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			// Verify the summary includes status section
			assert.Contains(t, input.Summary, "### Existing Finding Status")
			assert.Contains(t, input.Summary, "🔓 Open")
			assert.Contains(t, input.Summary, "✅ Acknowledged")
			assert.Contains(t, input.Summary, "❌ Disputed")
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: finding, DiffPosition: diff.IntPtr(5)},
	}

	_, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
		Review:      domain.Review{Summary: "Original summary"},
	})

	require.NoError(t, err)
}

func TestReviewPoster_PostReview_SummaryOmitsStatusSectionWhenEmpty(t *testing.T) {
	// When there are no existing comments, the summary should NOT include
	// the status breakdown section.
	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{}, nil // No existing comments
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			// Summary should NOT include status section
			assert.NotContains(t, input.Summary, "### Existing Finding Status")
			assert.Equal(t, "Clean review", input.Summary)
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "New issue"), DiffPosition: diff.IntPtr(5)},
	}

	_, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
		Review:      domain.Review{Summary: "Clean review"},
	})

	require.NoError(t, err)
}

// ==== Comment Verification Tests (Issue #129) ====

func TestReviewPoster_PostReview_VerifiesCommentCount(t *testing.T) {
	// When posting a review, verify that all expected comments were actually posted.
	const newReviewID = int64(456)

	client := &MockReviewClient{
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: newReviewID, HTMLURL: "https://example.com/review"}, nil
		},
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			// Return comments that match the review ID
			return []github.PullRequestComment{
				{ID: 1, PullRequestReviewID: newReviewID, Path: "file1.go", User: github.User{Login: "bot[bot]"}},
				{ID: 2, PullRequestReviewID: newReviewID, Path: "file2.go", User: github.User{Login: "bot[bot]"}},
			}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "Issue 1"), DiffPosition: diff.IntPtr(5)},
		{Finding: makeFinding("file2.go", 20, "medium", "Issue 2"), DiffPosition: diff.IntPtr(15)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.CommentsPosted, "expected comments sent")
	assert.Equal(t, 2, result.CommentsVerified, "actual comments verified")
	assert.False(t, result.CommentMismatch, "no mismatch expected")
}

func TestReviewPoster_PostReview_DetectsCommentMismatch(t *testing.T) {
	// When GitHub silently drops comments, we should detect the mismatch.
	const newReviewID = int64(456)

	client := &MockReviewClient{
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: newReviewID, HTMLURL: "https://example.com/review"}, nil
		},
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			// GitHub only accepted 1 of the 3 comments (silently dropped 2)
			return []github.PullRequestComment{
				{ID: 1, PullRequestReviewID: newReviewID, Path: "file1.go", User: github.User{Login: "bot[bot]"}},
			}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "Issue 1"), DiffPosition: diff.IntPtr(5)},
		{Finding: makeFinding("file2.go", 20, "medium", "Issue 2"), DiffPosition: diff.IntPtr(15)},
		{Finding: makeFinding("file3.go", 30, "low", "Issue 3"), DiffPosition: diff.IntPtr(25)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	assert.Equal(t, 3, result.CommentsPosted, "expected comments sent")
	assert.Equal(t, 1, result.CommentsVerified, "only 1 comment was actually posted")
	assert.True(t, result.CommentMismatch, "mismatch should be detected")
}

func TestReviewPoster_PostReview_VerificationSkippedWithoutBotUsername(t *testing.T) {
	// Without BotUsername, verification is skipped (no way to fetch comments for comparison).
	client := &MockReviewClient{
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123, HTMLURL: "https://example.com/review"}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "Issue"), DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "", // No bot username - verification skipped
	})

	require.NoError(t, err)
	assert.Equal(t, 1, result.CommentsPosted)
	assert.Equal(t, 0, result.CommentsVerified, "verification skipped without BotUsername")
	assert.False(t, result.CommentMismatch, "no mismatch when verification is skipped")
}

func TestReviewPoster_PostReview_VerificationErrorContinues(t *testing.T) {
	// If fetching comments for verification fails, the review posting still succeeds.
	// We already fetched comments before for deduplication, but after posting the
	// review, the second fetch (for verification) might fail.
	const newReviewID = int64(456)
	fetchCount := 0

	client := &MockReviewClient{
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: newReviewID, HTMLURL: "https://example.com/review"}, nil
		},
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			fetchCount++
			if fetchCount == 1 {
				// First call (for deduplication) succeeds
				return []github.PullRequestComment{}, nil
			}
			// Second call (for verification) fails
			return nil, errors.New("rate limited")
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "Issue"), DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	// Review should still succeed despite verification failure
	require.NoError(t, err)
	assert.Equal(t, int64(456), result.ReviewID)
	assert.Equal(t, 1, result.CommentsPosted)
	// Verification failed, so CommentsVerified is -1 to indicate error
	assert.Equal(t, -1, result.CommentsVerified, "verification failed indicator")
	assert.False(t, result.CommentMismatch, "no mismatch when verification failed")
}

func TestReviewPoster_PostReview_NoMismatchWhenZeroComments(t *testing.T) {
	// When we expect 0 comments (all out of diff), no mismatch should be reported.
	const newReviewID = int64(456)

	client := &MockReviewClient{
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: newReviewID, HTMLURL: "https://example.com/review"}, nil
		},
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			// No comments for this review (as expected)
			return []github.PullRequestComment{}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	// All findings are out of diff
	findings := []github.PositionedFinding{
		{Finding: makeFinding("file1.go", 10, "high", "Issue 1"), DiffPosition: nil},
		{Finding: makeFinding("file2.go", 20, "medium", "Issue 2"), DiffPosition: nil},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.CommentsPosted, "no comments expected")
	assert.Equal(t, 0, result.CommentsVerified, "no comments verified")
	assert.False(t, result.CommentMismatch, "no mismatch when 0 comments expected")
}

// TestReviewPoster_PostReview_NoNewFindingsWithTriagedExisting tests bug #149:
// When re-reviewing a PR where all existing findings have been triaged (acknowledged/disputed)
// and the LLM generates no new findings, the result should be APPROVE.
func TestReviewPoster_PostReview_NoNewFindingsWithTriagedExisting(t *testing.T) {
	// Scenario: Re-review where LLM finds nothing new, but there are existing triaged findings
	// - 3 existing findings: 1 acknowledged, 2 disputed
	// - No new findings from LLM (empty Findings slice)
	// Expected: APPROVE (not CHANGES_REQUESTED)

	fp1 := domain.FindingFingerprint("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1")
	fp2 := domain.FindingFingerprint("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa2")
	fp3 := domain.FindingFingerprint("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa3")

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				// Existing bot comments with fingerprints
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(fp1) + " -->\n**Severity:** high", User: github.User{Login: "bot[bot]"}},
				{ID: 2, Body: "<!-- CR_FINGERPRINT:" + string(fp2) + " -->\n**Severity:** high", User: github.User{Login: "bot[bot]"}},
				{ID: 3, Body: "<!-- CR_FINGERPRINT:" + string(fp3) + " -->\n**Severity:** critical", User: github.User{Login: "bot[bot]"}},
				// Author replies indicating triage
				{ID: 4, Body: "Acknowledged, will fix later", User: github.User{Login: "author"}, InReplyToID: 1},
				{ID: 5, Body: "This is a false positive", User: github.User{Login: "author"}, InReplyToID: 2},
				{ID: 6, Body: "Not applicable to our case", User: github.User{Login: "author"}, InReplyToID: 3},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	// Empty findings - LLM found nothing new on re-review
	findings := []github.PositionedFinding{}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// Bug #149: This was returning CHANGES_REQUESTED instead of APPROVE
	assert.Equal(t, github.EventApprove, result.Event, "empty findings with all triaged existing should APPROVE")
	assert.Equal(t, 0, result.CommentsPosted, "no new comments")
	assert.Equal(t, 1, result.AcknowledgedCount, "1 acknowledged")
	assert.Equal(t, 2, result.DisputedCount, "2 disputed")
	assert.Equal(t, 0, result.OpenCount, "0 open")
}

// TestReviewPoster_PostReview_RegeneratedTriagedFindings tests bug #149 scenario:
// When re-reviewing a PR where the LLM regenerates the same findings that have been
// triaged (acknowledged/disputed), the result should be APPROVE because all
// blocking findings have been addressed.
func TestReviewPoster_PostReview_RegeneratedTriagedFindings(t *testing.T) {
	// Scenario: LLM regenerates the same findings that exist in comments
	// - 3 existing findings: 1 acknowledged, 2 disputed
	// - LLM regenerates all 3 (same fingerprints)
	// - They get deduplicated for posting, but should also not block
	// Expected: APPROVE (not CHANGES_REQUESTED)

	// Create findings that will match the existing comments
	finding1 := makeFinding("file1.go", 10, "high", "Security issue 1")
	finding2 := makeFinding("file2.go", 20, "high", "Security issue 2")
	finding3 := makeFinding("file3.go", 30, "critical", "Critical bug")

	fp1 := domain.FingerprintFromFinding(finding1)
	fp2 := domain.FingerprintFromFinding(finding2)
	fp3 := domain.FingerprintFromFinding(finding3)

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				// Existing bot comments with matching fingerprints
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(fp1) + " -->\n**Severity:** high", User: github.User{Login: "bot[bot]"}},
				{ID: 2, Body: "<!-- CR_FINGERPRINT:" + string(fp2) + " -->\n**Severity:** high", User: github.User{Login: "bot[bot]"}},
				{ID: 3, Body: "<!-- CR_FINGERPRINT:" + string(fp3) + " -->\n**Severity:** critical", User: github.User{Login: "bot[bot]"}},
				// Author replies indicating triage
				{ID: 4, Body: "Acknowledged, will fix later", User: github.User{Login: "author"}, InReplyToID: 1},
				{ID: 5, Body: "This is a false positive", User: github.User{Login: "author"}, InReplyToID: 2},
				{ID: 6, Body: "Not applicable to our case", User: github.User{Login: "author"}, InReplyToID: 3},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	// LLM regenerated the same findings (same fingerprints)
	findings := []github.PositionedFinding{
		{Finding: finding1, DiffPosition: diff.IntPtr(5)},
		{Finding: finding2, DiffPosition: diff.IntPtr(15)},
		{Finding: finding3, DiffPosition: diff.IntPtr(25)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// Should APPROVE because all findings are triaged (acknowledged/disputed)
	assert.Equal(t, github.EventApprove, result.Event, "all triaged findings should APPROVE")
	// All findings should be deduplicated (already posted)
	assert.Equal(t, 3, result.DuplicatesSkipped, "all 3 should be skipped as duplicates")
	assert.Equal(t, 0, result.CommentsPosted, "no new comments")
	assert.Equal(t, 1, result.AcknowledgedCount, "1 acknowledged")
	assert.Equal(t, 2, result.DisputedCount, "2 disputed")
	assert.Equal(t, 0, result.OpenCount, "0 open")
}

// TestReviewPoster_PostReview_DifferentFingerprintsSameLocationTriaged tests the scenario
// where the LLM generates findings with DIFFERENT fingerprints than existing triaged comments.
// This could happen if the LLM generates slightly different descriptions on re-review.
// Without semantic dedup, these are treated as new findings and block.
func TestReviewPoster_PostReview_DifferentFingerprintsSameLocationTriaged(t *testing.T) {
	// Scenario: Existing triaged findings have different fingerprints than new findings
	// This demonstrates that fingerprint mismatch = new finding = potential block

	// Existing findings (already posted and triaged)
	existingFinding := makeFinding("file1.go", 10, "high", "Original description")
	existingFP := domain.FingerprintFromFinding(existingFinding)

	// New finding from LLM - same location but different description = different fingerprint
	newFinding := makeFinding("file1.go", 10, "high", "Slightly different description")
	newFP := domain.FingerprintFromFinding(newFinding)

	// Verify fingerprints are different
	require.NotEqual(t, existingFP, newFP, "fingerprints should be different")

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				// Existing bot comment with triaged status
				{ID: 1, Body: "<!-- CR_FINGERPRINT:" + string(existingFP) + " -->\n**Severity:** high", User: github.User{Login: "bot[bot]"}},
				{ID: 2, Body: "Acknowledged", User: github.User{Login: "author"}, InReplyToID: 1},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}
	poster := usecasegithub.NewReviewPoster(client)

	// LLM generates finding with different fingerprint
	findings := []github.PositionedFinding{
		{Finding: newFinding, DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)
	// Without semantic dedup, different fingerprint = new finding = REQUEST_CHANGES
	// This is correct behavior - semantic dedup would catch this upstream
	assert.Equal(t, github.EventRequestChanges, result.Event, "different fingerprint is treated as new finding")
	assert.Equal(t, 0, result.DuplicatesSkipped, "no exact fingerprint match")
	assert.Equal(t, 1, result.CommentsPosted, "new comment posted")
}

// TestReviewPoster_PostReview_SemanticDuplicatesDontBlock tests bug #149:
// When semantic deduplication identifies findings as duplicates of existing
// triaged comments, those findings should NOT block the PR.
// This test demonstrates the failure before the fix is applied.
func TestReviewPoster_PostReview_SemanticDuplicatesDontBlock(t *testing.T) {
	// Scenario: LLM generates findings that are semantic duplicates of existing triaged comments
	// - Existing comment has fingerprint A (acknowledged)
	// - New finding has fingerprint B (different)
	// - Semantic dedup identifies B as duplicate of A
	// - B should NOT block because A was acknowledged
	// Expected: APPROVE (not CHANGES_REQUESTED)

	// Existing finding (already posted and acknowledged)
	existingFP := domain.FindingFingerprint("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1")

	// New finding from LLM - different fingerprint, but will be marked as semantic duplicate
	newFinding := makeFinding("file1.go", 10, "high", "A high severity security issue")
	newFP := domain.FingerprintFromFinding(newFinding)

	// Verify fingerprints are different
	require.NotEqual(t, existingFP, newFP, "fingerprints should be different")

	client := &MockReviewClient{
		ListPullRequestCommentsFunc: func(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error) {
			return []github.PullRequestComment{
				// Existing bot comment - different fingerprint but will be semantic match
				{
					ID:   1,
					Path: "file1.go",
					Line: diff.IntPtr(10),
					Body: "<!-- CR_FINGERPRINT:" + string(existingFP) + " -->\n**Severity:** high | **Category:** test\n\n📍 Line 10\n\nSimilar security concern",
					User: github.User{Login: "bot[bot]"},
				},
				// Author acknowledged
				{ID: 2, Body: "Acknowledged, tracking in issue #123", User: github.User{Login: "author"}, InReplyToID: 1},
			}, nil
		},
		CreateReviewFunc: func(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error) {
			return &github.CreateReviewResponse{ID: 123}, nil
		},
	}

	// Create a mock semantic comparer that always returns the finding as a duplicate
	mockComparer := &mockSemanticComparer{
		compareFunc: func(ctx context.Context, candidates []dedup.CandidatePair) (*dedup.ComparisonResult, error) {
			// Mark all candidates as duplicates
			var duplicates []dedup.DuplicateMatch
			for _, c := range candidates {
				duplicates = append(duplicates, dedup.DuplicateMatch{
					NewFinding:          c.New,
					ExistingFingerprint: c.Existing.Fingerprint,
					Reason:              "Test: semantically similar",
				})
			}
			return &dedup.ComparisonResult{Duplicates: duplicates}, nil
		},
	}

	poster := usecasegithub.NewReviewPoster(
		client,
		usecasegithub.WithSemanticComparer(mockComparer, usecasegithub.SemanticDedupConfig{
			LineThreshold: 10,
			MaxCandidates: 50,
		}),
	)

	// LLM generates finding with different fingerprint
	findings := []github.PositionedFinding{
		{Finding: newFinding, DiffPosition: diff.IntPtr(5)},
	}

	result, err := poster.PostReview(context.Background(), usecasegithub.PostReviewRequest{
		Owner:       "owner",
		Repo:        "repo",
		PullNumber:  1,
		CommitSHA:   "sha",
		Findings:    findings,
		BotUsername: "bot[bot]",
	})

	require.NoError(t, err)

	// Bug #149: This SHOULD return APPROVE because:
	// 1. The finding was identified as a semantic duplicate
	// 2. The original finding it duplicates was acknowledged
	// 3. Therefore it shouldn't block
	assert.Equal(t, github.EventApprove, result.Event, "semantic duplicate of acknowledged finding should not block")
	assert.Equal(t, 1, result.SemanticDuplicatesSkipped, "should identify 1 semantic duplicate")
	assert.Equal(t, 0, result.CommentsPosted, "no new comments")
	assert.Equal(t, 1, result.AcknowledgedCount, "should count 1 acknowledged")
}
