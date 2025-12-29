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

	// M3 Write tools (session-based, for future implementation)
	s.registerTriageFindingTool()
	s.registerPostCommentTool()
	s.registerReplyToThreadTool()
	s.registerResolveThreadTool()
}

// Tool input/output types for M2 PR-based tools.

// ListAnnotationsInput is the input for the list_annotations tool.
type ListAnnotationsInput struct {
	Owner     string  `json:"owner" jsonschema:"description=Repository owner"`
	Repo      string  `json:"repo" jsonschema:"description=Repository name"`
	PRNumber  int     `json:"pr_number" jsonschema:"description=Pull request number"`
	CheckName *string `json:"check_name,omitempty" jsonschema:"description=Filter by check run name (e.g. 'code-reviewer')"`
	Level     *string `json:"level,omitempty" jsonschema:"description=Filter by annotation level,enum=notice,enum=warning,enum=failure"`
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
	Owner      string `json:"owner" jsonschema:"description=Repository owner"`
	Repo       string `json:"repo" jsonschema:"description=Repository name"`
	CheckRunID int64  `json:"check_run_id" jsonschema:"description=GitHub check run ID"`
	Index      int    `json:"index" jsonschema:"description=Annotation index (0-based)"`
}

// GetAnnotationOutput is the output for the get_annotation tool.
type GetAnnotationOutput struct {
	Annotation AnnotationOutput `json:"annotation"`
	Message    string           `json:"message"`
}

// ListFindingsInput is the input for the list_findings tool.
type ListFindingsInput struct {
	Owner    string  `json:"owner" jsonschema:"description=Repository owner"`
	Repo     string  `json:"repo" jsonschema:"description=Repository name"`
	PRNumber int     `json:"pr_number" jsonschema:"description=Pull request number"`
	Severity *string `json:"severity,omitempty" jsonschema:"description=Filter by severity,enum=critical,enum=high,enum=medium,enum=low"`
	Category *string `json:"category,omitempty" jsonschema:"description=Filter by category"`
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
	Owner     string `json:"owner" jsonschema:"description=Repository owner"`
	Repo      string `json:"repo" jsonschema:"description=Repository name"`
	PRNumber  int    `json:"pr_number" jsonschema:"description=Pull request number"`
	FindingID string `json:"finding_id" jsonschema:"description=Finding ID (fingerprint or comment ID)"`
}

// GetFindingOutput is the output for the get_finding tool.
type GetFindingOutput struct {
	Finding PRFindingOutput `json:"finding"`
	Message string          `json:"message"`
}

// GetSuggestionInput is the input for the get_suggestion tool.
type GetSuggestionInput struct {
	Owner     string `json:"owner" jsonschema:"description=Repository owner"`
	Repo      string `json:"repo" jsonschema:"description=Repository name"`
	PRNumber  int    `json:"pr_number" jsonschema:"description=Pull request number"`
	FindingID string `json:"finding_id" jsonschema:"description=Finding ID (fingerprint or comment ID)"`
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
	Owner        string `json:"owner" jsonschema:"description=Repository owner"`
	Repo         string `json:"repo" jsonschema:"description=Repository name"`
	PRNumber     int    `json:"pr_number" jsonschema:"description=Pull request number"`
	File         string `json:"file" jsonschema:"description=File path"`
	StartLine    int    `json:"start_line" jsonschema:"description=Start line (1-based)"`
	EndLine      int    `json:"end_line" jsonschema:"description=End line (1-based)"`
	ContextLines int    `json:"context_lines,omitempty" jsonschema:"description=Lines of context before and after (default: 3)"`
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
	Owner     string `json:"owner" jsonschema:"description=Repository owner"`
	Repo      string `json:"repo" jsonschema:"description=Repository name"`
	PRNumber  int    `json:"pr_number" jsonschema:"description=Pull request number"`
	File      string `json:"file" jsonschema:"description=File path"`
	StartLine int    `json:"start_line" jsonschema:"description=Start line (1-based, in new file)"`
	EndLine   int    `json:"end_line" jsonschema:"description=End line (1-based, in new file)"`
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

// Legacy M3 write tool types (kept for backward compatibility)

// TriageFindingInput is the input for the triage_finding tool.
type TriageFindingInput struct {
	SessionID string `json:"session_id" jsonschema:"description=Triage session ID"`
	FindingID string `json:"finding_id" jsonschema:"description=Finding ID to triage"`
	Status    string `json:"status" jsonschema:"description=New triage status,enum=accepted,enum=disputed,enum=question,enum=resolved,enum=wont_fix"`
	Reason    string `json:"reason,omitempty" jsonschema:"description=Reason for the triage decision"`
}

// TriageFindingOutput is the output for the triage_finding tool.
type TriageFindingOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// PostCommentInput is the input for the post_comment tool.
type PostCommentInput struct {
	SessionID string `json:"session_id" jsonschema:"description=Triage session ID"`
	FindingID string `json:"finding_id" jsonschema:"description=Finding ID to comment on"`
	Body      string `json:"body" jsonschema:"description=Comment body (markdown supported)"`
}

// PostCommentOutput is the output for the post_comment tool.
type PostCommentOutput struct {
	Success   bool   `json:"success"`
	CommentID int64  `json:"comment_id,omitempty"`
	Message   string `json:"message"`
}

// ReplyToThreadInput is the input for the reply_to_thread tool.
type ReplyToThreadInput struct {
	SessionID string `json:"session_id" jsonschema:"description=Triage session ID"`
	FindingID string `json:"finding_id" jsonschema:"description=Finding ID to reply to"`
	Body      string `json:"body" jsonschema:"description=Reply body (markdown supported)"`
}

// ReplyToThreadOutput is the output for the reply_to_thread tool.
type ReplyToThreadOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ResolveThreadInput is the input for the resolve_thread tool.
type ResolveThreadInput struct {
	SessionID string `json:"session_id" jsonschema:"description=Triage session ID"`
	FindingID string `json:"finding_id" jsonschema:"description=Finding ID whose thread to resolve"`
}

// ResolveThreadOutput is the output for the resolve_thread tool.
type ResolveThreadOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
