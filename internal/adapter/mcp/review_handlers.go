package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/bkyoung/code-reviewer/internal/adapter/llm/provider"
	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/review"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// =============================================================================
// Review Tool Handlers (Phase 3.5d - MCP Review Tools)
// =============================================================================

// registerEditFindingTool registers the edit_finding tool.
func (s *Server) registerEditFindingTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "edit_finding",
		Description: `Edit a finding's properties and return the modified finding.

This is a pure transformation tool - it takes a finding, applies optional overrides,
and returns a new finding with updated fingerprint. The original finding is unchanged.

Use this to:
- Adjust severity if the LLM was too aggressive/lenient
- Recategorize findings (e.g., "maintainability" → "security")
- Refine descriptions before posting to GitHub
- Filter findings by modifying and selectively posting`,
	}, s.handleEditFinding)
}

func (s *Server) handleEditFinding(ctx context.Context, req *mcp.CallToolRequest, input EditFindingInput) (*mcp.CallToolResult, EditFindingOutput, error) {
	// Convert input to domain finding
	finding := findingInputToDomain(input.Finding)

	// Track what fields were modified
	var modified []string

	// Apply overrides
	if input.Severity != nil {
		// Validate severity
		if !isValidSeverity(*input.Severity) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Invalid severity: %q. Valid values: %v", *input.Severity, ValidSeverities)},
				},
			}, EditFindingOutput{}, nil
		}
		finding.Severity = *input.Severity
		modified = append(modified, "severity")
	}

	if input.Category != nil {
		finding.Category = *input.Category
		modified = append(modified, "category")
	}

	if input.Description != nil {
		finding.Description = *input.Description
		modified = append(modified, "description")
	}

	if input.Suggestion != nil {
		finding.Suggestion = *input.Suggestion
		modified = append(modified, "suggestion")
	}

	// Convert to output (includes fingerprint calculation)
	output := domainFindingToOutput(finding)

	// Build result message
	var msg string
	fpShort := truncateFingerprint(output.Fingerprint, 16)
	if len(modified) == 0 {
		msg = fmt.Sprintf("Finding unchanged (fingerprint: %s)", fpShort)
	} else {
		msg = fmt.Sprintf("Modified %s (fingerprint: %s)", strings.Join(modified, ", "), fpShort)
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: msg},
			},
		}, EditFindingOutput{
			Finding:        output,
			FieldsModified: modified,
			Message:        msg,
		}, nil
}

// =============================================================================
// Conversion Helpers
// =============================================================================

// findingInputToDomain converts an MCP FindingInput to a domain.Finding.
func findingInputToDomain(input FindingInput) domain.Finding {
	return domain.Finding{
		ID:             input.ID,
		File:           input.File,
		LineStart:      input.LineStart,
		LineEnd:        input.LineEnd,
		Severity:       input.Severity,
		Category:       input.Category,
		Description:    input.Description,
		Suggestion:     input.Suggestion,
		ReviewerName:   input.ReviewerName,
		ReviewerWeight: input.ReviewerWeight,
	}
}

// getFingerprintFromInput returns the fingerprint for a FindingInput.
// If the input has a preserved fingerprint, it's returned as-is.
// Otherwise, a new fingerprint is computed from the domain representation.
func getFingerprintFromInput(input FindingInput) string {
	if input.Fingerprint != "" {
		return input.Fingerprint
	}
	return string(findingInputToDomain(input).Fingerprint())
}

// domainFindingToOutput converts a domain.Finding to an MCP FindingOutput.
func domainFindingToOutput(f domain.Finding) FindingOutput {
	return FindingOutput{
		ID:             f.ID,
		Fingerprint:    string(f.Fingerprint()),
		File:           f.File,
		LineStart:      f.LineStart,
		LineEnd:        f.LineEnd,
		Severity:       f.Severity,
		Category:       f.Category,
		Description:    f.Description,
		Suggestion:     f.Suggestion,
		ReviewerName:   f.ReviewerName,
		ReviewerWeight: f.ReviewerWeight,
	}
}

// domainFindingsToOutput converts a slice of domain.Finding to FindingOutput.
func domainFindingsToOutput(findings []domain.Finding) []FindingOutput {
	output := make([]FindingOutput, len(findings))
	for i, f := range findings {
		output[i] = domainFindingToOutput(f)
	}
	return output
}

// =============================================================================
// review_pr Tool
// =============================================================================

// registerReviewPRTool registers the review_pr tool.
func (s *Server) registerReviewPRTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "review_pr",
		Description: `Review a GitHub pull request and return findings.

This tool invokes code reviewers on a PR's diff and returns findings WITHOUT posting to GitHub.
The caller can then filter/modify findings using edit_finding and post selected ones using post_findings.

Uses configured LLM providers (via ANTHROPIC_API_KEY or OPENAI_API_KEY).
Falls back to MCP sampling if no API keys configured and client supports it.

Use this for:
- Running code review on any accessible PR
- Getting findings for human review before posting
- Filtering out false positives before they appear on the PR`,
	}, s.handleReviewPR)
}

func (s *Server) handleReviewPR(ctx context.Context, req *mcp.CallToolRequest, input ReviewPRInput) (*mcp.CallToolResult, ReviewPROutput, error) {
	// Validate inputs first (before checking dependencies)
	if input.Owner == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "owner is required"},
			},
		}, ReviewPROutput{}, nil
	}
	if input.Repo == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "repo is required"},
			},
		}, ReviewPROutput{}, nil
	}
	if input.PRNumber <= 0 {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("invalid PR number: %d (must be positive)", input.PRNumber)},
			},
		}, ReviewPROutput{}, nil
	}

	// Determine which reviewer to use:
	// 1. Prefer direct PRReviewer if available (from API keys)
	// 2. Fall back to per-request orchestrator with factory providers
	reviewer := s.deps.PRReviewer
	if reviewer == nil {
		// Try to create a per-request orchestrator using the factory
		perRequestReviewer, err := s.createPerRequestReviewer(req)
		if err != nil {
			return notImplementedResult(
					"review_pr requires either: " +
						"(1) LLM API keys (ANTHROPIC_API_KEY, OPENAI_API_KEY), or " +
						"(2) an MCP client that supports sampling"),
				ReviewPROutput{
					Findings:      []FindingOutput{},
					BySeverity:    make(map[string]int),
					ByCategory:    make(map[string]int),
					ReviewerStats: []ReviewerStat{},
				}, nil
		}
		reviewer = perRequestReviewer
	}

	// Build PRRequest
	prReq := review.PRRequest{
		Owner:        input.Owner,
		Repo:         input.Repo,
		PRNumber:     input.PRNumber,
		Reviewers:    input.Reviewers,
		PostToGitHub: false, // Never post - that's what post_findings is for
	}

	// Invoke review
	result, err := reviewer.ReviewPR(ctx, prReq)
	if err != nil {
		return nil, ReviewPROutput{}, fmt.Errorf("review PR: %w", err)
	}

	// Aggregate findings from all reviews
	var allFindings []domain.Finding
	var totalTokensIn, totalTokensOut int
	var totalCost float64

	for _, r := range result.Reviews {
		allFindings = append(allFindings, r.Findings...)
		totalTokensIn += r.TokensIn
		totalTokensOut += r.TokensOut
		totalCost += r.Cost
	}

	// Build output
	output := ReviewPROutput{
		Findings:      domainFindingsToOutput(allFindings),
		TotalFindings: len(allFindings),
		BySeverity:    countBySeverity(allFindings),
		ByCategory:    countByCategory(allFindings),
		ReviewerStats: buildReviewerStats(result.Reviews),
		TokensIn:      totalTokensIn,
		TokensOut:     totalTokensOut,
		Cost:          totalCost,
	}

	// Build summary
	if len(allFindings) == 0 {
		output.Summary = "No findings detected"
		output.Message = "PR review complete: no issues found"
	} else {
		output.Summary = buildFindingsSummary(allFindings)
		output.Message = fmt.Sprintf("PR review complete: %d findings", len(allFindings))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: output.Message},
		},
	}, output, nil
}

// =============================================================================
// post_findings Tool
// =============================================================================

// registerPostFindingsTool registers the post_findings tool.
func (s *Server) registerPostFindingsTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "post_findings",
		Description: `Post findings to a GitHub PR as inline review comments.

This tool takes a list of findings (typically from review_pr or edit_finding) and posts them
as a GitHub review with inline comments. Fingerprint-based deduplication prevents posting
findings that already exist on the PR.

Use this for:
- Posting curated findings after filtering out false positives
- Re-posting findings with updated severity/description
- Posting findings in batches for large reviews`,
	}, s.handlePostFindings)
}

func (s *Server) handlePostFindings(ctx context.Context, req *mcp.CallToolRequest, input PostFindingsInput) (*mcp.CallToolResult, PostFindingsOutput, error) {
	// Validate inputs first (before checking dependencies)
	if input.Owner == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "owner is required"},
			},
		}, PostFindingsOutput{}, nil
	}
	if input.Repo == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "repo is required"},
			},
		}, PostFindingsOutput{}, nil
	}
	if input.PRNumber <= 0 {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("invalid PR number: %d (must be positive)", input.PRNumber)},
			},
		}, PostFindingsOutput{}, nil
	}
	if len(input.Findings) == 0 {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "at least one finding is required"},
			},
		}, PostFindingsOutput{}, nil
	}

	// Validate severity for each finding
	for i, f := range input.Findings {
		if !isValidSeverity(f.Severity) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("finding %d has invalid severity: %q. Valid values: %v", i, f.Severity, ValidSeverities)},
				},
			}, PostFindingsOutput{}, nil
		}
	}

	// Check dependencies after validation
	if s.deps.FindingPoster == nil {
		return notImplementedResult("post_findings - FindingPoster not configured"), PostFindingsOutput{}, nil
	}
	if s.deps.RemoteGitHubClient == nil {
		return notImplementedResult("post_findings - RemoteGitHubClient not configured"), PostFindingsOutput{}, nil
	}

	// Dry run mode - just validate and return what would be posted
	if input.DryRun {
		fingerprints := make([]string, len(input.Findings))
		for i, f := range input.Findings {
			fingerprints[i] = getFingerprintFromInput(f)
		}
		return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Dry run: would post %d findings", len(input.Findings))},
				},
			}, PostFindingsOutput{
				Success:            true,
				Posted:             len(input.Findings),
				ReviewAction:       determineReviewAction(nil, input.ReviewAction, input.BlockingFindings, nil),
				PostedFingerprints: fingerprints,
				Message:            fmt.Sprintf("Dry run: would post %d findings", len(input.Findings)),
			}, nil
	}

	// Fetch PR metadata to get commit SHA
	metadata, err := s.deps.RemoteGitHubClient.GetPRMetadata(ctx, input.Owner, input.Repo, input.PRNumber)
	if err != nil {
		return nil, PostFindingsOutput{}, fmt.Errorf("fetch PR metadata: %w", err)
	}

	// Fetch PR diff for line position calculation
	diff, err := s.deps.RemoteGitHubClient.GetPRDiff(ctx, input.Owner, input.Repo, input.PRNumber)
	if err != nil {
		return nil, PostFindingsOutput{}, fmt.Errorf("fetch PR diff: %w", err)
	}

	// Convert findings to domain type
	domainFindings := make([]domain.Finding, len(input.Findings))
	for i, f := range input.Findings {
		domainFindings[i] = findingInputToDomain(f)
	}

	// Build review from findings
	domainReview := domain.Review{
		Findings: domainFindings,
		Summary:  buildFindingsSummary(domainFindings),
	}

	// Build fingerprints for blocking check (preserve original if provided)
	inputFingerprints := make([]string, len(input.Findings))
	for i, f := range input.Findings {
		inputFingerprints[i] = getFingerprintFromInput(f)
	}

	// Determine review action using preserved fingerprints
	reviewAction := determineReviewAction(domainFindings, input.ReviewAction, input.BlockingFindings, inputFingerprints)

	// Build post request
	postReq := review.GitHubPostRequest{
		Owner:     input.Owner,
		Repo:      input.Repo,
		PRNumber:  input.PRNumber,
		CommitSHA: metadata.HeadSHA,
		Review:    domainReview,
		Diff:      diff,
	}

	// Map review action to GitHub format
	switch reviewAction {
	case "REQUEST_CHANGES":
		postReq.ActionOnCritical = "request_changes"
		postReq.ActionOnHigh = "request_changes"
	case "APPROVE":
		postReq.ActionOnClean = "approve"
	default:
		postReq.ActionOnMedium = "comment"
		postReq.ActionOnLow = "comment"
	}

	// Post the review
	result, err := s.deps.FindingPoster.PostReview(ctx, postReq)
	if err != nil {
		return nil, PostFindingsOutput{}, fmt.Errorf("post review: %w", err)
	}

	// Reuse inputFingerprints computed earlier for blocking check
	output := PostFindingsOutput{
		Success:            true,
		Posted:             result.CommentsPosted,
		Skipped:            result.CommentsSkipped,
		SkippedDuplicates:  result.DuplicatesSkipped,
		ReviewID:           result.ReviewID,
		ReviewAction:       reviewAction,
		PostedFingerprints: inputFingerprints,
		Message:            fmt.Sprintf("Posted %d findings to PR #%d", result.CommentsPosted, input.PRNumber),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: output.Message},
		},
	}, output, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// countBySeverity counts findings by severity level.
func countBySeverity(findings []domain.Finding) map[string]int {
	counts := make(map[string]int)
	for _, f := range findings {
		sev := f.Severity
		if sev == "" {
			sev = "unknown"
		}
		counts[sev]++
	}
	return counts
}

// countByCategory counts findings by category.
func countByCategory(findings []domain.Finding) map[string]int {
	counts := make(map[string]int)
	for _, f := range findings {
		cat := f.Category
		if cat == "" {
			cat = "unknown"
		}
		counts[cat]++
	}
	return counts
}

// buildReviewerStats builds per-reviewer statistics.
func buildReviewerStats(reviews []domain.Review) []ReviewerStat {
	stats := make(map[string]int)
	for _, r := range reviews {
		for _, f := range r.Findings {
			name := f.ReviewerName
			if name == "" {
				name = r.ProviderName
			}
			stats[name]++
		}
	}

	result := make([]ReviewerStat, 0, len(stats))
	for name, count := range stats {
		result = append(result, ReviewerStat{Name: name, Findings: count})
	}
	return result
}

// buildFindingsSummary builds a human-readable summary of findings.
func buildFindingsSummary(findings []domain.Finding) string {
	if len(findings) == 0 {
		return "No findings detected"
	}

	counts := countBySeverity(findings)
	var parts []string
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		if c := counts[sev]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c, sev))
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d findings", len(findings))
	}
	return strings.Join(parts, ", ")
}

// determineReviewAction determines the GitHub review action based on findings.
// findingFingerprints should be the preserved/computed fingerprints matching the findings slice.
// If nil, fingerprints are computed from domain findings (for backward compatibility).
func determineReviewAction(findings []domain.Finding, override *string, blockingFingerprints []string, findingFingerprints []string) string {
	if override != nil && *override != "" {
		return strings.ToUpper(*override)
	}

	// Check for blocking fingerprints
	blockingSet := make(map[string]bool)
	for _, fp := range blockingFingerprints {
		blockingSet[fp] = true
	}

	hasBlocking := false
	for i, f := range findings {
		// Use provided fingerprint if available, otherwise compute
		var fp string
		if findingFingerprints != nil && i < len(findingFingerprints) {
			fp = findingFingerprints[i]
		} else {
			fp = string(f.Fingerprint())
		}

		if blockingSet[fp] {
			hasBlocking = true
			break
		}
		if f.Severity == "critical" || f.Severity == "high" {
			hasBlocking = true
			break
		}
	}

	if hasBlocking {
		return "REQUEST_CHANGES"
	}
	if len(findings) > 0 {
		return "COMMENT"
	}
	return "APPROVE"
}

// truncateFingerprint safely truncates a fingerprint for display purposes.
// Returns at most maxLen characters, or the full string if shorter.
func truncateFingerprint(fp string, maxLen int) string {
	if len(fp) <= maxLen {
		return fp
	}
	return fp[:maxLen]
}

// =============================================================================
// review_branch Tool
// =============================================================================

// registerReviewBranchTool registers the review_branch tool.
func (s *Server) registerReviewBranchTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "review_branch",
		Description: `Review a local git branch against a base reference. Returns findings without posting to any remote service.

Uses configured LLM providers (via ANTHROPIC_API_KEY or OPENAI_API_KEY).
Falls back to MCP sampling if no API keys configured and client supports it.

Use this for:
- Running code review on local changes before pushing
- Reviewing uncommitted changes in working tree
- Getting findings for manual triage before PR creation`,
	}, s.handleReviewBranch)
}

func (s *Server) handleReviewBranch(ctx context.Context, req *mcp.CallToolRequest, input ReviewBranchInput) (*mcp.CallToolResult, ReviewBranchOutput, error) {
	// Validate input first (before checking dependencies)
	if input.BaseRef == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "base_ref is required"},
			},
		}, ReviewBranchOutput{}, nil
	}

	// Determine which reviewer to use:
	// 1. Prefer direct BranchReviewer if available (from API keys)
	// 2. Fall back to per-request orchestrator with factory providers
	reviewer := s.deps.BranchReviewer
	if reviewer == nil {
		// Try to create a per-request orchestrator using the factory
		perRequestReviewer, err := s.createPerRequestReviewer(req)
		if err != nil {
			return notImplementedResult(
					"review_branch requires either: " +
						"(1) LLM API keys (ANTHROPIC_API_KEY, OPENAI_API_KEY), or " +
						"(2) an MCP client that supports sampling"),
				ReviewBranchOutput{
					Findings:      []FindingOutput{},
					BySeverity:    make(map[string]int),
					ByCategory:    make(map[string]int),
					ReviewerStats: []ReviewerStat{},
				}, nil
		}
		reviewer = perRequestReviewer
	}

	// Build BranchRequest
	branchReq := review.BranchRequest{
		BaseRef:            input.BaseRef,
		TargetRef:          input.TargetRef,
		IncludeUncommitted: input.IncludeUncommitted,
		Reviewers:          input.Reviewers,
		PostToGitHub:       false, // Never post from this tool
	}

	// Execute review
	result, err := reviewer.ReviewBranch(ctx, branchReq)
	if err != nil {
		return nil, ReviewBranchOutput{}, fmt.Errorf("review branch: %w", err)
	}

	// Aggregate findings from all reviews
	var allFindings []domain.Finding
	var totalTokensIn, totalTokensOut int
	var totalCost float64

	for _, r := range result.Reviews {
		allFindings = append(allFindings, r.Findings...)
		totalTokensIn += r.TokensIn
		totalTokensOut += r.TokensOut
		totalCost += r.Cost
	}

	// Build output (same pattern as review_pr)
	output := ReviewBranchOutput{
		Findings:      domainFindingsToOutput(allFindings),
		TotalFindings: len(allFindings),
		BySeverity:    countBySeverity(allFindings),
		ByCategory:    countByCategory(allFindings),
		ReviewerStats: buildReviewerStats(result.Reviews),
		TokensIn:      totalTokensIn,
		TokensOut:     totalTokensOut,
		Cost:          totalCost,
		BaseRef:       input.BaseRef,
		TargetRef:     input.TargetRef,
	}

	if len(allFindings) == 0 {
		output.Summary = "No findings detected"
		output.Message = "Branch review complete: no issues found"
	} else {
		output.Summary = buildFindingsSummary(allFindings)
		output.Message = fmt.Sprintf("Branch review complete: %d findings", len(allFindings))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: output.Message},
		},
	}, output, nil
}

// =============================================================================
// Per-Request Reviewer Support
// =============================================================================

// Reviewer is the common interface for both branch and PR reviewers.
// This allows the same per-request creation logic to be used for both tools.
type Reviewer interface {
	BranchReviewer
	PRReviewer
}

// createPerRequestReviewer creates a reviewer using the provider factory.
// It uses effective providers (direct or sampling fallback) to create an
// orchestrator per-request, enabling zero-config usage via MCP sampling.
//
// Thread Safety:
// This function is safe for concurrent calls. It only reads from immutable
// Server.deps fields (set at construction), EffectiveProviders returns a
// defensive copy of the provider map, and a new Orchestrator is created
// for each request with no shared mutable state.
//
// Returns an error if:
// - The factory is not configured
// - Required dependencies (Git, Merger) are not configured
// - No providers are available (no API keys and no sampling support)
func (s *Server) createPerRequestReviewer(req *mcp.CallToolRequest) (Reviewer, error) {
	// Check for required dependencies
	if s.deps.ProviderFactory == nil {
		return nil, fmt.Errorf("provider factory not configured")
	}
	if s.deps.Git == nil {
		return nil, fmt.Errorf("git engine not configured")
	}
	if s.deps.Merger == nil {
		return nil, fmt.Errorf("merger not configured")
	}

	// Get the session from the request (for sampling fallback)
	var session provider.SamplingSession
	if req != nil && req.Session != nil {
		session = &serverSessionAdapter{req.Session}
	}

	// Get effective providers from factory (direct or sampling)
	providers, err := s.deps.ProviderFactory.EffectiveProviders(session)
	if err != nil {
		return nil, fmt.Errorf("no providers available: %w", err)
	}

	// Create orchestrator with effective providers
	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git:              s.deps.Git,
		Providers:        providers,
		Merger:           s.deps.Merger,
		ReviewerRegistry: s.deps.ReviewerRegistry,
	})

	return orchestrator, nil
}

// serverSessionAdapter adapts mcp.ServerSession to provider.SamplingSession.
type serverSessionAdapter struct {
	session *mcp.ServerSession
}

func (a *serverSessionAdapter) CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
	return a.session.CreateMessage(ctx, params)
}

func (a *serverSessionAdapter) InitializeParams() *mcp.InitializeParams {
	return a.session.InitializeParams()
}
