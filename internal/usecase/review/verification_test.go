package review

import (
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
)

func TestParseSources(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single provider",
			input:    "openai",
			expected: []string{"openai"},
		},
		{
			name:     "merged with two providers",
			input:    "merged (openai, anthropic)",
			expected: []string{"openai", "anthropic"},
		},
		{
			name:     "merged with three providers",
			input:    "merged (openai, anthropic, gemini)",
			expected: []string{"openai", "anthropic", "gemini"},
		},
		{
			name:     "merged with no parentheses",
			input:    "merged",
			expected: []string{"merged"},
		},
		{
			name:     "merged with extra spaces",
			input:    "merged ( openai , anthropic )",
			expected: []string{"openai", "anthropic"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSources(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseSources(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("parseSources(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestConvertFindingsToCandidates(t *testing.T) {
	findings := []domain.Finding{
		{
			File:        "main.go",
			LineStart:   10,
			Description: "Null pointer dereference",
			Severity:    "high",
		},
		{
			File:        "utils.go",
			LineStart:   25,
			Description: "Unused variable",
			Severity:    "low",
		},
	}

	t.Run("converts findings with single provider", func(t *testing.T) {
		candidates := convertFindingsToCandidates(findings, "openai")

		if len(candidates) != 2 {
			t.Fatalf("expected 2 candidates, got %d", len(candidates))
		}

		if candidates[0].Finding.File != "main.go" {
			t.Errorf("expected file main.go, got %s", candidates[0].Finding.File)
		}
		if len(candidates[0].Sources) != 1 || candidates[0].Sources[0] != "openai" {
			t.Errorf("expected sources [openai], got %v", candidates[0].Sources)
		}
		if candidates[0].AgreementScore != 1.0 {
			t.Errorf("expected agreement score 1.0, got %f", candidates[0].AgreementScore)
		}
	})

	t.Run("converts findings with merged providers", func(t *testing.T) {
		candidates := convertFindingsToCandidates(findings, "merged (openai, anthropic)")

		if len(candidates[0].Sources) != 2 {
			t.Errorf("expected 2 sources, got %d", len(candidates[0].Sources))
		}
		if candidates[0].Sources[0] != "openai" || candidates[0].Sources[1] != "anthropic" {
			t.Errorf("expected sources [openai, anthropic], got %v", candidates[0].Sources)
		}
	})
}

func TestBuildVerifiedFindings(t *testing.T) {
	candidates := []domain.CandidateFinding{
		{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   10,
				Description: "Null pointer dereference",
				Severity:    "high",
			},
			Sources:        []string{"openai"},
			AgreementScore: 1.0,
		},
		{
			Finding: domain.Finding{
				File:        "utils.go",
				LineStart:   25,
				Description: "Unused variable",
				Severity:    "low",
			},
			Sources:        []string{"anthropic"},
			AgreementScore: 1.0,
		},
	}

	results := []domain.VerificationResult{
		{
			Verified:        true,
			Classification:  domain.ClassBlockingBug,
			Confidence:      90,
			Evidence:        "Confirmed null pointer at line 10",
			BlocksOperation: true,
		},
		{
			Verified:       false,
			Classification: domain.ClassStyle,
			Confidence:     30,
			Evidence:       "Variable is used in a different branch",
		},
	}

	t.Run("builds verified findings from results", func(t *testing.T) {
		verified := buildVerifiedFindings(candidates, results)

		if len(verified) != 2 {
			t.Fatalf("expected 2 verified findings, got %d", len(verified))
		}

		// First finding should be verified
		if !verified[0].Verified {
			t.Error("expected first finding to be verified")
		}
		if verified[0].Classification != domain.ClassBlockingBug {
			t.Errorf("expected classification blocking_bug, got %s", verified[0].Classification)
		}
		if verified[0].Confidence != 90 {
			t.Errorf("expected confidence 90, got %d", verified[0].Confidence)
		}
		if !verified[0].BlocksOperation {
			t.Error("expected first finding to block operation")
		}

		// Second finding should not be verified
		if verified[1].Verified {
			t.Error("expected second finding to not be verified")
		}
		if verified[1].Confidence != 30 {
			t.Errorf("expected confidence 30, got %d", verified[1].Confidence)
		}
	})

	t.Run("returns nil for mismatched lengths", func(t *testing.T) {
		verified := buildVerifiedFindings(candidates[:1], results)
		if verified != nil {
			t.Error("expected nil for mismatched lengths")
		}
	})
}

func TestFilterByConfidence(t *testing.T) {
	verified := []domain.VerifiedFinding{
		{
			Finding: domain.Finding{
				File:     "main.go",
				Severity: "critical",
			},
			Verified:   true,
			Confidence: 60, // Above 50 threshold for critical
		},
		{
			Finding: domain.Finding{
				File:     "utils.go",
				Severity: "high",
			},
			Verified:   true,
			Confidence: 55, // Below 60 threshold for high
		},
		{
			Finding: domain.Finding{
				File:     "helper.go",
				Severity: "low",
			},
			Verified:   true,
			Confidence: 85, // Above 80 threshold for low
		},
		{
			Finding: domain.Finding{
				File:     "test.go",
				Severity: "medium",
			},
			Verified:   false, // Unverified, should be excluded
			Confidence: 100,
		},
	}

	t.Run("filters by default thresholds", func(t *testing.T) {
		settings := VerificationSettings{} // Use defaults

		reportable := filterByConfidence(verified, settings)

		// Should include: critical (60 >= 50), low (85 >= 80)
		// Should exclude: high (55 < 60), medium (unverified)
		if len(reportable) != 2 {
			t.Fatalf("expected 2 reportable findings, got %d", len(reportable))
		}

		files := make(map[string]bool)
		for _, r := range reportable {
			files[r.Finding.File] = true
		}

		if !files["main.go"] {
			t.Error("expected main.go (critical) to be included")
		}
		if !files["helper.go"] {
			t.Error("expected helper.go (low) to be included")
		}
		if files["utils.go"] {
			t.Error("expected utils.go (high) to be excluded")
		}
		if files["test.go"] {
			t.Error("expected test.go (unverified) to be excluded")
		}
	})

	t.Run("uses custom thresholds", func(t *testing.T) {
		settings := VerificationSettings{
			ConfidenceHigh: 50, // Lower threshold for high
		}

		reportable := filterByConfidence(verified, settings)

		// Now high should be included (55 >= 50)
		found := false
		for _, r := range reportable {
			if r.Finding.File == "utils.go" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected utils.go (high) to be included with custom threshold")
		}
	})

	t.Run("uses default threshold as fallback", func(t *testing.T) {
		settings := VerificationSettings{
			ConfidenceDefault: 40, // Very low default
		}

		reportable := filterByConfidence(verified, settings)

		// With 40 default, high (55) should now be included
		found := false
		for _, r := range reportable {
			if r.Finding.File == "utils.go" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected utils.go (high) to be included with default threshold")
		}
	})
}

func TestGetThresholdForSeverity(t *testing.T) {
	tests := []struct {
		name     string
		severity string
		settings VerificationSettings
		expected int
	}{
		{
			name:     "critical with specific threshold",
			severity: "critical",
			settings: VerificationSettings{ConfidenceCritical: 45},
			expected: 45,
		},
		{
			name:     "high with default fallback",
			severity: "high",
			settings: VerificationSettings{ConfidenceDefault: 55},
			expected: 55,
		},
		{
			name:     "medium with no settings",
			severity: "medium",
			settings: VerificationSettings{},
			expected: 70, // Built-in default for medium
		},
		{
			name:     "unknown severity",
			severity: "unknown",
			settings: VerificationSettings{},
			expected: 70, // Built-in default
		},
		{
			name:     "case insensitive",
			severity: "CRITICAL",
			settings: VerificationSettings{ConfidenceCritical: 42},
			expected: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getThresholdForSeverity(tt.severity, tt.settings)
			if result != tt.expected {
				t.Errorf("getThresholdForSeverity(%q) = %d, want %d", tt.severity, result, tt.expected)
			}
		})
	}
}

func TestConvertVerifiedToFindings(t *testing.T) {
	verified := []domain.VerifiedFinding{
		{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   10,
				Description: "Issue 1",
			},
			Verified:   true,
			Confidence: 90,
		},
		{
			Finding: domain.Finding{
				File:        "utils.go",
				LineStart:   25,
				Description: "Issue 2",
			},
			Verified:   true,
			Confidence: 85,
		},
	}

	findings := convertVerifiedToFindings(verified)

	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}

	if findings[0].File != "main.go" || findings[0].Description != "Issue 1" {
		t.Errorf("first finding mismatch: %+v", findings[0])
	}
	if findings[1].File != "utils.go" || findings[1].Description != "Issue 2" {
		t.Errorf("second finding mismatch: %+v", findings[1])
	}
}
