package merge_test

import (
	"context"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/merge"
	"github.com/stretchr/testify/assert"
)

func TestMerge_Merge(t *testing.T) {
	// Given
	ctx := context.Background()
	finding1 := domain.NewFinding(domain.FindingInput{File: "file1.go", LineStart: 10, Description: "Bug A"})
	finding2 := domain.NewFinding(domain.FindingInput{File: "file2.go", LineStart: 20, Description: "Bug B"})
	finding3 := domain.NewFinding(domain.FindingInput{File: "file1.go", LineStart: 10, Description: "Bug A"}) // Duplicate of finding1

	review1 := domain.Review{
		ProviderName: "provider1",
		Findings:     []domain.Finding{finding1, finding2},
		TokensIn:     100,
		TokensOut:    50,
		Cost:         0.01,
	}
	review2 := domain.Review{
		ProviderName: "provider2",
		Findings:     []domain.Finding{finding3},
		TokensIn:     150,
		TokensOut:    75,
		Cost:         0.02,
	}

	merger := merge.NewService()

	// When
	mergedReview := merger.Merge(ctx, []domain.Review{review1, review2})

	// Then
	assert.Equal(t, "merged", mergedReview.ProviderName)
	assert.Equal(t, "consensus", mergedReview.ModelName)
	assert.Len(t, mergedReview.Findings, 2, "Expected 2 unique findings after merge")

	// Check that the findings are the ones we expect
	found1 := false
	found2 := false
	for _, f := range mergedReview.Findings {
		if f.ID == finding1.ID {
			found1 = true
		}
		if f.ID == finding2.ID {
			found2 = true
		}
	}

	assert.True(t, found1, "Finding 1 not found in merged review")
	assert.True(t, found2, "Finding 2 not found in merged review")

	// Verify usage metadata aggregation
	assert.Equal(t, 250, mergedReview.TokensIn, "Expected aggregated tokens in")
	assert.Equal(t, 125, mergedReview.TokensOut, "Expected aggregated tokens out")
	assert.InDelta(t, 0.03, mergedReview.Cost, 0.0001, "Expected aggregated cost")
}
