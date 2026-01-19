package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// GitHubActionDeps holds dependencies for the github-action command.
type GitHubActionDeps struct {
	BranchReviewer BranchReviewer
	AuthDeps       AuthDependencies
}

// NewGitHubActionCommand creates the 'github-action' subcommand for running in GitHub Actions.
//
// This command is designed to be invoked by a minimal composite action that:
//  1. Downloads the bop binary
//  2. Runs `bop github-action`
//
// All inputs are read from environment variables (set by the action):
//   - BOP_* variables for bop-specific inputs
//   - GITHUB_* variables for GitHub context
//
// Outputs are written to $GITHUB_OUTPUT and summaries to $GITHUB_STEP_SUMMARY.
func NewGitHubActionCommand(deps GitHubActionDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "github-action",
		Short: "Run code review in GitHub Actions context",
		Long: `Run bop code review optimized for GitHub Actions.

This command reads configuration from environment variables and writes outputs
in GitHub Actions format. It is designed to be called by the bop-github-action
composite action.

Environment variables:
  BOP_BASE_REF         Base branch to compare against (default: main)
  BOP_POST_FINDINGS    Whether to post findings to PR (default: true)
  BOP_REVIEWERS        Comma-separated list of reviewers
  BOP_BLOCK_THRESHOLD  Severity threshold for blocking (critical, high, medium, low, none)
  BOP_LOG_LEVEL        Log level (trace, debug, info, error)

  GITHUB_TOKEN         GitHub token for API access
  GITHUB_HEAD_REF      PR head branch
  GITHUB_REPOSITORY    Repository in owner/repo format
  GITHUB_EVENT_NAME    Event that triggered the workflow
  GITHUB_WORKSPACE     Workspace directory
  GITHUB_OUTPUT        File to write outputs
  GITHUB_STEP_SUMMARY  File to write job summary

GitHub pull_request event context:
  GITHUB_PR_NUMBER     Pull request number
  GITHUB_PR_SHA        Head commit SHA`,
		Hidden: false, // Visible but primarily for action use
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubAction(cmd.Context(), deps)
		},
	}

	return cmd
}

// runGitHubAction executes the review in GitHub Actions context.
func runGitHubAction(ctx context.Context, deps GitHubActionDeps) error {
	// Verify we're running in GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return errors.New("this command is designed to run in GitHub Actions; set GITHUB_ACTIONS=true or use 'bop review branch' instead")
	}

	// Validate event type
	eventName := os.Getenv("GITHUB_EVENT_NAME")
	if eventName != "pull_request" && eventName != "pull_request_target" {
		return fmt.Errorf("unsupported event type %q; only pull_request is supported", eventName)
	}
	if eventName == "pull_request_target" {
		// Security warning but allow it - user takes responsibility
		fmt.Fprintln(os.Stderr, "::warning::pull_request_target is potentially dangerous with untrusted PR code")
	}

	// Parse GitHub context
	ghCtx, err := parseGitHubContext()
	if err != nil {
		return fmt.Errorf("parse GitHub context: %w", err)
	}

	// Parse bop configuration from environment
	cfg := parseActionConfig()

	// Auth check for platform mode (if configured)
	checker, err := deps.AuthDeps.RequireAuth()
	if err != nil {
		return err
	}
	if checker != nil && !checker.CanReviewCode() {
		return errors.New("code review not available on your plan")
	}

	// Build review request
	req := review.BranchRequest{
		BaseRef:              cfg.BaseRef,
		TargetRef:            ghCtx.HeadRef,
		OutputDir:            filepath.Join(os.TempDir(), "bop-review"),
		Repository:           ghCtx.Repository,
		IncludeUncommitted:   false,
		PostToGitHub:         cfg.PostFindings,
		GitHubOwner:          ghCtx.Owner,
		GitHubRepo:           ghCtx.Repo,
		PRNumber:             ghCtx.PRNumber,
		CommitSHA:            ghCtx.PRSHA,
		BotUsername:          "github-actions[bot]",
		ActionOnCritical:     resolveBlockAction("critical", cfg.BlockThreshold),
		ActionOnHigh:         resolveBlockAction("high", cfg.BlockThreshold),
		ActionOnMedium:       resolveBlockAction("medium", cfg.BlockThreshold),
		ActionOnLow:          resolveBlockAction("low", cfg.BlockThreshold),
		ActionOnClean:        "approve",
		ActionOnNonBlocking:  "comment",
		AlwaysBlockCategories: cfg.AlwaysBlockCategories,
		Reviewers:            cfg.Reviewers,
	}

	// Run the review
	result, err := deps.BranchReviewer.ReviewBranch(ctx, req)

	// Write outputs regardless of error (partial results are valuable)
	if writeErr := writeGitHubOutputs(result, err); writeErr != nil {
		fmt.Fprintf(os.Stderr, "::warning::Failed to write GitHub outputs: %v\n", writeErr)
	}

	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	// Check if we should fail based on findings
	if cfg.FailOnFindings && shouldFailOnFindings(result, cfg.BlockThreshold) {
		return fmt.Errorf("findings exceed block threshold (%s)", cfg.BlockThreshold)
	}

	return nil
}

// gitHubContext holds parsed GitHub Actions context.
type gitHubContext struct {
	Owner      string
	Repo       string
	Repository string // owner/repo
	HeadRef    string
	PRNumber   int
	PRSHA      string
	Workspace  string
}

// parseGitHubContext extracts GitHub context from environment variables.
func parseGitHubContext() (*gitHubContext, error) {
	repository := os.Getenv("GITHUB_REPOSITORY")
	if repository == "" {
		return nil, errors.New("GITHUB_REPOSITORY is required")
	}

	parts := strings.SplitN(repository, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid GITHUB_REPOSITORY format: %s", repository)
	}

	headRef := os.Getenv("GITHUB_HEAD_REF")
	if headRef == "" {
		return nil, errors.New("GITHUB_HEAD_REF is required (are you running on a pull_request event?)")
	}

	prNumberStr := os.Getenv("GITHUB_PR_NUMBER")
	if prNumberStr == "" {
		// Try to get from event payload if not explicitly set
		prNumberStr = os.Getenv("GITHUB_EVENT_NUMBER")
	}
	prNumber, err := strconv.Atoi(prNumberStr)
	if err != nil || prNumber <= 0 {
		return nil, fmt.Errorf("invalid or missing PR number: %s", prNumberStr)
	}

	prSHA := os.Getenv("GITHUB_PR_SHA")
	if prSHA == "" {
		prSHA = os.Getenv("GITHUB_SHA") // Fallback to workflow SHA
	}
	if prSHA == "" {
		return nil, errors.New("GITHUB_PR_SHA or GITHUB_SHA is required")
	}

	return &gitHubContext{
		Owner:      parts[0],
		Repo:       parts[1],
		Repository: repository,
		HeadRef:    headRef,
		PRNumber:   prNumber,
		PRSHA:      prSHA,
		Workspace:  os.Getenv("GITHUB_WORKSPACE"),
	}, nil
}

// actionConfig holds bop configuration parsed from environment.
type actionConfig struct {
	BaseRef               string
	PostFindings          bool
	Reviewers             []string
	BlockThreshold        string
	FailOnFindings        bool
	AlwaysBlockCategories []string
}

// parseActionConfig extracts bop configuration from BOP_* environment variables.
func parseActionConfig() actionConfig {
	cfg := actionConfig{
		BaseRef:        getEnvOrDefault("BOP_BASE_REF", "main"),
		PostFindings:   getEnvBool("BOP_POST_FINDINGS", true),
		BlockThreshold: getEnvOrDefault("BOP_BLOCK_THRESHOLD", "none"),
		FailOnFindings: getEnvBool("BOP_FAIL_ON_FINDINGS", false),
	}

	if reviewers := os.Getenv("BOP_REVIEWERS"); reviewers != "" {
		cfg.Reviewers = strings.Split(reviewers, ",")
		for i := range cfg.Reviewers {
			cfg.Reviewers[i] = strings.TrimSpace(cfg.Reviewers[i])
		}
	}

	if categories := os.Getenv("BOP_ALWAYS_BLOCK_CATEGORIES"); categories != "" {
		cfg.AlwaysBlockCategories = strings.Split(categories, ",")
		for i := range cfg.AlwaysBlockCategories {
			cfg.AlwaysBlockCategories[i] = strings.TrimSpace(cfg.AlwaysBlockCategories[i])
		}
	}

	return cfg
}

// resolveBlockAction determines the review action based on severity and threshold.
func resolveBlockAction(severity, threshold string) string {
	levels := map[string]int{
		"critical": 4,
		"high":     3,
		"medium":   2,
		"low":      1,
		"none":     0,
	}

	sevLevel := levels[strings.ToLower(severity)]
	threshLevel := levels[strings.ToLower(threshold)]

	if sevLevel >= threshLevel && threshLevel > 0 {
		return "request_changes"
	}
	return "comment"
}

// shouldFailOnFindings determines if the action should fail based on findings.
func shouldFailOnFindings(result review.Result, threshold string) bool {
	levels := map[string]int{
		"critical": 4,
		"high":     3,
		"medium":   2,
		"low":      1,
		"none":     0,
	}
	threshLevel := levels[strings.ToLower(threshold)]
	if threshLevel == 0 {
		return false // "none" means never fail
	}

	// Check if any finding exceeds threshold (across all reviews)
	for _, r := range result.Reviews {
		for _, finding := range r.Findings {
			findingLevel := levels[strings.ToLower(finding.Severity)]
			if findingLevel >= threshLevel {
				return true
			}
		}
	}
	return false
}

// writeGitHubOutputs writes outputs to $GITHUB_OUTPUT and summary to $GITHUB_STEP_SUMMARY.
func writeGitHubOutputs(result review.Result, reviewErr error) error {
	// Collect all findings from all reviews
	var allFindings []domain.Finding
	for _, r := range result.Reviews {
		allFindings = append(allFindings, r.Findings...)
	}

	// Count findings by severity
	counts := map[string]int{
		"critical": 0,
		"high":     0,
		"medium":   0,
		"low":      0,
	}
	for _, f := range allFindings {
		counts[strings.ToLower(f.Severity)]++
	}
	total := len(allFindings)

	// Write to GITHUB_OUTPUT
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile != "" {
		if err := writeOutputFile(outputFile, allFindings, counts, total, reviewErr); err != nil {
			return err
		}
	}

	// Write to GITHUB_STEP_SUMMARY
	summaryFile := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryFile != "" {
		if err := writeSummaryFile(summaryFile, counts, total, reviewErr); err != nil {
			return err
		}
	}

	// Also print notice for visibility in logs
	fmt.Printf("::notice::bop review complete: %d findings (%d critical, %d high, %d medium, %d low)\n",
		total, counts["critical"], counts["high"], counts["medium"], counts["low"])

	return nil
}

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// getEnvBool parses a boolean environment variable.
func getEnvBool(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	return strings.EqualFold(v, "true") || v == "1"
}

// writeOutputFile writes findings data to the GITHUB_OUTPUT file.
func writeOutputFile(path string, findings []domain.Finding, counts map[string]int, total int, reviewErr error) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open GITHUB_OUTPUT: %w", err)
	}

	var writeErr error
	defer func() {
		if closeErr := f.Close(); closeErr != nil && writeErr == nil {
			writeErr = fmt.Errorf("close GITHUB_OUTPUT: %w", closeErr)
		}
	}()

	lines := []string{
		fmt.Sprintf("findings-count=%d", total),
		fmt.Sprintf("critical-count=%d", counts["critical"]),
		fmt.Sprintf("high-count=%d", counts["high"]),
		fmt.Sprintf("medium-count=%d", counts["medium"]),
		fmt.Sprintf("low-count=%d", counts["low"]),
	}

	if reviewErr != nil {
		lines = append(lines, fmt.Sprintf("error=%s", reviewErr.Error()))
	}

	// Write findings as JSON for downstream processing
	if total > 0 {
		findingsJSON, _ := json.Marshal(findings)
		lines = append(lines, fmt.Sprintf("findings<<EOF\n%s\nEOF", string(findingsJSON)))
	}

	for _, line := range lines {
		if _, err := fmt.Fprintln(f, line); err != nil {
			return fmt.Errorf("write to GITHUB_OUTPUT: %w", err)
		}
	}

	return writeErr
}

// writeSummaryFile writes a markdown summary to the GITHUB_STEP_SUMMARY file.
func writeSummaryFile(path string, counts map[string]int, total int, reviewErr error) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open GITHUB_STEP_SUMMARY: %w", err)
	}

	var writeErr error
	defer func() {
		if closeErr := f.Close(); closeErr != nil && writeErr == nil {
			writeErr = fmt.Errorf("close GITHUB_STEP_SUMMARY: %w", closeErr)
		}
	}()

	write := func(format string, args ...any) {
		if writeErr != nil {
			return
		}
		_, writeErr = fmt.Fprintf(f, format, args...)
	}

	write("## bop Code Review\n\n")

	if reviewErr != nil {
		write("> [!WARNING]\n> Review completed with errors: %s\n\n", reviewErr.Error())
	}

	if total == 0 {
		write("No findings.\n")
	} else {
		write("| Severity | Count |\n")
		write("|----------|-------|\n")
		if counts["critical"] > 0 {
			write("| Critical | %d |\n", counts["critical"])
		}
		if counts["high"] > 0 {
			write("| High | %d |\n", counts["high"])
		}
		if counts["medium"] > 0 {
			write("| Medium | %d |\n", counts["medium"])
		}
		if counts["low"] > 0 {
			write("| Low | %d |\n", counts["low"])
		}
		write("| **Total** | **%d** |\n", total)
	}

	if writeErr != nil {
		return fmt.Errorf("write to GITHUB_STEP_SUMMARY: %w", writeErr)
	}
	return nil
}
