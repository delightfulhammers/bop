package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/delightfulhammers/bop/internal/domain"
)

// mockGitChecker implements GitBranchChecker for testing.
type mockGitChecker struct {
	remoteURL       string
	remoteURLErr    error
	localBranches   map[string]bool
	remoteBranches  map[string]bool
	remoteBranchErr error
}

func (m *mockGitChecker) GetRemoteURL(ctx context.Context) (string, error) {
	return m.remoteURL, m.remoteURLErr
}

func (m *mockGitChecker) BranchExistsLocal(ctx context.Context, branch string) (bool, error) {
	return m.localBranches[branch], nil
}

func (m *mockGitChecker) BranchExistsRemote(ctx context.Context, branch string) (bool, error) {
	if m.remoteBranchErr != nil {
		return false, m.remoteBranchErr
	}
	return m.remoteBranches[branch], nil
}

// mockSessionStore implements LocalSessionStore for testing.
type mockSessionStore struct {
	sessions map[string]*domain.LocalSession
	reviews  map[string][]*domain.LocalReview
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{
		sessions: make(map[string]*domain.LocalSession),
		reviews:  make(map[string][]*domain.LocalReview),
	}
}

func (m *mockSessionStore) SaveSession(ctx context.Context, session *domain.LocalSession) error {
	m.sessions[session.ID] = session
	return nil
}

func (m *mockSessionStore) GetSession(ctx context.Context, sessionID string) (*domain.LocalSession, error) {
	return m.sessions[sessionID], nil
}

func (m *mockSessionStore) GetSessionByBranch(ctx context.Context, repository, branch string) (*domain.LocalSession, error) {
	id := domain.GenerateLocalSessionID(repository, branch)
	return m.sessions[id], nil
}

func (m *mockSessionStore) DeleteSession(ctx context.Context, sessionID string) error {
	delete(m.sessions, sessionID)
	return nil
}

func (m *mockSessionStore) ListSessions(ctx context.Context) ([]domain.LocalSession, error) {
	var sessions []domain.LocalSession
	for _, s := range m.sessions {
		sessions = append(sessions, *s)
	}
	return sessions, nil
}

func (m *mockSessionStore) ListSessionsByAge(ctx context.Context, olderThan time.Duration) ([]domain.LocalSession, error) {
	threshold := time.Now().Add(-olderThan)
	var stale []domain.LocalSession
	for _, s := range m.sessions {
		if s.LastReviewAt.Before(threshold) {
			stale = append(stale, *s)
		}
	}
	return stale, nil
}

func (m *mockSessionStore) SaveReview(ctx context.Context, review *domain.LocalReview) error {
	m.reviews[review.SessionID] = append(m.reviews[review.SessionID], review)
	return nil
}

func (m *mockSessionStore) GetReview(ctx context.Context, reviewID string) (*domain.LocalReview, error) {
	for _, reviews := range m.reviews {
		for _, r := range reviews {
			if r.ReviewID == reviewID {
				return r, nil
			}
		}
	}
	return nil, nil
}

func (m *mockSessionStore) GetLatestReview(ctx context.Context, sessionID string) (*domain.LocalReview, error) {
	reviews := m.reviews[sessionID]
	if len(reviews) == 0 {
		return nil, nil
	}
	return reviews[len(reviews)-1], nil
}

func (m *mockSessionStore) ListReviews(ctx context.Context, sessionID string) ([]domain.LocalReview, error) {
	var result []domain.LocalReview
	for _, r := range m.reviews[sessionID] {
		result = append(result, *r)
	}
	return result, nil
}

func TestService_GetOrCreateSession_New(t *testing.T) {
	ctx := context.Background()
	store := newMockSessionStore()
	gitChecker := &mockGitChecker{
		remoteURL: "https://github.com/owner/repo.git",
	}

	service := NewService(store, gitChecker, "/path/to/repo")

	session, err := service.GetOrCreateSession(ctx, "main")
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.Equal(t, "github.com/owner/repo", session.Repository)
	assert.Equal(t, "main", session.Branch)
}

func TestService_GetOrCreateSession_Existing(t *testing.T) {
	ctx := context.Background()
	store := newMockSessionStore()
	gitChecker := &mockGitChecker{
		remoteURL: "https://github.com/owner/repo.git",
	}

	service := NewService(store, gitChecker, "/path/to/repo")

	// Create first session
	session1, err := service.GetOrCreateSession(ctx, "main")
	require.NoError(t, err)

	// Get again should return same session
	session2, err := service.GetOrCreateSession(ctx, "main")
	require.NoError(t, err)

	assert.Equal(t, session1.ID, session2.ID)
}

func TestService_GetOrCreateSession_NoRemote(t *testing.T) {
	ctx := context.Background()
	store := newMockSessionStore()
	gitChecker := &mockGitChecker{
		remoteURL: "", // No remote
	}

	service := NewService(store, gitChecker, "/path/to/repo")

	session, err := service.GetOrCreateSession(ctx, "main")
	require.NoError(t, err)

	// Should use local path as fallback
	assert.Equal(t, "/path/to/repo", session.Repository)
}

func TestService_Prune_ByAge(t *testing.T) {
	ctx := context.Background()
	store := newMockSessionStore()

	// Create old and new sessions
	oldSession := domain.NewLocalSession("github.com/owner/repo", "old-branch")
	oldSession.LastReviewAt = time.Now().Add(-48 * time.Hour)
	store.sessions[oldSession.ID] = &oldSession

	newSession := domain.NewLocalSession("github.com/owner/repo", "new-branch")
	newSession.LastReviewAt = time.Now()
	store.sessions[newSession.ID] = &newSession

	service := NewService(store, nil, "")

	result, err := service.Prune(ctx, PruneOptions{
		OlderThan: 24 * time.Hour,
	})
	require.NoError(t, err)

	assert.Len(t, result.Pruned, 1)
	assert.Equal(t, oldSession.ID, result.Pruned[0].ID)

	// Verify old session is deleted
	assert.Nil(t, store.sessions[oldSession.ID])
	// New session still exists
	assert.NotNil(t, store.sessions[newSession.ID])
}

func TestService_Prune_DryRun(t *testing.T) {
	ctx := context.Background()
	store := newMockSessionStore()

	oldSession := domain.NewLocalSession("github.com/owner/repo", "old-branch")
	oldSession.LastReviewAt = time.Now().Add(-48 * time.Hour)
	store.sessions[oldSession.ID] = &oldSession

	service := NewService(store, nil, "")

	result, err := service.Prune(ctx, PruneOptions{
		OlderThan: 24 * time.Hour,
		DryRun:    true,
	})
	require.NoError(t, err)

	assert.True(t, result.DryRun)
	assert.Len(t, result.Pruned, 1)

	// Session should NOT be deleted in dry run
	assert.NotNil(t, store.sessions[oldSession.ID])
}

func TestService_Prune_Orphans(t *testing.T) {
	ctx := context.Background()
	store := newMockSessionStore()
	gitChecker := &mockGitChecker{
		remoteBranches: map[string]bool{
			"existing-branch": true,
		},
		localBranches: map[string]bool{
			"existing-branch": true,
		},
	}

	// Create sessions for existing and deleted branches
	existingSession := domain.NewLocalSession("github.com/owner/repo", "existing-branch")
	store.sessions[existingSession.ID] = &existingSession

	orphanSession := domain.NewLocalSession("github.com/owner/repo", "deleted-branch")
	store.sessions[orphanSession.ID] = &orphanSession

	service := NewService(store, gitChecker, "")

	result, err := service.Prune(ctx, PruneOptions{
		PruneOrphans: true,
	})
	require.NoError(t, err)

	assert.Len(t, result.Pruned, 1)
	assert.Equal(t, orphanSession.ID, result.Pruned[0].ID)

	// Orphan session is deleted
	assert.Nil(t, store.sessions[orphanSession.ID])
	// Existing session still exists
	assert.NotNil(t, store.sessions[existingSession.ID])
}

func TestService_Prune_OrphansWithNetworkFallback(t *testing.T) {
	ctx := context.Background()
	store := newMockSessionStore()
	gitChecker := &mockGitChecker{
		remoteBranchErr: errors.New("network error"),
		localBranches: map[string]bool{
			"local-branch": true,
		},
	}

	// Session for branch that exists locally but remote check fails
	localSession := domain.NewLocalSession("github.com/owner/repo", "local-branch")
	store.sessions[localSession.ID] = &localSession

	// Session for branch that doesn't exist locally
	orphanSession := domain.NewLocalSession("github.com/owner/repo", "orphan-branch")
	store.sessions[orphanSession.ID] = &orphanSession

	service := NewService(store, gitChecker, "")

	result, err := service.Prune(ctx, PruneOptions{
		PruneOrphans: true,
	})
	require.NoError(t, err)

	// Only orphan should be pruned (falls back to local check)
	assert.Len(t, result.Pruned, 1)
	assert.Equal(t, orphanSession.ID, result.Pruned[0].ID)
}

func TestService_Clean(t *testing.T) {
	ctx := context.Background()
	store := newMockSessionStore()

	session := domain.NewLocalSession("github.com/owner/repo", "main")
	store.sessions[session.ID] = &session

	service := NewService(store, nil, "")

	err := service.Clean(ctx, session.ID)
	require.NoError(t, err)

	assert.Nil(t, store.sessions[session.ID])
}
