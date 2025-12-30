package triage

import (
	"context"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// AnnotationReader provides read access to GitHub check run annotations (SARIF).
// This is a port that must be implemented by the GitHub adapter layer.
type AnnotationReader interface {
	// ListCheckRuns returns check runs for a commit, optionally filtered by name.
	// If checkName is nil, all check runs are returned.
	// Note: Returns up to 100 check runs in GitHub API order (not guaranteed to be sorted).
	// Callers should sort results if ordering is important.
	ListCheckRuns(ctx context.Context, owner, repo, ref string, checkName *string) ([]domain.CheckRunSummary, error)

	// GetAnnotations retrieves all annotations for a specific check run.
	// Returns annotations in their original order from the API.
	GetAnnotations(ctx context.Context, owner, repo string, checkRunID int64) ([]domain.Annotation, error)

	// GetAnnotation retrieves a single annotation by check run ID and index.
	// Returns ErrAnnotationNotFound if the index is out of range.
	GetAnnotation(ctx context.Context, owner, repo string, checkRunID int64, index int) (*domain.Annotation, error)
}

// CommentReader provides read access to PR review comments (accumulated findings).
// This is a port that must be implemented by the GitHub adapter layer.
type CommentReader interface {
	// ListPRComments retrieves all review comments on a PR.
	// If filterByFingerprint is true, comments are included if they have CR_FP markers
	// OR if they are from bot users. This ensures all automated review feedback is captured.
	ListPRComments(ctx context.Context, owner, repo string, prNumber int, filterByFingerprint bool) ([]domain.PRFinding, error)

	// GetPRComment retrieves a single comment by ID.
	// Returns ErrCommentNotFound if the comment doesn't exist or doesn't belong to the PR.
	// The prNumber is required to validate the comment belongs to the expected PR.
	GetPRComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*domain.PRFinding, error)

	// GetPRCommentByFingerprint retrieves a comment by its CR_FP fingerprint.
	// Returns ErrCommentNotFound if no matching comment exists.
	GetPRCommentByFingerprint(ctx context.Context, owner, repo string, prNumber int, fingerprint string) (*domain.PRFinding, error)

	// GetThreadHistory retrieves the reply chain for a comment thread.
	GetThreadHistory(ctx context.Context, owner, repo string, commentID int64) ([]domain.ThreadComment, error)
}

// PRReader provides read access to pull request metadata.
// This is a port that must be implemented by the GitHub adapter layer.
type PRReader interface {
	// GetPRMetadata retrieves metadata about a pull request.
	GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error)
}

// FileReader provides read access to file contents from git.
// This is a port that must be implemented by the git adapter layer.
type FileReader interface {
	// ReadFile retrieves the content of a file at a specific ref.
	// The ref can be a commit SHA, branch name, or tag.
	// Returns ErrFileNotFound if the file doesn't exist at that ref.
	ReadFile(ctx context.Context, path, ref string) (string, error)

	// ReadFileLines retrieves specific lines from a file with optional context.
	// startLine and endLine are 1-based and inclusive.
	// contextLines specifies how many lines to include before and after.
	// Returns a CodeContext with the content and metadata.
	ReadFileLines(ctx context.Context, path, ref string, startLine, endLine, contextLines int) (*domain.CodeContext, error)
}

// DiffReader provides read access to diff information.
// This is a port that must be implemented by the git adapter layer.
type DiffReader interface {
	// GetDiffHunk retrieves the diff hunk for a specific file and line range.
	// baseBranch is the base ref (e.g., "main").
	// targetRef is the target ref (e.g., PR head SHA).
	// Returns the unified diff hunk(s) covering the specified lines.
	GetDiffHunk(ctx context.Context, baseBranch, targetRef, file string, startLine, endLine int) (*domain.DiffContext, error)
}

// SuggestionExtractor extracts structured code suggestions from findings.
// This is a domain service that can work with either annotations or comments.
type SuggestionExtractor interface {
	// ExtractFromAnnotation extracts a suggestion from an annotation message.
	ExtractFromAnnotation(annotation *domain.Annotation) (*domain.Suggestion, error)

	// ExtractFromComment extracts a suggestion from a PR comment body.
	ExtractFromComment(finding *domain.PRFinding) (*domain.Suggestion, error)
}

// CommentWriter provides write access to PR review comments.
// This is a port that must be implemented by the GitHub adapter layer.
type CommentWriter interface {
	// ReplyToComment posts a reply to an existing PR review comment.
	// The replyTo parameter is the ID of the comment being replied to.
	// Returns the ID of the newly created reply comment.
	ReplyToComment(ctx context.Context, owner, repo string, prNumber int, replyTo int64, body string) (int64, error)

	// CreateComment creates a new review comment at a specific file and line.
	// This is used when responding to SARIF annotations which exist as check run
	// annotations rather than PR comments - we create a new comment at the same location.
	// The line parameter is the absolute line number in the file (1-based).
	// GitHub's API maps this to the appropriate diff position internally.
	// Comments are placed on the RIGHT side of the diff (new file version).
	// Returns the ID of the newly created comment.
	CreateComment(ctx context.Context, owner, repo string, prNumber int, commitSHA, path string, line int, body string) (int64, error)
}

// ReviewManager provides operations for managing PR review state.
// This is a port that must be implemented by the GitHub adapter layer.
type ReviewManager interface {
	// ResolveThread marks a review thread as resolved.
	// Requires GraphQL API as REST API doesn't support thread resolution.
	// Returns ErrThreadNotFound if the thread doesn't exist.
	ResolveThread(ctx context.Context, owner, repo string, threadID string) error

	// UnresolveThread marks a review thread as unresolved.
	// Requires GraphQL API as REST API doesn't support thread resolution.
	// Returns ErrThreadNotFound if the thread doesn't exist.
	UnresolveThread(ctx context.Context, owner, repo string, threadID string) error

	// DismissReview dismisses a PR review with a reason.
	// Returns ErrReviewNotFound if the review doesn't exist.
	DismissReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64, message string) error

	// RequestReviewers requests review from specified users or teams.
	// Returns ErrUserNotFound if any reviewer doesn't exist or can't be added.
	RequestReviewers(ctx context.Context, owner, repo string, prNumber int, reviewers []string, teamReviewers []string) error

	// ListReviews returns all reviews for a PR.
	// Needed for dismiss stale functionality to find bot reviews.
	ListReviews(ctx context.Context, owner, repo string, prNumber int) ([]Review, error)

	// FindThreadForComment finds the review thread ID for a given comment ID.
	// Returns the thread's GraphQL node ID (PRRT_...) which can be used with ResolveThread.
	// Returns ErrThreadNotFound if no thread contains the comment.
	FindThreadForComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*ThreadInfo, error)
}

// ThreadInfo contains information about a review thread.
type ThreadInfo struct {
	ID         string // GraphQL node ID (PRRT_...)
	IsResolved bool
}

// Review represents a PR review summary for the ReviewManager port.
type Review struct {
	ID          int64
	User        string
	UserType    string // "User" or "Bot"
	State       string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, PENDING
	SubmittedAt string
}
