package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/bkyoung/code-reviewer/internal/adapter/store/sqlite"
	"github.com/bkyoung/code-reviewer/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStore(t *testing.T) *sqlite.Store {
	t.Helper()

	// Use in-memory database for testing
	s, err := sqlite.NewStore(":memory:")
	require.NoError(t, err, "failed to create test store")

	t.Cleanup(func() {
		_ = s.Close()
	})

	return s
}

func TestStore_CreateRun_GetRun(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	run := store.Run{
		RunID:      "run-123",
		Timestamp:  time.Now().Truncate(time.Second), // Truncate to avoid precision issues
		Scope:      "main..feature",
		ConfigHash: "abc123",
		TotalCost:  0.05,
		BaseRef:    "main",
		TargetRef:  "feature",
		Repository: "test-repo",
	}

	// Create run
	err := s.CreateRun(ctx, run)
	require.NoError(t, err)

	// Retrieve run
	retrieved, err := s.GetRun(ctx, run.RunID)
	require.NoError(t, err)

	assert.Equal(t, run.RunID, retrieved.RunID)
	assert.Equal(t, run.Scope, retrieved.Scope)
	assert.Equal(t, run.ConfigHash, retrieved.ConfigHash)
	assert.Equal(t, run.TotalCost, retrieved.TotalCost)
	assert.Equal(t, run.BaseRef, retrieved.BaseRef)
	assert.Equal(t, run.TargetRef, retrieved.TargetRef)
	assert.Equal(t, run.Repository, retrieved.Repository)
	assert.True(t, run.Timestamp.Equal(retrieved.Timestamp))
}

func TestStore_ListRuns(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create multiple runs with different timestamps
	now := time.Now().Truncate(time.Second)
	runs := []store.Run{
		{
			RunID:      "run-1",
			Timestamp:  now.Add(-2 * time.Hour),
			Scope:      "main..feature-1",
			ConfigHash: "hash1",
			BaseRef:    "main",
			TargetRef:  "feature-1",
			Repository: "repo",
		},
		{
			RunID:      "run-2",
			Timestamp:  now.Add(-1 * time.Hour),
			Scope:      "main..feature-2",
			ConfigHash: "hash2",
			BaseRef:    "main",
			TargetRef:  "feature-2",
			Repository: "repo",
		},
		{
			RunID:      "run-3",
			Timestamp:  now,
			Scope:      "main..feature-3",
			ConfigHash: "hash3",
			BaseRef:    "main",
			TargetRef:  "feature-3",
			Repository: "repo",
		},
	}

	for _, run := range runs {
		err := s.CreateRun(ctx, run)
		require.NoError(t, err)
	}

	// List runs (should be in descending timestamp order)
	retrieved, err := s.ListRuns(ctx, 10)
	require.NoError(t, err)
	require.Len(t, retrieved, 3)

	// Verify order (most recent first)
	assert.Equal(t, "run-3", retrieved[0].RunID)
	assert.Equal(t, "run-2", retrieved[1].RunID)
	assert.Equal(t, "run-1", retrieved[2].RunID)

	// Test limit
	limited, err := s.ListRuns(ctx, 2)
	require.NoError(t, err)
	assert.Len(t, limited, 2)
}

func TestStore_SaveReview_GetReview(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create a run first (foreign key requirement)
	run := store.Run{
		RunID:      "run-123",
		Timestamp:  time.Now().Truncate(time.Second),
		Scope:      "main..feature",
		ConfigHash: "abc123",
		BaseRef:    "main",
		TargetRef:  "feature",
		Repository: "repo",
	}
	require.NoError(t, s.CreateRun(ctx, run))

	review := store.ReviewRecord{
		ReviewID:  "review-123",
		RunID:     "run-123",
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		Summary:   "Test summary",
		CreatedAt: time.Now().Truncate(time.Second),
	}

	// Save review
	err := s.SaveReview(ctx, review)
	require.NoError(t, err)

	// Retrieve review
	retrieved, err := s.GetReview(ctx, review.ReviewID)
	require.NoError(t, err)

	assert.Equal(t, review.ReviewID, retrieved.ReviewID)
	assert.Equal(t, review.RunID, retrieved.RunID)
	assert.Equal(t, review.Provider, retrieved.Provider)
	assert.Equal(t, review.Model, retrieved.Model)
	assert.Equal(t, review.Summary, retrieved.Summary)
	assert.True(t, review.CreatedAt.Equal(retrieved.CreatedAt))
}

func TestStore_GetReviewsByRun(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create a run
	run := store.Run{
		RunID:      "run-123",
		Timestamp:  time.Now().Truncate(time.Second),
		Scope:      "main..feature",
		ConfigHash: "abc123",
		BaseRef:    "main",
		TargetRef:  "feature",
		Repository: "repo",
	}
	require.NoError(t, s.CreateRun(ctx, run))

	// Create multiple reviews for the same run
	reviews := []store.ReviewRecord{
		{
			ReviewID:  "review-1",
			RunID:     "run-123",
			Provider:  "openai",
			Model:     "gpt-4o-mini",
			Summary:   "Summary 1",
			CreatedAt: time.Now().Add(-1 * time.Minute).Truncate(time.Second),
		},
		{
			ReviewID:  "review-2",
			RunID:     "run-123",
			Provider:  "anthropic",
			Model:     "claude-3-5-sonnet",
			Summary:   "Summary 2",
			CreatedAt: time.Now().Truncate(time.Second),
		},
	}

	for _, review := range reviews {
		err := s.SaveReview(ctx, review)
		require.NoError(t, err)
	}

	// Retrieve all reviews for the run
	retrieved, err := s.GetReviewsByRun(ctx, "run-123")
	require.NoError(t, err)
	assert.Len(t, retrieved, 2)

	// Verify order (ascending by created_at)
	assert.Equal(t, "review-1", retrieved[0].ReviewID)
	assert.Equal(t, "review-2", retrieved[1].ReviewID)
}

func TestStore_SaveFindings_GetFindings(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create run and review
	run := store.Run{
		RunID:      "run-123",
		Timestamp:  time.Now().Truncate(time.Second),
		Scope:      "main..feature",
		ConfigHash: "abc123",
		BaseRef:    "main",
		TargetRef:  "feature",
		Repository: "repo",
	}
	require.NoError(t, s.CreateRun(ctx, run))

	review := store.ReviewRecord{
		ReviewID:  "review-123",
		RunID:     "run-123",
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		Summary:   "Summary",
		CreatedAt: time.Now().Truncate(time.Second),
	}
	require.NoError(t, s.SaveReview(ctx, review))

	// Save findings
	findings := []store.FindingRecord{
		{
			FindingID:   "finding-1",
			ReviewID:    "review-123",
			FindingHash: "hash1",
			File:        "main.go",
			LineStart:   10,
			LineEnd:     15,
			Category:    "security",
			Severity:    "high",
			Description: "SQL injection vulnerability",
			Suggestion:  "Use parameterized queries",
			Evidence:    true,
		},
		{
			FindingID:   "finding-2",
			ReviewID:    "review-123",
			FindingHash: "hash2",
			File:        "utils.go",
			LineStart:   5,
			LineEnd:     8,
			Category:    "performance",
			Severity:    "medium",
			Description: "Inefficient loop",
			Suggestion:  "Use map instead",
			Evidence:    false,
		},
	}

	err := s.SaveFindings(ctx, findings)
	require.NoError(t, err)

	// Retrieve findings
	retrieved, err := s.GetFindingsByReview(ctx, "review-123")
	require.NoError(t, err)
	require.Len(t, retrieved, 2)

	// Verify order (ascending by line_start)
	assert.Equal(t, "finding-2", retrieved[0].FindingID)
	assert.Equal(t, "finding-1", retrieved[1].FindingID)

	// Verify first finding details
	f1 := retrieved[1]
	assert.Equal(t, "finding-1", f1.FindingID)
	assert.Equal(t, "review-123", f1.ReviewID)
	assert.Equal(t, "hash1", f1.FindingHash)
	assert.Equal(t, "main.go", f1.File)
	assert.Equal(t, 10, f1.LineStart)
	assert.Equal(t, 15, f1.LineEnd)
	assert.Equal(t, "security", f1.Category)
	assert.Equal(t, "high", f1.Severity)
	assert.True(t, f1.Evidence)
}

func TestStore_GetFinding(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Setup run, review, and finding
	run := store.Run{
		RunID:      "run-123",
		Timestamp:  time.Now().Truncate(time.Second),
		Scope:      "main..feature",
		ConfigHash: "abc123",
		BaseRef:    "main",
		TargetRef:  "feature",
		Repository: "repo",
	}
	require.NoError(t, s.CreateRun(ctx, run))

	review := store.ReviewRecord{
		ReviewID:  "review-123",
		RunID:     "run-123",
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		Summary:   "Summary",
		CreatedAt: time.Now().Truncate(time.Second),
	}
	require.NoError(t, s.SaveReview(ctx, review))

	findings := []store.FindingRecord{
		{
			FindingID:   "finding-1",
			ReviewID:    "review-123",
			FindingHash: "hash1",
			File:        "main.go",
			LineStart:   10,
			LineEnd:     15,
			Category:    "security",
			Severity:    "high",
			Description: "Test finding",
			Suggestion:  "Fix it",
			Evidence:    true,
		},
	}
	require.NoError(t, s.SaveFindings(ctx, findings))

	// Get single finding
	finding, err := s.GetFinding(ctx, "finding-1")
	require.NoError(t, err)
	assert.Equal(t, "finding-1", finding.FindingID)
	assert.Equal(t, "security", finding.Category)
}

func TestStore_RecordFeedback_GetFeedback(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Setup run, review, and finding
	run := store.Run{
		RunID:      "run-123",
		Timestamp:  time.Now().Truncate(time.Second),
		Scope:      "main..feature",
		ConfigHash: "abc123",
		BaseRef:    "main",
		TargetRef:  "feature",
		Repository: "repo",
	}
	require.NoError(t, s.CreateRun(ctx, run))

	review := store.ReviewRecord{
		ReviewID:  "review-123",
		RunID:     "run-123",
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		Summary:   "Summary",
		CreatedAt: time.Now().Truncate(time.Second),
	}
	require.NoError(t, s.SaveReview(ctx, review))

	findings := []store.FindingRecord{
		{
			FindingID:   "finding-1",
			ReviewID:    "review-123",
			FindingHash: "hash1",
			File:        "main.go",
			LineStart:   10,
			LineEnd:     15,
			Category:    "security",
			Severity:    "high",
			Description: "Test",
			Suggestion:  "Fix",
			Evidence:    true,
		},
	}
	require.NoError(t, s.SaveFindings(ctx, findings))

	// Record feedback
	feedback := store.Feedback{
		FindingID: "finding-1",
		Status:    "accepted",
		Timestamp: time.Now().Truncate(time.Second),
	}

	err := s.RecordFeedback(ctx, feedback)
	require.NoError(t, err)

	// Retrieve feedback
	feedbacks, err := s.GetFeedbackForFinding(ctx, "finding-1")
	require.NoError(t, err)
	require.Len(t, feedbacks, 1)

	assert.Equal(t, "finding-1", feedbacks[0].FindingID)
	assert.Equal(t, "accepted", feedbacks[0].Status)
}

func TestStore_PrecisionPriors(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Initially, no priors should exist
	priors, err := s.GetPrecisionPriors(ctx)
	require.NoError(t, err)
	assert.Empty(t, priors)

	// Update precision prior (first time creates it)
	err = s.UpdatePrecisionPrior(ctx, "openai", "security", 5, 1)
	require.NoError(t, err)

	// Retrieve priors
	priors, err = s.GetPrecisionPriors(ctx)
	require.NoError(t, err)
	require.Contains(t, priors, "openai")
	require.Contains(t, priors["openai"], "security")

	prior := priors["openai"]["security"]
	assert.Equal(t, "openai", prior.Provider)
	assert.Equal(t, "security", prior.Category)
	assert.Equal(t, 6.0, prior.Alpha) // 1.0 (uniform) + 5
	assert.Equal(t, 2.0, prior.Beta)  // 1.0 (uniform) + 1

	// Verify precision calculation
	expectedPrecision := 6.0 / 8.0 // 0.75
	assert.InDelta(t, expectedPrecision, prior.Precision(), 0.001)

	// Update again (should increment)
	err = s.UpdatePrecisionPrior(ctx, "openai", "security", 3, 2)
	require.NoError(t, err)

	priors, err = s.GetPrecisionPriors(ctx)
	require.NoError(t, err)

	prior = priors["openai"]["security"]
	assert.Equal(t, 9.0, prior.Alpha) // 6.0 + 3
	assert.Equal(t, 4.0, prior.Beta)  // 2.0 + 2
}

func TestStore_MultiplePrecisionPriors(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Create priors for multiple providers and categories
	err := s.UpdatePrecisionPrior(ctx, "openai", "security", 10, 2)
	require.NoError(t, err)

	err = s.UpdatePrecisionPrior(ctx, "openai", "performance", 8, 4)
	require.NoError(t, err)

	err = s.UpdatePrecisionPrior(ctx, "anthropic", "security", 12, 1)
	require.NoError(t, err)

	// Retrieve all priors
	priors, err := s.GetPrecisionPriors(ctx)
	require.NoError(t, err)

	// Verify structure
	require.Contains(t, priors, "openai")
	require.Contains(t, priors, "anthropic")
	require.Contains(t, priors["openai"], "security")
	require.Contains(t, priors["openai"], "performance")
	require.Contains(t, priors["anthropic"], "security")

	// Verify values
	assert.Equal(t, 11.0, priors["openai"]["security"].Alpha)
	assert.Equal(t, 9.0, priors["openai"]["performance"].Alpha)
	assert.Equal(t, 13.0, priors["anthropic"]["security"].Alpha)
}

func TestStore_ForeignKeyConstraints(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	// Try to save review without run (should fail)
	review := store.ReviewRecord{
		ReviewID:  "review-123",
		RunID:     "nonexistent-run",
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		Summary:   "Summary",
		CreatedAt: time.Now(),
	}

	err := s.SaveReview(ctx, review)
	assert.Error(t, err, "should fail due to foreign key constraint")
}
