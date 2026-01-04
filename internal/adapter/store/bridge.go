package store

import (
	"context"

	"github.com/delightfulhammers/bop/internal/store"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// Bridge adapts store.Store to review.Store interface.
// This avoids circular dependencies between packages.
type Bridge struct {
	store store.Store
}

// NewBridge creates a new store adapter.
func NewBridge(s store.Store) *Bridge {
	return &Bridge{store: s}
}

// CreateRun converts and saves a run record.
func (b *Bridge) CreateRun(ctx context.Context, run review.StoreRun) error {
	storeRun := store.Run{
		RunID:      run.RunID,
		Timestamp:  run.Timestamp,
		Scope:      run.Scope,
		ConfigHash: run.ConfigHash,
		TotalCost:  run.TotalCost,
		BaseRef:    run.BaseRef,
		TargetRef:  run.TargetRef,
		Repository: run.Repository,
	}
	return b.store.CreateRun(ctx, storeRun)
}

// UpdateRunCost updates the total cost for a run.
func (b *Bridge) UpdateRunCost(ctx context.Context, runID string, totalCost float64) error {
	return b.store.UpdateRunCost(ctx, runID, totalCost)
}

// SaveReview converts and saves a review record.
func (b *Bridge) SaveReview(ctx context.Context, review review.StoreReview) error {
	storeReview := store.ReviewRecord{
		ReviewID:  review.ReviewID,
		RunID:     review.RunID,
		Provider:  review.Provider,
		Model:     review.Model,
		Summary:   review.Summary,
		CreatedAt: review.CreatedAt,
	}
	return b.store.SaveReview(ctx, storeReview)
}

// SaveFindings converts and saves finding records.
func (b *Bridge) SaveFindings(ctx context.Context, findings []review.StoreFinding) error {
	storeFindings := make([]store.FindingRecord, len(findings))
	for i, f := range findings {
		storeFindings[i] = store.FindingRecord{
			FindingID:   f.FindingID,
			ReviewID:    f.ReviewID,
			FindingHash: f.FindingHash,
			File:        f.File,
			LineStart:   f.LineStart,
			LineEnd:     f.LineEnd,
			Category:    f.Category,
			Severity:    f.Severity,
			Description: f.Description,
			Suggestion:  f.Suggestion,
			Evidence:    f.Evidence,
		}
	}
	return b.store.SaveFindings(ctx, storeFindings)
}

// GetPrecisionPriors retrieves precision priors for all provider/category combinations.
func (b *Bridge) GetPrecisionPriors(ctx context.Context) (map[string]map[string]review.StorePrecisionPrior, error) {
	priors, err := b.store.GetPrecisionPriors(ctx)
	if err != nil {
		return nil, err
	}

	// Convert store.PrecisionPrior to review.StorePrecisionPrior
	result := make(map[string]map[string]review.StorePrecisionPrior)
	for provider, categoryMap := range priors {
		result[provider] = make(map[string]review.StorePrecisionPrior)
		for category, prior := range categoryMap {
			result[provider][category] = review.StorePrecisionPrior{
				Provider: prior.Provider,
				Category: prior.Category,
				Alpha:    prior.Alpha,
				Beta:     prior.Beta,
			}
		}
	}

	return result, nil
}

// Close closes the underlying store.
func (b *Bridge) Close() error {
	return b.store.Close()
}
