package mcp

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/delightfulhammers/bop/internal/adapter/llm/provider"
	"github.com/delightfulhammers/bop/internal/auth"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/delightfulhammers/bop/internal/usecase/triage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Auth-related errors for platform mode.
var (
	// ErrNotAuthenticated indicates the user hasn't logged in with the platform.
	ErrNotAuthenticated = errors.New("not authenticated - run 'bop auth login' first")

	// ErrAuthExpired indicates the authentication token has expired.
	ErrAuthExpired = errors.New("authentication expired - run 'bop auth login' to re-authenticate")
)

// PRReviewer defines the interface for invoking code reviews on GitHub PRs.
// This is implemented by review.Orchestrator.
type PRReviewer interface {
	ReviewPR(ctx context.Context, req review.PRRequest) (review.Result, error)
}

// FindingPoster defines the interface for posting findings to GitHub PRs.
// This is implemented by github.Poster.
type FindingPoster interface {
	PostReview(ctx context.Context, req review.GitHubPostRequest) (*review.GitHubPostResult, error)
}

// PRMetadataFetcher fetches PR metadata for the post_findings tool.
type PRMetadataFetcher interface {
	GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error)
	GetPRDiff(ctx context.Context, owner, repo string, prNumber int) (domain.Diff, error)
}

// BranchReviewer reviews local git branches (for review_branch tool).
// This is implemented by review.Orchestrator.
type BranchReviewer interface {
	ReviewBranch(ctx context.Context, req review.BranchRequest) (review.Result, error)
}

// GitEngine provides git operations for branch reviews.
// This is implemented by git.Engine.
type GitEngine interface {
	review.GitEngine
}

// Merger merges findings from multiple providers.
// This is implemented by merge.IntelligentMerger.
type Merger interface {
	review.Merger
}

const (
	// ServerName is the name reported to MCP clients.
	ServerName = "code-reviewer-triage"

	// ServerVersion is the version reported to MCP clients.
	ServerVersion = "0.6.0"
)

// ServerDeps contains the dependencies for the MCP server.
type ServerDeps struct {
	// PRService is the PR-based triage service (M2 scope).
	PRService *triage.PRService

	// TriageService is the session-based service (deprecated, for M3 write ops).
	TriageService triage.TriageService

	// PRReviewer invokes code reviews on GitHub PRs.
	// Optional: only required for review_pr tool.
	PRReviewer PRReviewer

	// FindingPoster posts findings to GitHub PRs.
	// Optional: only required for post_findings tool.
	FindingPoster FindingPoster

	// RemoteGitHubClient fetches PR metadata and diffs.
	// Optional: only required for post_findings tool.
	RemoteGitHubClient PRMetadataFetcher

	// BranchReviewer reviews local git branches.
	// Optional: only required for review_branch tool when direct API keys are available.
	BranchReviewer BranchReviewer

	// === Provider Factory and Dependencies ===
	// These support per-request provider creation with sampling fallback.

	// ProviderFactory creates LLM providers (direct or sampling-based).
	// Used when BranchReviewer/PRReviewer is nil to create per-request orchestrators.
	ProviderFactory *provider.Factory

	// Git provides git operations for branch/PR reviews.
	Git GitEngine

	// Merger merges findings from multiple providers.
	Merger Merger

	// ReviewerRegistry provides reviewer configurations for persona support.
	ReviewerRegistry review.ReviewerRegistry

	// PersonaPromptBuilder builds prompts for reviewer personas.
	// Required for per-request orchestrators.
	PersonaPromptBuilder *review.PersonaPromptBuilder

	// SeedGenerator generates deterministic seeds for reproducible reviews.
	// Required for per-request orchestrators.
	SeedGenerator review.SeedFunc

	// === Platform Authentication (Week 14) ===

	// AuthClient is the platform auth-service client (for token refresh).
	// Optional: only needed if tokens need refreshing.
	AuthClient *auth.Client

	// TokenStore loads and stores auth tokens.
	// Optional: only needed for platform authentication.
	TokenStore *auth.TokenStore

	// PlatformMode indicates if platform authentication is enabled.
	// When true and auth is invalid, tools may return auth errors.
	PlatformMode bool
}

// Server wraps the MCP server and provides triage tools.
type Server struct {
	mcpServer *mcp.Server
	deps      ServerDeps

	// auth is the loaded platform authentication context.
	// May be nil if not in platform mode or user is not logged in.
	auth *auth.StoredAuth
}

// NewServer creates a new MCP server with triage tools registered.
func NewServer(deps ServerDeps) *Server {
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    ServerName,
			Version: ServerVersion,
		},
		nil, // No additional options for now
	)

	s := &Server{
		mcpServer: mcpServer,
		deps:      deps,
	}

	// Load platform auth if available (Week 14)
	s.loadAuth()

	s.registerTools()

	return s
}

// loadAuth loads the platform authentication context if configured.
// Called once at server startup. Logs warnings but doesn't fail startup.
func (s *Server) loadAuth() {
	if s.deps.TokenStore == nil {
		return // Not in platform mode
	}

	stored, err := s.deps.TokenStore.Load()
	if err != nil {
		if errors.Is(err, auth.ErrNotLoggedIn) {
			// Not logged in - this is OK, user will get errors if they try
			// operations that require auth in platform mode
			return
		}
		// Other errors (corruption, parse failure) - warn user
		log.Printf("[WARN] Failed to load auth file (corrupt?): %v - try 'bop auth logout' to reset", err)
		return
	}

	// Check if token needs refresh
	if stored.NeedsRefresh() && s.deps.AuthClient != nil {
		// Validate required fields before attempting refresh
		if stored.RefreshToken == "" || stored.TenantID == "" {
			log.Printf("[WARN] Cannot refresh token: missing refresh_token or tenant_id - try 'bop auth logout' then 'bop auth login'")
		} else {
			// Try to refresh - best effort, don't fail if it doesn't work
			ctx := context.Background()
			newTokens, err := s.deps.AuthClient.RefreshToken(ctx, stored.TenantID, stored.RefreshToken)
			if err != nil {
				// Log refresh failure for debugging (without exposing tokens)
				log.Printf("[WARN] Token refresh failed: %v - auth may expire soon", err)
			} else if newTokens != nil {
				// Capture previous state for rollback on save failure
				prevAccessToken := stored.AccessToken
				prevRefreshToken := stored.RefreshToken
				prevExpiresAt := stored.ExpiresAt

				// Update stored auth with new tokens
				stored.AccessToken = newTokens.AccessToken
				// Only update refresh token if provided (some OAuth implementations don't rotate)
				if newTokens.RefreshToken != "" {
					stored.RefreshToken = newTokens.RefreshToken
				}

				// Calculate expiry time from ExpiresIn (seconds)
				// Validate ExpiresIn > 0 to avoid immediately-expired tokens
				if newTokens.ExpiresIn > 0 {
					stored.ExpiresAt = time.Now().Add(time.Duration(newTokens.ExpiresIn) * time.Second)
				} else {
					log.Printf("[WARN] Token refresh returned invalid ExpiresIn=%d, keeping existing expiry", newTokens.ExpiresIn)
				}

				// Save the refreshed tokens
				if saveErr := s.deps.TokenStore.Save(stored); saveErr != nil {
					log.Printf("[ERROR] Failed to save refreshed tokens: %v - reverting to previous state", saveErr)
					// Revert in-memory state to avoid inconsistency with disk
					stored.AccessToken = prevAccessToken
					stored.RefreshToken = prevRefreshToken
					stored.ExpiresAt = prevExpiresAt
				}
			}
		}
	}

	if !stored.IsExpired() {
		s.auth = stored
	}
}

// Auth returns the loaded platform authentication context.
// Returns nil if not authenticated or not in platform mode.
func (s *Server) Auth() *auth.StoredAuth {
	return s.auth
}

// RequireAuth checks if authentication is required and available.
// In platform mode, returns an error if user is not authenticated.
// In legacy mode (non-platform), always returns nil.
func (s *Server) RequireAuth() error {
	if !s.deps.PlatformMode {
		return nil // Legacy mode doesn't require platform auth
	}
	if s.auth == nil {
		return ErrNotAuthenticated
	}
	if s.auth.IsExpired() {
		return ErrAuthExpired
	}
	return nil
}

// UserID returns the authenticated user's ID, or empty string if not authenticated.
func (s *Server) UserID() string {
	if s.auth == nil {
		return ""
	}
	return s.auth.User.ID
}

// TenantID returns the authenticated user's tenant ID, or empty string if not authenticated.
func (s *Server) TenantID() string {
	if s.auth == nil {
		return ""
	}
	return s.auth.TenantID
}

// Run starts the MCP server on stdio transport.
// This blocks until the context is cancelled or an error occurs.
func (s *Server) Run(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcp.StdioTransport{})
}

// registerTools registers all triage MCP tools with the server.
func (s *Server) registerTools() {
	// M2 Read-only tools (PR-based, stateless)
	s.registerListAnnotationsTool()
	s.registerGetAnnotationTool()
	s.registerListFindingsTool()
	s.registerGetFindingTool()
	s.registerGetSuggestionTool()
	s.registerGetCodeContextTool()
	s.registerGetDiffContextTool()

	// M3 Write tools (PR-based, stateless)
	s.registerGetThreadTool()
	s.registerReplyToFindingTool()
	s.registerPostCommentTool()
	s.registerMarkResolvedTool()
	s.registerRequestRereviewTool()

	// Phase 3.5d Review tools
	s.registerEditFindingTool()
	s.registerReviewPRTool()
	s.registerPostFindingsTool()
	s.registerReviewBranchTool()
	s.registerReviewFilesTool()
}

// Tool input/output types for M2 PR-based tools.

// ListAnnotationsInput is the input for the list_annotations tool.
type ListAnnotationsInput struct {
	Owner     string  `json:"owner" jsonschema:"Repository owner"`
	Repo      string  `json:"repo" jsonschema:"Repository name"`
	PRNumber  int     `json:"pr_number" jsonschema:"Pull request number"`
	CheckName *string `json:"check_name,omitempty" jsonschema:"Filter by check run name (e.g. 'code-reviewer')"`
	Level     *string `json:"level,omitempty" jsonschema:"Filter by annotation level (notice, warning, failure)"`
}

// ListAnnotationsOutput is the output for the list_annotations tool.
type ListAnnotationsOutput struct {
	Annotations []AnnotationOutput `json:"annotations"`
	Total       int                `json:"total"`
}

// AnnotationOutput represents an annotation in tool output.
type AnnotationOutput struct {
	CheckRunID int64  `json:"check_run_id"`
	Index      int    `json:"index"`
	Path       string `json:"path"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Level      string `json:"level"`
	Message    string `json:"message"`
	Title      string `json:"title,omitempty"`
}

// GetAnnotationInput is the input for the get_annotation tool.
type GetAnnotationInput struct {
	Owner      string `json:"owner" jsonschema:"Repository owner"`
	Repo       string `json:"repo" jsonschema:"Repository name"`
	CheckRunID int64  `json:"check_run_id" jsonschema:"GitHub check run ID"`
	Index      int    `json:"index" jsonschema:"Annotation index (0-based)"`
}

// GetAnnotationOutput is the output for the get_annotation tool.
type GetAnnotationOutput struct {
	Annotation AnnotationOutput `json:"annotation"`
	Message    string           `json:"message"`
}

// ListFindingsInput is the input for the list_findings tool.
type ListFindingsInput struct {
	Owner       string  `json:"owner" jsonschema:"Repository owner"`
	Repo        string  `json:"repo" jsonschema:"Repository name"`
	PRNumber    int     `json:"pr_number" jsonschema:"Pull request number"`
	Severity    *string `json:"severity,omitempty" jsonschema:"Filter by severity (critical, high, medium, low)"`
	Category    *string `json:"category,omitempty" jsonschema:"Filter by category"`
	ReplyStatus *string `json:"reply_status,omitempty" jsonschema:"Filter by reply status (all, replied, unreplied)"`
}

// ListFindingsOutput is the output for the list_findings tool.
type ListFindingsOutput struct {
	Findings []PRFindingOutput `json:"findings"`
	Total    int               `json:"total"`
	Summary  *FindingsSummary  `json:"summary,omitempty"`
}

// FindingsSummary provides triage progress statistics.
type FindingsSummary struct {
	Total         int            `json:"total"`
	Replied       int            `json:"replied"`
	Unreplied     int            `json:"unreplied"`
	InDiff        int            `json:"in_diff"`     // Findings with diff position (review comments)
	OutOfDiff     int            `json:"out_of_diff"` // Findings without diff position (issue comments)
	BySeverity    map[string]int `json:"by_severity"`
	TriagePercent float64        `json:"triage_percent"`
}

// PRFindingOutput represents a PR comment finding in tool output.
type PRFindingOutput struct {
	CommentID    int64   `json:"comment_id"`
	Fingerprint  string  `json:"fingerprint,omitempty"`
	Path         string  `json:"path"`
	Line         int     `json:"line"`
	Severity     string  `json:"severity,omitempty"`
	Category     string  `json:"category,omitempty"`
	Body         string  `json:"body"`
	Author       string  `json:"author"`
	IsResolved   bool    `json:"is_resolved"`
	ReplyCount   int     `json:"reply_count"`
	HasReply     bool    `json:"has_reply"`
	LastReplyAt  *string `json:"last_reply_at,omitempty"`
	LastReplyBy  string  `json:"last_reply_by,omitempty"`
	ThreadStatus string  `json:"thread_status"`
	IsOutOfDiff  bool    `json:"is_out_of_diff"` // True if finding is from an issue comment (outside PR diff)
}

// GetFindingInput is the input for the get_finding tool.
type GetFindingInput struct {
	Owner     string `json:"owner" jsonschema:"Repository owner"`
	Repo      string `json:"repo" jsonschema:"Repository name"`
	PRNumber  int    `json:"pr_number" jsonschema:"Pull request number"`
	FindingID string `json:"finding_id" jsonschema:"Finding ID (fingerprint or comment ID)"`
}

// GetFindingOutput is the output for the get_finding tool.
type GetFindingOutput struct {
	Finding PRFindingOutput `json:"finding"`
	Message string          `json:"message"`
}

// GetSuggestionInput is the input for the get_suggestion tool.
type GetSuggestionInput struct {
	Owner     string `json:"owner" jsonschema:"Repository owner"`
	Repo      string `json:"repo" jsonschema:"Repository name"`
	PRNumber  int    `json:"pr_number" jsonschema:"Pull request number"`
	FindingID string `json:"finding_id" jsonschema:"Finding ID (fingerprint or comment ID)"`
}

// GetSuggestionOutput is the output for the get_suggestion tool.
type GetSuggestionOutput struct {
	File        string `json:"file"`
	OldCode     string `json:"old_code"`
	NewCode     string `json:"new_code"`
	Explanation string `json:"explanation,omitempty"`
	Message     string `json:"message"`
}

// GetCodeContextInput is the input for the get_code_context tool.
type GetCodeContextInput struct {
	Owner        string `json:"owner" jsonschema:"Repository owner"`
	Repo         string `json:"repo" jsonschema:"Repository name"`
	PRNumber     int    `json:"pr_number" jsonschema:"Pull request number"`
	File         string `json:"file" jsonschema:"File path"`
	StartLine    int    `json:"start_line" jsonschema:"Start line (1-based)"`
	EndLine      int    `json:"end_line" jsonschema:"End line (1-based)"`
	ContextLines int    `json:"context_lines,omitempty" jsonschema:"Lines of context before and after (default: 3)"`
}

// GetCodeContextOutput is the output for the get_code_context tool.
type GetCodeContextOutput struct {
	File          string `json:"file"`
	Ref           string `json:"ref"`
	StartLine     int    `json:"start_line"`
	EndLine       int    `json:"end_line"`
	Content       string `json:"content"`
	ContextBefore int    `json:"context_before"`
	ContextAfter  int    `json:"context_after"`
	Message       string `json:"message"`
}

// GetDiffContextInput is the input for the get_diff_context tool.
type GetDiffContextInput struct {
	Owner     string `json:"owner" jsonschema:"Repository owner"`
	Repo      string `json:"repo" jsonschema:"Repository name"`
	PRNumber  int    `json:"pr_number" jsonschema:"Pull request number"`
	File      string `json:"file" jsonschema:"File path"`
	StartLine int    `json:"start_line" jsonschema:"Start line (1-based, in new file)"`
	EndLine   int    `json:"end_line" jsonschema:"End line (1-based, in new file)"`
}

// GetDiffContextOutput is the output for the get_diff_context tool.
type GetDiffContextOutput struct {
	File        string `json:"file"`
	BaseBranch  string `json:"base_branch"`
	TargetRef   string `json:"target_ref"`
	HunkContent string `json:"hunk_content"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Message     string `json:"message"`
}

// =============================================================================
// M3 Write Tool Types (PR-based, stateless)
// =============================================================================

// ReplyToFindingInput is the input for the reply_to_finding tool.
type ReplyToFindingInput struct {
	Owner     string  `json:"owner" jsonschema:"Repository owner"`
	Repo      string  `json:"repo" jsonschema:"Repository name"`
	PRNumber  int     `json:"pr_number" jsonschema:"Pull request number"`
	FindingID string  `json:"finding_id" jsonschema:"Finding ID (fingerprint or comment ID)"`
	Body      string  `json:"body" jsonschema:"Reply body (markdown supported)"`
	Status    *string `json:"status,omitempty" jsonschema:"Optional status tag: acknowledged, disputed, fixed, wont_fix"`
}

// ReplyToFindingOutput is the output for the reply_to_finding tool.
type ReplyToFindingOutput struct {
	Success   bool   `json:"success"`
	CommentID int64  `json:"comment_id"`
	Message   string `json:"message"`
}

// PostCommentInput is the input for the post_comment tool.
type PostCommentInput struct {
	Owner    string `json:"owner" jsonschema:"Repository owner"`
	Repo     string `json:"repo" jsonschema:"Repository name"`
	PRNumber int    `json:"pr_number" jsonschema:"Pull request number"`
	File     string `json:"file" jsonschema:"File path to comment on"`
	Line     int    `json:"line" jsonschema:"Line number (1-based)"`
	Body     string `json:"body" jsonschema:"Comment body (markdown supported)"`
}

// PostCommentOutput is the output for the post_comment tool.
type PostCommentOutput struct {
	Success   bool   `json:"success"`
	CommentID int64  `json:"comment_id"`
	Message   string `json:"message"`
}

// MarkResolvedInput is the input for the mark_resolved tool.
type MarkResolvedInput struct {
	Owner    string `json:"owner" jsonschema:"Repository owner"`
	Repo     string `json:"repo" jsonschema:"Repository name"`
	ThreadID string `json:"thread_id" jsonschema:"Thread node ID (e.g., PRRT_kwDO...)"`
	Resolved bool   `json:"resolved" jsonschema:"True to resolve, false to unresolve"`
}

// MarkResolvedOutput is the output for the mark_resolved tool.
type MarkResolvedOutput struct {
	Success  bool   `json:"success"`
	Resolved bool   `json:"resolved"`
	Message  string `json:"message"`
}

// RequestRereviewInput is the input for the request_rereview tool.
type RequestRereviewInput struct {
	Owner         string   `json:"owner" jsonschema:"Repository owner"`
	Repo          string   `json:"repo" jsonschema:"Repository name"`
	PRNumber      int      `json:"pr_number" jsonschema:"Pull request number"`
	DismissStale  bool     `json:"dismiss_stale" jsonschema:"Dismiss stale bot reviews before requesting"`
	Reviewers     []string `json:"reviewers,omitempty" jsonschema:"User logins to request review from"`
	TeamReviewers []string `json:"team_reviewers,omitempty" jsonschema:"Team slugs to request review from"`
	Message       string   `json:"message,omitempty" jsonschema:"Message for dismissing stale reviews"`
}

// RequestRereviewOutput is the output for the request_rereview tool.
type RequestRereviewOutput struct {
	Success          bool   `json:"success"`
	ReviewsDismissed int    `json:"reviews_dismissed"`
	ReviewsRequested int    `json:"reviews_requested"`
	Message          string `json:"message"`
}

// GetThreadInput is the input for the get_thread tool.
type GetThreadInput struct {
	Owner     string `json:"owner" jsonschema:"Repository owner"`
	Repo      string `json:"repo" jsonschema:"Repository name"`
	PRNumber  int    `json:"pr_number,omitempty" jsonschema:"PR number (required for thread_id lookup)"`
	CommentID int64  `json:"comment_id" jsonschema:"Comment ID to get thread for"`
}

// GetThreadOutput is the output for the get_thread tool.
type GetThreadOutput struct {
	CommentID  int64                 `json:"comment_id"`
	ThreadID   string                `json:"thread_id,omitempty"` // GraphQL node ID (PRRT_...) for use with mark_resolved
	IsResolved bool                  `json:"is_resolved"`
	Comments   []ThreadCommentOutput `json:"comments"`
	Total      int                   `json:"total"`
	Message    string                `json:"message"`
}

// ThreadCommentOutput represents a comment in a thread.
type ThreadCommentOutput struct {
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	IsReply   bool   `json:"is_reply"`
}
