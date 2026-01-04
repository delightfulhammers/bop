package github

import "github.com/delightfulhammers/bop/internal/domain"

// PositionedFinding wraps a domain.Finding with GitHub-specific diff position.
// This type lives in the adapter layer to keep the domain layer pure and
// platform-agnostic.
type PositionedFinding struct {
	// Finding is the original domain finding with all review details.
	Finding domain.Finding

	// DiffPosition is the line position within the GitHub diff.
	// This is 1-indexed from the first @@ hunk header.
	// nil indicates the finding's line is not in the diff and cannot
	// receive an inline comment (should be included in summary only).
	DiffPosition *int
}

// InDiff returns true if the finding can receive an inline PR comment.
// Returns false if the finding's line is not part of the diff.
func (pf PositionedFinding) InDiff() bool {
	return pf.DiffPosition != nil
}
