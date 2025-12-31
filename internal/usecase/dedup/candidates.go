package dedup

import (
	"github.com/bkyoung/code-reviewer/internal/domain"
)

// FindCandidates identifies new findings that may be semantic duplicates of existing findings.
// Two findings are candidates for comparison if they are in the same file AND either:
//   - Within lineThreshold lines of each other (spatial proximity), OR
//   - Have the same category AND severity (content match, regardless of line distance)
//
// The content match criterion catches cases where code shifts between review rounds
// cause the same conceptual finding to appear at distant line numbers (Issue #165).
//
// Returns candidate pairs for semantic comparison, limited to maxCandidates.
// If there are more candidates than maxCandidates, the excess are returned as overflow
// (these should be treated as unique since we can't verify them).
func FindCandidates(
	newFindings []domain.Finding,
	existing []ExistingFinding,
	lineThreshold int,
	maxCandidates int,
) (candidates []CandidatePair, overflow []domain.Finding) {
	if len(existing) == 0 {
		// No existing findings to compare against
		return nil, nil
	}

	// Build index of existing findings by file for O(1) lookup
	existingByFile := make(map[string][]ExistingFinding)
	for _, ef := range existing {
		existingByFile[ef.File] = append(existingByFile[ef.File], ef)
	}

	// Track which new findings have been paired
	paired := make(map[int]bool)

	for i, nf := range newFindings {
		// Get existing findings in the same file
		fileExisting, ok := existingByFile[nf.File]
		if !ok {
			continue
		}

		// Check each existing finding for spatial OR content match
		for _, ef := range fileExisting {
			// Match criteria: spatial proximity OR content match (same category+severity)
			spatialMatch := linesOverlap(nf.LineStart, nf.LineEnd, ef.LineStart, ef.LineEnd, lineThreshold)
			contentMatch := contentMatches(nf, ef)

			if spatialMatch || contentMatch {
				if len(candidates) >= maxCandidates {
					// Hit the limit - remaining paired findings go to overflow
					if !paired[i] {
						overflow = append(overflow, nf)
						paired[i] = true
					}
					continue
				}

				candidates = append(candidates, CandidatePair{
					Existing: ef,
					New:      nf,
				})
				paired[i] = true
			}
		}
	}

	return candidates, overflow
}

// contentMatches returns true if findings have the same category AND severity.
// This catches semantic duplicates that may have shifted to distant line numbers.
func contentMatches(nf domain.Finding, ef ExistingFinding) bool {
	return nf.Category == ef.Category && nf.Severity == ef.Severity
}

// linesOverlap returns true if two line ranges are within threshold lines of each other.
// Ranges [a1, a2] and [b1, b2] overlap if:
//   - They directly overlap (a1 <= b2 && b1 <= a2), OR
//   - The gap between them is <= threshold
func linesOverlap(a1, a2, b1, b2, threshold int) bool {
	// Direct overlap
	if a1 <= b2 && b1 <= a2 {
		return true
	}

	// Gap calculation
	var gap int
	if a2 < b1 {
		// a is before b
		gap = b1 - a2
	} else {
		// b is before a
		gap = a1 - b2
	}

	return gap <= threshold
}

// ExtractUnpairedFindings returns findings that have no candidate pairs.
// These are findings in files with no existing comments, or too far from existing comments.
func ExtractUnpairedFindings(
	newFindings []domain.Finding,
	candidates []CandidatePair,
) []domain.Finding {
	// Build set of paired finding descriptions (using description as key since we don't have IDs)
	paired := make(map[string]bool)
	for _, cp := range candidates {
		// Use a composite key to uniquely identify findings
		key := findingKey(cp.New)
		paired[key] = true
	}

	var unpaired []domain.Finding
	for _, nf := range newFindings {
		if !paired[findingKey(nf)] {
			unpaired = append(unpaired, nf)
		}
	}

	return unpaired
}

// findingKey creates a unique key for a finding based on its identity fields.
func findingKey(f domain.Finding) string {
	return f.File + "|" + f.Category + "|" + f.Severity + "|" + f.Description
}
