package review_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/store"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore implements review.Store for testing
type mockStore struct {
	mu              sync.Mutex
	runs            []review.StoreRun
	reviews         []review.StoreReview
	findings        []review.StoreFinding
	saveErr         error // Legacy: applies to all operations
	createRunErr    error
	saveReviewErr   error
	saveFindingsErr error
	closeErr        error
	closed          bool
}

func (m *mockStore) CreateRun(ctx context.Context, run review.StoreRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createRunErr != nil {
		return m.createRunErr
	}
	if m.saveErr != nil {
		return m.saveErr
	}
	m.runs = append(m.runs, run)
	return nil
}

func (m *mockStore) UpdateRunCost(ctx context.Context, runID string, totalCost float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Find the run and update its cost
	for i := range m.runs {
		if m.runs[i].RunID == runID {
			m.runs[i].TotalCost = totalCost
			return nil
		}
	}
	return errors.New("run not found")
}

func (m *mockStore) SaveReview(ctx context.Context, r review.StoreReview) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveReviewErr != nil {
		return m.saveReviewErr
	}
	if m.saveErr != nil {
		return m.saveErr
	}
	m.reviews = append(m.reviews, r)
	return nil
}

func (m *mockStore) SaveFindings(ctx context.Context, findings []review.StoreFinding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveFindingsErr != nil {
		return m.saveFindingsErr
	}
	if m.saveErr != nil {
		return m.saveErr
	}
	m.findings = append(m.findings, findings...)
	return nil
}

func (m *mockStore) GetPrecisionPriors(ctx context.Context) (map[string]map[string]review.StorePrecisionPrior, error) {
	// Return empty map for tests - precision priors not needed in most test scenarios
	return make(map[string]map[string]review.StorePrecisionPrior), nil
}

func (m *mockStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return m.closeErr
}

func TestOrchestrator_SaveReviewToStore(t *testing.T) {
	t.Run("saves review and findings correctly", func(t *testing.T) {
		store := &mockStore{}
		orchestrator := createTestOrchestrator(store)

		domainReview := domain.Review{
			ProviderName: "openai",
			ModelName:    "gpt-4o-mini",
			Summary:      "Test summary",
			Findings: []domain.Finding{
				{
					File:        "main.go",
					LineStart:   10,
					LineEnd:     15,
					Category:    "security",
					Severity:    "high",
					Description: "SQL injection vulnerability",
					Suggestion:  "Use parameterized queries",
					Evidence:    true,
				},
			},
		}

		runID := "run-test-123"
		err := orchestrator.SaveReviewToStore(context.Background(), runID, domainReview)
		require.NoError(t, err)

		// Verify review was saved
		require.Len(t, store.reviews, 1)
		assert.Equal(t, "review-run-test-123-openai", store.reviews[0].ReviewID)
		assert.Equal(t, runID, store.reviews[0].RunID)
		assert.Equal(t, "openai", store.reviews[0].Provider)
		assert.Equal(t, "gpt-4o-mini", store.reviews[0].Model)
		assert.Equal(t, "Test summary", store.reviews[0].Summary)

		// Verify findings were saved
		require.Len(t, store.findings, 1)
		assert.Equal(t, "finding-review-run-test-123-openai-0000", store.findings[0].FindingID)
		assert.Equal(t, "review-run-test-123-openai", store.findings[0].ReviewID)
		assert.Equal(t, "main.go", store.findings[0].File)
		assert.Equal(t, 10, store.findings[0].LineStart)
		assert.Equal(t, 15, store.findings[0].LineEnd)
		assert.Equal(t, "security", store.findings[0].Category)
		assert.Equal(t, "high", store.findings[0].Severity)
		assert.NotEmpty(t, store.findings[0].FindingHash)
	})

	t.Run("handles empty findings list", func(t *testing.T) {
		store := &mockStore{}
		orchestrator := createTestOrchestrator(store)

		domainReview := domain.Review{
			ProviderName: "openai",
			ModelName:    "gpt-4o-mini",
			Summary:      "No issues found",
			Findings:     []domain.Finding{},
		}

		runID := "run-test-123"
		err := orchestrator.SaveReviewToStore(context.Background(), runID, domainReview)
		require.NoError(t, err)

		// Verify review was saved
		require.Len(t, store.reviews, 1)

		// Verify no findings were saved
		assert.Len(t, store.findings, 0)
	})

	t.Run("returns error on save failure", func(t *testing.T) {
		store := &mockStore{
			saveErr: errors.New("database error"),
		}
		orchestrator := createTestOrchestrator(store)

		domainReview := domain.Review{
			ProviderName: "openai",
			ModelName:    "gpt-4o-mini",
			Summary:      "Test",
			Findings:     []domain.Finding{},
		}

		runID := "run-test-123"
		err := orchestrator.SaveReviewToStore(context.Background(), runID, domainReview)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save review")
	})

	t.Run("works with nil store", func(t *testing.T) {
		orchestrator := createTestOrchestrator(nil)

		domainReview := domain.Review{
			ProviderName: "openai",
			ModelName:    "gpt-4o-mini",
			Summary:      "Test",
			Findings:     []domain.Finding{},
		}

		runID := "run-test-123"
		err := orchestrator.SaveReviewToStore(context.Background(), runID, domainReview)
		assert.NoError(t, err, "should not fail with nil store")
	})

	t.Run("generates correct finding hash", func(t *testing.T) {
		store := &mockStore{}
		orchestrator := createTestOrchestrator(store)

		domainReview := domain.Review{
			ProviderName: "openai",
			ModelName:    "gpt-4o-mini",
			Summary:      "Test",
			Findings: []domain.Finding{
				{
					File:        "main.go",
					LineStart:   10,
					LineEnd:     15,
					Description: "Test Finding",
				},
			},
		}

		runID := "run-test-123"
		err := orchestrator.SaveReviewToStore(context.Background(), runID, domainReview)
		require.NoError(t, err)

		// Finding hash should be consistent
		hash1 := store.findings[0].FindingHash

		// Save again with same finding
		store.findings = nil
		err = orchestrator.SaveReviewToStore(context.Background(), runID, domainReview)
		require.NoError(t, err)

		hash2 := store.findings[0].FindingHash
		assert.Equal(t, hash1, hash2, "finding hash should be deterministic")
	})
}

// TestIDGenerationMatchesStorePackage ensures review package ID generation
// stays in sync with store package implementations.
//
// NOTE: ID generation functions are intentionally duplicated between the
// review and store packages to avoid circular dependencies (clean architecture).
// The store package implements interfaces defined by the review package,
// so the review package cannot import store utilities.
//
// This test ensures the implementations don't accidentally diverge.
func TestIDGenerationMatchesStorePackage(t *testing.T) {
	mockStore := &mockStore{}
	orchestrator := createTestOrchestrator(mockStore)

	// Use store package to generate expected IDs
	timestamp := time.Date(2025, 10, 22, 10, 30, 0, 0, time.UTC)
	expectedRunID := store.GenerateRunID(timestamp, "main", "feature")
	expectedReviewID := store.GenerateReviewID(expectedRunID, "openai")
	expectedFindingID := store.GenerateFindingID(expectedReviewID, 0)

	// Create a review and save it using the orchestrator's internal ID generation
	domainReview := domain.Review{
		ProviderName: "openai",
		ModelName:    "gpt-4o-mini",
		Summary:      "Test summary",
		Findings: []domain.Finding{
			{
				File:        "main.go",
				LineStart:   10,
				LineEnd:     15,
				Category:    "style",
				Severity:    "low",
				Description: "Test finding",
			},
		},
	}

	// Save using the orchestrator (which uses internal ID generation)
	err := orchestrator.SaveReviewToStore(context.Background(), expectedRunID, domainReview)
	require.NoError(t, err)

	// Verify the orchestrator's internal ID generation matches store package
	require.Len(t, mockStore.reviews, 1)
	actualReviewID := mockStore.reviews[0].ReviewID
	assert.Equal(t, expectedReviewID, actualReviewID,
		"generateReviewID in review package must match store.GenerateReviewID")

	require.Len(t, mockStore.findings, 1)
	actualFindingID := mockStore.findings[0].FindingID
	assert.Equal(t, expectedFindingID, actualFindingID,
		"generateFindingID in review package must match store.GenerateFindingID")
}

// Helper to create a test orchestrator with minimal deps
func createTestOrchestrator(store review.Store) *review.Orchestrator {
	// Create a minimal reviewer registry for testing
	testReviewer := domain.Reviewer{
		Name:     "default",
		Provider: "test-provider",
		Model:    "test-model",
		Weight:   1.0,
		Enabled:  true,
	}
	registry := &mockReviewerRegistry{
		reviewers:        map[string]domain.Reviewer{"default": testReviewer},
		defaultReviewers: []string{"default"},
	}
	baseBuilder := review.NewEnhancedPromptBuilder()
	personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)

	return review.NewOrchestrator(review.OrchestratorDeps{
		Git:                  &mockGitEngine{},
		Providers:            map[string]review.Provider{},
		Merger:               &mockMerger{},
		Markdown:             &mockMarkdownWriter{},
		JSON:                 &mockJSONWriter{},
		SARIF:                &mockSARIFWriter{},
		SeedGenerator:        func(baseRef, targetRef string) uint64 { return 12345 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
		Store:                store,
	})
}
