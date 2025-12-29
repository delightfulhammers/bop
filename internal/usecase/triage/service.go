package triage

import (
	"context"
	"errors"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// Note: ErrNotImplemented is defined in errors.go

// ErrSessionNotFound is returned when a session ID doesn't exist.
var ErrSessionNotFound = errors.New("triage session not found")

// ErrFindingNotFound is returned when a finding ID doesn't exist in a session.
var ErrFindingNotFound = errors.New("finding not found in session")

// ErrNoActiveSession is returned when an operation requires an active session.
var ErrNoActiveSession = errors.New("no active triage session")

// ServiceDeps contains the dependencies for the triage service.
type ServiceDeps struct {
	ReviewRepo   ReviewRepository
	GitHubClient GitHubClient
	SessionStore SessionStore
}

// Service implements the TriageService interface.
type Service struct {
	deps ServiceDeps
}

// NewService creates a new triage service with the given dependencies.
func NewService(deps ServiceDeps) *Service {
	return &Service{deps: deps}
}

// StartSession begins a new triage session for a PR.
func (s *Service) StartSession(ctx context.Context, repository string, prNumber int) (*domain.TriageSession, error) {
	// M2: Implement full logic
	// 1. Fetch review from ReviewRepo
	// 2. Create TriageSession from findings
	// 3. Persist to SessionStore
	// 4. Return session
	return nil, ErrNotImplemented
}

// ResumeSession retrieves an existing session.
func (s *Service) ResumeSession(ctx context.Context, sessionID string) (*domain.TriageSession, error) {
	// M2: Implement full logic
	// 1. Load from SessionStore
	// 2. Return session or ErrSessionNotFound
	return nil, ErrNotImplemented
}

// GetCurrentFinding returns the current finding to triage.
func (s *Service) GetCurrentFinding(ctx context.Context, sessionID string) (*domain.TriageFinding, error) {
	// M2: Implement full logic
	// 1. Load session
	// 2. Return CurrentFinding() or next pending
	return nil, ErrNotImplemented
}

// GetFindingContext retrieves contextual information for a finding.
func (s *Service) GetFindingContext(ctx context.Context, sessionID string, findingID string) (*domain.TriageContext, error) {
	// M2: Implement full logic
	// 1. Load session and finding
	// 2. Fetch PR context from GitHub
	// 3. Fetch file content around finding lines
	// 4. Fetch thread history if comment exists
	return nil, ErrNotImplemented
}

// TriageFinding applies a triage decision to a finding.
func (s *Service) TriageFinding(ctx context.Context, sessionID string, findingID string, decision domain.TriageDecision) error {
	// M2: Implement full logic
	// 1. Load session
	// 2. Apply decision to finding
	// 3. Persist updated session
	return ErrNotImplemented
}

// GetProgress returns triage progress for a session.
func (s *Service) GetProgress(ctx context.Context, sessionID string) (triaged, total int, err error) {
	// M2: Implement full logic
	// 1. Load session
	// 2. Return session.Progress()
	return 0, 0, ErrNotImplemented
}

// ListFindings returns all findings in a session with optional filters.
func (s *Service) ListFindings(ctx context.Context, sessionID string, statusFilter *domain.TriageStatus) ([]domain.TriageFinding, error) {
	// M2: Implement full logic
	// 1. Load session
	// 2. Filter by status if provided
	// 3. Return findings
	return nil, ErrNotImplemented
}

// PostComment posts a review comment to GitHub for a finding.
func (s *Service) PostComment(ctx context.Context, sessionID string, findingID string, body string) error {
	// M3: Implement full logic (write operation)
	// 1. Load session and finding
	// 2. Post comment via GitHubClient
	// 3. Update finding with comment ID
	// 4. Persist session
	return ErrNotImplemented
}

// ReplyToFinding replies to an existing GitHub thread for a finding.
func (s *Service) ReplyToFinding(ctx context.Context, sessionID string, findingID string, body string) error {
	// M3: Implement full logic (write operation)
	// 1. Load session and finding
	// 2. Verify comment exists
	// 3. Reply via GitHubClient
	return ErrNotImplemented
}

// ResolveFinding marks a GitHub thread as resolved.
func (s *Service) ResolveFinding(ctx context.Context, sessionID string, findingID string) error {
	// M3: Implement full logic (write operation)
	// 1. Load session and finding
	// 2. Verify comment exists
	// 3. Resolve via GitHubClient
	// 4. Update finding.ThreadResolved
	// 5. Persist session
	return ErrNotImplemented
}

// Ensure Service implements TriageService at compile time.
var _ TriageService = (*Service)(nil)
