package mcp

import (
	"context"

	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
}

// Server wraps the MCP server and provides triage tools.
type Server struct {
	mcpServer *mcp.Server
	deps      ServerDeps
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

	s.registerTools()

	return s
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
	Owner    string  `json:"owner" jsonschema:"Repository owner"`
	Repo     string  `json:"repo" jsonschema:"Repository name"`
	PRNumber int     `json:"pr_number" jsonschema:"Pull request number"`
	Severity *string `json:"severity,omitempty" jsonschema:"Filter by severity (critical, high, medium, low)"`
	Category *string `json:"category,omitempty" jsonschema:"Filter by category"`
}

// ListFindingsOutput is the output for the list_findings tool.
type ListFindingsOutput struct {
	Findings []PRFindingOutput `json:"findings"`
	Total    int               `json:"total"`
}

// PRFindingOutput represents a PR comment finding in tool output.
type PRFindingOutput struct {
	CommentID    int64  `json:"comment_id"`
	Fingerprint  string `json:"fingerprint,omitempty"`
	Path         string `json:"path"`
	Line         int    `json:"line"`
	Severity     string `json:"severity,omitempty"`
	Category     string `json:"category,omitempty"`
	Body         string `json:"body"`
	Author       string `json:"author"`
	IsResolved   bool   `json:"is_resolved"`
	ReplyCount   int    `json:"reply_count"`
	ThreadStatus string `json:"thread_status"`
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
