package github

import (
	"context"
	"fmt"

	"github.com/delightfulhammers/bop/internal/usecase/triage"
)

// ReviewManagerAdapter wraps the GitHub Client to implement the triage.ReviewManager port.
// This adapter exists because Client.DismissReview returns (*DismissReviewResponse, error)
// while the port interface expects just error.
type ReviewManagerAdapter struct {
	client *Client
}

// NewReviewManagerAdapter creates a new adapter that implements triage.ReviewManager.
func NewReviewManagerAdapter(client *Client) *ReviewManagerAdapter {
	return &ReviewManagerAdapter{client: client}
}

// ResolveThread delegates to the underlying Client.
func (a *ReviewManagerAdapter) ResolveThread(ctx context.Context, owner, repo, threadID string) error {
	return a.client.ResolveThread(ctx, owner, repo, threadID)
}

// UnresolveThread delegates to the underlying Client.
func (a *ReviewManagerAdapter) UnresolveThread(ctx context.Context, owner, repo, threadID string) error {
	return a.client.UnresolveThread(ctx, owner, repo, threadID)
}

// DismissReview dismisses a review and discards the response details.
// The caller only needs to know if the operation succeeded.
func (a *ReviewManagerAdapter) DismissReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64, message string) error {
	if reviewID <= 0 {
		return fmt.Errorf("reviewID must be positive")
	}
	_, err := a.client.DismissReview(ctx, owner, repo, prNumber, reviewID, message)
	return err
}

// RequestReviewers delegates to the underlying Client.
func (a *ReviewManagerAdapter) RequestReviewers(ctx context.Context, owner, repo string, prNumber int, reviewers []string, teamReviewers []string) error {
	return a.client.RequestReviewers(ctx, owner, repo, prNumber, reviewers, teamReviewers)
}

// ListReviews returns all reviews for a PR, converting from github types to port types.
// This is needed for dismiss stale functionality to find bot reviews.
func (a *ReviewManagerAdapter) ListReviews(ctx context.Context, owner, repo string, prNumber int) ([]triage.Review, error) {
	summaries, err := a.client.ListReviews(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	reviews := make([]triage.Review, len(summaries))
	for i, s := range summaries {
		// User is a value type (not pointer), so Login/Type may be empty strings
		// for system-generated reviews, but this won't cause a nil panic.
		reviews[i] = triage.Review{
			ID:          s.ID,
			User:        s.User.Login,
			UserType:    s.User.Type,
			State:       s.State,
			SubmittedAt: s.SubmittedAt,
		}
	}
	return reviews, nil
}

// FindThreadForComment finds the review thread ID for a given comment ID.
// Converts from github.ThreadInfo to triage.ThreadInfo.
func (a *ReviewManagerAdapter) FindThreadForComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*triage.ThreadInfo, error) {
	info, err := a.client.FindThreadForComment(ctx, owner, repo, prNumber, commentID)
	if err != nil {
		return nil, err
	}
	return &triage.ThreadInfo{
		ID:         info.ID,
		IsResolved: info.IsResolved,
	}, nil
}
