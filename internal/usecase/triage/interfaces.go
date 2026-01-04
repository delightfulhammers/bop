package triage

import (
	"context"

	"github.com/delightfulhammers/bop/internal/domain"
)

// ReviewRepository provides access to stored review data.
// This is a port that must be implemented by the adapter layer.
type ReviewRepository interface {
	// GetReviewByPR retrieves the most recent review for a PR.
	GetReviewByPR(ctx context.Context, repository string, prNumber int) (*domain.Review, error)

	// GetReviewByID retrieves a specific review by its ID.
	GetReviewByID(ctx context.Context, reviewID string) (*domain.Review, error)

	// ListReviewsForPR retrieves all reviews for a PR.
	ListReviewsForPR(ctx context.Context, repository string, prNumber int) ([]domain.Review, error)
}

// GitHubClient provides GitHub API operations for triage.
// This is a port that must be implemented by the adapter layer.
type GitHubClient interface {
	// GetPRContext retrieves contextual information about a PR.
	GetPRContext(ctx context.Context, owner, repo string, prNumber int) (*domain.TriageContext, error)

	// GetFileContent retrieves the content of a file at a specific ref.
	GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error)

	// GetReviewComments retrieves all review comments on a PR.
	GetReviewComments(ctx context.Context, owner, repo string, prNumber int) ([]domain.ThreadComment, error)

	// PostReviewComment posts a new review comment on a PR.
	PostReviewComment(ctx context.Context, owner, repo string, prNumber int, path string, line int, body string) (int64, error)

	// ReplyToThread posts a reply to an existing review thread.
	ReplyToThread(ctx context.Context, owner, repo string, prNumber int, commentID int64, body string) error

	// ResolveThread marks a review thread as resolved.
	ResolveThread(ctx context.Context, owner, repo string, prNumber int, threadID int64) error
}

// SessionStore provides persistence for triage sessions.
// This allows resuming triage work across MCP reconnections.
type SessionStore interface {
	// SaveSession persists a triage session.
	SaveSession(ctx context.Context, session *domain.TriageSession) error

	// GetSession retrieves a session by ID.
	GetSession(ctx context.Context, sessionID string) (*domain.TriageSession, error)

	// GetSessionByPR retrieves the active session for a PR, if any.
	GetSessionByPR(ctx context.Context, repository string, prNumber int) (*domain.TriageSession, error)

	// DeleteSession removes a session.
	DeleteSession(ctx context.Context, sessionID string) error
}

// TriageService defines the core triage workflow operations.
// This is the primary use case interface exposed to the MCP adapter.
type TriageService interface {
	// StartSession begins a new triage session for a PR.
	StartSession(ctx context.Context, repository string, prNumber int) (*domain.TriageSession, error)

	// ResumeSession retrieves an existing session.
	ResumeSession(ctx context.Context, sessionID string) (*domain.TriageSession, error)

	// GetCurrentFinding returns the current finding to triage.
	GetCurrentFinding(ctx context.Context, sessionID string) (*domain.TriageFinding, error)

	// GetFindingContext retrieves contextual information for a finding.
	GetFindingContext(ctx context.Context, sessionID string, findingID string) (*domain.TriageContext, error)

	// TriageFinding applies a triage decision to a finding.
	TriageFinding(ctx context.Context, sessionID string, findingID string, decision domain.TriageDecision) error

	// GetProgress returns triage progress for a session.
	GetProgress(ctx context.Context, sessionID string) (triaged, total int, err error)

	// ListFindings returns all findings in a session with optional filters.
	ListFindings(ctx context.Context, sessionID string, statusFilter *domain.TriageStatus) ([]domain.TriageFinding, error)

	// PostComment posts a review comment to GitHub for a finding.
	PostComment(ctx context.Context, sessionID string, findingID string, body string) error

	// ReplyToFinding replies to an existing GitHub thread for a finding.
	ReplyToFinding(ctx context.Context, sessionID string, findingID string, body string) error

	// ResolveFinding marks a GitHub thread as resolved.
	ResolveFinding(ctx context.Context, sessionID string, findingID string) error
}
