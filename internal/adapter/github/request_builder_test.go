package github_test

import (
	"testing"

	"github.com/bkyoung/code-reviewer/internal/adapter/github"
	"github.com/bkyoung/code-reviewer/internal/diff"
	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildReviewComments_OnlyIncludesInDiffFindings(t *testing.T) {
	findings := []github.PositionedFinding{
		{
			Finding:      makeFinding("file1.go", 10, "high", "Issue 1"),
			DiffPosition: diff.IntPtr(5), // In diff
		},
		{
			Finding:      makeFinding("file2.go", 20, "medium", "Issue 2"),
			DiffPosition: nil, // NOT in diff - should be excluded
		},
		{
			Finding:      makeFinding("file3.go", 30, "low", "Issue 3"),
			DiffPosition: diff.IntPtr(15), // In diff
		},
	}

	comments := github.BuildReviewComments(findings)

	require.Len(t, comments, 2)
	assert.Equal(t, "file1.go", comments[0].Path)
	assert.Equal(t, 5, comments[0].Position)
	assert.Equal(t, "file3.go", comments[1].Path)
	assert.Equal(t, 15, comments[1].Position)
}

func TestBuildReviewComments_Empty(t *testing.T) {
	comments := github.BuildReviewComments([]github.PositionedFinding{})
	assert.Empty(t, comments)
}

func TestBuildReviewComments_AllOutOfDiff(t *testing.T) {
	findings := []github.PositionedFinding{
		{
			Finding:      makeFinding("file1.go", 10, "high", "Issue 1"),
			DiffPosition: nil,
		},
		{
			Finding:      makeFinding("file2.go", 20, "medium", "Issue 2"),
			DiffPosition: nil,
		},
	}

	comments := github.BuildReviewComments(findings)
	assert.Empty(t, comments)
}

func TestFormatFindingComment(t *testing.T) {
	finding := makeFinding("main.go", 42, "high", "SQL injection vulnerability")
	finding.Category = "security"
	finding.Suggestion = "Use parameterized queries instead"

	comment := github.FormatFindingComment(finding)

	assert.Contains(t, comment, "**Severity:** high")
	assert.Contains(t, comment, "**Category:** security")
	assert.Contains(t, comment, "SQL injection vulnerability")
	assert.Contains(t, comment, "Use parameterized queries instead")
}

func TestFormatFindingComment_NoSuggestion(t *testing.T) {
	finding := makeFinding("main.go", 42, "low", "Minor style issue")
	finding.Suggestion = ""

	comment := github.FormatFindingComment(finding)

	assert.Contains(t, comment, "Minor style issue")
	assert.NotContains(t, comment, "**Suggestion:**")
}

func TestFormatFindingComment_LineRange(t *testing.T) {
	finding := domain.Finding{
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15, // Multi-line finding
		Severity:    "medium",
		Description: "Long function",
	}

	comment := github.FormatFindingComment(finding)

	assert.Contains(t, comment, "Lines 10-15")
}

func TestFormatFindingComment_SingleLine(t *testing.T) {
	finding := domain.Finding{
		File:        "main.go",
		LineStart:   42,
		LineEnd:     42, // Same line
		Severity:    "medium",
		Description: "Issue on line 42",
	}

	comment := github.FormatFindingComment(finding)

	assert.Contains(t, comment, "Line 42")
	assert.NotContains(t, comment, "Lines")
}

func TestDetermineReviewEvent_RequestChangesOnHighSeverity(t *testing.T) {
	findings := []github.PositionedFinding{
		{Finding: makeFinding("a.go", 1, "low", "minor"), DiffPosition: diff.IntPtr(1)},
		{Finding: makeFinding("b.go", 2, "high", "critical bug"), DiffPosition: diff.IntPtr(2)},
	}

	event := github.DetermineReviewEvent(findings)

	assert.Equal(t, github.EventRequestChanges, event)
}

func TestDetermineReviewEvent_RequestChangesOnCriticalSeverity(t *testing.T) {
	findings := []github.PositionedFinding{
		{Finding: makeFinding("a.go", 1, "critical", "security issue"), DiffPosition: diff.IntPtr(1)},
	}

	event := github.DetermineReviewEvent(findings)

	assert.Equal(t, github.EventRequestChanges, event)
}

func TestDetermineReviewEvent_ApproveOnMediumSeverity(t *testing.T) {
	// Medium/low findings don't trigger REQUEST_CHANGES by default,
	// so the review should APPROVE (with comments attached)
	findings := []github.PositionedFinding{
		{Finding: makeFinding("a.go", 1, "medium", "code smell"), DiffPosition: diff.IntPtr(1)},
		{Finding: makeFinding("b.go", 2, "low", "minor issue"), DiffPosition: diff.IntPtr(2)},
	}

	event := github.DetermineReviewEvent(findings)

	assert.Equal(t, github.EventApprove, event)
}

func TestDetermineReviewEvent_ApproveOnNoFindings(t *testing.T) {
	event := github.DetermineReviewEvent([]github.PositionedFinding{})

	assert.Equal(t, github.EventApprove, event)
}

func TestDetermineReviewEvent_IgnoresOutOfDiffFindings(t *testing.T) {
	// High severity finding but NOT in diff - should not trigger REQUEST_CHANGES
	findings := []github.PositionedFinding{
		{Finding: makeFinding("a.go", 1, "high", "critical"), DiffPosition: nil},
		{Finding: makeFinding("b.go", 2, "low", "minor"), DiffPosition: diff.IntPtr(1)},
	}

	event := github.DetermineReviewEvent(findings)

	// Only in-diff findings count, so only low severity → APPROVE (non-blocking)
	assert.Equal(t, github.EventApprove, event)
}

func TestCountInDiffFindings(t *testing.T) {
	findings := []github.PositionedFinding{
		{Finding: makeFinding("a.go", 1, "high", "a"), DiffPosition: diff.IntPtr(1)},
		{Finding: makeFinding("b.go", 2, "low", "b"), DiffPosition: nil},
		{Finding: makeFinding("c.go", 3, "low", "c"), DiffPosition: diff.IntPtr(3)},
	}

	count := github.CountInDiffFindings(findings)

	assert.Equal(t, 2, count)
}

// Helper to create a finding for tests
func makeFinding(file string, line int, severity, description string) domain.Finding {
	return domain.Finding{
		ID:          "test-id",
		File:        file,
		LineStart:   line,
		LineEnd:     line,
		Severity:    severity,
		Category:    "test",
		Description: description,
	}
}

func TestNormalizeAction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected github.ReviewEvent
		valid    bool
	}{
		// Valid APPROVE variations
		{"uppercase APPROVE", "APPROVE", github.EventApprove, true},
		{"lowercase approve", "approve", github.EventApprove, true},
		{"mixed case Approve", "Approve", github.EventApprove, true},
		{"with whitespace", "  approve  ", github.EventApprove, true},

		// Valid REQUEST_CHANGES variations
		{"uppercase REQUEST_CHANGES", "REQUEST_CHANGES", github.EventRequestChanges, true},
		{"lowercase request_changes", "request_changes", github.EventRequestChanges, true},
		{"hyphenated request-changes", "request-changes", github.EventRequestChanges, true},
		{"mixed case Request_Changes", "Request_Changes", github.EventRequestChanges, true},

		// Valid COMMENT variations
		{"uppercase COMMENT", "COMMENT", github.EventComment, true},
		{"lowercase comment", "comment", github.EventComment, true},
		{"mixed case Comment", "Comment", github.EventComment, true},

		// Invalid values - should return EventComment and false
		{"empty string", "", github.EventComment, false},
		{"invalid value", "invalid", github.EventComment, false},
		{"typo", "approv", github.EventComment, false},
		{"only whitespace", "   ", github.EventComment, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, valid := github.NormalizeAction(tt.input)
			assert.Equal(t, tt.expected, event, "event mismatch")
			assert.Equal(t, tt.valid, valid, "valid mismatch")
		})
	}
}

func TestDetermineReviewEventWithActions(t *testing.T) {
	// Custom actions for testing - none trigger REQUEST_CHANGES
	nonBlockingActions := github.ReviewActions{
		OnCritical:    "comment", // Doesn't block
		OnHigh:        "comment", // Doesn't block
		OnMedium:      "comment", // Doesn't block
		OnLow:         "comment", // Doesn't block
		OnClean:       "comment", // Custom clean action
		OnNonBlocking: "comment", // Custom non-blocking action
	}

	// Actions where critical triggers REQUEST_CHANGES
	blockingCriticalActions := github.ReviewActions{
		OnCritical:    "request_changes", // Blocks
		OnHigh:        "comment",         // Doesn't block
		OnMedium:      "comment",
		OnLow:         "comment",
		OnNonBlocking: "approve", // When no blocking findings
	}

	tests := []struct {
		name     string
		findings []github.PositionedFinding
		actions  github.ReviewActions
		expected github.ReviewEvent
	}{
		{
			name:     "clean code with custom action",
			findings: []github.PositionedFinding{},
			actions:  nonBlockingActions,
			expected: github.EventComment, // Uses OnClean
		},
		{
			name: "non-blocking findings use OnNonBlocking",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "critical", "security issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  nonBlockingActions,
			expected: github.EventComment, // OnCritical=comment doesn't block, uses OnNonBlocking
		},
		{
			name: "blocking finding triggers REQUEST_CHANGES",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "critical", "security issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  blockingCriticalActions,
			expected: github.EventRequestChanges, // OnCritical=request_changes blocks
		},
		{
			name: "high finding with non-blocking config",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "high", "bug"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  blockingCriticalActions,
			expected: github.EventApprove, // OnHigh=comment doesn't block, uses OnNonBlocking
		},
		{
			name: "medium finding with non-blocking config",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "medium", "code smell"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  nonBlockingActions,
			expected: github.EventComment, // No blocking, uses OnNonBlocking=comment
		},
		{
			name: "low finding with non-blocking config",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "low", "minor issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  nonBlockingActions,
			expected: github.EventComment, // No blocking, uses OnNonBlocking=comment
		},
		{
			name: "any blocking finding triggers REQUEST_CHANGES",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "high", "bug"), DiffPosition: diff.IntPtr(1)},
				{Finding: makeFinding("b.go", 2, "critical", "security issue"), DiffPosition: diff.IntPtr(2)},
			},
			actions:  blockingCriticalActions,
			expected: github.EventRequestChanges, // critical blocks
		},
		{
			name: "case insensitive action values",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "critical", "issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions: github.ReviewActions{
				OnCritical: "REQUEST_CHANGES", // uppercase
			},
			expected: github.EventRequestChanges,
		},
		{
			name: "out of diff findings ignored",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "critical", "not in diff"), DiffPosition: nil},
				{Finding: makeFinding("b.go", 2, "low", "in diff"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  blockingCriticalActions,
			expected: github.EventApprove, // critical out of diff, only low in diff → uses OnNonBlocking
		},
		{
			name: "default actions - high severity blocks",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "high", "bug"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  github.ReviewActions{},     // empty config uses defaults
			expected: github.EventRequestChanges, // default OnHigh=request_changes
		},
		{
			name: "default actions - medium severity approves",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "medium", "code smell"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  github.ReviewActions{}, // empty config uses defaults
			expected: github.EventApprove,    // default OnMedium=comment doesn't block, uses default OnNonBlocking=approve
		},
		{
			name:     "clean code with empty config uses approve fallback",
			findings: []github.PositionedFinding{},
			actions:  github.ReviewActions{}, // empty config
			expected: github.EventApprove,    // default OnClean=approve
		},
		{
			name: "OnNonBlocking defaults to approve when empty",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "low", "minor"), DiffPosition: diff.IntPtr(1)},
			},
			actions: github.ReviewActions{
				OnLow:         "comment", // doesn't block
				OnNonBlocking: "",        // explicitly empty - should default to approve
			},
			expected: github.EventApprove,
		},
		{
			name: "OnNonBlocking can be set to comment",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "low", "minor"), DiffPosition: diff.IntPtr(1)},
			},
			actions: github.ReviewActions{
				OnLow:         "comment",
				OnNonBlocking: "comment", // explicitly comment - should use comment
			},
			expected: github.EventComment,
		},
		{
			name: "medium/low with blocking config triggers request_changes",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "medium", "code smell"), DiffPosition: diff.IntPtr(1)},
			},
			actions: github.ReviewActions{
				OnMedium: "request_changes", // medium configured to block
			},
			expected: github.EventRequestChanges,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := github.DetermineReviewEventWithActions(tt.findings, tt.actions)
			assert.Equal(t, tt.expected, event)
		})
	}
}

func TestFormatFindingCommentWithFingerprint(t *testing.T) {
	finding := domain.Finding{
		ID:          "test-id",
		File:        "main.go",
		LineStart:   42,
		LineEnd:     42,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection vulnerability",
		Suggestion:  "Use parameterized queries",
	}

	fingerprint := domain.FingerprintFromFinding(finding)
	comment := github.FormatFindingCommentWithFingerprint(finding, fingerprint)

	// Should contain all the normal content
	assert.Contains(t, comment, "**Severity:** high")
	assert.Contains(t, comment, "**Category:** security")
	assert.Contains(t, comment, "SQL injection vulnerability")
	assert.Contains(t, comment, "Use parameterized queries")

	// Should contain the fingerprint in an HTML comment (new compact format)
	assert.Contains(t, comment, "<!-- CR_FP:")
	assert.Contains(t, comment, string(fingerprint))
	assert.Contains(t, comment, "-->")
}

func TestExtractFingerprintFromComment(t *testing.T) {
	// Valid 32-character hex fingerprint for testing
	validFP := domain.FindingFingerprint("abc123def456abc123def456abc12345")

	tests := []struct {
		name        string
		commentBody string
		wantFP      domain.FindingFingerprint
		wantFound   bool
	}{
		{
			name:        "valid fingerprint",
			commentBody: "**Severity:** high\n\n<!-- CR_FINGERPRINT:" + string(validFP) + " -->\n",
			wantFP:      validFP,
			wantFound:   true,
		},
		{
			name:        "fingerprint at end",
			commentBody: "Some content\n<!-- CR_FINGERPRINT:0123456789abcdef0123456789abcdef -->",
			wantFP:      "0123456789abcdef0123456789abcdef",
			wantFound:   true,
		},
		{
			name:        "fingerprint in middle",
			commentBody: "Before\n<!-- CR_FINGERPRINT:fedcba9876543210fedcba9876543210 -->\nAfter",
			wantFP:      "fedcba9876543210fedcba9876543210",
			wantFound:   true,
		},
		{
			name:        "no fingerprint",
			commentBody: "Regular comment without fingerprint",
			wantFP:      "",
			wantFound:   false,
		},
		{
			name:        "partial marker only",
			commentBody: "<!-- CR_FINGERPRINT:",
			wantFP:      "",
			wantFound:   false,
		},
		{
			name:        "empty fingerprint",
			commentBody: "<!-- CR_FINGERPRINT: -->",
			wantFP:      "",
			wantFound:   false,
		},
		{
			name:        "legacy comment without fingerprint",
			commentBody: "**Severity:** high\n\n📍 Line 42\n\nSQL injection",
			wantFP:      "",
			wantFound:   false,
		},
		// Fingerprint format validation tests
		{
			name:        "too short fingerprint rejected",
			commentBody: "<!-- CR_FINGERPRINT:abc123 -->",
			wantFP:      "",
			wantFound:   false,
		},
		{
			name:        "too long fingerprint rejected",
			commentBody: "<!-- CR_FINGERPRINT:abc123def456abc123def456abc12345extra -->",
			wantFP:      "",
			wantFound:   false,
		},
		{
			name:        "uppercase fingerprint normalized to lowercase",
			commentBody: "<!-- CR_FINGERPRINT:ABC123DEF456ABC123DEF456ABC12345 -->",
			wantFP:      "abc123def456abc123def456abc12345", // normalized to lowercase
			wantFound:   true,
		},
		{
			name:        "mixed case fingerprint normalized to lowercase",
			commentBody: "<!-- CR_FINGERPRINT:AbC123dEf456AbC123dEf456AbC12345 -->",
			wantFP:      "abc123def456abc123def456abc12345", // normalized to lowercase
			wantFound:   true,
		},
		{
			name:        "invalid characters rejected - special chars",
			commentBody: "<!-- CR_FINGERPRINT:abc123def456abc123def456abc1234! -->",
			wantFP:      "",
			wantFound:   false,
		},
		{
			name:        "invalid characters rejected - injection attempt",
			commentBody: "<!-- CR_FINGERPRINT:<script>alert(1)</script>abc -->",
			wantFP:      "",
			wantFound:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp, found := github.ExtractFingerprintFromComment(tt.commentBody)
			assert.Equal(t, tt.wantFound, found, "found mismatch")
			if found {
				assert.Equal(t, tt.wantFP, fp, "fingerprint mismatch")
			}
		})
	}
}

func TestHasBlockingFindings(t *testing.T) {
	tests := []struct {
		name     string
		findings []github.PositionedFinding
		actions  github.ReviewActions
		expected bool
	}{
		{
			name:     "empty findings returns false",
			findings: []github.PositionedFinding{},
			actions:  github.ReviewActions{},
			expected: false,
		},
		{
			name: "critical severity blocks by default",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "critical", "issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  github.ReviewActions{},
			expected: true,
		},
		{
			name: "high severity blocks by default",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "high", "issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  github.ReviewActions{},
			expected: true,
		},
		{
			name: "medium severity does not block by default",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "medium", "issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  github.ReviewActions{},
			expected: false,
		},
		{
			name: "low severity does not block by default",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "low", "issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  github.ReviewActions{},
			expected: false,
		},
		{
			name: "unknown severity does not block",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "info", "issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions:  github.ReviewActions{},
			expected: false,
		},
		{
			name: "out of diff findings are ignored",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "critical", "issue"), DiffPosition: nil},
			},
			actions:  github.ReviewActions{},
			expected: false,
		},
		{
			name: "critical configured to comment does not block",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "critical", "issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions: github.ReviewActions{
				OnCritical: "comment",
			},
			expected: false,
		},
		{
			name: "medium configured to request_changes blocks",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "medium", "issue"), DiffPosition: diff.IntPtr(1)},
			},
			actions: github.ReviewActions{
				OnMedium: "request_changes",
			},
			expected: true,
		},
		{
			name: "mixed severities with one blocking",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "low", "minor"), DiffPosition: diff.IntPtr(1)},
				{Finding: makeFinding("b.go", 2, "high", "bug"), DiffPosition: diff.IntPtr(2)},
			},
			actions:  github.ReviewActions{},
			expected: true,
		},
		{
			name: "invalid action string falls back to default blocking",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "high", "bug"), DiffPosition: diff.IntPtr(1)},
			},
			actions: github.ReviewActions{
				OnHigh: "req_changes", // typo - should fall back to default (blocking)
			},
			expected: true, // high blocks by default, typo shouldn't disable it
		},
		{
			name: "invalid action string on non-blocking severity stays non-blocking",
			findings: []github.PositionedFinding{
				{Finding: makeFinding("a.go", 1, "low", "minor"), DiffPosition: diff.IntPtr(1)},
			},
			actions: github.ReviewActions{
				OnLow: "invalid_action", // typo - should fall back to default (non-blocking)
			},
			expected: false, // low doesn't block by default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := github.HasBlockingFindings(tt.findings, tt.actions)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasBlockingFindings_AlwaysBlockCategories(t *testing.T) {
	// Helper to create a finding with specific category
	makeFindingWithCategory := func(file string, line int, severity, category, description string) domain.Finding {
		return domain.Finding{
			ID:          "test-id",
			File:        file,
			LineStart:   line,
			LineEnd:     line,
			Severity:    severity,
			Category:    category,
			Description: description,
		}
	}

	tests := []struct {
		name     string
		findings []github.PositionedFinding
		actions  github.ReviewActions
		expected bool
	}{
		{
			name: "category in always-block list triggers blocking",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "low", "security", "minor security issue"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{"security"},
			},
			expected: true, // low severity wouldn't block, but security category does
		},
		{
			name: "category not in always-block list follows severity rules",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "low", "style", "style issue"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{"security"},
			},
			expected: false, // low severity doesn't block, style not in list
		},
		{
			name: "case insensitive category matching",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "low", "SECURITY", "uppercase category"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{"security"},
			},
			expected: true, // SECURITY matches security (case-insensitive)
		},
		{
			name: "mixed case in config list",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "low", "security", "lowercase finding"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{"Security", "Performance"},
			},
			expected: true, // security matches Security (case-insensitive)
		},
		{
			name: "multiple categories in list",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "low", "performance", "perf issue"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{"security", "performance", "correctness"},
			},
			expected: true, // performance is in the list
		},
		{
			name: "empty category on finding doesn't match",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "low", "", "no category"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{"security"},
			},
			expected: false, // empty category doesn't match
		},
		{
			name: "whitespace-only category on finding doesn't match",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "low", "   ", "whitespace category"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{"security"},
			},
			expected: false, // whitespace-only category doesn't match
		},
		{
			name: "whitespace in config category list is trimmed",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "low", "security", "security issue"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{"  security  ", "  "},
			},
			expected: true, // "  security  " matches "security" after trimming
		},
		{
			name: "empty always-block list has no effect",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "low", "security", "security issue"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{},
			},
			expected: false, // no categories to block, low severity doesn't block
		},
		{
			name: "category blocking is additive with severity blocking",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "critical", "security", "critical security"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{"security"},
			},
			expected: true, // blocks via both severity AND category
		},
		{
			name: "category blocking works even when severity is set to comment",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "critical", "security", "issue"),
					DiffPosition: diff.IntPtr(1),
				},
			},
			actions: github.ReviewActions{
				OnCritical:            "comment", // would not block
				AlwaysBlockCategories: []string{"security"},
			},
			expected: true, // category override wins
		},
		{
			name: "out of diff category finding ignored",
			findings: []github.PositionedFinding{
				{
					Finding:      makeFindingWithCategory("a.go", 1, "low", "security", "out of diff"),
					DiffPosition: nil, // not in diff
				},
			},
			actions: github.ReviewActions{
				AlwaysBlockCategories: []string{"security"},
			},
			expected: false, // out of diff findings are always ignored
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := github.HasBlockingFindings(tt.findings, tt.actions)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractCommentDetails(t *testing.T) {
	tests := []struct {
		name            string
		commentBody     string
		wantNil         bool
		wantSeverity    string
		wantCategory    string
		wantLineStart   int
		wantLineEnd     int
		wantDescription string
	}{
		{
			name: "full comment with all fields",
			commentBody: `**Severity:** high | **Category:** security

📍 Line 42

SQL injection vulnerability detected in user input handling.

**Suggestion:** Use parameterized queries

<!-- CR_FINGERPRINT:abc123def456abc123def456abc12345 -->`,
			wantNil:         false,
			wantSeverity:    "high",
			wantCategory:    "security",
			wantLineStart:   42,
			wantLineEnd:     42,
			wantDescription: "SQL injection vulnerability detected in user input handling.",
		},
		{
			name: "multi-line range",
			commentBody: `**Severity:** medium | **Category:** complexity

📍 Lines 10-25

Function is too long and should be refactored.

<!-- CR_FINGERPRINT:abc123def456abc123def456abc12345 -->`,
			wantNil:         false,
			wantSeverity:    "medium",
			wantCategory:    "complexity",
			wantLineStart:   10,
			wantLineEnd:     25,
			wantDescription: "Function is too long and should be refactored.",
		},
		{
			name: "no suggestion",
			commentBody: `**Severity:** low | **Category:** style

📍 Line 5

Variable name could be more descriptive.

<!-- CR_FINGERPRINT:abc123def456abc123def456abc12345 -->`,
			wantNil:         false,
			wantSeverity:    "low",
			wantCategory:    "style",
			wantLineStart:   5,
			wantLineEnd:     5,
			wantDescription: "Variable name could be more descriptive.",
		},
		{
			name: "no category",
			commentBody: `**Severity:** critical

📍 Line 100

Critical issue found.

<!-- CR_FINGERPRINT:abc123def456abc123def456abc12345 -->`,
			wantNil:         false,
			wantSeverity:    "critical",
			wantCategory:    "",
			wantLineStart:   100,
			wantLineEnd:     100,
			wantDescription: "Critical issue found.",
		},
		{
			name:        "no fingerprint - returns nil",
			commentBody: "Just a regular comment without fingerprint",
			wantNil:     true,
		},
		{
			name: "legacy comment format",
			commentBody: `Some old format comment

without proper fingerprint markers`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := github.ExtractCommentDetails(tt.commentBody)

			if tt.wantNil {
				assert.Nil(t, details, "expected nil details")
				return
			}

			require.NotNil(t, details, "expected non-nil details")
			assert.Equal(t, tt.wantSeverity, details.Severity, "severity mismatch")
			assert.Equal(t, tt.wantCategory, details.Category, "category mismatch")
			assert.Equal(t, tt.wantLineStart, details.LineStart, "lineStart mismatch")
			assert.Equal(t, tt.wantLineEnd, details.LineEnd, "lineEnd mismatch")
			assert.Equal(t, tt.wantDescription, details.Description, "description mismatch")
		})
	}
}
