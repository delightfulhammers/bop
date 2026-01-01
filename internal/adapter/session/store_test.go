package session

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

func TestFileStore_SaveAndGetSession(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	// Session ID must be exactly 16 hex characters (path traversal protection)
	sessionID := "abc123def4567890"

	session := &domain.LocalSession{
		ID:           sessionID,
		Repository:   "github.com/owner/repo",
		Branch:       "main",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		LastReviewAt: time.Now(),
		ReviewCount:  5,
		Metadata:     map[string]string{"key": "value"},
	}

	// Save session
	err = store.SaveSession(ctx, session)
	require.NoError(t, err)

	// Get session
	retrieved, err := store.GetSession(ctx, sessionID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, session.ID, retrieved.ID)
	assert.Equal(t, session.Repository, retrieved.Repository)
	assert.Equal(t, session.Branch, retrieved.Branch)
	assert.Equal(t, session.ReviewCount, retrieved.ReviewCount)
	assert.Equal(t, session.Metadata["key"], retrieved.Metadata["key"])
}

func TestFileStore_GetSession_NotFound(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	// Use valid 16-char hex ID that doesn't exist
	retrieved, err := store.GetSession(ctx, "0000000000000000")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestFileStore_GetSessionByBranch(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	session := domain.NewLocalSession("github.com/owner/repo", "feature")
	err = store.SaveSession(ctx, &session)
	require.NoError(t, err)

	// Get by branch
	retrieved, err := store.GetSessionByBranch(ctx, "github.com/owner/repo", "feature")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, session.ID, retrieved.ID)
}

func TestFileStore_DeleteSession(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	session := domain.NewLocalSession("github.com/owner/repo", "main")
	err = store.SaveSession(ctx, &session)
	require.NoError(t, err)

	// Verify exists
	retrieved, err := store.GetSession(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Delete
	err = store.DeleteSession(ctx, session.ID)
	require.NoError(t, err)

	// Verify gone
	retrieved, err = store.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestFileStore_DeleteSession_NotFound(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	// Delete non-existent session should not error (use valid 16-char hex ID)
	err = store.DeleteSession(ctx, "0000000000000000")
	require.NoError(t, err)
}

func TestFileStore_ListSessions(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	// Create multiple sessions
	session1 := domain.NewLocalSession("github.com/owner/repo1", "main")
	session1.LastReviewAt = time.Now().Add(-2 * time.Hour)
	err = store.SaveSession(ctx, &session1)
	require.NoError(t, err)

	session2 := domain.NewLocalSession("github.com/owner/repo2", "main")
	session2.LastReviewAt = time.Now().Add(-1 * time.Hour)
	err = store.SaveSession(ctx, &session2)
	require.NoError(t, err)

	session3 := domain.NewLocalSession("github.com/owner/repo3", "main")
	session3.LastReviewAt = time.Now()
	err = store.SaveSession(ctx, &session3)
	require.NoError(t, err)

	// List sessions
	sessions, err := store.ListSessions(ctx)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)

	// Should be sorted by last review time, most recent first
	assert.Equal(t, session3.ID, sessions[0].ID)
	assert.Equal(t, session2.ID, sessions[1].ID)
	assert.Equal(t, session1.ID, sessions[2].ID)
}

func TestFileStore_ListSessionsByAge(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	// Create sessions with different ages
	oldSession := domain.NewLocalSession("github.com/owner/old", "main")
	oldSession.LastReviewAt = time.Now().Add(-48 * time.Hour)
	err = store.SaveSession(ctx, &oldSession)
	require.NoError(t, err)

	newSession := domain.NewLocalSession("github.com/owner/new", "main")
	newSession.LastReviewAt = time.Now()
	err = store.SaveSession(ctx, &newSession)
	require.NoError(t, err)

	// List sessions older than 24 hours
	stale, err := store.ListSessionsByAge(ctx, 24*time.Hour)
	require.NoError(t, err)
	assert.Len(t, stale, 1)
	assert.Equal(t, oldSession.ID, stale[0].ID)
}

func TestFileStore_SaveAndGetReview(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	// Create session first
	session := domain.NewLocalSession("github.com/owner/repo", "main")
	err = store.SaveSession(ctx, &session)
	require.NoError(t, err)

	// Save review
	review := &domain.LocalReview{
		ReviewID:     "review-123",
		SessionID:    session.ID,
		CreatedAt:    time.Now(),
		BaseRef:      "main",
		HeadCommit:   "abc123",
		TotalCost:    0.05,
		FindingCount: 3,
		Findings: []domain.Finding{
			{
				ID:       "f1",
				File:     "main.go",
				Severity: "medium",
			},
		},
	}

	err = store.SaveReview(ctx, review)
	require.NoError(t, err)

	// Get review
	retrieved, err := store.GetReview(ctx, "review-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, review.ReviewID, retrieved.ReviewID)
	assert.Equal(t, review.SessionID, retrieved.SessionID)
	assert.Equal(t, review.BaseRef, retrieved.BaseRef)
	assert.Equal(t, review.HeadCommit, retrieved.HeadCommit)
	assert.Equal(t, review.TotalCost, retrieved.TotalCost)
	assert.Len(t, retrieved.Findings, 1)

	// Verify session was updated
	updatedSession, err := store.GetSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, updatedSession.ReviewCount)
}

func TestFileStore_GetLatestReview(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	session := domain.NewLocalSession("github.com/owner/repo", "main")
	err = store.SaveSession(ctx, &session)
	require.NoError(t, err)

	// Save multiple reviews
	review1 := &domain.LocalReview{
		ReviewID:  "review-1",
		SessionID: session.ID,
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}
	err = store.SaveReview(ctx, review1)
	require.NoError(t, err)

	review2 := &domain.LocalReview{
		ReviewID:  "review-2",
		SessionID: session.ID,
		CreatedAt: time.Now(),
	}
	err = store.SaveReview(ctx, review2)
	require.NoError(t, err)

	// Get latest should return review2
	latest, err := store.GetLatestReview(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, "review-2", latest.ReviewID)
}

func TestFileStore_GetLatestReview_Empty(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	session := domain.NewLocalSession("github.com/owner/repo", "main")
	err = store.SaveSession(ctx, &session)
	require.NoError(t, err)

	latest, err := store.GetLatestReview(ctx, session.ID)
	require.NoError(t, err)
	assert.Nil(t, latest)
}

func TestFileStore_ListReviews(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	session := domain.NewLocalSession("github.com/owner/repo", "main")
	err = store.SaveSession(ctx, &session)
	require.NoError(t, err)

	// Save multiple reviews
	for i := 0; i < 3; i++ {
		review := &domain.LocalReview{
			ReviewID:  fmt.Sprintf("review-%d", i),
			SessionID: session.ID,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Hour),
		}
		err = store.SaveReview(ctx, review)
		require.NoError(t, err)
	}

	reviews, err := store.ListReviews(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, reviews, 3)

	// Should be sorted by creation time, most recent first
	assert.Equal(t, "review-2", reviews[0].ReviewID)
	assert.Equal(t, "review-1", reviews[1].ReviewID)
	assert.Equal(t, "review-0", reviews[2].ReviewID)
}

func TestFileStore_ContextCancellation(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	session := domain.NewLocalSession("github.com/owner/repo", "main")
	err = store.SaveSession(ctx, &session)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestFileStore_PathTraversalProtection(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	testCases := []struct {
		name      string
		sessionID string
		valid     bool
	}{
		{"valid 16 hex chars", "0123456789abcdef", true},
		{"valid uppercase hex", "ABCDEF0123456789", true},
		{"path traversal attempt", "../../../etc/passwd", false},
		{"too short", "abc123", false},
		{"too long", "0123456789abcdef0", false},
		{"non-hex characters", "0123456789abcdeg", false},
		{"empty string", "", false},
		{"slashes in ID", "01234567/abcdef", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			session := &domain.LocalSession{
				ID:         tc.sessionID,
				Repository: "github.com/owner/repo",
				Branch:     "main",
			}

			err := store.SaveSession(ctx, session)
			if tc.valid {
				assert.NoError(t, err, "expected save to succeed for valid ID")
			} else {
				assert.Error(t, err, "expected save to fail for invalid ID")
			}
		})
	}
}

func TestFileStore_SaveReview_SessionNotFound(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	// Try to save a review without first creating the session
	review := &domain.LocalReview{
		ReviewID:  "review-123",
		SessionID: "0123456789abcdef", // Valid format but session doesn't exist
		CreatedAt: time.Now(),
	}

	err = store.SaveReview(ctx, review)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestFileStore_ReviewIDValidation(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	// Create a session first
	session := domain.NewLocalSession("github.com/owner/repo", "main")
	err = store.SaveSession(ctx, &session)
	require.NoError(t, err)

	testCases := []struct {
		name     string
		reviewID string
		valid    bool
	}{
		{"valid alphanumeric", "review123", true},
		{"valid with hyphens", "review-123-abc", true},
		{"valid with underscores", "review_123_abc", true},
		{"path traversal attempt", "../../../etc/passwd", false},
		{"slashes in ID", "review/123", false},
		{"empty string", "", false},
		{"dots only", "..", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			review := &domain.LocalReview{
				ReviewID:  tc.reviewID,
				SessionID: session.ID,
				CreatedAt: time.Now(),
			}

			err := store.SaveReview(ctx, review)
			if tc.valid {
				assert.NoError(t, err, "expected save to succeed for valid review ID")
			} else {
				assert.Error(t, err, "expected save to fail for invalid review ID")
			}
		})
	}
}
