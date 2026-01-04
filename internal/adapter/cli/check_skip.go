package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/usecase/skip"
)

// ErrShouldReview is returned when no skip trigger is found,
// indicating the review should proceed. Use this as a sentinel
// error in the GitHub Action workflow.
var ErrShouldReview = errors.New("should review")

// checkSkipCommand creates the check-skip subcommand.
// This command checks the head commit message and PR metadata for skip triggers.
//
// Exit codes:
//   - 0: Skip trigger found, review should be skipped
//   - 1: No skip trigger, review should proceed
func checkSkipCommand() *cobra.Command {
	var commitMessages []string
	var prTitle string
	var prDescription string

	cmd := &cobra.Command{
		Use:   "check-skip",
		Short: "Check if code review should be skipped",
		Long: `Check the head commit message and PR metadata for skip triggers.

Supported skip trigger patterns:
  [skip code-review]
  [skip-code-review]

Patterns are case-insensitive and can appear anywhere in the text.

Exit codes:
  0 - Skip trigger found, review should be skipped
  1 - No skip trigger, review should proceed

Example usage in GitHub Actions:
  if ./cr check-skip --commit-message "${{ github.event.head_commit.message }}"; then
    echo "Skipping code review"
    exit 0
  fi`,
		RunE: func(cmd *cobra.Command, args []string) error {
			req := skip.CheckRequest{
				CommitMessages: commitMessages,
				PRTitle:        prTitle,
				PRDescription:  prDescription,
			}

			result := skip.Check(req)

			if result.ShouldSkip {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "skip: %s\n", result.Reason)
				return nil // Exit 0
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "review: no skip trigger found")
			return ErrShouldReview // Exit 1
		},
	}

	cmd.Flags().StringArrayVar(&commitMessages, "commit-message", nil, "Commit message(s) to check (can be repeated)")
	cmd.Flags().StringVar(&prTitle, "pr-title", "", "PR title to check")
	cmd.Flags().StringVar(&prDescription, "pr-description", "", "PR description/body to check")

	return cmd
}
