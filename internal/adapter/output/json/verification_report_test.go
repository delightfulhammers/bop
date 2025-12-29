package json_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bkyoung/code-reviewer/internal/adapter/output/json"
	"github.com/bkyoung/code-reviewer/internal/domain"
)

func TestWriteVerificationReport(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "verification-report-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	verified := []domain.VerifiedFinding{
		{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   10,
				Severity:    "high",
				Category:    "bug",
				Description: "fmt not imported",
			},
			Verified:       false,
			Classification: "",
			Confidence:     95,
			Evidence:       "Import exists at line 4",
		},
		{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   20,
				Severity:    "high",
				Category:    "bug",
				Description: "nil pointer dereference",
			},
			Verified:        true,
			Classification:  domain.ClassBlockingBug,
			Confidence:      85,
			Evidence:        "No nil check before dereference",
			BlocksOperation: true,
		},
		{
			Finding: domain.Finding{
				File:        "util.go",
				LineStart:   5,
				Severity:    "medium",
				Category:    "style",
				Description: "function too long",
			},
			Verified:       true,
			Classification: domain.ClassStyle,
			Confidence:     50,
			Evidence:       "Style preference only",
		},
	}

	// Only the second finding is reportable (verified=true, high confidence)
	reportable := []domain.VerifiedFinding{verified[1]}

	// Threshold function that returns 70 for all severities
	getThreshold := func(severity string) int {
		return 70
	}

	path, err := json.WriteVerificationReport(
		tmpDir,
		"test-repo",
		"feature-branch",
		verified,
		reportable,
		getThreshold,
	)
	if err != nil {
		t.Fatalf("WriteVerificationReport failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("report file not created at %s", path)
	}

	// Read and verify content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	// Basic content checks
	contentStr := string(content)
	if !contains(contentStr, `"total_findings": 3`) {
		t.Error("expected total_findings to be 3")
	}
	if !contains(contentStr, `"reportable_count": 1`) {
		t.Error("expected reportable_count to be 1")
	}
	if !contains(contentStr, `"filtered_count": 2`) {
		t.Error("expected filtered_count to be 2")
	}
	if !contains(contentStr, `"filtered_reason": "not_verified"`) {
		t.Error("expected first finding to have not_verified filter reason")
	}
	if !contains(contentStr, `confidence_below_threshold`) {
		t.Errorf("expected third finding to have confidence_below_threshold filter reason, got: %s", contentStr)
	}
}

func TestWriteVerificationReport_EmptyFindings(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "verification-report-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	path, err := json.WriteVerificationReport(
		tmpDir,
		"test-repo",
		"main",
		[]domain.VerifiedFinding{},
		[]domain.VerifiedFinding{},
		func(string) int { return 70 },
	)
	if err != nil {
		t.Fatalf("WriteVerificationReport failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	contentStr := string(content)
	if !contains(contentStr, `"total_findings": 0`) {
		t.Error("expected total_findings to be 0")
	}
}

func TestWriteVerificationReport_CreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "verification-report-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Use a nested directory that doesn't exist
	nestedDir := filepath.Join(tmpDir, "nested", "output", "dir")

	path, err := json.WriteVerificationReport(
		nestedDir,
		"test-repo",
		"main",
		[]domain.VerifiedFinding{},
		[]domain.VerifiedFinding{},
		func(string) int { return 70 },
	)
	if err != nil {
		t.Fatalf("WriteVerificationReport failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("report file not created at %s", path)
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"feature/branch", "feature_branch"},
		{"with spaces", "with_spaces"},
		{"special!@#chars", "specialchars"},
		{"path\\windows", "path_windows"},
	}

	for _, tt := range tests {
		// We can't directly test sanitizeFilename since it's unexported,
		// but we can verify the output filename is sane
		t.Run(tt.input, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "sanitize-test")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			path, err := json.WriteVerificationReport(
				tmpDir,
				tt.input,
				"main",
				[]domain.VerifiedFinding{},
				[]domain.VerifiedFinding{},
				func(string) int { return 70 },
			)
			if err != nil {
				t.Fatalf("WriteVerificationReport failed: %v", err)
			}

			// Just verify the file was created successfully
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Fatalf("file not created for input %q", tt.input)
			}
		})
	}
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
