package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// prCommand creates the 'review pr' subcommand for reviewing GitHub PRs remotely.
// This allows reviewing any PR without needing a local clone.
func prCommand(prReviewer PRReviewer, authDeps AuthDependencies, defaultOutput, defaultInstructions string, defaultActions DefaultReviewActions, defaultVerification DefaultVerification, defaultPostOutOfDiff bool) *cobra.Command {
	var outputDir string
	var customInstructions string
	var noArchitecture bool
	var noAutoContext bool

	// GitHub posting flag (off by default for PR review)
	var post bool

	// Review action override flags
	var actionCritical string
	var actionHigh string
	var actionMedium string
	var actionLow string
	var actionClean string
	var actionNonBlocking string
	var blockThreshold string
	var alwaysBlockCategories []string

	// Verification flags
	var verify bool
	var noVerify bool
	var verificationDepth string
	var verificationCostCeiling float64
	var confidenceDefault int
	var confidenceCritical int
	var confidenceHigh int
	var confidenceMedium int
	var confidenceLow int

	// Phase 3.2: Reviewer Personas
	var reviewers []string

	cmd := &cobra.Command{
		Use:   "pr <identifier>",
		Short: "Review a GitHub pull request remotely",
		Long: `Review a GitHub pull request without needing a local clone.

The PR can be specified as:
  - owner/repo#123
  - https://github.com/owner/repo/pull/123
  - github.mycompany.com/owner/repo/pull/123 (GHE)

By default, the review is written to local files only. Use --post to upload
findings to the PR as inline comments.

Examples:
  bop review pr delightfulhammers/bop#172
  bop review pr https://github.com/owner/repo/pull/123 --reviewers security
  bop review pr owner/repo#1 --post --output ./review-output`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Auth check for platform mode
			if authDeps.TokenStore != nil {
				checker, err := authDeps.RequireAuth()
				if err != nil {
					return err
				}
				if checker != nil && !checker.CanReviewCode() {
					return fmt.Errorf("code review not available on your plan")
				}
			}

			identifier := args[0]
			ctx := cmd.Context()

			// Parse PR identifier
			owner, repo, prNumber, err := review.ParsePRIdentifier(identifier)
			if err != nil {
				return err
			}

			// Use config instructions as fallback if --instructions flag not provided
			if customInstructions == "" {
				customInstructions = defaultInstructions
			}

			// Resolve review actions (same logic as branchCommand)
			cliThresholdActions, err := expandBlockThresholdCLI(cmd, blockThreshold)
			if err != nil {
				return err
			}
			resolvedActionCritical := resolveActionWithThreshold(actionCritical, cliThresholdActions.OnCritical, defaultActions.OnCritical)
			resolvedActionHigh := resolveActionWithThreshold(actionHigh, cliThresholdActions.OnHigh, defaultActions.OnHigh)
			resolvedActionMedium := resolveActionWithThreshold(actionMedium, cliThresholdActions.OnMedium, defaultActions.OnMedium)
			resolvedActionLow := resolveActionWithThreshold(actionLow, cliThresholdActions.OnLow, defaultActions.OnLow)
			resolvedActionClean := resolveAction(actionClean, defaultActions.OnClean)
			resolvedActionNonBlocking := resolveAction(actionNonBlocking, defaultActions.OnNonBlocking)
			resolvedAlwaysBlockCategories := mergeAlwaysBlockCategories(alwaysBlockCategories, defaultActions.AlwaysBlockCategories)

			// Resolve bot username for auto-dismiss
			resolvedBotUsername := "github-actions[bot]"

			// Resolve verification settings
			resolvedVerifyEnabled := resolveVerifyEnabled(cmd, verify, noVerify, defaultVerification.Enabled)
			resolvedDepth := resolveVerificationDepth(cmd, verificationDepth, defaultVerification.Depth)
			resolvedCostCeiling := resolveFloat64(cmd, "verification-cost-ceiling", verificationCostCeiling, defaultVerification.CostCeiling)
			resolvedConfDefault := resolveInt(cmd, "confidence-default", confidenceDefault, defaultVerification.ConfidenceDefault)
			resolvedConfCritical := resolveInt(cmd, "confidence-critical", confidenceCritical, defaultVerification.ConfidenceCritical)
			resolvedConfHigh := resolveInt(cmd, "confidence-high", confidenceHigh, defaultVerification.ConfidenceHigh)
			resolvedConfMedium := resolveInt(cmd, "confidence-medium", confidenceMedium, defaultVerification.ConfidenceMedium)
			resolvedConfLow := resolveInt(cmd, "confidence-low", confidenceLow, defaultVerification.ConfidenceLow)

			_, err = prReviewer.ReviewPR(ctx, review.PRRequest{
				Owner:              owner,
				Repo:               repo,
				PRNumber:           prNumber,
				OutputDir:          outputDir,
				Repository:         fmt.Sprintf("%s/%s", owner, repo),
				CustomInstructions: customInstructions,
				NoArchitecture:     noArchitecture,
				NoAutoContext:      noAutoContext,
				PostToGitHub:       post,

				// Review action configuration
				ActionOnCritical:        resolvedActionCritical,
				ActionOnHigh:            resolvedActionHigh,
				ActionOnMedium:          resolvedActionMedium,
				ActionOnLow:             resolvedActionLow,
				ActionOnClean:           resolvedActionClean,
				ActionOnNonBlocking:     resolvedActionNonBlocking,
				AlwaysBlockCategories:   resolvedAlwaysBlockCategories,
				BotUsername:             resolvedBotUsername,
				PostOutOfDiffAsComments: defaultPostOutOfDiff,

				// Verification settings
				SkipVerification: !resolvedVerifyEnabled,
				VerificationConfig: review.VerificationSettings{
					Depth:              resolvedDepth,
					CostCeiling:        resolvedCostCeiling,
					ConfidenceDefault:  resolvedConfDefault,
					ConfidenceCritical: resolvedConfCritical,
					ConfidenceHigh:     resolvedConfHigh,
					ConfidenceMedium:   resolvedConfMedium,
					ConfidenceLow:      resolvedConfLow,
				},

				Reviewers: reviewers,
			})
			return err
		},
	}

	// Output configuration
	if defaultOutput == "" {
		defaultOutput = "out"
	}
	cmd.Flags().StringVar(&outputDir, "output", defaultOutput, "Directory to write review artifacts")
	cmd.Flags().StringVar(&customInstructions, "instructions", "", "Custom instructions to include in review prompts")
	cmd.Flags().BoolVar(&noArchitecture, "no-architecture", false, "Skip loading ARCHITECTURE.md from remote")
	cmd.Flags().BoolVar(&noAutoContext, "no-auto-context", false, "Disable automatic context gathering")

	// GitHub posting flag (simpler than branch command - just --post)
	cmd.Flags().BoolVar(&post, "post", false, "Post review findings to GitHub PR")

	// Review action configuration flags
	cmd.Flags().StringVar(&actionCritical, "action-critical", "", "Review action for critical severity (approve, comment, request_changes)")
	cmd.Flags().StringVar(&actionHigh, "action-high", "", "Review action for high severity")
	cmd.Flags().StringVar(&actionMedium, "action-medium", "", "Review action for medium severity")
	cmd.Flags().StringVar(&actionLow, "action-low", "", "Review action for low severity")
	cmd.Flags().StringVar(&actionClean, "action-clean", "", "Review action when no findings")
	cmd.Flags().StringVar(&actionNonBlocking, "action-non-blocking", "", "Review action when findings exist but none block")
	cmd.Flags().StringVar(&blockThreshold, "block-threshold", "", "Minimum severity to trigger REQUEST_CHANGES (critical, high, medium, low, none)")
	cmd.Flags().StringSliceVar(&alwaysBlockCategories, "always-block-category", []string{}, "Categories that always trigger REQUEST_CHANGES (repeatable)")

	// Verification flags
	cmd.Flags().BoolVar(&verify, "verify", false, "Enable agent-based verification of findings")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "Skip agent-based verification")
	cmd.Flags().StringVar(&verificationDepth, "verification-depth", "", "Verification depth: minimal, medium, or thorough")
	cmd.Flags().Float64Var(&verificationCostCeiling, "verification-cost-ceiling", 0, "Max cost in dollars for verification")
	cmd.Flags().IntVar(&confidenceDefault, "confidence-default", 0, "Default confidence threshold")
	cmd.Flags().IntVar(&confidenceCritical, "confidence-critical", 0, "Confidence threshold for critical findings")
	cmd.Flags().IntVar(&confidenceHigh, "confidence-high", 0, "Confidence threshold for high severity")
	cmd.Flags().IntVar(&confidenceMedium, "confidence-medium", 0, "Confidence threshold for medium severity")
	cmd.Flags().IntVar(&confidenceLow, "confidence-low", 0, "Confidence threshold for low severity")

	// Phase 3.2: Reviewer Personas
	cmd.Flags().StringSliceVar(&reviewers, "reviewers", []string{}, "Reviewers to use (comma-separated, overrides config default)")

	// Mark some flags as hidden (used mainly for consistency with branch command)
	for _, flag := range []string{
		"action-critical", "action-high", "action-medium", "action-low",
		"action-clean", "action-non-blocking", "block-threshold", "always-block-category",
		"verification-cost-ceiling", "confidence-default",
		"confidence-critical", "confidence-high", "confidence-medium", "confidence-low",
	} {
		if f := cmd.Flags().Lookup(flag); f != nil {
			_ = cmd.Flags().MarkHidden(flag)
		}
	}

	return cmd
}
