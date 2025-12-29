package triage

import (
	"context"
	"fmt"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// PRServiceDeps contains the dependencies for the PR-based triage service.
type PRServiceDeps struct {
	AnnotationReader AnnotationReader
	CommentReader    CommentReader
	PRReader         PRReader
	FileReader       FileReader
	DiffReader       DiffReader
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
		return nil, fmt.Errorf("invalid severity filter: %s (valid: critical, high, medium, low)", *severity)
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

	return s.deps.CommentReader.GetPRComment(ctx, owner, repo, commentID)
}

// GetSuggestion extracts a structured code suggestion from a finding.
//
// TODO: Not yet implemented. Will support extracting code suggestions from
// both annotations (raw_details field) and PR comments (markdown suggestion blocks).
// Returns ErrNotImplemented until parsing logic is added.
func (s *PRService) GetSuggestion(ctx context.Context, owner, repo string, prNumber int, findingID string) (*domain.Suggestion, error) {
	// Check dependencies - requires CommentReader to fetch finding content
	if s.deps.CommentReader == nil {
		return nil, ErrNotImplemented
	}
	// Not yet implemented - requires parsing suggestion blocks from finding messages
	return nil, ErrNotImplemented
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
func isValidSeverity(s string) bool {
	switch s {
	case "critical", "high", "medium", "low":
		return true
	default:
		return false
	}
}
