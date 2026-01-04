package merge

import (
	"context"

	"github.com/delightfulhammers/bop/internal/domain"
)

// Service defines the merging logic.
type Service struct{}

// NewService creates a new merge service.
func NewService() *Service {
	return &Service{}
}

// Merge combines multiple reviews into a single review, de-duplicating findings
// and aggregating usage metadata (tokens, cost) from all providers.
func (s *Service) Merge(ctx context.Context, reviews []domain.Review) domain.Review {
	mergedReview := domain.Review{
		ProviderName: "merged",
		ModelName:    "consensus",
		Summary:      "This is a merged review.",
	}

	seenFindings := make(map[string]bool)
	var findings []domain.Finding

	// Aggregate usage metadata from all providers
	var totalTokensIn, totalTokensOut int
	var totalCost float64

	for _, review := range reviews {
		// Aggregate usage
		totalTokensIn += review.TokensIn
		totalTokensOut += review.TokensOut
		totalCost += review.Cost

		// De-duplicate findings
		for _, finding := range review.Findings {
			if !seenFindings[finding.ID] {
				seenFindings[finding.ID] = true
				findings = append(findings, finding)
			}
		}
	}

	mergedReview.Findings = findings
	mergedReview.TokensIn = totalTokensIn
	mergedReview.TokensOut = totalTokensOut
	mergedReview.Cost = totalCost
	return mergedReview
}
