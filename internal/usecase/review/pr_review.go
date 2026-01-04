package review

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/delightfulhammers/bop/internal/domain"
)

// RemoteGitHubClient defines the outbound port for GitHub API operations.
// This is used by ReviewPR to fetch PR data remotely without needing a local clone.
type RemoteGitHubClient interface {
	// GetPRMetadata retrieves metadata about a pull request.
	GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error)

	// GetPRDiff fetches the diff for a pull request.
	GetPRDiff(ctx context.Context, owner, repo string, prNumber int) (domain.Diff, error)

	// GetFileContent fetches a file's content from a repository at a specific ref.
	// Used for context gathering (ARCHITECTURE.md, README.md, etc.).
	GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error)
}

// PRRequest represents an inbound request to review a GitHub PR.
type PRRequest struct {
	// Required: identifies the PR
	Owner    string
	Repo     string
	PRNumber int

	// Optional: output configuration
	OutputDir  string // Directory to write review artifacts
	Repository string // Optional repository name override (for output filenames)

	// Optional: custom instructions and context
	CustomInstructions string   // Optional: custom review instructions
	ContextFiles       []string // Optional: additional context files to include
	NoArchitecture     bool     // Skip loading ARCHITECTURE.md from remote
	NoAutoContext      bool     // Disable automatic context gathering

	// GitHub posting (off by default for PR review)
	PostToGitHub bool // Enable posting review to GitHub PR

	// Review action configuration (only used if PostToGitHub is true)
	ActionOnCritical      string
	ActionOnHigh          string
	ActionOnMedium        string
	ActionOnLow           string
	ActionOnClean         string
	ActionOnNonBlocking   string
	AlwaysBlockCategories []string
	BotUsername           string

	// Verification settings
	SkipVerification   bool
	VerificationConfig VerificationSettings

	// Reviewers to use (empty = use defaults from config)
	Reviewers []string
}

// ReviewPR reviews a GitHub pull request by fetching its diff remotely.
// This allows reviewing any PR without needing a local clone.
func (o *Orchestrator) ReviewPR(ctx context.Context, req PRRequest) (Result, error) {
	// Validate RemoteGitHubClient is configured
	if o.deps.RemoteGitHubClient == nil {
		return Result{}, fmt.Errorf("RemoteGitHubClient is required for PR review")
	}

	// Validate request
	if req.Owner == "" {
		return Result{}, fmt.Errorf("owner is required")
	}
	if req.Repo == "" {
		return Result{}, fmt.Errorf("repo is required")
	}
	if req.PRNumber <= 0 {
		return Result{}, fmt.Errorf("invalid PR number: %d (must be positive)", req.PRNumber)
	}

	// Fetch PR metadata
	metadata, err := o.deps.RemoteGitHubClient.GetPRMetadata(ctx, req.Owner, req.Repo, req.PRNumber)
	if err != nil {
		return Result{}, fmt.Errorf("fetch PR metadata: %w", err)
	}

	// Fetch PR diff
	diff, err := o.deps.RemoteGitHubClient.GetPRDiff(ctx, req.Owner, req.Repo, req.PRNumber)
	if err != nil {
		return Result{}, fmt.Errorf("fetch PR diff: %w", err)
	}

	// Add commit hashes to diff
	diff.FromCommitHash = metadata.BaseSHA
	diff.ToCommitHash = metadata.HeadSHA

	// Try to gather remote context (best effort)
	projectCtx := o.gatherRemoteContext(ctx, req, metadata)

	// Convert PRRequest to BranchRequest for review execution
	branchReq := o.prToBranchRequest(req, metadata)

	// Run the review using the existing orchestration logic
	return o.executeReviewWithDiff(ctx, branchReq, diff, projectCtx)
}

// gatherRemoteContext attempts to fetch context files from the remote repository.
// This is best-effort - failures are logged but don't block the review.
func (o *Orchestrator) gatherRemoteContext(ctx context.Context, req PRRequest, metadata *domain.PRMetadata) ProjectContext {
	projectCtx := ProjectContext{
		ChangedPaths: extractChangedPaths(metadata),
	}

	if req.NoAutoContext {
		return projectCtx
	}

	// Try to fetch ARCHITECTURE.md
	if !req.NoArchitecture {
		content, err := o.deps.RemoteGitHubClient.GetFileContent(ctx, req.Owner, req.Repo, "ARCHITECTURE.md", metadata.HeadSHA)
		if err == nil && content != "" {
			projectCtx.Architecture = content
		}
	}

	// Try to fetch README.md
	readme, err := o.deps.RemoteGitHubClient.GetFileContent(ctx, req.Owner, req.Repo, "README.md", metadata.HeadSHA)
	if err == nil && readme != "" {
		projectCtx.README = readme
	}

	// Add custom instructions if provided
	projectCtx.CustomInstructions = req.CustomInstructions

	return projectCtx
}

// extractChangedPaths extracts file paths from PR metadata or diff.
func extractChangedPaths(metadata *domain.PRMetadata) []string {
	// PRMetadata doesn't include file paths directly; this would come from the diff
	// For now, return nil - the changed paths will be extracted from diff during review
	return nil
}

// prToBranchRequest converts a PRRequest to a BranchRequest for the existing review flow.
func (o *Orchestrator) prToBranchRequest(req PRRequest, metadata *domain.PRMetadata) BranchRequest {
	repository := req.Repository
	if repository == "" {
		repository = fmt.Sprintf("%s/%s", req.Owner, req.Repo)
	}

	return BranchRequest{
		BaseRef:               metadata.BaseRef,
		TargetRef:             metadata.HeadRef,
		OutputDir:             req.OutputDir,
		Repository:            repository,
		IncludeUncommitted:    false, // Remote PRs don't have uncommitted changes
		CustomInstructions:    req.CustomInstructions,
		ContextFiles:          req.ContextFiles,
		NoArchitecture:        true, // We already gathered context remotely
		NoAutoContext:         true, // We already gathered context remotely
		Interactive:           false,
		PostToGitHub:          req.PostToGitHub,
		GitHubOwner:           req.Owner,
		GitHubRepo:            req.Repo,
		PRNumber:              req.PRNumber,
		CommitSHA:             metadata.HeadSHA,
		ActionOnCritical:      req.ActionOnCritical,
		ActionOnHigh:          req.ActionOnHigh,
		ActionOnMedium:        req.ActionOnMedium,
		ActionOnLow:           req.ActionOnLow,
		ActionOnClean:         req.ActionOnClean,
		ActionOnNonBlocking:   req.ActionOnNonBlocking,
		AlwaysBlockCategories: req.AlwaysBlockCategories,
		BotUsername:           req.BotUsername,
		SkipVerification:      req.SkipVerification,
		VerificationConfig:    req.VerificationConfig,
		Reviewers:             req.Reviewers,
	}
}

// executeReviewWithDiff runs the review with a pre-fetched diff and context.
// This delegates to ReviewBranchWithDiff which shares the core logic with ReviewBranch.
func (o *Orchestrator) executeReviewWithDiff(ctx context.Context, req BranchRequest, diff domain.Diff, projectCtx ProjectContext) (Result, error) {
	return o.ReviewBranchWithDiff(ctx, req, diff, projectCtx)
}

// ParsePRIdentifier parses a PR identifier from various formats:
// - owner/repo#123
// - https://github.com/owner/repo/pull/123
// - github.mycompany.com/owner/repo/pull/123 (GHE)
func ParsePRIdentifier(input string) (owner, repo string, number int, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", 0, fmt.Errorf("empty PR identifier")
	}

	// Try owner/repo#number format first (capture the number part even if non-numeric)
	shorthandRegex := regexp.MustCompile(`^([^/]+)/([^#]+)#(.+)$`)
	if matches := shorthandRegex.FindStringSubmatch(input); len(matches) == 4 {
		num, err := strconv.Atoi(matches[3])
		if err != nil {
			return "", "", 0, fmt.Errorf("invalid PR number: %s", matches[3])
		}
		return matches[1], matches[2], num, nil
	}

	// Try URL format
	// Normalize: add https:// if missing
	urlInput := input
	if !strings.HasPrefix(strings.ToLower(urlInput), "http://") && !strings.HasPrefix(strings.ToLower(urlInput), "https://") {
		urlInput = "https://" + urlInput
	}

	parsed, err := url.Parse(urlInput)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid PR identifier: %s", input)
	}

	// Parse path: /owner/repo/pull/123
	path := strings.Trim(parsed.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 4 && parts[2] == "pull" {
		num, err := strconv.Atoi(parts[3])
		if err != nil {
			return "", "", 0, fmt.Errorf("invalid PR number in URL: %s", parts[3])
		}
		return parts[0], parts[1], num, nil
	}

	return "", "", 0, fmt.Errorf("invalid PR identifier: %s (expected owner/repo#number or GitHub URL)", input)
}
