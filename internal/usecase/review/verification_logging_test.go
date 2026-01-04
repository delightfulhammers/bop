package review

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
)

func TestLogVerificationDetails(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	originalOutput := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(originalOutput) // Restore original output

	verified := []domain.VerifiedFinding{
		{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   10,
				Severity:    "high",
				Description: "fmt not imported but file uses fmt.Println",
			},
			Verified:       false,
			Classification: "",
			Confidence:     95,
			Evidence:       "Import exists at line 4, this is a false positive",
		},
		{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   20,
				Severity:    "high",
				Description: "nil pointer dereference possible",
			},
			Verified:        true,
			Classification:  domain.ClassBlockingBug,
			Confidence:      85,
			Evidence:        "No nil check before dereference at line 20",
			BlocksOperation: true,
		},
		{
			Finding: domain.Finding{
				File:        "util.go",
				LineStart:   5,
				Severity:    "medium",
				Description: "function is too long",
			},
			Verified:       true,
			Classification: domain.ClassStyle,
			Confidence:     50,
			Evidence:       "Style preference, not a real issue",
		},
	}

	// Only second finding is reportable
	reportable := []domain.VerifiedFinding{verified[1]}

	settings := VerificationSettings{
		ConfidenceDefault: 70,
		ConfidenceHigh:    60,
		ConfidenceMedium:  70,
	}

	logVerificationDetails(context.Background(), verified, reportable, settings, nil)

	output := buf.String()

	// Verify header
	if !strings.Contains(output, "=== VERIFICATION REPORT ===") {
		t.Error("expected verification report header")
	}

	// Verify counts
	if !strings.Contains(output, "Total findings: 3") {
		t.Error("expected total findings count")
	}
	if !strings.Contains(output, "Reportable: 1") {
		t.Error("expected reportable count")
	}
	if !strings.Contains(output, "Filtered: 2") {
		t.Error("expected filtered count")
	}

	// Verify first finding (not verified - should be filtered)
	if !strings.Contains(output, "FILTERED") {
		t.Error("expected FILTERED status for non-verified finding")
	}
	if !strings.Contains(output, "NOT_VERIFIED") {
		t.Error("expected NOT_VERIFIED filter reason")
	}

	// Verify second finding (verified and passes threshold - should pass)
	if !strings.Contains(output, "PASS") {
		t.Error("expected PASS status for reportable finding")
	}

	// Verify third finding (verified but below threshold - should be filtered)
	if !strings.Contains(output, "CONFIDENCE_BELOW_THRESHOLD") {
		t.Error("expected CONFIDENCE_BELOW_THRESHOLD filter reason")
	}

	// Verify evidence is logged
	if !strings.Contains(output, "Evidence:") {
		t.Error("expected evidence to be logged")
	}
}

func TestLogVerificationDetails_EmptyFindings(t *testing.T) {
	var buf bytes.Buffer
	originalOutput := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(originalOutput)

	logVerificationDetails(
		context.Background(),
		[]domain.VerifiedFinding{},
		[]domain.VerifiedFinding{},
		VerificationSettings{},
		nil,
	)

	output := buf.String()

	if !strings.Contains(output, "Total findings: 0") {
		t.Error("expected total findings to be 0")
	}
	if !strings.Contains(output, "Reportable: 0") {
		t.Error("expected reportable to be 0")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is longer than ten", 10, "this is lo"},
		{"", 5, ""},
		{"test", 0, ""},
	}

	for _, tt := range tests {
		result := truncateString(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateString(%q, %d) = %q, want %q",
				tt.input, tt.maxLen, result, tt.expected)
		}
	}
}
