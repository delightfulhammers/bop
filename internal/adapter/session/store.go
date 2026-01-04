package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/delightfulhammers/bop/internal/domain"
)

// Errors returned by FileStore operations.
var (
	ErrInvalidSessionID = errors.New("invalid session ID: must be exactly 16 hex characters")
	ErrInvalidReviewID  = errors.New("invalid review ID: contains path traversal characters")
	ErrSessionNotFound  = errors.New("session not found")
)

// FileStore implements LocalSessionStore using the filesystem.
// Storage layout:
//
//	~/.cache/bop/sessions/<session_id>/
//	  meta.json     - Session metadata (LocalSession without findings)
//	  reviews/      - Directory containing individual reviews
//	    <review_id>.json
type FileStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileStore creates a new file-based session store.
// If baseDir is empty, defaults to ~/.cache/bop/sessions.
func NewFileStore(baseDir string) (*FileStore, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
		baseDir = filepath.Join(home, ".cache", "bop", "sessions")
	}

	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("create base directory: %w", err)
	}

	return &FileStore{baseDir: baseDir}, nil
}

// SaveSession persists a local session.
func (s *FileStore) SaveSession(ctx context.Context, session *domain.LocalSession) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	sessionDir, err := s.sessionDir(session.ID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}

	// Create reviews directory
	reviewsDir := filepath.Join(sessionDir, "reviews")
	if err := os.MkdirAll(reviewsDir, 0o700); err != nil {
		return fmt.Errorf("create reviews directory: %w", err)
	}

	return s.writeSessionMeta(session)
}

// GetSession retrieves a session by ID.
// Note: A read lock is sufficient here because the only creation path
// (SaveSession) uses a write lock. Concurrent GetSession calls during
// GetOrCreateSession are safe - duplicate creation attempts are prevented
// by SaveSession's write lock, and the session will be found on retry.
func (s *FileStore) GetSession(ctx context.Context, sessionID string) (*domain.LocalSession, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if !isValidSessionID(sessionID) {
		return nil, ErrInvalidSessionID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.readSessionMeta(sessionID)
}

// GetSessionByBranch retrieves the session for a repository/branch pair.
func (s *FileStore) GetSessionByBranch(ctx context.Context, repository, branch string) (*domain.LocalSession, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	sessionID := domain.GenerateLocalSessionID(repository, branch)
	return s.GetSession(ctx, sessionID)
}

// DeleteSession removes a session and all its associated reviews.
func (s *FileStore) DeleteSession(ctx context.Context, sessionID string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	sessionDir, err := s.sessionDir(sessionID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if session exists
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return nil // Already deleted, no error
	}

	return os.RemoveAll(sessionDir)
}

// ListSessions returns all sessions.
func (s *FileStore) ListSessions(ctx context.Context) ([]domain.LocalSession, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	var sessions []domain.LocalSession
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip directories with invalid session ID format
		if !isValidSessionID(entry.Name()) {
			continue
		}

		session, err := s.readSessionMeta(entry.Name())
		if err != nil {
			// Skip corrupted sessions but continue
			continue
		}
		if session == nil {
			continue
		}

		sessions = append(sessions, *session)
	}

	// Sort by last review time, most recent first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastReviewAt.After(sessions[j].LastReviewAt)
	})

	return sessions, nil
}

// ListSessionsByAge returns sessions with lastReviewAt older than the given duration.
func (s *FileStore) ListSessionsByAge(ctx context.Context, olderThan time.Duration) ([]domain.LocalSession, error) {
	sessions, err := s.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	threshold := time.Now().Add(-olderThan)
	var stale []domain.LocalSession
	for _, session := range sessions {
		if session.LastReviewAt.Before(threshold) {
			stale = append(stale, session)
		}
	}

	return stale, nil
}

// SaveReview saves a review to a session.
func (s *FileStore) SaveReview(ctx context.Context, review *domain.LocalReview) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	reviewPath, err := s.reviewPath(review.SessionID, review.ReviewID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify session exists BEFORE writing review file to prevent orphaned reviews
	session, err := s.readSessionMeta(review.SessionID)
	if err != nil {
		return fmt.Errorf("read session meta: %w", err)
	}
	if session == nil {
		return fmt.Errorf("save review: %w", ErrSessionNotFound)
	}

	reviewDir := filepath.Dir(reviewPath)
	if err := os.MkdirAll(reviewDir, 0o700); err != nil {
		return fmt.Errorf("create reviews directory: %w", err)
	}

	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal review: %w", err)
	}

	if err := os.WriteFile(reviewPath, data, 0o600); err != nil {
		return fmt.Errorf("write review file: %w", err)
	}

	// Update session metadata
	session.LastReviewAt = review.CreatedAt
	session.ReviewCount++

	return s.writeSessionMeta(session)
}

// GetReview retrieves a specific review by ID.
func (s *FileStore) GetReview(ctx context.Context, reviewID string) (*domain.LocalReview, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if !isValidReviewID(reviewID) {
		return nil, ErrInvalidReviewID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// We need to search all sessions for this review
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip directories with invalid session ID format
		if !isValidSessionID(entry.Name()) {
			continue
		}

		reviewPath, err := s.reviewPath(entry.Name(), reviewID)
		if err != nil {
			continue
		}
		if _, err := os.Stat(reviewPath); err == nil {
			return s.readReview(reviewPath)
		}
	}

	return nil, fmt.Errorf("review not found: %s", reviewID)
}

// GetLatestReview retrieves the most recent review for a session.
func (s *FileStore) GetLatestReview(ctx context.Context, sessionID string) (*domain.LocalReview, error) {
	reviews, err := s.ListReviews(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if len(reviews) == 0 {
		return nil, nil
	}

	// Reviews are sorted by creation time, latest first
	return &reviews[0], nil
}

// ListReviews returns all reviews for a session, ordered by creation time (latest first).
func (s *FileStore) ListReviews(ctx context.Context, sessionID string) ([]domain.LocalReview, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	sessionDir, err := s.sessionDir(sessionID)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	reviewsDir := filepath.Join(sessionDir, "reviews")
	entries, err := os.ReadDir(reviewsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read reviews directory: %w", err)
	}

	var reviews []domain.LocalReview
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		reviewPath := filepath.Join(reviewsDir, entry.Name())
		review, err := s.readReview(reviewPath)
		if err != nil {
			// Skip corrupted reviews
			continue
		}

		reviews = append(reviews, *review)
	}

	// Sort by creation time, most recent first
	sort.Slice(reviews, func(i, j int) bool {
		return reviews[i].CreatedAt.After(reviews[j].CreatedAt)
	})

	return reviews, nil
}

// Helper methods

// sessionDir returns the directory path for a session, validating the session ID.
func (s *FileStore) sessionDir(sessionID string) (string, error) {
	if !isValidSessionID(sessionID) {
		return "", ErrInvalidSessionID
	}
	return filepath.Join(s.baseDir, sessionID), nil
}

// isValidSessionID validates that a session ID is exactly 16 hex characters.
// This prevents path traversal attacks via malicious session IDs.
func isValidSessionID(id string) bool {
	if len(id) != 16 {
		return false
	}
	for _, c := range id {
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		isUpperHex := c >= 'A' && c <= 'F'
		if !isDigit && !isLowerHex && !isUpperHex {
			return false
		}
	}
	return true
}

// Maximum length for review IDs to prevent DoS via long filenames.
const maxReviewIDLength = 255

// isValidReviewID validates that a review ID doesn't contain path traversal characters.
// Review IDs can be alphanumeric with hyphens and underscores, max 255 chars.
func isValidReviewID(id string) bool {
	if id == "" || len(id) > maxReviewIDLength {
		return false
	}
	for _, c := range id {
		isAlphaNum := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isSafe := c == '-' || c == '_'
		if !isAlphaNum && !isSafe {
			return false
		}
	}
	return true
}

// reviewPath returns the file path for a review, validating both IDs.
func (s *FileStore) reviewPath(sessionID, reviewID string) (string, error) {
	sessionDir, err := s.sessionDir(sessionID)
	if err != nil {
		return "", err
	}
	if !isValidReviewID(reviewID) {
		return "", ErrInvalidReviewID
	}
	return filepath.Join(sessionDir, "reviews", reviewID+".json"), nil
}

func (s *FileStore) writeSessionMeta(session *domain.LocalSession) error {
	sessionDir, err := s.sessionDir(session.ID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	metaPath := filepath.Join(sessionDir, "meta.json")
	if err := os.WriteFile(metaPath, data, 0o600); err != nil {
		return fmt.Errorf("write meta file: %w", err)
	}

	return nil
}

func (s *FileStore) readSessionMeta(sessionID string) (*domain.LocalSession, error) {
	sessionDir, err := s.sessionDir(sessionID)
	if err != nil {
		return nil, err
	}

	metaPath := filepath.Join(sessionDir, "meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read meta file: %w", err)
	}

	var session domain.LocalSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	return &session, nil
}

func (s *FileStore) readReview(path string) (*domain.LocalReview, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read review file: %w", err)
	}

	var review domain.LocalReview
	if err := json.Unmarshal(data, &review); err != nil {
		return nil, fmt.Errorf("unmarshal review: %w", err)
	}

	return &review, nil
}
