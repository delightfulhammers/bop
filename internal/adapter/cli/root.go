package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// ErrVersionRequested indicates the user requested the CLI version and no further work should be done.
var ErrVersionRequested = errors.New("version requested")

// BranchReviewer defines the dependency required to run the branch command.
type BranchReviewer interface {
	ReviewBranch(ctx context.Context, req review.BranchRequest) (review.Result, error)
	CurrentBranch(ctx context.Context) (string, error)
}

// PRReviewer defines the dependency required to run the pr command.
type PRReviewer interface {
	ReviewPR(ctx context.Context, req review.PRRequest) (review.Result, error)
}

// Arguments encapsulates IO writers injected from the host process.
type Arguments struct {
	OutWriter io.Writer
	ErrWriter io.Writer
}

// DefaultReviewActions holds default review action configuration from config.
type DefaultReviewActions struct {
	OnCritical    string
	OnHigh        string
	OnMedium      string
	OnLow         string
	OnClean       string
	OnNonBlocking string

	// BlockThreshold is syntactic sugar for setting per-severity actions.
	// Values: "critical", "high", "medium", "low", "none"
	// This is already expanded by the config layer into OnCritical/OnHigh/etc.
	// Stored here for documentation; the expanded values are used directly.
	BlockThreshold string

	// AlwaysBlockCategories lists finding categories that always trigger REQUEST_CHANGES.
	// This provides an additive override for specific categories like "security".
	AlwaysBlockCategories []string
}

// DefaultVerification holds default verification configuration from config.
type DefaultVerification struct {
	Enabled            bool
	Depth              string
	CostCeiling        float64
	ConfidenceDefault  int
	ConfidenceCritical int
	ConfidenceHigh     int
	ConfidenceMedium   int
	ConfidenceLow      int
}

// Dependencies captures the collaborators for the CLI.
type Dependencies struct {
	BranchReviewer       BranchReviewer
	PRReviewer           PRReviewer       // Optional: only required for cr review pr
	FindingsPoster       FindingsPoster   // Optional: only required for cr post
	SessionManager       SessionManager   // Optional: only required for cr sessions
	AuthDeps             AuthDependencies // Optional: only required for bop auth commands
	Args                 Arguments
	DefaultOutput        string
	DefaultRepo          string
	DefaultInstructions  string // From config review.instructions
	DefaultReviewActions DefaultReviewActions
	DefaultBotUsername   string // Bot username for auto-dismissing stale reviews
	DefaultVerification  DefaultVerification
	DefaultPostOutOfDiff bool // Post out-of-diff findings as issue comments (default: true)
	Version              string
}

// NewRootCommand constructs the root Cobra command.
func NewRootCommand(deps Dependencies) *cobra.Command {
	versionString := deps.Version
	if versionString == "" {
		versionString = "v0.0.0"
	}

	root := &cobra.Command{
		Use:   "bop",
		Short: "Multi-LLM code review CLI",
	}
	root.SilenceUsage = true
	root.SilenceErrors = true

	outWriter := deps.Args.OutWriter
	if outWriter == nil {
		outWriter = os.Stdout
	}
	errWriter := deps.Args.ErrWriter
	if errWriter == nil {
		errWriter = os.Stderr
	}
	root.SetOut(outWriter)
	root.SetErr(errWriter)

	reviewCmd := &cobra.Command{
		Use:   "review",
		Short: "Run a code review",
	}
	reviewCmd.AddCommand(branchCommand(deps.BranchReviewer, deps.AuthDeps, deps.DefaultOutput, deps.DefaultRepo, deps.DefaultInstructions, deps.DefaultReviewActions, deps.DefaultBotUsername, deps.DefaultVerification, deps.DefaultPostOutOfDiff))
	if deps.PRReviewer != nil {
		reviewCmd.AddCommand(prCommand(deps.PRReviewer, deps.AuthDeps, deps.DefaultOutput, deps.DefaultInstructions, deps.DefaultReviewActions, deps.DefaultVerification, deps.DefaultPostOutOfDiff))
	}
	root.AddCommand(reviewCmd)
	root.AddCommand(checkSkipCommand())
	if deps.FindingsPoster != nil {
		root.AddCommand(PostCommand(deps.FindingsPoster))
	}
	if deps.SessionManager != nil {
		root.AddCommand(SessionsCommand(deps.SessionManager))
	}
	// Add auth commands if auth is configured (platform mode)
	if deps.AuthDeps.TokenStore != nil {
		root.AddCommand(NewAuthCommand(deps.AuthDeps))
	}

	var showVersion bool
	root.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "Show version and exit")

	// --log-level is parsed early (before config loading) via parseLogLevelFlag() in main.go.
	// We define it here so it appears in --help output for discoverability.
	// The actual value is already applied to config by the time this command runs.
	root.PersistentFlags().String("log-level", "", "Log level: trace, debug, info, error (default from config)")

	versionHandler := func(cmd *cobra.Command, args []string) error {
		if showVersion {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), versionString)
			return ErrVersionRequested
		}
		return nil
	}
	root.PersistentPreRunE = versionHandler
	root.PreRunE = versionHandler
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if err := versionHandler(cmd, args); err != nil {
			return err
		}
		return cmd.Help()
	}

	return root
}

func branchCommand(branchReviewer BranchReviewer, authDeps AuthDependencies, defaultOutput, defaultRepo, defaultInstructions string, defaultActions DefaultReviewActions, defaultBotUsername string, defaultVerification DefaultVerification, defaultPostOutOfDiff bool) *cobra.Command {
	var baseRef string
	var targetRef string
	var outputDir string
	var repository string
	var includeUncommitted bool
	var detectTarget bool
	var customInstructions string
	var contextFiles []string
	var interactive bool
	var noPlanning bool
	var planOnly bool
	var noArchitecture bool
	var noAutoContext bool

	// GitHub integration flags
	var postGitHubReview bool
	var githubOwner string
	var githubRepo string
	var prNumber int
	var commitSHA string

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
		Use:   "branch [target]",
		Short: "Review a branch against a base reference",
		Args:  cobra.MaximumNArgs(1),
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

			if len(args) > 0 {
				targetRef = args[0]
			}
			ctx := cmd.Context()
			if targetRef == "" && detectTarget {
				resolved, err := branchReviewer.CurrentBranch(ctx)
				if err != nil {
					return fmt.Errorf("detect target branch: %w", err)
				}
				targetRef = resolved
			}
			if targetRef == "" {
				return fmt.Errorf("target branch not specified; pass as an argument, use --target, or disable --detect-target")
			}

			// Use config instructions as fallback if --instructions flag not provided
			if customInstructions == "" {
				customInstructions = defaultInstructions
			}

			// Validate GitHub flags if posting to GitHub
			if postGitHubReview {
				if githubOwner == "" || githubRepo == "" {
					return fmt.Errorf("--github-owner and --github-repo are required when --post-github-review is set")
				}
				if prNumber <= 0 {
					return fmt.Errorf("--pr-number must be a positive integer when --post-github-review is set")
				}
				if commitSHA == "" {
					return fmt.Errorf("--commit-sha is required when --post-github-review is set")
				}
			}

			// Resolve review actions: CLI flags override defaults from config
			// Priority: explicit CLI per-severity > CLI threshold > config (already has threshold expanded)
			//
			// If CLI --block-threshold is set, expand it to per-severity values,
			// then apply explicit CLI per-severity overrides on top.
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

			// Resolve always-block categories: CLI values add to config values (additive)
			resolvedAlwaysBlockCategories := mergeAlwaysBlockCategories(alwaysBlockCategories, defaultActions.AlwaysBlockCategories)

			// Resolve bot username for auto-dismiss feature
			// "none" (case-insensitive) explicitly disables auto-dismiss; empty uses default
			resolvedBotUsername := strings.TrimSpace(defaultBotUsername)
			if resolvedBotUsername == "" {
				resolvedBotUsername = "github-actions[bot]"
			} else if strings.EqualFold(resolvedBotUsername, "none") {
				// Explicit opt-out: pass empty to poster (which skips dismissal)
				resolvedBotUsername = ""
			}

			// Resolve verification settings: CLI flags override config defaults
			// --no-verify takes precedence, then --verify, then config
			resolvedVerifyEnabled := resolveVerifyEnabled(cmd, verify, noVerify, defaultVerification.Enabled)
			resolvedDepth := resolveVerificationDepth(cmd, verificationDepth, defaultVerification.Depth)
			resolvedCostCeiling := resolveFloat64(cmd, "verification-cost-ceiling", verificationCostCeiling, defaultVerification.CostCeiling)
			resolvedConfDefault := resolveInt(cmd, "confidence-default", confidenceDefault, defaultVerification.ConfidenceDefault)
			resolvedConfCritical := resolveInt(cmd, "confidence-critical", confidenceCritical, defaultVerification.ConfidenceCritical)
			resolvedConfHigh := resolveInt(cmd, "confidence-high", confidenceHigh, defaultVerification.ConfidenceHigh)
			resolvedConfMedium := resolveInt(cmd, "confidence-medium", confidenceMedium, defaultVerification.ConfidenceMedium)
			resolvedConfLow := resolveInt(cmd, "confidence-low", confidenceLow, defaultVerification.ConfidenceLow)

			_, err = branchReviewer.ReviewBranch(ctx, review.BranchRequest{
				BaseRef:                 baseRef,
				TargetRef:               targetRef,
				OutputDir:               outputDir,
				Repository:              repository,
				IncludeUncommitted:      includeUncommitted,
				CustomInstructions:      customInstructions,
				ContextFiles:            contextFiles,
				NoArchitecture:          noArchitecture,
				NoAutoContext:           noAutoContext,
				Interactive:             interactive,
				PostToGitHub:            postGitHubReview,
				GitHubOwner:             githubOwner,
				GitHubRepo:              githubRepo,
				PRNumber:                prNumber,
				CommitSHA:               commitSHA,
				ActionOnCritical:        resolvedActionCritical,
				ActionOnHigh:            resolvedActionHigh,
				ActionOnMedium:          resolvedActionMedium,
				ActionOnLow:             resolvedActionLow,
				ActionOnClean:           resolvedActionClean,
				ActionOnNonBlocking:     resolvedActionNonBlocking,
				AlwaysBlockCategories:   resolvedAlwaysBlockCategories,
				BotUsername:             resolvedBotUsername,
				PostOutOfDiffAsComments: defaultPostOutOfDiff,
				SkipVerification:        !resolvedVerifyEnabled,
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

	cmd.Flags().StringVar(&baseRef, "base", "main", "Base reference to diff against")
	cmd.Flags().StringVar(&targetRef, "target", "", "Target branch to review (overrides positional)")
	if defaultOutput == "" {
		defaultOutput = "out"
	}
	cmd.Flags().StringVar(&outputDir, "output", defaultOutput, "Directory to write review artifacts")
	cmd.Flags().StringVar(&repository, "repository", defaultRepo, "Optional repository name override")
	cmd.Flags().BoolVar(&includeUncommitted, "include-uncommitted", false, "Include uncommitted changes on the target branch")
	cmd.Flags().BoolVar(&detectTarget, "detect-target", true, "Automatically detect the checked out branch when no target is provided")
	cmd.Flags().StringVar(&customInstructions, "instructions", "", "Custom instructions to include in review prompts")
	cmd.Flags().StringSliceVar(&contextFiles, "context", []string{}, "Additional context files to include in prompts")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "Enable interactive planning mode (asks clarifying questions before review)")
	cmd.Flags().BoolVar(&noPlanning, "no-planning", false, "Skip planning in interactive mode")
	_ = cmd.Flags().MarkHidden("no-planning") // Not yet implemented
	cmd.Flags().BoolVar(&planOnly, "plan-only", false, "Dry-run showing what context would be gathered")
	_ = cmd.Flags().MarkHidden("plan-only") // Not yet implemented
	cmd.Flags().BoolVar(&noArchitecture, "no-architecture", false, "Skip loading ARCHITECTURE.md")
	cmd.Flags().BoolVar(&noAutoContext, "no-auto-context", false, "Disable automatic context gathering (design docs, relevant docs)")

	// GitHub integration flags
	cmd.Flags().BoolVar(&postGitHubReview, "post-github-review", false, "Post review as GitHub PR review with inline comments")
	cmd.Flags().StringVar(&githubOwner, "github-owner", "", "GitHub repository owner (required with --post-github-review)")
	cmd.Flags().StringVar(&githubRepo, "github-repo", "", "GitHub repository name (required with --post-github-review)")
	cmd.Flags().IntVar(&prNumber, "pr-number", 0, "Pull request number (required with --post-github-review)")
	cmd.Flags().StringVar(&commitSHA, "commit-sha", "", "Head commit SHA (required with --post-github-review)")

	// Review action configuration flags (override config file values)
	cmd.Flags().StringVar(&actionCritical, "action-critical", "", "Review action for critical severity (approve, comment, request_changes)")
	cmd.Flags().StringVar(&actionHigh, "action-high", "", "Review action for high severity (approve, comment, request_changes)")
	cmd.Flags().StringVar(&actionMedium, "action-medium", "", "Review action for medium severity (approve, comment, request_changes)")
	cmd.Flags().StringVar(&actionLow, "action-low", "", "Review action for low severity (approve, comment, request_changes)")
	cmd.Flags().StringVar(&actionClean, "action-clean", "", "Review action when no findings (approve, comment, request_changes)")
	cmd.Flags().StringVar(&actionNonBlocking, "action-non-blocking", "", "Review action when findings exist but none block (approve, comment)")
	cmd.Flags().StringVar(&blockThreshold, "block-threshold", "", "Minimum severity to trigger REQUEST_CHANGES (critical, high, medium, low, none)")
	cmd.Flags().StringSliceVar(&alwaysBlockCategories, "always-block-category", []string{}, "Categories that always trigger REQUEST_CHANGES regardless of severity (repeatable)")

	// Verification flags
	cmd.Flags().BoolVar(&verify, "verify", false, "Enable agent-based verification of findings (overrides config)")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "Skip agent-based verification of findings (faster, but may include more false positives)")
	cmd.Flags().StringVar(&verificationDepth, "verification-depth", "", "Verification depth: minimal, medium, or thorough (default from config)")
	cmd.Flags().Float64Var(&verificationCostCeiling, "verification-cost-ceiling", 0, "Max cost in dollars for verification (0 uses config default)")
	cmd.Flags().IntVar(&confidenceDefault, "confidence-default", 0, "Default confidence threshold (0 uses config default)")
	cmd.Flags().IntVar(&confidenceCritical, "confidence-critical", 0, "Confidence threshold for critical findings (0 uses config default)")
	cmd.Flags().IntVar(&confidenceHigh, "confidence-high", 0, "Confidence threshold for high severity findings (0 uses config default)")
	cmd.Flags().IntVar(&confidenceMedium, "confidence-medium", 0, "Confidence threshold for medium severity findings (0 uses config default)")
	cmd.Flags().IntVar(&confidenceLow, "confidence-low", 0, "Confidence threshold for low severity findings (0 uses config default)")

	// Phase 3.2: Reviewer Personas
	cmd.Flags().StringSliceVar(&reviewers, "reviewers", []string{}, "Reviewers to use for this review (comma-separated, overrides config default)")

	return cmd
}

// resolveAction returns the override value if non-empty, otherwise the default.
func resolveAction(override, defaultValue string) string {
	if override != "" {
		return override
	}
	return defaultValue
}

// resolveVerifyEnabled determines whether verification is enabled based on CLI flags and config.
// Priority: --no-verify (disables) > --verify (enables) > config default
func resolveVerifyEnabled(cmd *cobra.Command, verify, noVerify, configDefault bool) bool {
	// --no-verify explicitly disables verification
	if cmd.Flags().Changed("no-verify") && noVerify {
		return false
	}
	// --verify explicitly enables verification
	if cmd.Flags().Changed("verify") && verify {
		return true
	}
	// Fall back to config default
	return configDefault
}

// resolveVerificationDepth validates and resolves the verification depth setting.
// Returns the CLI value if set and valid, otherwise the config default.
// Invalid values trigger a warning and fall back to the config default.
func resolveVerificationDepth(cmd *cobra.Command, cliValue, configDefault string) string {
	if !cmd.Flags().Changed("verification-depth") || cliValue == "" {
		return configDefault
	}

	validDepths := map[string]bool{"minimal": true, "medium": true, "thorough": true}
	if validDepths[cliValue] {
		return cliValue
	}

	// Warn and fall back to config default for invalid values
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: invalid verification depth %q, using config default %q\n", cliValue, configDefault)
	return configDefault
}

// resolveFloat64 returns the CLI value if the flag was explicitly set,
// otherwise returns the config default. Validates the value is non-negative.
func resolveFloat64(cmd *cobra.Command, flagName string, cliValue, configDefault float64) float64 {
	if !cmd.Flags().Changed(flagName) {
		return configDefault
	}
	// Validate non-negative (cost ceiling, etc. should not be negative)
	if cliValue < 0 {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: negative value %.2f for --%s, using config default %.2f\n", cliValue, flagName, configDefault)
		return configDefault
	}
	return cliValue
}

// resolveInt returns the CLI value if the flag was explicitly set,
// otherwise returns the config default. For confidence flags (0-100), validates the range.
func resolveInt(cmd *cobra.Command, flagName string, cliValue, configDefault int) int {
	const safeConfidenceDefault = 75

	// Helper to validate confidence range
	isConfidenceFlag := strings.HasPrefix(flagName, "confidence-")
	validateConfidence := func(value int, source string) int {
		if value < 0 || value > 100 {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s confidence value %d out of range (0-100) for --%s, using safe default %d\n", source, value, flagName, safeConfidenceDefault)
			return safeConfidenceDefault
		}
		return value
	}

	if !cmd.Flags().Changed(flagName) {
		// Validate config default for confidence flags
		if isConfidenceFlag {
			return validateConfidence(configDefault, "config")
		}
		return configDefault
	}

	// Validate CLI value
	if isConfidenceFlag {
		return validateConfidence(cliValue, "CLI")
	}

	// Other int flags should be non-negative
	if cliValue < 0 {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: negative value %d for --%s, using config default %d\n", cliValue, flagName, configDefault)
		return configDefault
	}
	return cliValue
}

// cliThresholdActions holds expanded threshold values for CLI resolution.
type cliThresholdActions struct {
	OnCritical string
	OnHigh     string
	OnMedium   string
	OnLow      string
}

// expandBlockThresholdCLI expands a --block-threshold CLI flag to per-severity actions.
// If the flag was not set, returns an empty struct and nil error.
// If the flag was set to an invalid value, returns an error.
// This mirrors the logic in config.expandBlockThreshold but for CLI flags.
func expandBlockThresholdCLI(cmd *cobra.Command, threshold string) (cliThresholdActions, error) {
	if !cmd.Flags().Changed("block-threshold") || threshold == "" {
		return cliThresholdActions{}, nil
	}

	// Severity levels: higher = blocks first
	severityLevels := map[string]int{
		"critical": 4,
		"high":     3,
		"medium":   2,
		"low":      1,
		"none":     5, // Above all severities - nothing blocks
	}

	thresholdLevel, ok := severityLevels[strings.ToLower(threshold)]
	if !ok {
		return cliThresholdActions{}, fmt.Errorf("invalid --block-threshold value %q: must be one of: critical, high, medium, low, none", threshold)
	}

	const (
		criticalLevel = 4
		highLevel     = 3
		mediumLevel   = 2
		lowLevel      = 1
	)

	actions := cliThresholdActions{}

	// A severity blocks if its level >= threshold level
	if criticalLevel >= thresholdLevel {
		actions.OnCritical = "request_changes"
	} else {
		actions.OnCritical = "comment"
	}

	if highLevel >= thresholdLevel {
		actions.OnHigh = "request_changes"
	} else {
		actions.OnHigh = "comment"
	}

	if mediumLevel >= thresholdLevel {
		actions.OnMedium = "request_changes"
	} else {
		actions.OnMedium = "comment"
	}

	if lowLevel >= thresholdLevel {
		actions.OnLow = "request_changes"
	} else {
		actions.OnLow = "comment"
	}

	return actions, nil
}

// resolveActionWithThreshold resolves an action value with three-level precedence:
// 1. Explicit CLI per-severity flag (highest priority)
// 2. CLI threshold-expanded value
// 3. Config default (lowest priority)
func resolveActionWithThreshold(cliPerSeverity, cliFromThreshold, configDefault string) string {
	// Explicit CLI per-severity flag takes highest priority
	if cliPerSeverity != "" {
		return cliPerSeverity
	}
	// CLI threshold-expanded value takes second priority
	if cliFromThreshold != "" {
		return cliFromThreshold
	}
	// Fall back to config default
	return configDefault
}

// mergeAlwaysBlockCategories combines CLI and config categories, deduplicating.
// CLI categories add to config categories (additive, not replacement).
func mergeAlwaysBlockCategories(cliCategories, configCategories []string) []string {
	if len(cliCategories) == 0 {
		return configCategories
	}
	if len(configCategories) == 0 {
		return cliCategories
	}

	// Build set for deduplication (case-insensitive)
	seen := make(map[string]bool)
	var result []string

	// Add config categories first
	for _, cat := range configCategories {
		lower := strings.ToLower(cat)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, cat)
		}
	}

	// Add CLI categories
	for _, cat := range cliCategories {
		lower := strings.ToLower(cat)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, cat)
		}
	}

	return result
}
