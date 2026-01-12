// Package post provides the usecase for posting findings to GitHub PRs.
// This enables a review-then-post workflow where findings from a previous
// review can be inspected, filtered, and then posted to a PR.
package post

import (
	"context"
	"fmt"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// PRClient provides access to PR metadata and diffs.
// This is implemented by github.Client.
type PRClient interface {
	GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error)
	GetPRDiff(ctx context.Context, owner, repo string, prNumber int) (domain.Diff, error)
}

// Poster posts reviews to GitHub PRs.
// This is implemented by github.ReviewPoster.
type Poster interface {
	PostReview(ctx context.Context, req review.GitHubPostRequest) (*review.GitHubPostResult, error)
}

// Request contains the parameters for posting findings.
type Request struct {
	Owner    string
	Repo     string
	PRNumber int
	Findings []domain.Finding

	// DryRun shows what would be posted without actually posting.
	DryRun bool

	// ReviewAction overrides the automatic review action.
	// Valid values: "COMMENT", "REQUEST_CHANGES", "APPROVE"
	// If nil, the action is determined automatically based on finding severity.
	ReviewAction *string
}

// Result contains the outcome of posting findings.
type Result struct {
	ReviewID   int64
	Posted     int
	Skipped    int
	Duplicates int
	ReviewURL  string

	// DryRun is true if this was a dry run.
	DryRun bool

	// WouldPost is set in dry run mode to show what would be posted.
	WouldPost int
}

// Service handles posting findings to GitHub PRs.
type Service struct {
	prClient PRClient
	poster   Poster
}

// NewService creates a new post service.
// Panics if prClient or poster is nil (programming error).
func NewService(prClient PRClient, poster Poster) *Service {
	if prClient == nil {
		panic("post.NewService: prClient is nil")
	}
	if poster == nil {
		panic("post.NewService: poster is nil")
	}
	return &Service{
		prClient: prClient,
		poster:   poster,
	}
}

// PostFindings posts findings to a GitHub PR.
func (s *Service) PostFindings(ctx context.Context, req Request) (*Result, error) {
	// Validate request
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	// Handle dry run mode
	if req.DryRun {
		return &Result{
			DryRun:    true,
			WouldPost: len(req.Findings),
		}, nil
	}

	// Fetch PR metadata for commit SHA
	metadata, err := s.prClient.GetPRMetadata(ctx, req.Owner, req.Repo, req.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR metadata: %w", err)
	}

	// Fetch PR diff for position calculation
	diff, err := s.prClient.GetPRDiff(ctx, req.Owner, req.Repo, req.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR diff: %w", err)
	}

	// Build the post request
	postReq := review.GitHubPostRequest{
		Owner:     req.Owner,
		Repo:      req.Repo,
		PRNumber:  req.PRNumber,
		CommitSHA: metadata.HeadSHA,
		Review: domain.Review{
			Findings: req.Findings,
		},
		Diff:                    diff,
		PostOutOfDiffAsComments: true, // Post out-of-diff findings as issue comments for MCP visibility
	}

	// Apply review action override if specified
	if req.ReviewAction != nil {
		action := *req.ReviewAction
		postReq.ActionOnCritical = action
		postReq.ActionOnHigh = action
		postReq.ActionOnMedium = action
		postReq.ActionOnLow = action
		postReq.ActionOnClean = action
		postReq.ActionOnNonBlocking = action
	}

	// Post the review
	postResult, err := s.poster.PostReview(ctx, postReq)
	if err != nil {
		return nil, fmt.Errorf("failed to post review: %w", err)
	}

	return &Result{
		ReviewID:   postResult.ReviewID,
		Posted:     postResult.CommentsPosted,
		Skipped:    postResult.CommentsSkipped,
		Duplicates: postResult.DuplicatesSkipped,
		ReviewURL:  postResult.HTMLURL,
	}, nil
}

func validateRequest(req Request) error {
	if req.Owner == "" {
		return fmt.Errorf("owner is required")
	}
	if req.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if req.PRNumber <= 0 {
		return fmt.Errorf("PR number is required")
	}
	return nil
}
