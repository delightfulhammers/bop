package session

import (
	"context"
	"time"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// Service provides session management operations including pruning.
type Service struct {
	store       LocalSessionStore
	gitChecker  GitBranchChecker
	repoRootDir string
}

// NewService creates a new session service.
func NewService(store LocalSessionStore, gitChecker GitBranchChecker, repoRootDir string) *Service {
	return &Service{
		store:       store,
		gitChecker:  gitChecker,
		repoRootDir: repoRootDir,
	}
}

// GetOrCreateSession retrieves an existing session or creates a new one.
// The repository is normalized from the git remote URL.
func (s *Service) GetOrCreateSession(ctx context.Context, branch string) (*domain.LocalSession, error) {
	remoteURL, err := s.gitChecker.GetRemoteURL(ctx)
	if err != nil {
		return nil, err
	}

	repository := NormalizeRemoteURL(remoteURL)
	if repository == "" {
		// No remote configured - use local path as fallback
		repository = s.repoRootDir
	}

	session, err := s.store.GetSessionByBranch(ctx, repository, branch)
	if err != nil {
		return nil, err
	}

	if session != nil {
		return session, nil
	}

	// Create new session
	newSession := domain.NewLocalSession(repository, branch)
	if err := s.store.SaveSession(ctx, &newSession); err != nil {
		return nil, err
	}

	return &newSession, nil
}

// SaveReview saves a review to the session.
func (s *Service) SaveReview(ctx context.Context, review *domain.LocalReview) error {
	return s.store.SaveReview(ctx, review)
}

// GetLatestReview retrieves the most recent review for a session.
func (s *Service) GetLatestReview(ctx context.Context, sessionID string) (*domain.LocalReview, error) {
	return s.store.GetLatestReview(ctx, sessionID)
}

// ListSessions returns all sessions.
func (s *Service) ListSessions(ctx context.Context) ([]domain.LocalSession, error) {
	return s.store.ListSessions(ctx)
}

// PruneResult contains the results of a prune operation.
type PruneResult struct {
	Pruned    []domain.LocalSession
	Errors    []PruneError
	DryRun    bool
	Threshold time.Duration
}

// PruneError represents an error encountered while pruning a specific session.
type PruneError struct {
	Session domain.LocalSession
	Error   error
}

// PruneOptions configures the prune operation.
type PruneOptions struct {
	// OlderThan prunes sessions with LastReviewAt older than this duration.
	// If zero, age-based pruning is skipped.
	OlderThan time.Duration

	// PruneOrphans removes sessions whose branch no longer exists.
	// Checks remote first, falls back to local if network unavailable.
	PruneOrphans bool

	// DryRun if true, doesn't actually delete sessions.
	DryRun bool
}

// Prune removes stale sessions based on the provided options.
func (s *Service) Prune(ctx context.Context, opts PruneOptions) (*PruneResult, error) {
	result := &PruneResult{
		DryRun:    opts.DryRun,
		Threshold: opts.OlderThan,
	}

	var candidateSessions []domain.LocalSession

	// Get sessions to consider for pruning
	if opts.OlderThan > 0 {
		stale, err := s.store.ListSessionsByAge(ctx, opts.OlderThan)
		if err != nil {
			return nil, err
		}
		candidateSessions = append(candidateSessions, stale...)
	} else if opts.PruneOrphans {
		// If only pruning orphans, get all sessions
		all, err := s.store.ListSessions(ctx)
		if err != nil {
			return nil, err
		}
		candidateSessions = all
	}

	// Remove duplicates (in case a session matches multiple criteria)
	seen := make(map[string]bool)
	var uniqueSessions []domain.LocalSession
	for _, session := range candidateSessions {
		if !seen[session.ID] {
			seen[session.ID] = true
			uniqueSessions = append(uniqueSessions, session)
		}
	}

	// Check each session
	for _, session := range uniqueSessions {
		shouldPrune := false

		// Age-based check already done by ListSessionsByAge
		if opts.OlderThan > 0 && session.IsStale(opts.OlderThan) {
			shouldPrune = true
		}

		// Orphan check
		if opts.PruneOrphans && !shouldPrune {
			exists, err := s.branchExists(ctx, session.Branch)
			if err != nil {
				result.Errors = append(result.Errors, PruneError{
					Session: session,
					Error:   err,
				})
				continue
			}
			if !exists {
				shouldPrune = true
			}
		}

		if shouldPrune {
			if !opts.DryRun {
				if err := s.store.DeleteSession(ctx, session.ID); err != nil {
					result.Errors = append(result.Errors, PruneError{
						Session: session,
						Error:   err,
					})
					continue
				}
			}
			result.Pruned = append(result.Pruned, session)
		}
	}

	return result, nil
}

// branchExists checks if a branch exists, preferring remote check with local fallback.
func (s *Service) branchExists(ctx context.Context, branch string) (bool, error) {
	if s.gitChecker == nil {
		return true, nil // No checker, assume exists
	}

	// Try remote first
	exists, err := s.gitChecker.BranchExistsRemote(ctx, branch)
	if err == nil {
		return exists, nil
	}

	// Network error - fall back to local
	return s.gitChecker.BranchExistsLocal(ctx, branch)
}

// Clean removes a specific session by ID.
func (s *Service) Clean(ctx context.Context, sessionID string) error {
	return s.store.DeleteSession(ctx, sessionID)
}
