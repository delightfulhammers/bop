package session

import (
	"context"
	"time"

	"github.com/delightfulhammers/bop/internal/domain"
)

// LocalSessionStore provides persistence for branch-based review sessions.
// This allows tracking review history across multiple runs and enables
// deduplication of findings.
type LocalSessionStore interface {
	// SaveSession persists a local session.
	// Creates the session if it doesn't exist, updates if it does.
	SaveSession(ctx context.Context, session *domain.LocalSession) error

	// GetSession retrieves a session by ID.
	GetSession(ctx context.Context, sessionID string) (*domain.LocalSession, error)

	// GetSessionByBranch retrieves the session for a repository/branch pair.
	// Returns nil, nil if no session exists.
	GetSessionByBranch(ctx context.Context, repository, branch string) (*domain.LocalSession, error)

	// DeleteSession removes a session and all its associated reviews.
	DeleteSession(ctx context.Context, sessionID string) error

	// ListSessions returns all sessions.
	ListSessions(ctx context.Context) ([]domain.LocalSession, error)

	// ListSessionsByAge returns sessions with lastReviewAt older than the given duration.
	ListSessionsByAge(ctx context.Context, olderThan time.Duration) ([]domain.LocalSession, error)

	// SaveReview saves a review to a session.
	SaveReview(ctx context.Context, review *domain.LocalReview) error

	// GetReview retrieves a specific review by ID.
	GetReview(ctx context.Context, reviewID string) (*domain.LocalReview, error)

	// GetLatestReview retrieves the most recent review for a session.
	GetLatestReview(ctx context.Context, sessionID string) (*domain.LocalReview, error)

	// ListReviews returns all reviews for a session, ordered by creation time.
	ListReviews(ctx context.Context, sessionID string) ([]domain.LocalReview, error)
}

// GitBranchChecker provides git operations for checking branch existence.
// Used by the prune service to detect orphaned sessions.
type GitBranchChecker interface {
	// BranchExistsLocal checks if a branch exists locally.
	BranchExistsLocal(ctx context.Context, branch string) (bool, error)

	// BranchExistsRemote checks if a branch exists on the origin remote.
	// Returns error if network is unavailable.
	BranchExistsRemote(ctx context.Context, branch string) (bool, error)

	// GetRemoteURL returns the origin remote URL.
	GetRemoteURL(ctx context.Context) (string, error)
}
