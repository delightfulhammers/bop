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
	ServerVersion = "0.5.0"
)

// ServerDeps contains the dependencies for the MCP server.
type ServerDeps struct {
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
	// Read-only tools (M2 scope)
	s.registerStartSessionTool()
	s.registerGetCurrentFindingTool()
	s.registerGetFindingContextTool()
	s.registerListFindingsTool()
	s.registerGetProgressTool()

	// Write tools (M3 scope)
	s.registerTriageFindingTool()
	s.registerPostCommentTool()
	s.registerReplyToThreadTool()
	s.registerResolveThreadTool()
}

// Tool input/output types are defined below.
// The MCP SDK uses these structs to auto-generate JSON schemas.

// StartSessionInput is the input for the start_triage_session tool.
type StartSessionInput struct {
	Repository string `json:"repository" jsonschema:"description=Repository in owner/repo format"`
	PRNumber   int    `json:"pr_number" jsonschema:"description=Pull request number"`
}

// StartSessionOutput is the output for the start_triage_session tool.
type StartSessionOutput struct {
	SessionID    string `json:"session_id"`
	FindingCount int    `json:"finding_count"`
	Message      string `json:"message"`
}

// GetCurrentFindingInput is the input for the get_current_finding tool.
type GetCurrentFindingInput struct {
	SessionID string `json:"session_id" jsonschema:"description=Triage session ID"`
}

// FindingOutput represents a finding in tool output.
type FindingOutput struct {
	ID           string `json:"id"`
	File         string `json:"file"`
	LineStart    int    `json:"line_start"`
	LineEnd      int    `json:"line_end"`
	Severity     string `json:"severity"`
	Category     string `json:"category"`
	Description  string `json:"description"`
	Suggestion   string `json:"suggestion,omitempty"`
	TriageStatus string `json:"triage_status"`
}

// GetFindingContextInput is the input for the get_finding_context tool.
type GetFindingContextInput struct {
	SessionID string `json:"session_id" jsonschema:"description=Triage session ID"`
	FindingID string `json:"finding_id" jsonschema:"description=Finding ID to get context for"`
}

// ContextOutput represents context information in tool output.
type ContextOutput struct {
	PRTitle       string          `json:"pr_title"`
	PRDescription string          `json:"pr_description,omitempty"`
	PRAuthor      string          `json:"pr_author"`
	FileContent   string          `json:"file_content,omitempty"`
	ThreadHistory []CommentOutput `json:"thread_history,omitempty"`
}

// CommentOutput represents a comment in tool output.
type CommentOutput struct {
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// ListFindingsInput is the input for the list_findings tool.
type ListFindingsInput struct {
	SessionID    string  `json:"session_id" jsonschema:"description=Triage session ID"`
	StatusFilter *string `json:"status_filter,omitempty" jsonschema:"description=Filter by triage status,enum=pending,enum=accepted,enum=disputed,enum=question,enum=resolved,enum=wont_fix"`
}

// ListFindingsOutput is the output for the list_findings tool.
type ListFindingsOutput struct {
	Findings []FindingOutput `json:"findings"`
	Total    int             `json:"total"`
}

// GetProgressInput is the input for the get_progress tool.
type GetProgressInput struct {
	SessionID string `json:"session_id" jsonschema:"description=Triage session ID"`
}

// GetProgressOutput is the output for the get_progress tool.
type GetProgressOutput struct {
	Triaged int    `json:"triaged"`
	Total   int    `json:"total"`
	Percent int    `json:"percent"`
	Message string `json:"message"`
}

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
