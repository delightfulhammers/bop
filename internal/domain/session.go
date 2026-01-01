package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// LocalSession represents a branch-based review session for local development.
// Unlike TriageSession (which is PR-centric), LocalSession tracks reviews
// across a repository/branch pair for deduplication and history.
type LocalSession struct {
	ID           string            `json:"id"`
	Repository   string            `json:"repository"`   // Normalized remote URL (e.g., "github.com/owner/repo")
	Branch       string            `json:"branch"`       // Branch name
	CreatedAt    time.Time         `json:"createdAt"`    // When session was first created
	LastReviewAt time.Time         `json:"lastReviewAt"` // When last review was performed
	ReviewCount  int               `json:"reviewCount"`  // Total number of reviews in this session
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// LocalReview represents a single review within a local session.
// This is stored separately from the session metadata to keep the session
// file small for fast listing operations.
type LocalReview struct {
	ReviewID     string    `json:"reviewId"`
	SessionID    string    `json:"sessionId"`
	CreatedAt    time.Time `json:"createdAt"`
	BaseRef      string    `json:"baseRef"`
	HeadCommit   string    `json:"headCommit"`
	TotalCost    float64   `json:"totalCost"`
	FindingCount int       `json:"findingCount"`
	Findings     []Finding `json:"findings"` // Full findings for deduplication
}

// NewLocalSession creates a new local session for the given repository and branch.
func NewLocalSession(repository, branch string) LocalSession {
	now := time.Now()
	return LocalSession{
		ID:           GenerateLocalSessionID(repository, branch),
		Repository:   repository,
		Branch:       branch,
		CreatedAt:    now,
		LastReviewAt: now,
		ReviewCount:  0,
		Metadata:     make(map[string]string),
	}
}

// GenerateLocalSessionID creates a deterministic session ID from repository and branch.
// The ID is the first 16 hex characters of SHA-256(repository + "/" + branch).
func GenerateLocalSessionID(repository, branch string) string {
	payload := repository + "/" + branch
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:8]) // 16 hex chars
}

// Age returns the duration since the session was last used.
func (s *LocalSession) Age() time.Duration {
	return time.Since(s.LastReviewAt)
}

// IsStale returns true if the session hasn't been used in the given duration.
func (s *LocalSession) IsStale(threshold time.Duration) bool {
	return s.Age() > threshold
}
