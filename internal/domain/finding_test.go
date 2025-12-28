package domain_test

import (
	"testing"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

func TestFindingDeterministicID(t *testing.T) {
	finding := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     12,
		Severity:    "high",
		Category:    "bug",
		Description: "Example bug",
		Suggestion:  "Fix bug",
		Evidence:    true,
	})

	again := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     12,
		Severity:    "high",
		Category:    "bug",
		Description: "Example bug",
		Suggestion:  "Fix bug",
		Evidence:    true,
	})

	if finding.ID != again.ID {
		t.Fatalf("expected deterministic IDs, got %s and %s", finding.ID, again.ID)
	}
}

func TestFinding_Fingerprint_MatchesFingerprintFromFinding(t *testing.T) {
	finding := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk in query builder",
		Suggestion:  "Use parameterized queries",
		Evidence:    true,
	})

	methodResult := finding.Fingerprint()
	functionResult := domain.FingerprintFromFinding(finding)

	if methodResult != functionResult {
		t.Errorf("Fingerprint() = %s, want %s (from FingerprintFromFinding)", methodResult, functionResult)
	}
}

func TestFinding_Fingerprint_Deterministic(t *testing.T) {
	finding1 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk",
		Suggestion:  "Use parameterized queries",
		Evidence:    true,
	})

	finding2 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk",
		Suggestion:  "Use parameterized queries",
		Evidence:    true,
	})

	fp := finding1.Fingerprint()
	if fp == "" {
		t.Error("fingerprint should not be empty")
	}

	// Verify fingerprint format: should be 32 hex characters (SHA-256 truncated to 16 bytes)
	// See NewFindingFingerprint in tracking.go: hex.EncodeToString(sum[:16])
	if len(fp) != 32 {
		t.Errorf("fingerprint should be 32 hex characters, got %d: %s", len(fp), fp)
	}
	for _, c := range fp {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("fingerprint should be lowercase hex, found char: %c in %s", c, fp)
			break
		}
	}

	if fp != finding2.Fingerprint() {
		t.Errorf("fingerprints should be deterministic: %s != %s",
			fp, finding2.Fingerprint())
	}
}

func TestFinding_Fingerprint_StableAcrossLineChanges(t *testing.T) {
	// Same issue at different line numbers should have same fingerprint
	finding1 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk",
		Suggestion:  "Use parameterized queries",
		Evidence:    true,
	})

	finding2 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   50, // Different line
		LineEnd:     55, // Different line
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk",
		Suggestion:  "Use parameterized queries",
		Evidence:    true,
	})

	if finding1.Fingerprint() != finding2.Fingerprint() {
		t.Errorf("fingerprints should be stable across line changes: %s != %s",
			finding1.Fingerprint(), finding2.Fingerprint())
	}
}

func TestFinding_Fingerprint_DifferentForDifferentFiles(t *testing.T) {
	finding1 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk",
		Suggestion:  "Fix it",
		Evidence:    true,
	})

	finding2 := domain.NewFinding(domain.FindingInput{
		File:        "db.go", // Different file
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk",
		Suggestion:  "Fix it",
		Evidence:    true,
	})

	if finding1.Fingerprint() == finding2.Fingerprint() {
		t.Error("fingerprints should differ for different files")
	}
}

func TestFinding_Fingerprint_DifferentForDifferentCategories(t *testing.T) {
	finding1 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "Issue description",
		Suggestion:  "Fix it",
		Evidence:    true,
	})

	finding2 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "performance", // Different category
		Description: "Issue description",
		Suggestion:  "Fix it",
		Evidence:    true,
	})

	if finding1.Fingerprint() == finding2.Fingerprint() {
		t.Error("fingerprints should differ for different categories")
	}
}

func TestFinding_Fingerprint_DifferentForDifferentSeverities(t *testing.T) {
	finding1 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "Issue description",
		Suggestion:  "Fix it",
		Evidence:    true,
	})

	finding2 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "low", // Different severity
		Category:    "security",
		Description: "Issue description",
		Suggestion:  "Fix it",
		Evidence:    true,
	})

	if finding1.Fingerprint() == finding2.Fingerprint() {
		t.Error("fingerprints should differ for different severities")
	}
}

func TestFinding_Fingerprint_DifferentForDifferentDescriptions(t *testing.T) {
	finding1 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk in user input",
		Suggestion:  "Fix it",
		Evidence:    true,
	})

	finding2 := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "XSS vulnerability in output encoding", // Different description
		Suggestion:  "Fix it",
		Evidence:    true,
	})

	if finding1.Fingerprint() == finding2.Fingerprint() {
		t.Error("fingerprints should differ for different descriptions")
	}
}

func TestFinding_Fingerprint_ExcludesNonIdentityFields(t *testing.T) {
	// Fingerprint should only include: file, category, severity, description prefix.
	// It should NOT include: Suggestion, Evidence, LineStart, LineEnd.
	base := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk",
		Suggestion:  "Use parameterized queries",
		Evidence:    true,
	})

	// Different Suggestion - fingerprint should be the same
	differentSuggestion := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk",
		Suggestion:  "Completely different suggestion text",
		Evidence:    true,
	})

	if base.Fingerprint() != differentSuggestion.Fingerprint() {
		t.Error("fingerprint should not change when only Suggestion differs")
	}

	// Different Evidence - fingerprint should be the same
	differentEvidence := domain.NewFinding(domain.FindingInput{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection risk",
		Suggestion:  "Use parameterized queries",
		Evidence:    false, // Different Evidence
	})

	if base.Fingerprint() != differentEvidence.Fingerprint() {
		t.Error("fingerprint should not change when only Evidence differs")
	}
}
