package skip_test

import (
	"testing"

	"github.com/delightfulhammers/bop/internal/usecase/skip"
)

func TestContainsSkipTrigger(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		// Bracket format with space
		{
			name:     "bracket format with space",
			text:     "[skip code-review]",
			expected: true,
		},
		{
			name:     "bracket format with space in commit message",
			text:     "fix: update README [skip code-review]",
			expected: true,
		},
		{
			name:     "bracket format with space at beginning",
			text:     "[skip code-review] WIP: initial commit",
			expected: true,
		},
		// Bracket format with hyphen
		{
			name:     "bracket format with hyphen",
			text:     "[skip-code-review]",
			expected: true,
		},
		{
			name:     "bracket format with hyphen in commit message",
			text:     "chore: documentation [skip-code-review]",
			expected: true,
		},
		// Case insensitivity
		{
			name:     "uppercase",
			text:     "[SKIP CODE-REVIEW]",
			expected: true,
		},
		{
			name:     "mixed case",
			text:     "[Skip Code-Review]",
			expected: true,
		},
		{
			name:     "uppercase hyphen format",
			text:     "[SKIP-CODE-REVIEW]",
			expected: true,
		},
		// Multiline text (PR descriptions)
		{
			name:     "multiline with trigger in middle",
			text:     "## Description\n\nThis is a WIP PR.\n\n[skip code-review]\n\n## Changes",
			expected: true,
		},
		{
			name:     "multiline with trigger at end",
			text:     "Some description\n\n[skip-code-review]",
			expected: true,
		},
		// Negative cases
		{
			name:     "no trigger",
			text:     "fix: update tests",
			expected: false,
		},
		{
			name:     "empty string",
			text:     "",
			expected: false,
		},
		{
			name:     "partial match - missing brackets",
			text:     "skip code-review",
			expected: false,
		},
		{
			name:     "partial match - only opening bracket",
			text:     "[skip code-review",
			expected: false,
		},
		{
			name:     "partial match - only closing bracket",
			text:     "skip code-review]",
			expected: false,
		},
		{
			name:     "similar but different text",
			text:     "[skip-ci]",
			expected: false,
		},
		{
			name:     "typo in trigger",
			text:     "[skip codereview]",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := skip.ContainsSkipTrigger(tt.text)
			if result != tt.expected {
				t.Errorf("ContainsSkipTrigger(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestCheckRequest(t *testing.T) {
	tests := []struct {
		name           string
		request        skip.CheckRequest
		expectedSkip   bool
		expectedReason string
	}{
		// Skip from commit message
		{
			name: "skip from commit message",
			request: skip.CheckRequest{
				CommitMessages: []string{
					"feat: add new feature [skip code-review]",
				},
			},
			expectedSkip:   true,
			expectedReason: "commit message",
		},
		{
			name: "skip from later commit message",
			request: skip.CheckRequest{
				CommitMessages: []string{
					"feat: initial work",
					"fix: follow up [skip code-review]",
				},
			},
			expectedSkip:   true,
			expectedReason: "commit message",
		},
		// Skip from PR description
		{
			name: "skip from PR description",
			request: skip.CheckRequest{
				PRDescription: "## WIP\n\n[skip code-review]\n\nNot ready yet.",
			},
			expectedSkip:   true,
			expectedReason: "PR description",
		},
		// Skip from either source
		{
			name: "skip from both - commit takes precedence",
			request: skip.CheckRequest{
				CommitMessages: []string{"[skip code-review]"},
				PRDescription:  "[skip code-review]",
			},
			expectedSkip:   true,
			expectedReason: "commit message",
		},
		// No skip
		{
			name: "no skip - normal commit and PR",
			request: skip.CheckRequest{
				CommitMessages: []string{"feat: add feature"},
				PRDescription:  "This is a normal PR",
			},
			expectedSkip:   false,
			expectedReason: "",
		},
		{
			name: "no skip - empty request",
			request: skip.CheckRequest{
				CommitMessages: nil,
				PRDescription:  "",
			},
			expectedSkip:   false,
			expectedReason: "",
		},
		// PR title
		{
			name: "skip from PR title",
			request: skip.CheckRequest{
				PRTitle: "WIP: Draft feature [skip code-review]",
			},
			expectedSkip:   true,
			expectedReason: "PR title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := skip.Check(tt.request)
			if result.ShouldSkip != tt.expectedSkip {
				t.Errorf("Check() ShouldSkip = %v, want %v", result.ShouldSkip, tt.expectedSkip)
			}
			if result.Reason != tt.expectedReason {
				t.Errorf("Check() Reason = %q, want %q", result.Reason, tt.expectedReason)
			}
		})
	}
}
