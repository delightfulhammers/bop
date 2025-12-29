package triage

import (
	"context"
	"fmt"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// PRServiceDeps contains the dependencies for the PR-based triage service.
type PRServiceDeps struct {
	AnnotationReader    AnnotationReader
	CommentReader       CommentReader
	PRReader            PRReader
	FileReader          FileReader
	DiffReader          DiffReader
	SuggestionExtractor SuggestionExtractor
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
		if err == nil && finding != nil {
			suggestion, err := s.deps.SuggestionExtractor.ExtractFromComment(finding)
			if err == nil {
				return suggestion, nil
			}
			// If extraction failed (no suggestion block), fall through to try annotation
			if err != ErrNoSuggestion {
				return nil, err
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
func parseAnnotationID(id string) (checkRunID int64, index int, ok bool) {
	var cid int64
	var idx int
	n, err := fmt.Sscanf(id, "%d:%d", &cid, &idx)
	if err != nil || n != 2 {
		return 0, 0, false
	}
	return cid, idx, true
}
