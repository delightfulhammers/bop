package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

func TestParseActionConfig(t *testing.T) {
	// Save and restore environment
	envVars := []string{
		"BOP_BASE_REF",
		"BOP_POST_FINDINGS",
		"BOP_REVIEWERS",
		"BOP_BLOCK_THRESHOLD",
		"BOP_FAIL_ON_FINDINGS",
		"BOP_ALWAYS_BLOCK_CATEGORIES",
	}
	restore := saveAndClearEnv(t, envVars)
	defer restore()

	tests := []struct {
		name     string
		env      map[string]string
		expected actionConfig
	}{
		{
			name: "defaults",
			env:  map[string]string{},
			expected: actionConfig{
				BaseRef:        "main",
				PostFindings:   true,
				BlockThreshold: "none",
				FailOnFindings: false,
			},
		},
		{
			name: "custom base ref",
			env: map[string]string{
				"BOP_BASE_REF": "develop",
			},
			expected: actionConfig{
				BaseRef:        "develop",
				PostFindings:   true,
				BlockThreshold: "none",
				FailOnFindings: false,
			},
		},
		{
			name: "post findings false",
			env: map[string]string{
				"BOP_POST_FINDINGS": "false",
			},
			expected: actionConfig{
				BaseRef:        "main",
				PostFindings:   false,
				BlockThreshold: "none",
				FailOnFindings: false,
			},
		},
		{
			name: "reviewers list",
			env: map[string]string{
				"BOP_REVIEWERS": "security, architecture, code-reviewer",
			},
			expected: actionConfig{
				BaseRef:        "main",
				PostFindings:   true,
				BlockThreshold: "none",
				FailOnFindings: false,
				Reviewers:      []string{"security", "architecture", "code-reviewer"},
			},
		},
		{
			name: "block threshold high",
			env: map[string]string{
				"BOP_BLOCK_THRESHOLD":  "high",
				"BOP_FAIL_ON_FINDINGS": "true",
			},
			expected: actionConfig{
				BaseRef:        "main",
				PostFindings:   true,
				BlockThreshold: "high",
				FailOnFindings: true,
			},
		},
		{
			name: "always block categories",
			env: map[string]string{
				"BOP_ALWAYS_BLOCK_CATEGORIES": "security,bug",
			},
			expected: actionConfig{
				BaseRef:               "main",
				PostFindings:          true,
				BlockThreshold:        "none",
				FailOnFindings:        false,
				AlwaysBlockCategories: []string{"security", "bug"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all BOP_ vars and set test vars
			clearEnv(envVars)
			setEnv(tt.env)

			got, err := parseActionConfig()
			if err != nil {
				t.Fatalf("parseActionConfig() error = %v", err)
			}

			if got.BaseRef != tt.expected.BaseRef {
				t.Errorf("BaseRef = %q, want %q", got.BaseRef, tt.expected.BaseRef)
			}
			if got.PostFindings != tt.expected.PostFindings {
				t.Errorf("PostFindings = %v, want %v", got.PostFindings, tt.expected.PostFindings)
			}
			if got.BlockThreshold != tt.expected.BlockThreshold {
				t.Errorf("BlockThreshold = %q, want %q", got.BlockThreshold, tt.expected.BlockThreshold)
			}
			if got.FailOnFindings != tt.expected.FailOnFindings {
				t.Errorf("FailOnFindings = %v, want %v", got.FailOnFindings, tt.expected.FailOnFindings)
			}
			if len(got.Reviewers) != len(tt.expected.Reviewers) {
				t.Errorf("Reviewers length = %d, want %d", len(got.Reviewers), len(tt.expected.Reviewers))
			} else {
				for i, r := range got.Reviewers {
					if r != tt.expected.Reviewers[i] {
						t.Errorf("Reviewers[%d] = %q, want %q", i, r, tt.expected.Reviewers[i])
					}
				}
			}
		})
	}
}

func TestParseActionConfig_InvalidThreshold(t *testing.T) {
	envVars := []string{"BOP_BLOCK_THRESHOLD"}
	restore := saveAndClearEnv(t, envVars)
	defer restore()

	setEnv(map[string]string{
		"BOP_BLOCK_THRESHOLD": "hgh", // typo
	})

	_, err := parseActionConfig()
	if err == nil {
		t.Error("expected error for invalid threshold, got nil")
	}
	if !contains(err.Error(), "invalid BOP_BLOCK_THRESHOLD") {
		t.Errorf("error should mention invalid threshold, got: %v", err)
	}
}

func TestResolveBlockAction(t *testing.T) {
	tests := []struct {
		severity  string
		threshold string
		expected  string
	}{
		{"critical", "critical", "request_changes"},
		{"critical", "high", "request_changes"},
		{"critical", "none", "comment"},
		{"high", "critical", "comment"},
		{"high", "high", "request_changes"},
		{"high", "medium", "request_changes"},
		{"medium", "high", "comment"},
		{"medium", "medium", "request_changes"},
		{"low", "low", "request_changes"},
		{"low", "medium", "comment"},
		{"low", "none", "comment"},
	}

	for _, tt := range tests {
		t.Run(tt.severity+"_"+tt.threshold, func(t *testing.T) {
			got := resolveBlockAction(tt.severity, tt.threshold)
			if got != tt.expected {
				t.Errorf("resolveBlockAction(%q, %q) = %q, want %q",
					tt.severity, tt.threshold, got, tt.expected)
			}
		})
	}
}

func TestShouldFailOnFindings(t *testing.T) {
	tests := []struct {
		name      string
		findings  []domain.Finding
		threshold string
		expected  bool
	}{
		{
			name:      "no findings",
			findings:  nil,
			threshold: "critical",
			expected:  false,
		},
		{
			name: "critical finding with critical threshold",
			findings: []domain.Finding{
				{Severity: "critical"},
			},
			threshold: "critical",
			expected:  true,
		},
		{
			name: "high finding with critical threshold",
			findings: []domain.Finding{
				{Severity: "high"},
			},
			threshold: "critical",
			expected:  false,
		},
		{
			name: "high finding with high threshold",
			findings: []domain.Finding{
				{Severity: "high"},
			},
			threshold: "high",
			expected:  true,
		},
		{
			name: "low finding with none threshold",
			findings: []domain.Finding{
				{Severity: "low"},
			},
			threshold: "none",
			expected:  false,
		},
		{
			name: "mixed findings with medium threshold",
			findings: []domain.Finding{
				{Severity: "low"},
				{Severity: "medium"},
			},
			threshold: "medium",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrap findings in a Review to match the Result structure
			result := review.Result{
				Reviews: []domain.Review{
					{Findings: tt.findings},
				},
			}
			got := shouldFailOnFindings(result, tt.threshold)
			if got != tt.expected {
				t.Errorf("shouldFailOnFindings() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWriteGitHubOutputs(t *testing.T) {
	// Create temp files for output and summary
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output")
	summaryFile := filepath.Join(tmpDir, "summary")

	// Set environment
	envVars := []string{"GITHUB_OUTPUT", "GITHUB_STEP_SUMMARY"}
	restore := saveAndClearEnv(t, envVars)
	defer restore()
	setEnv(map[string]string{
		"GITHUB_OUTPUT":       outputFile,
		"GITHUB_STEP_SUMMARY": summaryFile,
	})

	result := review.Result{
		Reviews: []domain.Review{
			{
				Findings: []domain.Finding{
					{Severity: "critical", Description: "Critical issue"},
					{Severity: "high", Description: "High issue"},
					{Severity: "medium", Description: "Medium issue"},
					{Severity: "low", Description: "Low issue 1"},
					{Severity: "low", Description: "Low issue 2"},
				},
			},
		},
	}

	err := writeGitHubOutputs(result, nil)
	if err != nil {
		t.Fatalf("writeGitHubOutputs() error = %v", err)
	}

	// Check output file
	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	outputStr := string(output)

	expectedOutputs := []string{
		"findings-count=5",
		"critical-count=1",
		"high-count=1",
		"medium-count=1",
		"low-count=2",
	}
	for _, expected := range expectedOutputs {
		if !contains(outputStr, expected) {
			t.Errorf("output missing %q", expected)
		}
	}

	// Check summary file
	summary, err := os.ReadFile(summaryFile)
	if err != nil {
		t.Fatalf("failed to read summary file: %v", err)
	}
	summaryStr := string(summary)

	if !contains(summaryStr, "## bop Code Review") {
		t.Error("summary missing header")
	}
	if !contains(summaryStr, "| Critical | 1 |") {
		t.Error("summary missing critical count")
	}
	if !contains(summaryStr, "| **Total** | **5** |") {
		t.Error("summary missing total")
	}
}

func TestWriteGitHubOutputs_EmptyFindings(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output")

	envVars := []string{"GITHUB_OUTPUT", "GITHUB_STEP_SUMMARY"}
	restore := saveAndClearEnv(t, envVars)
	defer restore()
	setEnv(map[string]string{
		"GITHUB_OUTPUT": outputFile,
	})

	result := review.Result{} // No findings

	err := writeGitHubOutputs(result, nil)
	if err != nil {
		t.Fatalf("writeGitHubOutputs() error = %v", err)
	}

	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	outputStr := string(output)

	// Should still have findings output with empty array
	if !contains(outputStr, "findings<<BOP_EOF_") {
		t.Error("should output findings even when empty")
	}
	if !contains(outputStr, "[]") {
		t.Error("empty findings should be []")
	}
	if !contains(outputStr, "findings-count=0") {
		t.Error("findings-count should be 0")
	}
}

func TestWriteGitHubOutputs_ErrorWithNewlines(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output")

	envVars := []string{"GITHUB_OUTPUT", "GITHUB_STEP_SUMMARY"}
	restore := saveAndClearEnv(t, envVars)
	defer restore()
	setEnv(map[string]string{
		"GITHUB_OUTPUT": outputFile,
	})

	result := review.Result{}
	testErr := errors.New("error with\nnewline and\rcarriage return")

	err := writeGitHubOutputs(result, testErr)
	if err != nil {
		t.Fatalf("writeGitHubOutputs() error = %v", err)
	}

	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	outputStr := string(output)

	// Error should be sanitized - no newlines in the error= line
	if contains(outputStr, "error=error with\n") {
		t.Error("error output should not contain raw newlines")
	}
	if !contains(outputStr, "error=error with newline and carriage return") {
		t.Error("error should be sanitized with spaces replacing newlines")
	}
}

func TestWriteGitHubOutputs_UniqueDelimiter(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output")

	envVars := []string{"GITHUB_OUTPUT", "GITHUB_STEP_SUMMARY"}
	restore := saveAndClearEnv(t, envVars)
	defer restore()
	setEnv(map[string]string{
		"GITHUB_OUTPUT": outputFile,
	})

	result := review.Result{
		Reviews: []domain.Review{
			{
				Findings: []domain.Finding{
					{Severity: "high", Description: "Test finding with EOF in text"},
				},
			},
		},
	}

	err := writeGitHubOutputs(result, nil)
	if err != nil {
		t.Fatalf("writeGitHubOutputs() error = %v", err)
	}

	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	outputStr := string(output)

	// Should use a unique delimiter, not hardcoded "EOF"
	if contains(outputStr, "findings<<EOF\n") {
		t.Error("should use unique delimiter, not hardcoded EOF")
	}
	if !contains(outputStr, "findings<<BOP_EOF_") {
		t.Error("should use BOP_EOF_ prefix for delimiter")
	}
}

func TestGenerateDelimiter(t *testing.T) {
	// Generate multiple delimiters and ensure they're unique
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		d, err := generateDelimiter()
		if err != nil {
			t.Fatalf("generateDelimiter() error = %v", err)
		}
		if seen[d] {
			t.Errorf("generated duplicate delimiter: %s", d)
		}
		seen[d] = true

		if !contains(d, "BOP_EOF_") {
			t.Errorf("delimiter should start with BOP_EOF_, got: %s", d)
		}
	}
}

func TestTruncateUTF8Safe(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		expected string
	}{
		{
			name:     "no truncation needed",
			input:    "hello",
			maxBytes: 10,
			expected: "hello",
		},
		{
			name:     "exact fit",
			input:    "hello",
			maxBytes: 5,
			expected: "hello",
		},
		{
			name:     "simple ASCII truncation",
			input:    "hello world",
			maxBytes: 5,
			expected: "hello",
		},
		{
			name:     "multi-byte char not split",
			input:    "hello 世界", // 世 is 3 bytes, 界 is 3 bytes
			maxBytes: 8,          // "hello " (6) + partial 世 would be invalid
			expected: "hello ",   // Should stop before 世
		},
		{
			name:     "emoji not split",
			input:    "hi 👋 there",
			maxBytes: 5, // "hi " (3) + partial emoji would be invalid
			expected: "hi ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateUTF8Safe(tt.input, tt.maxBytes)
			if got != tt.expected {
				t.Errorf("truncateUTF8Safe(%q, %d) = %q, want %q", tt.input, tt.maxBytes, got, tt.expected)
			}
		})
	}
}

func TestParseGitHubContext(t *testing.T) {
	// Save and restore environment
	envVars := []string{
		"GITHUB_REPOSITORY",
		"GITHUB_HEAD_REF",
		"GITHUB_PR_NUMBER",
		"GITHUB_PR_SHA",
		"GITHUB_SHA",
		"GITHUB_WORKSPACE",
	}
	restore := saveAndClearEnv(t, envVars)
	defer restore()

	t.Run("valid context", func(t *testing.T) {
		setEnv(map[string]string{
			"GITHUB_REPOSITORY": "owner/repo",
			"GITHUB_HEAD_REF":   "feature-branch",
			"GITHUB_PR_NUMBER":  "42",
			"GITHUB_PR_SHA":     "abc123",
			"GITHUB_WORKSPACE":  "/workspace",
		})

		ctx, err := parseGitHubContext()
		if err != nil {
			t.Fatalf("parseGitHubContext() error = %v", err)
		}

		if ctx.Owner != "owner" {
			t.Errorf("Owner = %q, want %q", ctx.Owner, "owner")
		}
		if ctx.Repo != "repo" {
			t.Errorf("Repo = %q, want %q", ctx.Repo, "repo")
		}
		if ctx.HeadRef != "feature-branch" {
			t.Errorf("HeadRef = %q, want %q", ctx.HeadRef, "feature-branch")
		}
		if ctx.PRNumber != 42 {
			t.Errorf("PRNumber = %d, want %d", ctx.PRNumber, 42)
		}
		if ctx.PRSHA != "abc123" {
			t.Errorf("PRSHA = %q, want %q", ctx.PRSHA, "abc123")
		}
	})

	t.Run("missing repository", func(t *testing.T) {
		clearEnv(envVars)

		_, err := parseGitHubContext()
		if err == nil {
			t.Error("expected error for missing GITHUB_REPOSITORY")
		}
	})

	t.Run("fallback to GITHUB_SHA", func(t *testing.T) {
		clearEnv(envVars)
		setEnv(map[string]string{
			"GITHUB_REPOSITORY": "owner/repo",
			"GITHUB_HEAD_REF":   "branch",
			"GITHUB_PR_NUMBER":  "1",
			"GITHUB_SHA":        "fallback-sha",
		})

		ctx, err := parseGitHubContext()
		if err != nil {
			t.Fatalf("parseGitHubContext() error = %v", err)
		}
		if ctx.PRSHA != "fallback-sha" {
			t.Errorf("PRSHA = %q, want %q", ctx.PRSHA, "fallback-sha")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// saveAndClearEnv saves current environment values and clears them.
// Returns a function to restore the original values.
func saveAndClearEnv(t *testing.T, keys []string) func() {
	t.Helper()
	orig := make(map[string]string)
	for _, k := range keys {
		orig[k] = os.Getenv(k)
	}
	clearEnv(keys)
	return func() {
		for k, v := range orig {
			if v != "" {
				_ = os.Setenv(k, v)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}
}

// clearEnv unsets the specified environment variables.
func clearEnv(keys []string) {
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
}

// setEnv sets environment variables from a map.
func setEnv(vars map[string]string) {
	for k, v := range vars {
		_ = os.Setenv(k, v)
	}
}
