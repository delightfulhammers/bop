package triage

import (
	"context"
	"errors"
	"fmt"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// PRServiceDeps contains the dependencies for the PR-based triage service.
type PRServiceDeps struct {
	// Read operations
	AnnotationReader    AnnotationReader
	CommentReader       CommentReader
	PRReader            PRReader
	FileReader          FileReader
	DiffReader          DiffReader
	SuggestionExtractor SuggestionExtractor

	// Write operations (optional - set to nil for read-only mode)
	CommentWriter CommentWriter
	ReviewManager ReviewManager
}

// PRService implements read-only triage operations for a PR.
// Unlike the session-based Service, this operates statelessly on PR data.
type PRService struct {
	deps PRServiceDeps
}

// NewPRService creates a new PR-based triage service.
func NewPRService(deps PRServiceDeps) *PRService {
	return &PRService{deps: deps}
}

// ListAnnotations returns all SARIF annotations for a PR's head commit.
// Optionally filters by check name and/or annotation level.
func (s *PRService) ListAnnotations(ctx context.Context, owner, repo string, prNumber int, checkName *string, level *domain.AnnotationLevel) ([]domain.Annotation, error) {
	// Validate required dependencies
	if s.deps.PRReader == nil || s.deps.AnnotationReader == nil {
		return nil, ErrNotImplemented
	}

	// Get PR metadata to find head SHA
	prMeta, err := s.deps.PRReader.GetPRMetadata(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("get PR metadata: %w", err)
	}

	// List check runs for the head commit
	checkRuns, err := s.deps.AnnotationReader.ListCheckRuns(ctx, owner, repo, prMeta.HeadSHA, checkName)
	if err != nil {
		return nil, fmt.Errorf("list check runs: %w", err)
	}

	if len(checkRuns) == 0 {
		return []domain.Annotation{}, nil
	}

	// Collect annotations from all matching check runs
	var allAnnotations []domain.Annotation
	for _, cr := range checkRuns {
		if cr.AnnotationCount == 0 {
			continue
		}

		annotations, err := s.deps.AnnotationReader.GetAnnotations(ctx, owner, repo, cr.ID)
		if err != nil {
			return nil, fmt.Errorf("get annotations for check run %d: %w", cr.ID, err)
		}

		// Apply level filter if specified
		for _, ann := range annotations {
			if ann.MatchesLevelFilter(level) {
				allAnnotations = append(allAnnotations, ann)
			}
		}
	}

	return allAnnotations, nil
}

// GetAnnotation retrieves a single annotation by check run ID and index.
func (s *PRService) GetAnnotation(ctx context.Context, owner, repo string, checkRunID int64, index int) (*domain.Annotation, error) {
	if s.deps.AnnotationReader == nil {
		return nil, ErrNotImplemented
	}
	return s.deps.AnnotationReader.GetAnnotation(ctx, owner, repo, checkRunID, index)
}

// ListFindings returns all PR comments that are code reviewer findings.
// Optionally filters by severity and/or category.
func (s *PRService) ListFindings(ctx context.Context, owner, repo string, prNumber int, severity, category *string) ([]domain.PRFinding, error) {
	if s.deps.CommentReader == nil {
		return nil, ErrNotImplemented
	}

	// Validate severity filter if provided
	if severity != nil && !isValidSeverity(*severity) {
		return nil, fmt.Errorf("%w: severity %q (valid: %v)", ErrInvalidFilter, *severity, ValidSeverities)
	}

	// Get all comments with fingerprints (code reviewer findings)
	findings, err := s.deps.CommentReader.ListPRComments(ctx, owner, repo, prNumber, true)
	if err != nil {
		return nil, fmt.Errorf("list PR comments: %w", err)
	}

	// Apply filters
	var filtered []domain.PRFinding
	for _, f := range findings {
		if severity != nil && f.Severity != *severity {
			continue
		}
		if category != nil && f.Category != *category {
			continue
		}
		filtered = append(filtered, f)
	}

	return filtered, nil
}

// GetFinding retrieves a single finding by ID.
// The ID can be either a fingerprint (CR_FP:xxx) or a GitHub comment ID.
func (s *PRService) GetFinding(ctx context.Context, owner, repo string, prNumber int, findingID string) (*domain.PRFinding, error) {
	if s.deps.CommentReader == nil {
		return nil, ErrNotImplemented
	}

	fingerprint, commentID, isFingerprint := domain.ResolveFindingID(findingID)

	if isFingerprint {
		return s.deps.CommentReader.GetPRCommentByFingerprint(ctx, owner, repo, prNumber, fingerprint)
	}

	return s.deps.CommentReader.GetPRComment(ctx, owner, repo, prNumber, commentID)
}

// GetSuggestion extracts a structured code suggestion from a finding.
// Supports extracting from both PR comments (markdown suggestion blocks)
// and annotations (raw_details field).
func (s *PRService) GetSuggestion(ctx context.Context, owner, repo string, prNumber int, findingID string) (*domain.Suggestion, error) {
	// Check dependencies
	if s.deps.SuggestionExtractor == nil {
		return nil, ErrNotImplemented
	}

	// Try to fetch as a PR comment first
	if s.deps.CommentReader != nil {
		finding, err := s.GetFinding(ctx, owner, repo, prNumber, findingID)
		if err != nil {
			// Only fall through to annotation lookup for "not found" errors.
			// Real errors (network, auth, etc.) should be propagated.
			if !errors.Is(err, ErrCommentNotFound) {
				return nil, fmt.Errorf("get finding: %w", err)
			}
			// Comment not found - fall through to try annotation lookup
		} else if finding != nil {
			suggestion, extractErr := s.deps.SuggestionExtractor.ExtractFromComment(finding)
			if extractErr == nil {
				return suggestion, nil
			}
			// If extraction failed (no suggestion block), fall through to try annotation
			if extractErr != ErrNoSuggestion {
				return nil, extractErr
			}
		}
	}

	// Try to fetch as an annotation
	// The findingID might be a check run ID + index format like "checkRunID:index"
	if s.deps.AnnotationReader != nil {
		// For annotations, the ID format could be "checkRunID:index"
		// Parse and fetch the annotation
		checkRunID, index, ok := parseAnnotationID(findingID)
		if ok {
			annotation, err := s.deps.AnnotationReader.GetAnnotation(ctx, owner, repo, checkRunID, index)
			if err == nil && annotation != nil {
				return s.deps.SuggestionExtractor.ExtractFromAnnotation(annotation)
			}
		}
	}

	return nil, ErrNoSuggestion
}

// GetCodeContext retrieves file content at the PR's head ref.
func (s *PRService) GetCodeContext(ctx context.Context, owner, repo string, prNumber int, file string, startLine, endLine, contextLines int) (*domain.CodeContext, error) {
	if s.deps.FileReader == nil || s.deps.PRReader == nil {
		return nil, ErrNotImplemented
	}

	// Get PR metadata to find head SHA
	prMeta, err := s.deps.PRReader.GetPRMetadata(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("get PR metadata: %w", err)
	}

	return s.deps.FileReader.ReadFileLines(ctx, file, prMeta.HeadSHA, startLine, endLine, contextLines)
}

// GetDiffContext retrieves the diff hunk for a file at specific lines.
func (s *PRService) GetDiffContext(ctx context.Context, owner, repo string, prNumber int, file string, startLine, endLine int) (*domain.DiffContext, error) {
	if s.deps.DiffReader == nil || s.deps.PRReader == nil {
		return nil, ErrNotImplemented
	}

	// Get PR metadata to find refs
	prMeta, err := s.deps.PRReader.GetPRMetadata(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("get PR metadata: %w", err)
	}

	return s.deps.DiffReader.GetDiffHunk(ctx, prMeta.BaseRef, prMeta.HeadSHA, file, startLine, endLine)
}

// isValidSeverity checks if a severity string is a valid value.
// Uses ValidSeverities to ensure consistency.
func isValidSeverity(s string) bool {
	for _, valid := range ValidSeverities {
		if s == valid {
			return true
		}
	}
	return false
}

// parseAnnotationID parses an annotation ID in the format "checkRunID:index".
// Returns the parsed values and whether parsing succeeded.
// The entire string must match the format exactly - no trailing content is allowed.
func parseAnnotationID(id string) (checkRunID int64, index int, ok bool) {
	var cid int64
	var idx int
	n, err := fmt.Sscanf(id, "%d:%d", &cid, &idx)
	if err != nil || n != 2 {
		return 0, 0, false
	}
	// Verify no trailing content by roundtripping: format back and compare
	// This rejects inputs like "1001:0xyz" or "1001:0 extra"
	if fmt.Sprintf("%d:%d", cid, idx) != id {
		return 0, 0, false
	}
	return cid, idx, true
}

// =============================================================================
// Write Operations
// =============================================================================

// ReplyToFinding posts a reply to a code reviewer finding.
// The findingID can be a fingerprint (CR_FP:xxx) or a GitHub comment ID.
// Returns the ID of the newly created reply comment.
func (s *PRService) ReplyToFinding(ctx context.Context, owner, repo string, prNumber int, findingID, body string) (int64, error) {
	if s.deps.CommentWriter == nil {
		return 0, ErrNotImplemented
	}
	if s.deps.CommentReader == nil {
		return 0, fmt.Errorf("CommentReader required to look up finding: %w", ErrNotImplemented)
	}

	// Look up the finding to get its comment ID
	finding, err := s.GetFinding(ctx, owner, repo, prNumber, findingID)
	if err != nil {
		return 0, fmt.Errorf("get finding: %w", err)
	}

	// Reply to the finding's comment
	return s.deps.CommentWriter.ReplyToComment(ctx, owner, repo, prNumber, finding.CommentID, body)
}

// PostComment creates a new review comment at a specific file and line.
// This is used when responding to SARIF annotations - we create a new comment
// at the same location since annotations cannot be replied to directly.
func (s *PRService) PostComment(ctx context.Context, owner, repo string, prNumber int, file string, line int, body string) (int64, error) {
	if s.deps.CommentWriter == nil {
		return 0, ErrNotImplemented
	}
	if s.deps.PRReader == nil {
		return 0, fmt.Errorf("PRReader required to get head SHA: %w", ErrNotImplemented)
	}

	// Get PR metadata to find head SHA (required for creating comments)
	prMeta, err := s.deps.PRReader.GetPRMetadata(ctx, owner, repo, prNumber)
	if err != nil {
		return 0, fmt.Errorf("get PR metadata: %w", err)
	}

	return s.deps.CommentWriter.CreateComment(ctx, owner, repo, prNumber, prMeta.HeadSHA, file, line, body)
}

// ResolveThread marks a review thread as resolved.
// The threadID should be the node_id of the review thread (e.g., "PRRT_kwDO...").
func (s *PRService) ResolveThread(ctx context.Context, owner, repo, threadID string) error {
	if s.deps.ReviewManager == nil {
		return ErrNotImplemented
	}

	return s.deps.ReviewManager.ResolveThread(ctx, owner, repo, threadID)
}

// UnresolveThread marks a review thread as unresolved.
// The threadID should be the node_id of the review thread (e.g., "PRRT_kwDO...").
func (s *PRService) UnresolveThread(ctx context.Context, owner, repo, threadID string) error {
	if s.deps.ReviewManager == nil {
		return ErrNotImplemented
	}

	return s.deps.ReviewManager.UnresolveThread(ctx, owner, repo, threadID)
}

// DismissReview dismisses a PR review with the given message.
// This is used to clear stale bot reviews when re-requesting review.
func (s *PRService) DismissReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64, message string) error {
	if s.deps.ReviewManager == nil {
		return ErrNotImplemented
	}

	return s.deps.ReviewManager.DismissReview(ctx, owner, repo, prNumber, reviewID, message)
}

// RequestReview requests review from specified users or teams.
// This triggers a new review request notification.
func (s *PRService) RequestReview(ctx context.Context, owner, repo string, prNumber int, reviewers []string, teamReviewers []string) error {
	if s.deps.ReviewManager == nil {
		return ErrNotImplemented
	}

	return s.deps.ReviewManager.RequestReviewers(ctx, owner, repo, prNumber, reviewers, teamReviewers)
}

// GetThreadHistory retrieves the reply chain for a comment thread.
func (s *PRService) GetThreadHistory(ctx context.Context, owner, repo string, commentID int64) ([]domain.ThreadComment, error) {
	if s.deps.CommentReader == nil {
		return nil, ErrNotImplemented
	}

	return s.deps.CommentReader.GetThreadHistory(ctx, owner, repo, commentID)
}

// ListReviews returns all reviews for a PR.
// This is primarily used for the dismiss stale functionality to find bot reviews.
func (s *PRService) ListReviews(ctx context.Context, owner, repo string, prNumber int) ([]Review, error) {
	if s.deps.ReviewManager == nil {
		return nil, ErrNotImplemented
	}

	return s.deps.ReviewManager.ListReviews(ctx, owner, repo, prNumber)
}
