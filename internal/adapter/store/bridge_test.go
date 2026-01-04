package store_test

import (
	"context"
	"testing"
	"time"

	storeAdapter "github.com/delightfulhammers/bop/internal/adapter/store"
	"github.com/delightfulhammers/bop/internal/store"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore implements store.Store for testing
type mockStore struct {
	runs     []store.Run
	reviews  []store.ReviewRecord
	findings []store.FindingRecord
	closed   bool
}

func (m *mockStore) CreateRun(ctx context.Context, run store.Run) error {
	m.runs = append(m.runs, run)
	return nil
}

func (m *mockStore) UpdateRunCost(ctx context.Context, runID string, totalCost float64) error {
	for i := range m.runs {
		if m.runs[i].RunID == runID {
			m.runs[i].TotalCost = totalCost
			return nil
		}
	}
	return nil // Not found, but don't error in tests
}

func (m *mockStore) GetRun(ctx context.Context, runID string) (store.Run, error) {
	return store.Run{}, nil
}

func (m *mockStore) ListRuns(ctx context.Context, limit int) ([]store.Run, error) {
	return nil, nil
}

func (m *mockStore) SaveReview(ctx context.Context, r store.ReviewRecord) error {
	m.reviews = append(m.reviews, r)
	return nil
}

func (m *mockStore) GetReview(ctx context.Context, reviewID string) (store.ReviewRecord, error) {
	return store.ReviewRecord{}, nil
}

func (m *mockStore) GetReviewsByRun(ctx context.Context, runID string) ([]store.ReviewRecord, error) {
	return nil, nil
}

func (m *mockStore) SaveFindings(ctx context.Context, findings []store.FindingRecord) error {
	m.findings = append(m.findings, findings...)
	return nil
}

func (m *mockStore) GetFinding(ctx context.Context, findingID string) (store.FindingRecord, error) {
	return store.FindingRecord{}, nil
}

func (m *mockStore) GetFindingsByReview(ctx context.Context, reviewID string) ([]store.FindingRecord, error) {
	return nil, nil
}

func (m *mockStore) RecordFeedback(ctx context.Context, feedback store.Feedback) error {
	return nil
}

func (m *mockStore) GetFeedbackForFinding(ctx context.Context, findingID string) ([]store.Feedback, error) {
	return nil, nil
}

func (m *mockStore) GetPrecisionPriors(ctx context.Context) (map[string]map[string]store.PrecisionPrior, error) {
	return nil, nil
}

func (m *mockStore) UpdatePrecisionPrior(ctx context.Context, provider, category string, accepted, rejected int) error {
	return nil
}

func (m *mockStore) Close() error {
	m.closed = true
	return nil
}

func TestBridge_CreateRun(t *testing.T) {
	mock := &mockStore{}
	bridge := storeAdapter.NewBridge(mock)

	now := time.Now()
	reviewRun := review.StoreRun{
		RunID:      "run-123",
		Timestamp:  now,
		Scope:      "main..feature",
		ConfigHash: "abc123",
		TotalCost:  0.5,
		BaseRef:    "main",
		TargetRef:  "feature",
		Repository: "test-repo",
	}

	err := bridge.CreateRun(context.Background(), reviewRun)
	require.NoError(t, err)

	// Verify conversion
	require.Len(t, mock.runs, 1)
	assert.Equal(t, "run-123", mock.runs[0].RunID)
	assert.True(t, now.Equal(mock.runs[0].Timestamp))
	assert.Equal(t, "main..feature", mock.runs[0].Scope)
	assert.Equal(t, "abc123", mock.runs[0].ConfigHash)
	assert.Equal(t, 0.5, mock.runs[0].TotalCost)
	assert.Equal(t, "main", mock.runs[0].BaseRef)
	assert.Equal(t, "feature", mock.runs[0].TargetRef)
	assert.Equal(t, "test-repo", mock.runs[0].Repository)
}

func TestBridge_SaveReview(t *testing.T) {
	mock := &mockStore{}
	bridge := storeAdapter.NewBridge(mock)

	now := time.Now()
	reviewRecord := review.StoreReview{
		ReviewID:  "review-123",
		RunID:     "run-123",
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		Summary:   "Test summary",
		CreatedAt: now,
	}

	err := bridge.SaveReview(context.Background(), reviewRecord)
	require.NoError(t, err)

	// Verify conversion
	require.Len(t, mock.reviews, 1)
	assert.Equal(t, "review-123", mock.reviews[0].ReviewID)
	assert.Equal(t, "run-123", mock.reviews[0].RunID)
	assert.Equal(t, "openai", mock.reviews[0].Provider)
	assert.Equal(t, "gpt-4o-mini", mock.reviews[0].Model)
	assert.Equal(t, "Test summary", mock.reviews[0].Summary)
	assert.True(t, now.Equal(mock.reviews[0].CreatedAt))
}

func TestBridge_SaveFindings(t *testing.T) {
	mock := &mockStore{}
	bridge := storeAdapter.NewBridge(mock)

	findings := []review.StoreFinding{
		{
			FindingID:   "finding-1",
			ReviewID:    "review-123",
			FindingHash: "hash1",
			File:        "main.go",
			LineStart:   10,
			LineEnd:     15,
			Category:    "security",
			Severity:    "high",
			Description: "SQL injection",
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
			Suggestion:  "Use map",
			Evidence:    false,
		},
	}

	err := bridge.SaveFindings(context.Background(), findings)
	require.NoError(t, err)

	// Verify conversion
	require.Len(t, mock.findings, 2)

	assert.Equal(t, "finding-1", mock.findings[0].FindingID)
	assert.Equal(t, "review-123", mock.findings[0].ReviewID)
	assert.Equal(t, "hash1", mock.findings[0].FindingHash)
	assert.Equal(t, "main.go", mock.findings[0].File)
	assert.Equal(t, 10, mock.findings[0].LineStart)
	assert.Equal(t, 15, mock.findings[0].LineEnd)
	assert.Equal(t, "security", mock.findings[0].Category)
	assert.Equal(t, "high", mock.findings[0].Severity)
	assert.Equal(t, "SQL injection", mock.findings[0].Description)
	assert.Equal(t, "Use parameterized queries", mock.findings[0].Suggestion)
	assert.True(t, mock.findings[0].Evidence)

	assert.Equal(t, "finding-2", mock.findings[1].FindingID)
	assert.False(t, mock.findings[1].Evidence)
}

func TestBridge_SaveFindings_Empty(t *testing.T) {
	mock := &mockStore{}
	bridge := storeAdapter.NewBridge(mock)

	err := bridge.SaveFindings(context.Background(), []review.StoreFinding{})
	require.NoError(t, err)

	// Should handle empty slice
	assert.Len(t, mock.findings, 0)
}

func TestBridge_Close(t *testing.T) {
	mock := &mockStore{}
	bridge := storeAdapter.NewBridge(mock)

	err := bridge.Close()
	require.NoError(t, err)

	// Verify Close was called on underlying store
	assert.True(t, mock.closed)
}
