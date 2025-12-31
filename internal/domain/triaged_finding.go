package domain

// TriagedFinding represents a finding that has been previously addressed.
// This is used to provide context to LLMs during review so they don't
// re-raise concerns that have already been acknowledged or disputed.
//
// Unlike the full Finding struct, TriagedFinding captures only what the
// LLM needs to know to avoid duplication:
// - What the concern was (file, description)
// - How it was resolved (status, reason)
type TriagedFinding struct {
	// File is the path to the file containing the finding.
	File string `json:"file"`

	// LineStart is the starting line number (1-based).
	// May differ from original if code has shifted.
	LineStart int `json:"lineStart"`

	// LineEnd is the ending line number (1-based, inclusive).
	LineEnd int `json:"lineEnd"`

	// Category is the finding category (security, bug, performance, etc.).
	Category string `json:"category"`

	// Severity is the finding severity (critical, high, medium, low).
	Severity string `json:"severity"`

	// Description is the original finding description.
	Description string `json:"description"`

	// Status indicates how the finding was resolved.
	Status FindingStatus `json:"status"`

	// StatusReason is the human-readable explanation of the status.
	// For acknowledged: "Author accepted the concern but won't fix"
	// For disputed: "Author disputed as false positive"
	StatusReason string `json:"statusReason"`

	// Fingerprint is the stable identifier for this finding.
	Fingerprint FindingFingerprint `json:"fingerprint"`

	// ReviewerName is the name of the reviewer that created this finding.
	// Used for filtering prior findings by reviewer during persona-aware reviews.
	// Part of Phase 3.2 - Reviewer Personas.
	ReviewerName string `json:"reviewerName,omitempty"`
}

// TriagedFindingContext holds all triaged findings for a PR.
// This is injected into the LLM prompt to prevent re-raising addressed concerns.
type TriagedFindingContext struct {
	// PRNumber is the pull request number.
	PRNumber int `json:"prNumber"`

	// Findings is the list of triaged (acknowledged or disputed) findings.
	Findings []TriagedFinding `json:"findings"`
}

// HasFindings returns true if there are any triaged findings.
func (c TriagedFindingContext) HasFindings() bool {
	return len(c.Findings) > 0
}

// AcknowledgedFindings returns findings with StatusAcknowledged.
func (c TriagedFindingContext) AcknowledgedFindings() []TriagedFinding {
	var result []TriagedFinding
	for _, f := range c.Findings {
		if f.Status == StatusAcknowledged {
			result = append(result, f)
		}
	}
	return result
}

// DisputedFindings returns findings with StatusDisputed.
func (c TriagedFindingContext) DisputedFindings() []TriagedFinding {
	var result []TriagedFinding
	for _, f := range c.Findings {
		if f.Status == StatusDisputed {
			result = append(result, f)
		}
	}
	return result
}

// FilterByReviewer returns a new TriagedFindingContext containing only
// findings from the specified reviewer. Returns nil if no matching findings.
// Part of Phase 3.2 - Reviewer Personas.
func (c *TriagedFindingContext) FilterByReviewer(reviewerName string) *TriagedFindingContext {
	if c == nil || !c.HasFindings() {
		return nil
	}

	var filtered []TriagedFinding
	for _, f := range c.Findings {
		if f.ReviewerName == reviewerName {
			filtered = append(filtered, f)
		}
	}

	if len(filtered) == 0 {
		return nil
	}

	return &TriagedFindingContext{
		PRNumber: c.PRNumber,
		Findings: filtered,
	}
}

// StatusReasonForAcknowledged returns a human-readable reason for acknowledged status.
func StatusReasonForAcknowledged() string {
	return "Author acknowledged the concern"
}

// StatusReasonForDisputed returns a human-readable reason for disputed status.
func StatusReasonForDisputed() string {
	return "Author disputed as false positive or not applicable"
}
