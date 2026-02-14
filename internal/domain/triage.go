package domain

import "time"

// TriageStatus represents the workflow state of a finding during triage.
// Unlike FindingStatus (which detects keywords from discussion text),
// TriageStatus tracks explicit workflow transitions made by the triager.
type TriageStatus string

const (
	// TriageStatusPending indicates the finding has not been triaged yet.
	TriageStatusPending TriageStatus = "pending"

	// TriageStatusAccepted indicates the finding was accepted as valid.
	TriageStatusAccepted TriageStatus = "accepted"

	// TriageStatusDisputed indicates the finding was disputed/rejected.
	TriageStatusDisputed TriageStatus = "disputed"

	// TriageStatusQuestion indicates the triager has a question about the finding.
	TriageStatusQuestion TriageStatus = "question"

	// TriageStatusResolved indicates the finding was addressed (code fixed).
	TriageStatusResolved TriageStatus = "resolved"

	// TriageStatusWontFix indicates the finding is acknowledged but won't be fixed.
	TriageStatusWontFix TriageStatus = "wont_fix"
)

// AllTriageStatuses returns all valid triage status values.
func AllTriageStatuses() []TriageStatus {
	return []TriageStatus{
		TriageStatusPending,
		TriageStatusAccepted,
		TriageStatusDisputed,
		TriageStatusQuestion,
		TriageStatusResolved,
		TriageStatusWontFix,
	}
}

// IsValid returns true if the status is a recognized value.
func (s TriageStatus) IsValid() bool {
	for _, valid := range AllTriageStatuses() {
		if s == valid {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the status represents a final state
// that typically requires no further action.
func (s TriageStatus) IsTerminal() bool {
	switch s {
	case TriageStatusDisputed, TriageStatusResolved, TriageStatusWontFix:
		return true
	default:
		return false
	}
}

// ThreadComment represents a comment in a GitHub review thread.
type ThreadComment struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	IsReply   bool      `json:"isReply"`
}
