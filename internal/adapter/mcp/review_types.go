package mcp

// =============================================================================
// Review Tool Types (Phase 3.5d - MCP Review Tools)
// =============================================================================

// FindingInput represents a finding in MCP tool input.
// This mirrors domain.Finding but uses MCP-friendly field names and tags.
type FindingInput struct {
	ID             string  `json:"id,omitempty"`
	Fingerprint    string  `json:"fingerprint,omitempty" jsonschema:"Original fingerprint (preserves identity when passing between tools)"`
	File           string  `json:"file" jsonschema:"File path where the finding occurs"`
	LineStart      int     `json:"line_start" jsonschema:"Starting line number (1-based)"`
	LineEnd        int     `json:"line_end" jsonschema:"Ending line number (1-based)"`
	Severity       string  `json:"severity" jsonschema:"Severity level (critical, high, medium, low)"`
	Category       string  `json:"category" jsonschema:"Finding category (security, bug, maintainability, etc.)"`
	Description    string  `json:"description" jsonschema:"Description of the issue"`
	Suggestion     string  `json:"suggestion,omitempty" jsonschema:"Suggested fix or remediation"`
	ReviewerName   string  `json:"reviewer_name,omitempty" jsonschema:"Name of the reviewer that found this issue"`
	ReviewerWeight float64 `json:"reviewer_weight,omitempty" jsonschema:"Weight of the reviewer (for merge prioritization)"`
}

// FindingOutput represents a finding in MCP tool output.
// Includes the fingerprint for deduplication.
type FindingOutput struct {
	ID             string  `json:"id"`
	Fingerprint    string  `json:"fingerprint"`
	File           string  `json:"file"`
	LineStart      int     `json:"line_start"`
	LineEnd        int     `json:"line_end"`
	Severity       string  `json:"severity"`
	Category       string  `json:"category"`
	Description    string  `json:"description"`
	Suggestion     string  `json:"suggestion,omitempty"`
	ReviewerName   string  `json:"reviewer_name,omitempty"`
	ReviewerWeight float64 `json:"reviewer_weight,omitempty"`
}

// =============================================================================
// edit_finding Tool Types
// =============================================================================

// EditFindingInput is the input for the edit_finding tool.
type EditFindingInput struct {
	Finding     FindingInput `json:"finding" jsonschema:"The finding to edit"`
	Severity    *string      `json:"severity,omitempty" jsonschema:"New severity (critical, high, medium, low)"`
	Category    *string      `json:"category,omitempty" jsonschema:"New category"`
	Description *string      `json:"description,omitempty" jsonschema:"New description"`
	Suggestion  *string      `json:"suggestion,omitempty" jsonschema:"New suggestion"`
}

// EditFindingOutput is the output for the edit_finding tool.
type EditFindingOutput struct {
	Finding        FindingOutput `json:"finding"`
	FieldsModified []string      `json:"fields_modified"`
	Message        string        `json:"message"`
}

// =============================================================================
// review_pr Tool Types
// =============================================================================

// ReviewPRInput is the input for the review_pr tool.
type ReviewPRInput struct {
	Owner     string   `json:"owner" jsonschema:"Repository owner"`
	Repo      string   `json:"repo" jsonschema:"Repository name"`
	PRNumber  int      `json:"pr_number" jsonschema:"Pull request number"`
	Reviewers []string `json:"reviewers,omitempty" jsonschema:"Specific reviewers to use (defaults to cr.yaml configuration)"`
}

// ReviewPROutput is the output for the review_pr tool.
type ReviewPROutput struct {
	Findings      []FindingOutput   `json:"findings"`
	Summary       string            `json:"summary"`
	TotalFindings int               `json:"total_findings"`
	BySeverity    map[string]int    `json:"by_severity"`
	ByCategory    map[string]int    `json:"by_category"`
	ReviewerStats []ReviewerStat    `json:"reviewer_stats,omitempty"`
	TokensIn      int               `json:"tokens_in"`
	TokensOut     int               `json:"tokens_out"`
	Cost          float64           `json:"cost"`
	Message       string            `json:"message"`
	Metadata      *ReviewPRMetadata `json:"metadata,omitempty"`
}

// ReviewerStat captures per-reviewer statistics.
type ReviewerStat struct {
	Name     string `json:"name"`
	Findings int    `json:"findings"`
}

// ReviewPRMetadata captures PR metadata for context.
type ReviewPRMetadata struct {
	Title      string   `json:"title"`
	Author     string   `json:"author"`
	BaseRef    string   `json:"base_ref"`
	HeadRef    string   `json:"head_ref"`
	HeadSHA    string   `json:"head_sha"`
	FilesCount int      `json:"files_count"`
	Additions  int      `json:"additions"`
	Deletions  int      `json:"deletions"`
	Labels     []string `json:"labels,omitempty"`
}

// =============================================================================
// post_findings Tool Types
// =============================================================================

// PostFindingsInput is the input for the post_findings tool.
type PostFindingsInput struct {
	Owner            string         `json:"owner" jsonschema:"Repository owner"`
	Repo             string         `json:"repo" jsonschema:"Repository name"`
	PRNumber         int            `json:"pr_number" jsonschema:"Pull request number"`
	Findings         []FindingInput `json:"findings" jsonschema:"Findings to post as review comments"`
	SkipDuplicates   bool           `json:"skip_duplicates,omitempty" jsonschema:"Skip findings that already exist (based on fingerprint)"`
	DryRun           bool           `json:"dry_run,omitempty" jsonschema:"Show what would be posted without actually posting"`
	ReviewAction     *string        `json:"review_action,omitempty" jsonschema:"GitHub review action (COMMENT, REQUEST_CHANGES, APPROVE). Defaults based on severity."`
	IncludeSummary   bool           `json:"include_summary,omitempty" jsonschema:"Include a summary comment at the end of the review"`
	BlockingFindings []string       `json:"blocking_findings,omitempty" jsonschema:"Fingerprints of findings that should block the PR"`
}

// PostFindingsOutput is the output for the post_findings tool.
type PostFindingsOutput struct {
	Success            bool     `json:"success"`
	Posted             int      `json:"posted"`
	Skipped            int      `json:"skipped"`
	SkippedDuplicates  int      `json:"skipped_duplicates"`
	ReviewID           int64    `json:"review_id,omitempty"`
	ReviewAction       string   `json:"review_action"`
	PostedFingerprints []string `json:"posted_fingerprints,omitempty"`
	Message            string   `json:"message"`
}

// =============================================================================
// review_branch Tool Types
// =============================================================================

// ReviewBranchInput is the input for the review_branch tool.
type ReviewBranchInput struct {
	BaseRef            string   `json:"base_ref" jsonschema:"Base ref to diff against (e.g. main),required"`
	TargetRef          string   `json:"target_ref,omitempty" jsonschema:"Target branch to review (defaults to current branch)"`
	IncludeUncommitted bool     `json:"include_uncommitted,omitempty" jsonschema:"Include uncommitted working tree changes in the review"`
	Reviewers          []string `json:"reviewers,omitempty" jsonschema:"Specific reviewers to use (from cr.yaml config)"`
	RepoDir            string   `json:"repo_dir,omitempty" jsonschema:"Repository directory to review (defaults to server working directory)"`
}

// ReviewBranchOutput is the output for the review_branch tool.
type ReviewBranchOutput struct {
	Findings      []FindingOutput `json:"findings"`
	Summary       string          `json:"summary"`
	TotalFindings int             `json:"total_findings"`
	BySeverity    map[string]int  `json:"by_severity"`
	ByCategory    map[string]int  `json:"by_category"`
	ReviewerStats []ReviewerStat  `json:"reviewer_stats,omitempty"`
	TokensIn      int             `json:"tokens_in"`
	TokensOut     int             `json:"tokens_out"`
	Cost          float64         `json:"cost"`
	Message       string          `json:"message"`
	BaseRef       string          `json:"base_ref"`
	TargetRef     string          `json:"target_ref"`
}

// =============================================================================
// review_files Tool Types
// =============================================================================

// ReviewFilesInput is the input for the review_files tool.
// This tool reviews files in a directory without requiring git.
type ReviewFilesInput struct {
	Path      string   `json:"path" jsonschema:"Directory to review,required"`
	Patterns  []string `json:"patterns,omitempty" jsonschema:"Glob patterns to include (e.g., **/*.go, **/*.ts). If empty, reviews all files."`
	Exclude   []string `json:"exclude,omitempty" jsonschema:"Glob patterns to exclude (e.g., *_test.go, vendor/**)"`
	Reviewers []string `json:"reviewers,omitempty" jsonschema:"Specific reviewers to use (from cr.yaml config)"`
}

// ReviewFilesOutput is the output for the review_files tool.
type ReviewFilesOutput struct {
	Findings      []FindingOutput `json:"findings"`
	Summary       string          `json:"summary"`
	TotalFindings int             `json:"total_findings"`
	BySeverity    map[string]int  `json:"by_severity"`
	ByCategory    map[string]int  `json:"by_category"`
	ReviewerStats []ReviewerStat  `json:"reviewer_stats,omitempty"`
	TokensIn      int             `json:"tokens_in"`
	TokensOut     int             `json:"tokens_out"`
	Cost          float64         `json:"cost"`
	Message       string          `json:"message"`
	Path          string          `json:"path"`
	FilesReviewed int             `json:"files_reviewed"`
}

// =============================================================================
// Severity and Category Constants
// =============================================================================

// ValidSeverities contains the valid severity levels.
var ValidSeverities = []string{"critical", "high", "medium", "low"}

// isValidSeverity checks if a severity is valid.
func isValidSeverity(severity string) bool {
	for _, s := range ValidSeverities {
		if severity == s {
			return true
		}
	}
	return false
}
