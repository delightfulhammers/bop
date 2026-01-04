package review

import (
	"context"

	"github.com/delightfulhammers/bop/internal/domain"
)

// DiffComputer determines the appropriate diff for a review request.
type DiffComputer struct {
	git GitEngine
}

// NewDiffComputer creates a DiffComputer with the given git engine.
func NewDiffComputer(git GitEngine) *DiffComputer {
	return &DiffComputer{git: git}
}

// ComputeDiffForReview computes the cumulative diff for a review request.
// It returns the diff between BaseRef and TargetRef, optionally including
// uncommitted changes.
func (dc *DiffComputer) ComputeDiffForReview(
	ctx context.Context,
	req BranchRequest,
) (domain.Diff, error) {
	return dc.git.GetCumulativeDiff(ctx, req.BaseRef, req.TargetRef, req.IncludeUncommitted)
}
