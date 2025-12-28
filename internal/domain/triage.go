package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

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

// TriageDecision captures a triage action taken on a finding.
type TriageDecision struct {
	FindingID   string       `json:"findingId"`
	Fingerprint string       `json:"fingerprint"`
	Status      TriageStatus `json:"status"`
	Reason      string       `json:"reason,omitempty"`
	TriagedAt   time.Time    `json:"triagedAt"`
	TriagedBy   string       `json:"triagedBy,omitempty"`
}

// TriageSession represents an active triage workflow session.
// It tracks progress through a set of findings from a review.
type TriageSession struct {
	ID           string            `json:"id"`
	PRNumber     int               `json:"prNumber"`
	Repository   string            `json:"repository"`
	ReviewID     string            `json:"reviewId,omitempty"`
	StartedAt    time.Time         `json:"startedAt"`
	Findings     []TriageFinding   `json:"findings"`
	Decisions    []TriageDecision  `json:"decisions"`
	CurrentIndex int               `json:"currentIndex"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// TriageFinding extends a Finding with triage-specific state.
type TriageFinding struct {
	Finding
	TriageStatus    TriageStatus `json:"triageStatus"`
	GitHubCommentID int64        `json:"githubCommentId,omitempty"`
	ThreadResolved  bool         `json:"threadResolved"`
}

// NewTriageSession creates a new triage session for the given PR.
func NewTriageSession(prNumber int, repository string, findings []Finding) TriageSession {
	now := time.Now()
	sessionID := generateTriageSessionID(prNumber, repository, now)

	triageFindings := make([]TriageFinding, len(findings))
	for i, f := range findings {
		triageFindings[i] = TriageFinding{
			Finding:      f,
			TriageStatus: TriageStatusPending,
		}
	}

	return TriageSession{
		ID:           sessionID,
		PRNumber:     prNumber,
		Repository:   repository,
		StartedAt:    now,
		Findings:     triageFindings,
		Decisions:    make([]TriageDecision, 0),
		CurrentIndex: 0,
		Metadata:     make(map[string]string),
	}
}

// CurrentFinding returns the finding at the current index, or nil if exhausted.
func (s *TriageSession) CurrentFinding() *TriageFinding {
	if s.CurrentIndex < 0 || s.CurrentIndex >= len(s.Findings) {
		return nil
	}
	return &s.Findings[s.CurrentIndex]
}

// Progress returns the number of triaged findings and total findings.
func (s *TriageSession) Progress() (triaged, total int) {
	total = len(s.Findings)
	for _, f := range s.Findings {
		if f.TriageStatus != TriageStatusPending {
			triaged++
		}
	}
	return triaged, total
}

// IsComplete returns true if all findings have been triaged.
func (s *TriageSession) IsComplete() bool {
	triaged, total := s.Progress()
	return triaged >= total
}

// PendingFindings returns findings that still need triage.
func (s *TriageSession) PendingFindings() []TriageFinding {
	var pending []TriageFinding
	for _, f := range s.Findings {
		if f.TriageStatus == TriageStatusPending {
			pending = append(pending, f)
		}
	}
	return pending
}

// FindingByID returns the triage finding with the given ID, or nil if not found.
func (s *TriageSession) FindingByID(id string) *TriageFinding {
	for i := range s.Findings {
		if s.Findings[i].ID == id {
			return &s.Findings[i]
		}
	}
	return nil
}

// FindingByFingerprint returns the triage finding with the given fingerprint.
func (s *TriageSession) FindingByFingerprint(fp string) *TriageFinding {
	for i := range s.Findings {
		if string(s.Findings[i].Fingerprint()) == fp {
			return &s.Findings[i]
		}
	}
	return nil
}

// ApplyDecision applies a triage decision to the session.
func (s *TriageSession) ApplyDecision(decision TriageDecision) error {
	finding := s.FindingByID(decision.FindingID)
	if finding == nil {
		finding = s.FindingByFingerprint(decision.Fingerprint)
	}
	if finding == nil {
		return fmt.Errorf("finding not found: id=%s fingerprint=%s", decision.FindingID, decision.Fingerprint)
	}

	finding.TriageStatus = decision.Status
	s.Decisions = append(s.Decisions, decision)
	return nil
}

func generateTriageSessionID(prNumber int, repository string, timestamp time.Time) string {
	payload := fmt.Sprintf("%d|%s|%d", prNumber, repository, timestamp.UnixNano())
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:8]) // 16 hex chars
}

// TriageContext provides contextual information for triage decisions.
// This is populated by reading GitHub PR data, file contents, etc.
type TriageContext struct {
	PRTitle       string            `json:"prTitle"`
	PRDescription string            `json:"prDescription"`
	PRAuthor      string            `json:"prAuthor"`
	FileContents  map[string]string `json:"fileContents,omitempty"`
	ThreadHistory []ThreadComment   `json:"threadHistory,omitempty"`
}

// ThreadComment represents a comment in a GitHub review thread.
type ThreadComment struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	IsReply   bool      `json:"isReply"`
}
