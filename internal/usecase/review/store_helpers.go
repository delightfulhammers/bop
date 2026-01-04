package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/delightfulhammers/bop/internal/domain"
)

// calculateConfigHash creates a deterministic hash of the BranchRequest configuration.
// This is used to track which settings produced which results.
func calculateConfigHash(req BranchRequest) string {
	// Create a stable representation of the config
	configStr := fmt.Sprintf("%s|%s|%s|%s|%v",
		req.BaseRef,
		req.TargetRef,
		req.OutputDir,
		req.Repository,
		req.IncludeUncommitted,
	)

	hash := sha256.Sum256([]byte(configStr))
	return hex.EncodeToString(hash[:8]) // 16 char hash
}

// ID Generation Functions
//
// The following functions (generateRunID, generateReviewID, generateFindingID, generateFindingHash)
// are intentionally duplicated from internal/store/util.go to maintain clean architecture principles.
//
// WHY DUPLICATION EXISTS:
// - This package (internal/usecase/review) is in the USE CASE layer
// - The store package (internal/adapter/store) is in the ADAPTER layer
// - Clean Architecture's Dependency Rule states: dependencies point INWARD toward use cases
// - Use case layer CANNOT import adapter layer (this would violate the dependency direction)
// - The store adapter implements the Store interface DEFINED by this use case layer
// - If we imported store utilities here → circular dependency and architecture violation
//
// MAINTAINING CONSISTENCY:
// - See internal/usecase/review/store_helpers_test.go::TestIDGenerationMatchesStorePackage
// - This test ensures implementations stay in sync between packages
// - If implementations diverge, the test will fail
//
// TRADEOFF:
// - Code duplication (DRY violation) vs. Clean Architecture (dependency direction)
// - We choose architecture integrity over eliminating duplication
// - Test coverage ensures correctness despite duplication

// generateRunID creates a unique, time-ordered run ID.
func generateRunID(timestamp time.Time, baseRef, targetRef string) string {
	ts := timestamp.UTC().Format("20060102T150405Z")

	input := fmt.Sprintf("%s|%s|%d", baseRef, targetRef, timestamp.UnixNano())
	hash := sha256.Sum256([]byte(input))
	shortHash := hex.EncodeToString(hash[:3])

	return fmt.Sprintf("run-%s-%s", ts, shortHash)
}

// generateReviewID creates a unique ID for a review.
func generateReviewID(runID, provider string) string {
	return fmt.Sprintf("review-%s-%s", runID, provider)
}

// generateFindingID creates a unique ID for a finding.
func generateFindingID(reviewID string, index int) string {
	return fmt.Sprintf("finding-%s-%04d", reviewID, index)
}

// generateFindingHash creates a deterministic hash for a finding.
// Duplicates the implementation from store package to avoid circular dependency.
func generateFindingHash(file string, lineStart, lineEnd int, description string) string {
	// Normalize description: lowercase, trim, and collapse multiple spaces
	normalized := strings.ToLower(strings.TrimSpace(description))
	normalized = strings.Join(strings.Fields(normalized), " ")

	input := fmt.Sprintf("%s:%d-%d:%s", file, lineStart, lineEnd, normalized)
	hash := sha256.Sum256([]byte(input))

	return hex.EncodeToString(hash[:])
}

// SaveReviewToStore persists a review and its findings to the store.
// This is exported for testing purposes.
func (o *Orchestrator) SaveReviewToStore(ctx context.Context, runID string, review domain.Review) error {
	if o.deps.Store == nil {
		return nil // Store is optional
	}

	// Create review record
	reviewID := generateReviewID(runID, review.ProviderName)
	reviewRecord := StoreReview{
		ReviewID:  reviewID,
		RunID:     runID,
		Provider:  review.ProviderName,
		Model:     review.ModelName,
		Summary:   review.Summary,
		CreatedAt: time.Now(),
	}

	if err := o.deps.Store.SaveReview(ctx, reviewRecord); err != nil {
		return fmt.Errorf("failed to save review: %w", err)
	}

	// Create finding records
	if len(review.Findings) == 0 {
		return nil // No findings to save
	}

	findings := make([]StoreFinding, len(review.Findings))
	for i, f := range review.Findings {
		findings[i] = StoreFinding{
			FindingID:   generateFindingID(reviewID, i),
			ReviewID:    reviewID,
			FindingHash: generateFindingHash(f.File, f.LineStart, f.LineEnd, f.Description),
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

	if err := o.deps.Store.SaveFindings(ctx, findings); err != nil {
		return fmt.Errorf("failed to save findings: %w", err)
	}

	return nil
}
