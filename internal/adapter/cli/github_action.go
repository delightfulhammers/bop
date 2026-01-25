package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/auth"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// knownSeverities defines the valid severity levels for validation.
var knownSeverities = map[string]int{
	"critical": 4,
	"high":     3,
	"medium":   2,
	"low":      1,
	"none":     0,
}

// maxSummarySize is the maximum size for GITHUB_STEP_SUMMARY (GitHub limit is 1MB).
const maxSummarySize = 900 * 1024 // 900KB to leave buffer

// GitHubActionDeps holds dependencies for the github-action command.
type GitHubActionDeps struct {
	BranchReviewer BranchReviewer
	AuthDeps       AuthDependencies
	PlatformURL    string // Platform URL for OIDC audience
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
  BOP_LOG_LEVEL        Log level (trace, debug, info, warn, error)
  BOP_TENANT_ID        Tenant ID for OIDC authentication (required for platform mode)

  GITHUB_TOKEN         GitHub token for API access
  GITHUB_HEAD_REF      PR head branch
  GITHUB_REPOSITORY    Repository in owner/repo format
  GITHUB_EVENT_NAME    Event that triggered the workflow
  GITHUB_WORKSPACE     Workspace directory
  GITHUB_OUTPUT        File to write outputs
  GITHUB_STEP_SUMMARY  File to write job summary

GitHub pull_request event context:
  GITHUB_PR_NUMBER     Pull request number
  GITHUB_PR_SHA        Head commit SHA

OIDC authentication (recommended):
  When running with 'permissions: id-token: write', bop automatically uses
  GitHub Actions OIDC for keyless authentication with the platform. Set
  BOP_TENANT_ID to your organization's tenant ID.`,
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
	cfg, err := parseActionConfig()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Validate GITHUB_TOKEN is present when posting findings
	if cfg.PostFindings && os.Getenv("GITHUB_TOKEN") == "" {
		return errors.New("GITHUB_TOKEN is required when post-findings is enabled")
	}

	// Authenticate: try stored auth first (may already be populated by OIDC bootstrap
	// in runPlatformMode), then fall back to direct OIDC if available.
	var repoAccessChecker review.RepoAccessChecker
	checker, authErr := deps.AuthDeps.RequireAuth()
	if authErr != nil && auth.IsAvailable() && deps.PlatformURL != "" {
		// No stored auth but OIDC is available - authenticate directly.
		// This handles the case where the github-action command is invoked
		// without the platform mode bootstrap (e.g., via legacy mode).
		oidcResult, oidcErr := authenticateOIDC(ctx, deps)
		if oidcErr != nil {
			return fmt.Errorf("OIDC authentication: %w", oidcErr)
		}
		if !oidcResult.Entitlements.CanReviewCode() {
			return errors.New("code review not available on your plan")
		}
		repoAccessChecker = oidcResult.Entitlements
	} else if authErr != nil {
		return authErr
	} else {
		if checker != nil && !checker.CanReviewCode() {
			return errors.New("code review not available on your plan")
		}
		if checker != nil {
			repoAccessChecker = checker
		}
	}

	// Create unique output directory to avoid collisions with concurrent jobs
	outputDir, err := os.MkdirTemp("", "bop-review-*")
	if err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(outputDir); removeErr != nil {
			fmt.Fprintf(os.Stderr, "::warning::Failed to cleanup output directory: %v\n", removeErr)
		}
	}()

	// Build review request
	req := review.BranchRequest{
		BaseRef:               cfg.BaseRef,
		TargetRef:             ghCtx.HeadRef,
		OutputDir:             outputDir,
		Repository:            ghCtx.Repository,
		IncludeUncommitted:    false,
		PostToGitHub:          cfg.PostFindings,
		GitHubOwner:           ghCtx.Owner,
		GitHubRepo:            ghCtx.Repo,
		PRNumber:              ghCtx.PRNumber,
		CommitSHA:             ghCtx.PRSHA,
		BotUsername:           "github-actions[bot]",
		ActionOnCritical:      resolveBlockAction("critical", cfg.BlockThreshold),
		ActionOnHigh:          resolveBlockAction("high", cfg.BlockThreshold),
		ActionOnMedium:        resolveBlockAction("medium", cfg.BlockThreshold),
		ActionOnLow:           resolveBlockAction("low", cfg.BlockThreshold),
		ActionOnClean:         "approve",
		ActionOnNonBlocking:   "comment",
		AlwaysBlockCategories: cfg.AlwaysBlockCategories,
		Reviewers:             cfg.Reviewers,
		RepoAccessChecker:     repoAccessChecker,
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
// Returns an error if required configuration values are invalid.
func parseActionConfig() (actionConfig, error) {
	threshold := strings.ToLower(getEnvOrDefault("BOP_BLOCK_THRESHOLD", "none"))
	if _, ok := knownSeverities[threshold]; !ok {
		return actionConfig{}, fmt.Errorf("invalid BOP_BLOCK_THRESHOLD %q; must be one of: critical, high, medium, low, none", threshold)
	}

	cfg := actionConfig{
		BaseRef:        getEnvOrDefault("BOP_BASE_REF", "main"),
		PostFindings:   getEnvBool("BOP_POST_FINDINGS", true),
		BlockThreshold: threshold,
		FailOnFindings: getEnvBool("BOP_FAIL_ON_FINDINGS", false),
	}

	if reviewers := os.Getenv("BOP_REVIEWERS"); reviewers != "" {
		cfg.Reviewers = splitAndFilter(reviewers)
	}

	if categories := os.Getenv("BOP_ALWAYS_BLOCK_CATEGORIES"); categories != "" {
		cfg.AlwaysBlockCategories = splitAndFilter(categories)
	}

	return cfg, nil
}

// resolveBlockAction determines the review action based on severity and threshold.
// Unknown severities are treated as "low" level.
func resolveBlockAction(severity, threshold string) string {
	sevLevel, ok := knownSeverities[strings.ToLower(severity)]
	if !ok {
		sevLevel = knownSeverities["low"] // Treat unknown as low
	}
	threshLevel := knownSeverities[strings.ToLower(threshold)]

	if sevLevel >= threshLevel && threshLevel > 0 {
		return "request_changes"
	}
	return "comment"
}

// shouldFailOnFindings determines if the action should fail based on findings.
// Unknown severities are treated as "low" level.
func shouldFailOnFindings(result review.Result, threshold string) bool {
	threshLevel := knownSeverities[strings.ToLower(threshold)]
	if threshLevel == 0 {
		return false // "none" means never fail
	}

	// Check if any finding exceeds threshold (across all reviews)
	for _, r := range result.Reviews {
		for _, finding := range r.Findings {
			findingLevel, ok := knownSeverities[strings.ToLower(finding.Severity)]
			if !ok {
				findingLevel = knownSeverities["low"] // Treat unknown as low
			}
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

	// Count findings by severity, normalizing unknown to "low"
	counts := map[string]int{
		"critical": 0,
		"high":     0,
		"medium":   0,
		"low":      0,
	}
	for _, f := range allFindings {
		sev := strings.ToLower(f.Severity)
		if sev == "none" {
			// 'none' is a valid severity but shouldn't be counted in buckets
			continue
		}
		if _, ok := knownSeverities[sev]; !ok {
			// Unknown severity - warn and count as low
			fmt.Fprintf(os.Stderr, "::warning::Unknown severity %q in finding, counting as low\n", f.Severity)
			sev = "low"
		}
		counts[sev]++
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

// authenticateOIDC performs OIDC authentication for GitHub Actions.
// Returns the authentication result containing entitlements for access checking.
func authenticateOIDC(ctx context.Context, deps GitHubActionDeps) (*auth.OIDCAuthResult, error) {
	tenantID := os.Getenv("BOP_TENANT_ID")
	if tenantID == "" {
		return nil, fmt.Errorf("BOP_TENANT_ID is required for OIDC authentication; " +
			"set it to your organization's tenant ID")
	}

	client, err := auth.NewClient(auth.ClientConfig{
		BaseURL:   deps.PlatformURL,
		ProductID: "bop",
	})
	if err != nil {
		return nil, fmt.Errorf("create auth client: %w", err)
	}

	oidc := auth.NewGitHubActionsOIDC(client, deps.PlatformURL)
	result, err := oidc.Authenticate(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "::notice::Authenticated via OIDC as %s (tenant: %s)\n",
		result.StoredAuth.User.GitHubLogin, result.StoredAuth.TenantID)

	return result, nil
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

// splitAndFilter splits a comma-separated string and filters out empty values.
func splitAndFilter(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
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
		// Sanitize error message: replace newlines with spaces to prevent output injection
		sanitized := strings.ReplaceAll(reviewErr.Error(), "\n", " ")
		sanitized = strings.ReplaceAll(sanitized, "\r", " ")
		lines = append(lines, fmt.Sprintf("error=%s", sanitized))
	}

	// Write findings as JSON for downstream processing using heredoc with unique delimiter
	// Always output findings (empty array when none) so downstream workflows can parse it
	var findingsJSON []byte
	if len(findings) == 0 {
		findingsJSON = []byte("[]")
	} else {
		var marshalErr error
		findingsJSON, marshalErr = json.Marshal(findings)
		if marshalErr != nil {
			fmt.Fprintf(os.Stderr, "::warning::Failed to marshal findings to JSON: %v\n", marshalErr)
			// Write empty array as fallback
			findingsJSON = []byte("[]")
		}
	}
	delimiter, delimErr := generateDelimiter()
	if delimErr != nil {
		return fmt.Errorf("generate output delimiter: %w", delimErr)
	}
	lines = append(lines, fmt.Sprintf("findings<<%s\n%s\n%s", delimiter, string(findingsJSON), delimiter))

	for _, line := range lines {
		if _, err := fmt.Fprintln(f, line); err != nil {
			return fmt.Errorf("write to GITHUB_OUTPUT: %w", err)
		}
	}

	return writeErr
}

// generateDelimiter creates a unique delimiter for heredoc output to prevent injection.
// Returns an error if entropy source fails (fail-closed for security).
func generateDelimiter() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random delimiter: %w", err)
	}
	return "BOP_EOF_" + hex.EncodeToString(b), nil
}

// truncateUTF8Safe truncates a string to at most maxBytes while preserving UTF-8 validity.
// It ensures truncation happens at a character boundary, not in the middle of a multi-byte sequence.
func truncateUTF8Safe(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	// Find the last valid rune boundary at or before maxBytes
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// writeSummaryFile writes a markdown summary to the GITHUB_STEP_SUMMARY file.
// Respects GitHub's size limit by truncating if necessary.
func writeSummaryFile(path string, counts map[string]int, total int, reviewErr error) error {
	// Build summary in memory first to check size
	var sb strings.Builder

	sb.WriteString("## bop Code Review\n\n")

	if reviewErr != nil {
		// Sanitize error for markdown (escape special chars, limit length)
		errMsg := reviewErr.Error()
		if len(errMsg) > 500 {
			errMsg = errMsg[:500] + "..."
		}
		sb.WriteString(fmt.Sprintf("> [!WARNING]\n> Review completed with errors: %s\n\n", errMsg))
	}

	if total == 0 {
		sb.WriteString("No findings.\n")
	} else {
		sb.WriteString("| Severity | Count |\n")
		sb.WriteString("|----------|-------|\n")
		if counts["critical"] > 0 {
			sb.WriteString(fmt.Sprintf("| Critical | %d |\n", counts["critical"]))
		}
		if counts["high"] > 0 {
			sb.WriteString(fmt.Sprintf("| High | %d |\n", counts["high"]))
		}
		if counts["medium"] > 0 {
			sb.WriteString(fmt.Sprintf("| Medium | %d |\n", counts["medium"]))
		}
		if counts["low"] > 0 {
			sb.WriteString(fmt.Sprintf("| Low | %d |\n", counts["low"]))
		}
		sb.WriteString(fmt.Sprintf("| **Total** | **%d** |\n", total))
	}

	content := sb.String()

	// Check size limit (GitHub has 1MB limit, we use 900KB for safety)
	if len(content) > maxSummarySize {
		content = truncateUTF8Safe(content, maxSummarySize-50) + "\n\n*Summary truncated due to size limits.*\n"
	}

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

	if _, writeErr = f.WriteString(content); writeErr != nil {
		return fmt.Errorf("write to GITHUB_STEP_SUMMARY: %w", writeErr)
	}

	return writeErr
}
