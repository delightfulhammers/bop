package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTriageStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status TriageStatus
		want   bool
	}{
		{"pending is valid", TriageStatusPending, true},
		{"accepted is valid", TriageStatusAccepted, true},
		{"disputed is valid", TriageStatusDisputed, true},
		{"question is valid", TriageStatusQuestion, true},
		{"resolved is valid", TriageStatusResolved, true},
		{"wont_fix is valid", TriageStatusWontFix, true},
		{"empty is invalid", TriageStatus(""), false},
		{"unknown is invalid", TriageStatus("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsValid()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTriageStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		name   string
		status TriageStatus
		want   bool
	}{
		{"pending is not terminal", TriageStatusPending, false},
		{"accepted is not terminal", TriageStatusAccepted, false},
		{"question is not terminal", TriageStatusQuestion, false},
		{"disputed is terminal", TriageStatusDisputed, true},
		{"resolved is terminal", TriageStatusResolved, true},
		{"wont_fix is terminal", TriageStatusWontFix, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsTerminal()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAllTriageStatuses(t *testing.T) {
	statuses := AllTriageStatuses()
	assert.Len(t, statuses, 6)
	assert.Contains(t, statuses, TriageStatusPending)
	assert.Contains(t, statuses, TriageStatusAccepted)
	assert.Contains(t, statuses, TriageStatusDisputed)
	assert.Contains(t, statuses, TriageStatusQuestion)
	assert.Contains(t, statuses, TriageStatusResolved)
	assert.Contains(t, statuses, TriageStatusWontFix)
}

func TestNewTriageSession(t *testing.T) {
	findings := []Finding{
		{ID: "f1", File: "main.go", LineStart: 10, LineEnd: 15, Severity: "high", Category: "security"},
		{ID: "f2", File: "util.go", LineStart: 20, LineEnd: 25, Severity: "medium", Category: "bug"},
	}

	session := NewTriageSession(42, "owner/repo", findings)

	assert.NotEmpty(t, session.ID)
	assert.Equal(t, 42, session.PRNumber)
	assert.Equal(t, "owner/repo", session.Repository)
	assert.Len(t, session.Findings, 2)
	assert.Empty(t, session.Decisions)
	assert.Equal(t, 0, session.CurrentIndex)

	// All findings should start as pending
	for _, f := range session.Findings {
		assert.Equal(t, TriageStatusPending, f.TriageStatus)
	}
}

func TestTriageSession_CurrentFinding(t *testing.T) {
	findings := []Finding{
		{ID: "f1", File: "main.go"},
		{ID: "f2", File: "util.go"},
	}
	session := NewTriageSession(1, "owner/repo", findings)

	// Initial current finding should be first
	current := session.CurrentFinding()
	require.NotNil(t, current)
	assert.Equal(t, "f1", current.ID)

	// Advance to second finding
	session.CurrentIndex = 1
	current = session.CurrentFinding()
	require.NotNil(t, current)
	assert.Equal(t, "f2", current.ID)

	// Past end should return nil
	session.CurrentIndex = 2
	current = session.CurrentFinding()
	assert.Nil(t, current)

	// Negative index should return nil
	session.CurrentIndex = -1
	current = session.CurrentFinding()
	assert.Nil(t, current)
}

func TestTriageSession_Progress(t *testing.T) {
	findings := []Finding{
		{ID: "f1"},
		{ID: "f2"},
		{ID: "f3"},
	}
	session := NewTriageSession(1, "owner/repo", findings)

	// Initially none triaged
	triaged, total := session.Progress()
	assert.Equal(t, 0, triaged)
	assert.Equal(t, 3, total)

	// Triage one finding
	session.Findings[0].TriageStatus = TriageStatusAccepted
	triaged, total = session.Progress()
	assert.Equal(t, 1, triaged)
	assert.Equal(t, 3, total)

	// Triage all findings
	session.Findings[1].TriageStatus = TriageStatusDisputed
	session.Findings[2].TriageStatus = TriageStatusResolved
	triaged, total = session.Progress()
	assert.Equal(t, 3, triaged)
	assert.Equal(t, 3, total)
}

func TestTriageSession_IsComplete(t *testing.T) {
	findings := []Finding{{ID: "f1"}, {ID: "f2"}}
	session := NewTriageSession(1, "owner/repo", findings)

	assert.False(t, session.IsComplete())

	session.Findings[0].TriageStatus = TriageStatusAccepted
	assert.False(t, session.IsComplete())

	session.Findings[1].TriageStatus = TriageStatusDisputed
	assert.True(t, session.IsComplete())
}

func TestTriageSession_PendingFindings(t *testing.T) {
	findings := []Finding{
		{ID: "f1"},
		{ID: "f2"},
		{ID: "f3"},
	}
	session := NewTriageSession(1, "owner/repo", findings)

	// Initially all pending
	pending := session.PendingFindings()
	assert.Len(t, pending, 3)

	// Triage one
	session.Findings[1].TriageStatus = TriageStatusAccepted
	pending = session.PendingFindings()
	assert.Len(t, pending, 2)
	assert.Equal(t, "f1", pending[0].ID)
	assert.Equal(t, "f3", pending[1].ID)
}

func TestTriageSession_FindingByID(t *testing.T) {
	findings := []Finding{
		{ID: "f1", File: "main.go"},
		{ID: "f2", File: "util.go"},
	}
	session := NewTriageSession(1, "owner/repo", findings)

	found := session.FindingByID("f1")
	require.NotNil(t, found)
	assert.Equal(t, "main.go", found.File)

	found = session.FindingByID("f2")
	require.NotNil(t, found)
	assert.Equal(t, "util.go", found.File)

	found = session.FindingByID("nonexistent")
	assert.Nil(t, found)
}

func TestTriageSession_ApplyDecision(t *testing.T) {
	findings := []Finding{
		{ID: "f1", File: "main.go", Severity: "high", Category: "security", Description: "test"},
	}
	session := NewTriageSession(1, "owner/repo", findings)

	decision := TriageDecision{
		FindingID: "f1",
		Status:    TriageStatusAccepted,
		Reason:    "valid finding",
		TriagedAt: time.Now(),
		TriagedBy: "reviewer",
	}

	err := session.ApplyDecision(decision)
	require.NoError(t, err)

	assert.Equal(t, TriageStatusAccepted, session.Findings[0].TriageStatus)
	assert.Len(t, session.Decisions, 1)
	assert.Equal(t, decision.Reason, session.Decisions[0].Reason)
}

func TestTriageSession_ApplyDecision_ByFingerprint(t *testing.T) {
	findings := []Finding{
		{ID: "f1", File: "main.go", Severity: "high", Category: "security", Description: "test"},
	}
	session := NewTriageSession(1, "owner/repo", findings)

	fp := string(session.Findings[0].Fingerprint())
	decision := TriageDecision{
		FindingID:   "wrong-id",
		Fingerprint: fp,
		Status:      TriageStatusDisputed,
	}

	err := session.ApplyDecision(decision)
	require.NoError(t, err)
	assert.Equal(t, TriageStatusDisputed, session.Findings[0].TriageStatus)
}

func TestTriageSession_ApplyDecision_NotFound(t *testing.T) {
	findings := []Finding{{ID: "f1"}}
	session := NewTriageSession(1, "owner/repo", findings)

	decision := TriageDecision{
		FindingID:   "wrong-id",
		Fingerprint: "wrong-fp",
		Status:      TriageStatusAccepted,
	}

	err := session.ApplyDecision(decision)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "finding not found")
}
