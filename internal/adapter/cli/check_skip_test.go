package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/cli"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// stubBranchReviewer is a minimal stub for tests that don't need branch review functionality.
type stubBranchReviewer struct{}

func (s *stubBranchReviewer) ReviewBranch(ctx context.Context, req review.BranchRequest) (review.Result, error) {
	return review.Result{}, nil
}

func (s *stubBranchReviewer) CurrentBranch(ctx context.Context) (string, error) {
	return "", errors.New("not implemented")
}

func TestCheckSkipCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedOutput string
		expectSkip     bool // true = skip (exit 0), false = review (exit 1)
	}{
		{
			name:           "skip from commit message",
			args:           []string{"check-skip", "--commit-message", "feat: add feature [skip code-review]"},
			expectedOutput: "skip: commit message\n",
			expectSkip:     true,
		},
		{
			name:           "skip from PR title",
			args:           []string{"check-skip", "--pr-title", "WIP: Draft [skip code-review]"},
			expectedOutput: "skip: PR title\n",
			expectSkip:     true,
		},
		{
			name:           "skip from PR description",
			args:           []string{"check-skip", "--pr-description", "## WIP\n\n[skip code-review]\n\nNot ready"},
			expectedOutput: "skip: PR description\n",
			expectSkip:     true,
		},
		{
			name:           "no skip",
			args:           []string{"check-skip", "--commit-message", "feat: add feature"},
			expectedOutput: "review: no skip trigger found\n",
			expectSkip:     false,
		},
		{
			name:           "no skip with multiple commits",
			args:           []string{"check-skip", "--commit-message", "feat: initial", "--commit-message", "fix: follow up"},
			expectedOutput: "review: no skip trigger found\n",
			expectSkip:     false,
		},
		{
			name:           "skip with multiple commits (one has trigger)",
			args:           []string{"check-skip", "--commit-message", "feat: initial", "--commit-message", "[skip code-review]"},
			expectedOutput: "skip: commit message\n",
			expectSkip:     true,
		},
		{
			name:           "skip with hyphen format",
			args:           []string{"check-skip", "--commit-message", "[skip-code-review]"},
			expectedOutput: "skip: commit message\n",
			expectSkip:     true,
		},
		{
			name:           "skip with uppercase",
			args:           []string{"check-skip", "--commit-message", "[SKIP CODE-REVIEW]"},
			expectedOutput: "skip: commit message\n",
			expectSkip:     true,
		},
		{
			name:           "commit takes precedence over PR",
			args:           []string{"check-skip", "--commit-message", "[skip code-review]", "--pr-description", "[skip code-review]"},
			expectedOutput: "skip: commit message\n",
			expectSkip:     true,
		},
		{
			name:           "no inputs",
			args:           []string{"check-skip"},
			expectedOutput: "review: no skip trigger found\n",
			expectSkip:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer

			deps := cli.Dependencies{
				BranchReviewer: &stubBranchReviewer{},
				Args: cli.Arguments{
					OutWriter: &stdout,
					ErrWriter: io.Discard,
				},
			}

			cmd := cli.NewRootCommand(deps)
			cmd.SetArgs(tt.args)

			err := cmd.ExecuteContext(context.Background())

			// Check error vs success
			if tt.expectSkip {
				// Should skip = no error (exit 0)
				if err != nil {
					t.Errorf("expected no error (skip), got: %v", err)
				}
			} else {
				// Should review = ErrShouldReview (exit 1)
				if !errors.Is(err, cli.ErrShouldReview) {
					t.Errorf("expected ErrShouldReview, got: %v", err)
				}
			}

			// Check output
			gotOutput := stdout.String()
			if gotOutput != tt.expectedOutput {
				t.Errorf("output = %q, want %q", gotOutput, tt.expectedOutput)
			}
		})
	}
}
