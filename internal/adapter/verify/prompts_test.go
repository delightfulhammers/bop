package verify_test

import (
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/verify"
	"github.com/delightfulhammers/bop/internal/config"
	"github.com/delightfulhammers/bop/internal/domain"
	usecaseverify "github.com/delightfulhammers/bop/internal/usecase/verify"
)

func TestVerificationPrompt(t *testing.T) {
	repo := &mockRepository{}
	tools := verify.NewToolRegistry(repo)

	prompt := verify.VerificationPrompt(tools)

	t.Run("contains classification criteria", func(t *testing.T) {
		if !strings.Contains(prompt, "blocking_bug") {
			t.Error("missing blocking_bug classification")
		}
		if !strings.Contains(prompt, "security") {
			t.Error("missing security classification")
		}
		if !strings.Contains(prompt, "performance") {
			t.Error("missing performance classification")
		}
		if !strings.Contains(prompt, "style") {
			t.Error("missing style classification")
		}
	})

	t.Run("contains confidence scoring guidance", func(t *testing.T) {
		if !strings.Contains(prompt, "90-100") {
			t.Error("missing high confidence range")
		}
		if !strings.Contains(prompt, "Below 50") {
			t.Error("missing low confidence guidance")
		}
	})

	t.Run("lists all tools", func(t *testing.T) {
		for _, tool := range tools {
			if !strings.Contains(prompt, tool.Name()) {
				t.Errorf("missing tool %s in prompt", tool.Name())
			}
		}
	})

	t.Run("contains response format", func(t *testing.T) {
		if !strings.Contains(prompt, "verified") {
			t.Error("missing verified field in response format")
		}
		if !strings.Contains(prompt, "classification") {
			t.Error("missing classification field in response format")
		}
		if !strings.Contains(prompt, "confidence") {
			t.Error("missing confidence field in response format")
		}
		if !strings.Contains(prompt, "evidence") {
			t.Error("missing evidence field in response format")
		}
	})

	t.Run("contains false positive patterns guidance", func(t *testing.T) {
		if !strings.Contains(prompt, "False Positive Patterns") {
			t.Error("missing false positive patterns section")
		}
		if !strings.Contains(prompt, "Short-circuit null guards") {
			t.Error("missing short-circuit null guards pattern")
		}
		if !strings.Contains(prompt, "Short-circuit OR guards") {
			t.Error("missing short-circuit OR guards pattern")
		}
		if !strings.Contains(prompt, "Optional chaining operators") {
			t.Error("missing optional chaining pattern")
		}
		if !strings.Contains(prompt, "Guard clauses with early return") {
			t.Error("missing guard clauses pattern")
		}
		// Check language examples
		if !strings.Contains(prompt, "Go") {
			t.Error("missing Go example")
		}
		if !strings.Contains(prompt, "JavaScript") {
			t.Error("missing JavaScript example")
		}
		if !strings.Contains(prompt, "Python") {
			t.Error("missing Python example")
		}
	})
}

func TestCandidatePrompt(t *testing.T) {
	t.Run("includes all finding details", func(t *testing.T) {
		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				File:        "handler.go",
				LineStart:   42,
				LineEnd:     45,
				Severity:    "high",
				Description: "Null pointer dereference",
				Category:    "bug",
				Suggestion:  "Add nil check",
			},
			Sources:        []string{"openai", "anthropic"},
			AgreementScore: 0.85,
		}

		prompt := verify.CandidatePrompt(candidate)

		if !strings.Contains(prompt, "handler.go") {
			t.Error("missing file name")
		}
		if !strings.Contains(prompt, "42") {
			t.Error("missing line start")
		}
		if !strings.Contains(prompt, "45") {
			t.Error("missing line end")
		}
		if !strings.Contains(prompt, "high") {
			t.Error("missing severity")
		}
		if !strings.Contains(prompt, "Null pointer dereference") {
			t.Error("missing description")
		}
		if !strings.Contains(prompt, "bug") {
			t.Error("missing category")
		}
		if !strings.Contains(prompt, "Add nil check") {
			t.Error("missing suggestion")
		}
		if !strings.Contains(prompt, "85%") {
			t.Error("missing agreement score")
		}
		if !strings.Contains(prompt, "openai") {
			t.Error("missing source")
		}
	})

	t.Run("handles single line", func(t *testing.T) {
		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				File:      "main.go",
				LineStart: 10,
				LineEnd:   10,
			},
		}

		prompt := verify.CandidatePrompt(candidate)

		if !strings.Contains(prompt, "Line**: 10") {
			t.Error("should show single line format")
		}
	})

	t.Run("handles no line information", func(t *testing.T) {
		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				File:        "main.go",
				Description: "Some issue",
			},
		}

		prompt := verify.CandidatePrompt(candidate)

		if strings.Contains(prompt, "Line**:") {
			t.Error("should not show line when not provided")
		}
	})
}

func TestToolResultPrompt(t *testing.T) {
	result := verify.ToolResultPrompt("read_file", "main.go", "package main\n\nfunc main() {}")

	if !strings.Contains(result, "read_file") {
		t.Error("missing tool name")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("missing input")
	}
	if !strings.Contains(result, "package main") {
		t.Error("missing output")
	}
}

func TestConfidenceThreshold(t *testing.T) {
	t.Run("uses specific thresholds when set", func(t *testing.T) {
		thresholds := config.ConfidenceThresholds{
			Critical: 40,
			High:     50,
			Medium:   60,
			Low:      70,
		}

		if got := verify.ConfidenceThreshold("critical", thresholds); got != 40 {
			t.Errorf("critical: got %d, want 40", got)
		}
		if got := verify.ConfidenceThreshold("high", thresholds); got != 50 {
			t.Errorf("high: got %d, want 50", got)
		}
		if got := verify.ConfidenceThreshold("medium", thresholds); got != 60 {
			t.Errorf("medium: got %d, want 60", got)
		}
		if got := verify.ConfidenceThreshold("low", thresholds); got != 70 {
			t.Errorf("low: got %d, want 70", got)
		}
	})

	t.Run("uses default when specific not set", func(t *testing.T) {
		thresholds := config.ConfidenceThresholds{
			Default: 55,
		}

		if got := verify.ConfidenceThreshold("high", thresholds); got != 55 {
			t.Errorf("got %d, want 55", got)
		}
	})

	t.Run("uses fallback when nothing set", func(t *testing.T) {
		thresholds := config.ConfidenceThresholds{}

		// Fallback defaults
		if got := verify.ConfidenceThreshold("critical", thresholds); got != 50 {
			t.Errorf("critical fallback: got %d, want 50", got)
		}
		if got := verify.ConfidenceThreshold("high", thresholds); got != 60 {
			t.Errorf("high fallback: got %d, want 60", got)
		}
		if got := verify.ConfidenceThreshold("medium", thresholds); got != 70 {
			t.Errorf("medium fallback: got %d, want 70", got)
		}
		if got := verify.ConfidenceThreshold("low", thresholds); got != 80 {
			t.Errorf("low fallback: got %d, want 80", got)
		}
	})

	t.Run("is case insensitive", func(t *testing.T) {
		thresholds := config.ConfidenceThresholds{Critical: 40}

		if got := verify.ConfidenceThreshold("CRITICAL", thresholds); got != 40 {
			t.Errorf("uppercase: got %d, want 40", got)
		}
		if got := verify.ConfidenceThreshold("Critical", thresholds); got != 40 {
			t.Errorf("mixed case: got %d, want 40", got)
		}
	})
}

func TestShouldBlockOperation(t *testing.T) {
	tests := []struct {
		name     string
		result   domain.VerificationResult
		expected bool
	}{
		{
			name: "unverified never blocks",
			result: domain.VerificationResult{
				Verified:       false,
				Classification: domain.ClassBlockingBug,
				Confidence:     100,
			},
			expected: false,
		},
		{
			name: "blocking bug always blocks",
			result: domain.VerificationResult{
				Verified:       true,
				Classification: domain.ClassBlockingBug,
				Confidence:     60,
			},
			expected: true,
		},
		{
			name: "security always blocks",
			result: domain.VerificationResult{
				Verified:       true,
				Classification: domain.ClassSecurity,
				Confidence:     60,
			},
			expected: true,
		},
		{
			name: "style never blocks",
			result: domain.VerificationResult{
				Verified:       true,
				Classification: domain.ClassStyle,
				Confidence:     100,
			},
			expected: false,
		},
		{
			name: "performance blocks at high confidence",
			result: domain.VerificationResult{
				Verified:       true,
				Classification: domain.ClassPerformance,
				Confidence:     85,
			},
			expected: true,
		},
		{
			name: "performance does not block at low confidence",
			result: domain.VerificationResult{
				Verified:       true,
				Classification: domain.ClassPerformance,
				Confidence:     70,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verify.ShouldBlockOperation(tt.result)
			if got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

// Compile-time interface check (using usecaseverify for Repository)
var _ usecaseverify.Repository = (*mockRepository)(nil)
