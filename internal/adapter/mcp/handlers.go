package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ValidStatusTags contains the allowed status tags for reply_to_finding.
// These are used to mark the disposition of a finding during triage.
var ValidStatusTags = []string{"acknowledged", "disputed", "fixed", "wont_fix"}

// isValidStatusTag checks if a status tag is valid (case-insensitive).
func isValidStatusTag(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	for _, valid := range ValidStatusTags {
		if normalized == valid {
			return true
		}
	}
	return false
}

// notImplementedResult returns a standard "not implemented" MCP result with IsError=true.
func notImplementedResult(scope string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Tool not yet implemented (%s scope)", scope)},
		},
	}
}

// validateLineRange validates PR number and line range inputs.
// Returns an error result if validation fails, nil otherwise.
func validateLineRange(prNumber, startLine, endLine int) *mcp.CallToolResult {
	if prNumber <= 0 {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Invalid PR number: %d (must be positive)", prNumber)},
			},
		}
	}
	if startLine <= 0 {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Invalid start line: %d (must be positive)", startLine)},
			},
		}
	}
	if endLine < startLine {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Invalid line range: end (%d) < start (%d)", endLine, startLine)},
			},
		}
	}
	return nil
}

// M2 Read-only Tool Handlers (PR-based, stateless)

// registerListAnnotationsTool registers the list_annotations tool.
func (s *Server) registerListAnnotationsTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_annotations",
		Description: "List SARIF annotations for a pull request's head commit. Returns annotations from check runs.",
	}, s.handleListAnnotations)
}

func (s *Server) handleListAnnotations(ctx context.Context, req *mcp.CallToolRequest, input ListAnnotationsInput) (*mcp.CallToolResult, ListAnnotationsOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M2"), ListAnnotationsOutput{}, nil
	}

	// Convert level filter if provided
	var level *domain.AnnotationLevel
	if input.Level != nil {
		l := domain.AnnotationLevel(*input.Level)
		if !l.IsValid() {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("invalid level filter: %s (valid: notice, warning, failure)", *input.Level)},
				},
			}, ListAnnotationsOutput{}, nil
		}
		level = &l
	}

	annotations, err := s.deps.PRService.ListAnnotations(ctx, input.Owner, input.Repo, input.PRNumber, input.CheckName, level)
	if err != nil {
		return nil, ListAnnotationsOutput{}, fmt.Errorf("list annotations: %w", err)
	}

	output := ListAnnotationsOutput{
		Annotations: make([]AnnotationOutput, len(annotations)),
		Total:       len(annotations),
	}

	for i, ann := range annotations {
		output.Annotations[i] = annotationToOutput(ann)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d annotations", len(annotations))},
		},
	}, output, nil
}

// registerGetAnnotationTool registers the get_annotation tool.
func (s *Server) registerGetAnnotationTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_annotation",
		Description: "Get a single annotation by check run ID and index.",
	}, s.handleGetAnnotation)
}

func (s *Server) handleGetAnnotation(ctx context.Context, req *mcp.CallToolRequest, input GetAnnotationInput) (*mcp.CallToolResult, GetAnnotationOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M2"), GetAnnotationOutput{}, nil
	}

	annotation, err := s.deps.PRService.GetAnnotation(ctx, input.Owner, input.Repo, input.CheckRunID, input.Index)
	if err != nil {
		if errors.Is(err, triage.ErrAnnotationNotFound) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Annotation not found: check_run_id=%d, index=%d", input.CheckRunID, input.Index)},
				},
			}, GetAnnotationOutput{}, nil
		}
		return nil, GetAnnotationOutput{}, fmt.Errorf("get annotation: %w", err)
	}

	// Guard against nil annotation (defensive)
	if annotation == nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Annotation not found: check_run_id=%d, index=%d", input.CheckRunID, input.Index)},
			},
		}, GetAnnotationOutput{}, nil
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Annotation in %s (line %d): %s", annotation.Path, annotation.StartLine, annotation.Message)},
			},
		}, GetAnnotationOutput{
			Annotation: annotationToOutput(*annotation),
			Message:    "Annotation retrieved successfully",
		}, nil
}

// registerListFindingsTool registers the list_findings tool.
func (s *Server) registerListFindingsTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_findings",
		Description: "List PR comment findings (code review comments with fingerprints).",
	}, s.handleListFindings)
}

func (s *Server) handleListFindings(ctx context.Context, req *mcp.CallToolRequest, input ListFindingsInput) (*mcp.CallToolResult, ListFindingsOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M2"), ListFindingsOutput{}, nil
	}

	// Convert reply_status string to triage.ReplyStatus if provided
	// Normalize input at boundary: trim whitespace and lowercase
	var replyStatus *triage.ReplyStatus
	if input.ReplyStatus != nil {
		normalized := strings.ToLower(strings.TrimSpace(*input.ReplyStatus))
		rs := triage.ReplyStatus(normalized)
		replyStatus = &rs
	}

	findings, err := s.deps.PRService.ListFindings(ctx, input.Owner, input.Repo, input.PRNumber, input.Severity, input.Category, replyStatus)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M2 - CommentReader"), ListFindingsOutput{}, nil
		}
		if errors.Is(err, triage.ErrInvalidFilter) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
			}, ListFindingsOutput{}, nil
		}
		return nil, ListFindingsOutput{}, fmt.Errorf("list findings: %w", err)
	}

	output := ListFindingsOutput{
		Findings: make([]PRFindingOutput, len(findings)),
		Total:    len(findings),
		Summary:  computeFindingsSummary(findings),
	}

	for i, f := range findings {
		output.Findings[i] = findingToOutput(f)
	}

	// Build informative message with triage progress
	msg := fmt.Sprintf("Found %d findings", len(findings))
	if output.Summary != nil && output.Summary.Total > 0 {
		msg = fmt.Sprintf("Found %d findings (%d replied, %d unreplied, %.0f%% triaged)",
			output.Summary.Total, output.Summary.Replied, output.Summary.Unreplied, output.Summary.TriagePercent)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
	}, output, nil
}

// registerGetFindingTool registers the get_finding tool.
func (s *Server) registerGetFindingTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_finding",
		Description: "Get a single finding by ID (fingerprint or comment ID).",
	}, s.handleGetFinding)
}

func (s *Server) handleGetFinding(ctx context.Context, req *mcp.CallToolRequest, input GetFindingInput) (*mcp.CallToolResult, GetFindingOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M2"), GetFindingOutput{}, nil
	}

	finding, err := s.deps.PRService.GetFinding(ctx, input.Owner, input.Repo, input.PRNumber, input.FindingID)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M2 - CommentReader"), GetFindingOutput{}, nil
		}
		if errors.Is(err, triage.ErrCommentNotFound) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Finding not found: %s. The comment may have been deleted. Use list_findings to get current findings.", input.FindingID)},
				},
			}, GetFindingOutput{}, nil
		}
		return nil, GetFindingOutput{}, fmt.Errorf("get finding: %w", err)
	}

	// Guard against nil finding (should not happen, but defensive)
	if finding == nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Finding not found: %s", input.FindingID)},
			},
		}, GetFindingOutput{}, nil
	}

	// Safely truncate body for display (UTF-8 safe - truncate by runes, not bytes)
	bodyPreview := finding.Body
	bodyRunes := []rune(bodyPreview)
	if len(bodyRunes) > 100 {
		bodyPreview = string(bodyRunes[:100]) + "..."
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Finding in %s (line %d): %s", finding.Path, finding.Line, bodyPreview)},
			},
		}, GetFindingOutput{
			Finding: findingToOutput(*finding),
			Message: "Finding retrieved successfully",
		}, nil
}

// registerGetSuggestionTool registers the get_suggestion tool.
func (s *Server) registerGetSuggestionTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_suggestion",
		Description: "Extract structured code suggestion from a finding for use with str_replace.",
	}, s.handleGetSuggestion)
}

func (s *Server) handleGetSuggestion(ctx context.Context, req *mcp.CallToolRequest, input GetSuggestionInput) (*mcp.CallToolResult, GetSuggestionOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M2"), GetSuggestionOutput{}, nil
	}

	suggestion, err := s.deps.PRService.GetSuggestion(ctx, input.Owner, input.Repo, input.PRNumber, input.FindingID)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M2 - SuggestionExtractor"), GetSuggestionOutput{}, nil
		}
		if errors.Is(err, triage.ErrNoSuggestion) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("No structured suggestion found in finding %s. Use get_code_context to read the current code and craft a fix manually.", input.FindingID)},
				},
			}, GetSuggestionOutput{}, nil
		}
		return nil, GetSuggestionOutput{}, fmt.Errorf("get suggestion: %w", err)
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Suggestion for %s: replace %d chars with %d chars", suggestion.File, len(suggestion.OldCode), len(suggestion.NewCode))},
			},
		}, GetSuggestionOutput{
			File:        suggestion.File,
			OldCode:     suggestion.OldCode,
			NewCode:     suggestion.NewCode,
			Explanation: suggestion.Explanation,
			Message:     "Suggestion extracted successfully",
		}, nil
}

// registerGetCodeContextTool registers the get_code_context tool.
func (s *Server) registerGetCodeContextTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_code_context",
		Description: "Get file content at specific lines from the PR's head commit.",
	}, s.handleGetCodeContext)
}

func (s *Server) handleGetCodeContext(ctx context.Context, req *mcp.CallToolRequest, input GetCodeContextInput) (*mcp.CallToolResult, GetCodeContextOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M2"), GetCodeContextOutput{}, nil
	}

	// Validate inputs before calling service
	if errResult := validateLineRange(input.PRNumber, input.StartLine, input.EndLine); errResult != nil {
		return errResult, GetCodeContextOutput{}, nil
	}

	// Default context lines to 3 if not specified, clamp negative values to 0
	contextLines := input.ContextLines
	if contextLines < 0 {
		contextLines = 0
	} else if contextLines == 0 {
		contextLines = 3
	}

	codeCtx, err := s.deps.PRService.GetCodeContext(ctx, input.Owner, input.Repo, input.PRNumber, input.File, input.StartLine, input.EndLine, contextLines)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M2 - FileReader"), GetCodeContextOutput{}, nil
		}
		if errors.Is(err, triage.ErrFileNotFound) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("File not found: %s. The file may have been deleted or renamed since the review. Use request_rereview to get updated findings.", input.File)},
				},
			}, GetCodeContextOutput{}, nil
		}
		if errors.Is(err, triage.ErrInvalidLineRange) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Invalid line range: %d-%d. Ensure start <= end and both are positive.", input.StartLine, input.EndLine)},
				},
			}, GetCodeContextOutput{}, nil
		}
		if errors.Is(err, triage.ErrLineOutOfBounds) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Line range %d-%d out of bounds for file %s. The file may have changed since the review. Use request_rereview to get updated findings.", input.StartLine, input.EndLine, input.File)},
				},
			}, GetCodeContextOutput{}, nil
		}
		if errors.Is(err, triage.ErrFileTruncated) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("File too large: %s exceeds 10MB limit", input.File)},
				},
			}, GetCodeContextOutput{}, nil
		}
		return nil, GetCodeContextOutput{}, fmt.Errorf("get code context: %w", err)
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d lines from %s (lines %d-%d)", codeCtx.LineCount(), input.File, codeCtx.StartLine, codeCtx.EndLine)},
			},
		}, GetCodeContextOutput{
			File:          codeCtx.File,
			Ref:           codeCtx.Ref,
			StartLine:     codeCtx.StartLine,
			EndLine:       codeCtx.EndLine,
			Content:       codeCtx.Content,
			ContextBefore: codeCtx.ContextBefore,
			ContextAfter:  codeCtx.ContextAfter,
			Message:       "Code context retrieved successfully",
		}, nil
}

// registerGetDiffContextTool registers the get_diff_context tool.
func (s *Server) registerGetDiffContextTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_diff_context",
		Description: "Get the diff hunk for a file at specific lines.",
	}, s.handleGetDiffContext)
}

func (s *Server) handleGetDiffContext(ctx context.Context, req *mcp.CallToolRequest, input GetDiffContextInput) (*mcp.CallToolResult, GetDiffContextOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M2"), GetDiffContextOutput{}, nil
	}

	// Validate inputs before calling service
	if errResult := validateLineRange(input.PRNumber, input.StartLine, input.EndLine); errResult != nil {
		return errResult, GetDiffContextOutput{}, nil
	}

	diffCtx, err := s.deps.PRService.GetDiffContext(ctx, input.Owner, input.Repo, input.PRNumber, input.File, input.StartLine, input.EndLine)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M2 - DiffReader"), GetDiffContextOutput{}, nil
		}
		return nil, GetDiffContextOutput{}, fmt.Errorf("get diff context: %w", err)
	}

	hasChanges := "no changes"
	if diffCtx.HasChanges() {
		hasChanges = "has changes"
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Diff for %s lines %d-%d (%s)", input.File, input.StartLine, input.EndLine, hasChanges)},
			},
		}, GetDiffContextOutput{
			File:        diffCtx.File,
			BaseBranch:  diffCtx.BaseBranch,
			TargetRef:   diffCtx.TargetRef,
			HunkContent: diffCtx.HunkContent,
			StartLine:   diffCtx.StartLine,
			EndLine:     diffCtx.EndLine,
			Message:     "Diff context retrieved successfully",
		}, nil
}

// =============================================================================
// M3 Write Tool Handlers (PR-based, stateless)
// =============================================================================

// registerGetThreadTool registers the get_thread tool.
func (s *Server) registerGetThreadTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_thread",
		Description: "Get the full comment thread for a review comment, including all replies.",
	}, s.handleGetThread)
}

func (s *Server) handleGetThread(ctx context.Context, req *mcp.CallToolRequest, input GetThreadInput) (*mcp.CallToolResult, GetThreadOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M3"), GetThreadOutput{}, nil
	}

	// Get thread history from the CommentReader
	comments, err := s.deps.PRService.GetThreadHistory(ctx, input.Owner, input.Repo, input.CommentID)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M3"), GetThreadOutput{}, nil
		}
		return nil, GetThreadOutput{}, fmt.Errorf("get thread: %w", err)
	}

	output := GetThreadOutput{
		CommentID: input.CommentID,
		Comments:  make([]ThreadCommentOutput, len(comments)),
		Total:     len(comments),
		Message:   fmt.Sprintf("Found %d comments in thread", len(comments)),
	}

	// If PR number provided, look up the thread ID for use with mark_resolved
	if input.PRNumber > 0 {
		threadInfo, threadErr := s.deps.PRService.FindThreadForComment(ctx, input.Owner, input.Repo, input.PRNumber, input.CommentID)
		if threadErr == nil && threadInfo != nil {
			output.ThreadID = threadInfo.ID
			output.IsResolved = threadInfo.IsResolved
		} else if threadErr != nil && !errors.Is(threadErr, triage.ErrNotImplemented) {
			// Add warning to message - thread lookup failed but thread content is still available
			output.Message += fmt.Sprintf(" (note: thread_id lookup failed: %v)", threadErr)
		}
	}

	for i, c := range comments {
		output.Comments[i] = ThreadCommentOutput{
			Author:    c.Author,
			Body:      c.Body,
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
			IsReply:   c.IsReply,
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: output.Message},
		},
	}, output, nil
}

// registerReplyToFindingTool registers the reply_to_finding tool.
func (s *Server) registerReplyToFindingTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "reply_to_finding",
		Description: "Reply to a code reviewer finding or SARIF comment. Optionally include a status tag.",
	}, s.handleReplyToFinding)
}

func (s *Server) handleReplyToFinding(ctx context.Context, req *mcp.CallToolRequest, input ReplyToFindingInput) (*mcp.CallToolResult, ReplyToFindingOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M3"), ReplyToFindingOutput{Success: false}, nil
	}

	// Build the reply body with optional status tag
	body := input.Body
	if input.Status != nil && *input.Status != "" {
		// Reject oversized inputs before allocation (valid tags are ≤12 chars)
		const maxStatusLen = 30
		if len(*input.Status) > maxStatusLen {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Status tag too long (max %d chars). Valid values: %v", maxStatusLen, ValidStatusTags)},
				},
			}, ReplyToFindingOutput{Success: false, Message: "Invalid status tag"}, nil
		}
		// Validate and normalize the status tag
		normalizedStatus := strings.ToLower(strings.TrimSpace(*input.Status))
		if !isValidStatusTag(normalizedStatus) {
			// Use normalizedStatus in error message to avoid log injection from original input
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Invalid status tag: %q. Valid values: %v", normalizedStatus, ValidStatusTags)},
				},
			}, ReplyToFindingOutput{Success: false, Message: "Invalid status tag"}, nil
		}
		// Prepend status tag for machine parsing
		body = fmt.Sprintf("**Status:** %s\n\n%s", normalizedStatus, input.Body)
	}

	commentID, err := s.deps.PRService.ReplyToFinding(ctx, input.Owner, input.Repo, input.PRNumber, input.FindingID, body)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M3"), ReplyToFindingOutput{Success: false}, nil
		}
		if errors.Is(err, triage.ErrCommentNotFound) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Finding not found: %s. The comment may have been deleted. Use list_findings to refresh the findings list.", input.FindingID)},
				},
			}, ReplyToFindingOutput{Success: false, Message: "Finding not found"}, nil
		}
		return nil, ReplyToFindingOutput{}, fmt.Errorf("reply to finding: %w", err)
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Reply posted (comment ID: %d)", commentID)},
			},
		}, ReplyToFindingOutput{
			Success:   true,
			CommentID: commentID,
			Message:   "Reply posted successfully",
		}, nil
}

// registerPostCommentTool registers the post_comment tool.
func (s *Server) registerPostCommentTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "post_comment",
		Description: "Post a new review comment at a specific file and line. Use for responding to SARIF annotations.",
	}, s.handlePostComment)
}

func (s *Server) handlePostComment(ctx context.Context, req *mcp.CallToolRequest, input PostCommentInput) (*mcp.CallToolResult, PostCommentOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M3"), PostCommentOutput{Success: false}, nil
	}

	// Validate inputs
	if input.File == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "file path is required"},
			},
		}, PostCommentOutput{Success: false, Message: "Missing file path"}, nil
	}
	if input.Line <= 0 {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Invalid line number: %d (must be positive)", input.Line)},
			},
		}, PostCommentOutput{Success: false, Message: "Invalid line number"}, nil
	}

	commentID, err := s.deps.PRService.PostComment(ctx, input.Owner, input.Repo, input.PRNumber, input.File, input.Line, input.Body)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M3"), PostCommentOutput{Success: false}, nil
		}
		return nil, PostCommentOutput{}, fmt.Errorf("post comment: %w", err)
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Comment posted at %s:%d (ID: %d)", input.File, input.Line, commentID)},
			},
		}, PostCommentOutput{
			Success:   true,
			CommentID: commentID,
			Message:   "Comment posted successfully",
		}, nil
}

// registerMarkResolvedTool registers the mark_resolved tool.
func (s *Server) registerMarkResolvedTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "mark_resolved",
		Description: "Mark a review thread as resolved or unresolved. Requires the thread's node_id (e.g., PRRT_kwDO...).",
	}, s.handleMarkResolved)
}

func (s *Server) handleMarkResolved(ctx context.Context, req *mcp.CallToolRequest, input MarkResolvedInput) (*mcp.CallToolResult, MarkResolvedOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M3"), MarkResolvedOutput{Success: false}, nil
	}

	// Validate required inputs
	if input.Owner == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "owner is required"}},
		}, MarkResolvedOutput{Success: false, Message: "owner is required"}, nil
	}
	if input.Repo == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "repo is required"}},
		}, MarkResolvedOutput{Success: false, Message: "repo is required"}, nil
	}
	if input.ThreadID == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "thread_id is required"}},
		}, MarkResolvedOutput{Success: false, Message: "thread_id is required"}, nil
	}
	// Validate thread ID format (should be a GraphQL node ID starting with PRRT_)
	if !strings.HasPrefix(input.ThreadID, "PRRT_") {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("invalid thread_id format: expected PRRT_... prefix, got %q", input.ThreadID)}},
		}, MarkResolvedOutput{Success: false, Message: "invalid thread_id format"}, nil
	}

	var err error
	if input.Resolved {
		err = s.deps.PRService.ResolveThread(ctx, input.Owner, input.Repo, input.ThreadID)
	} else {
		err = s.deps.PRService.UnresolveThread(ctx, input.Owner, input.Repo, input.ThreadID)
	}

	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M3"), MarkResolvedOutput{Success: false}, nil
		}
		if errors.Is(err, triage.ErrThreadNotFound) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Thread not found: %s. The thread may have been deleted or the ID is incorrect.", input.ThreadID)},
				},
			}, MarkResolvedOutput{Success: false, Message: "Thread not found"}, nil
		}
		// Handle idempotent cases - thread already in desired state is a success, not an error
		if errors.Is(err, triage.ErrThreadAlreadyResolved) {
			return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: "Thread already resolved (no action needed)"},
					},
				}, MarkResolvedOutput{
					Success:  true,
					Resolved: true,
					Message:  "Thread already resolved",
				}, nil
		}
		if errors.Is(err, triage.ErrThreadAlreadyUnresolved) {
			return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: "Thread already unresolved (no action needed)"},
					},
				}, MarkResolvedOutput{
					Success:  true,
					Resolved: false,
					Message:  "Thread already unresolved",
				}, nil
		}
		return nil, MarkResolvedOutput{}, fmt.Errorf("mark resolved: %w", err)
	}

	action := "resolved"
	if !input.Resolved {
		action = "unresolved"
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Thread marked as %s", action)},
			},
		}, MarkResolvedOutput{
			Success:  true,
			Resolved: input.Resolved,
			Message:  fmt.Sprintf("Thread marked as %s", action),
		}, nil
}

// registerRequestRereviewTool registers the request_rereview tool.
func (s *Server) registerRequestRereviewTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "request_rereview",
		Description: "Dismiss stale bot reviews and request fresh review from specified reviewers.",
	}, s.handleRequestRereview)
}

func (s *Server) handleRequestRereview(ctx context.Context, req *mcp.CallToolRequest, input RequestRereviewInput) (*mcp.CallToolResult, RequestRereviewOutput, error) {
	if s.deps.PRService == nil {
		return notImplementedResult("M3"), RequestRereviewOutput{Success: false}, nil
	}

	var reviewsDismissed int
	var dismissErrors []string

	// Dismiss stale bot reviews if requested
	if input.DismissStale {
		reviews, err := s.deps.PRService.ListReviews(ctx, input.Owner, input.Repo, input.PRNumber)
		if err != nil {
			if errors.Is(err, triage.ErrNotImplemented) {
				return notImplementedResult("M3 - ListReviews"), RequestRereviewOutput{Success: false}, nil
			}
			return nil, RequestRereviewOutput{}, fmt.Errorf("list reviews: %w", err)
		}

		// Find bot reviews with actionable states (APPROVED or CHANGES_REQUESTED)
		// These are "stale" because the code may have changed since the bot reviewed
		dismissMessage := input.Message
		if dismissMessage == "" {
			dismissMessage = "Dismissed stale bot review to allow fresh re-review"
		}

		for _, review := range reviews {
			// Only dismiss bot reviews
			if review.UserType != "Bot" {
				continue
			}
			// Only dismiss actionable review states
			if review.State != "APPROVED" && review.State != "CHANGES_REQUESTED" {
				continue
			}

			err := s.deps.PRService.DismissReview(ctx, input.Owner, input.Repo, input.PRNumber, review.ID, dismissMessage)
			if err != nil {
				// Track error but continue - some reviews may not be dismissable (e.g., permissions)
				dismissErrors = append(dismissErrors, fmt.Sprintf("review %d: %v", review.ID, err))
				continue
			}
			reviewsDismissed++
		}
	}

	// Request review from specified reviewers
	reviewsRequested := len(input.Reviewers) + len(input.TeamReviewers)
	if reviewsRequested > 0 {
		err := s.deps.PRService.RequestReview(ctx, input.Owner, input.Repo, input.PRNumber, input.Reviewers, input.TeamReviewers)
		if err != nil {
			if errors.Is(err, triage.ErrNotImplemented) {
				return notImplementedResult("M3"), RequestRereviewOutput{Success: false}, nil
			}
			if errors.Is(err, triage.ErrUserNotFound) {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("One or more requested reviewers not found or not collaborators: %v. Verify usernames are correct and users have repository access.", input.Reviewers)},
					},
				}, RequestRereviewOutput{Success: false, Message: "Reviewer not found"}, nil
			}
			return nil, RequestRereviewOutput{}, fmt.Errorf("request review: %w", err)
		}
	}

	// Build result message including any dismiss errors
	message := fmt.Sprintf("Dismissed %d stale reviews, requested review from %d reviewers", reviewsDismissed, reviewsRequested)
	if reviewsDismissed == 0 && reviewsRequested == 0 {
		message = "No reviews dismissed or requested"
	} else if reviewsDismissed == 0 {
		message = fmt.Sprintf("Requested review from %d reviewers", reviewsRequested)
	} else if reviewsRequested == 0 {
		message = fmt.Sprintf("Dismissed %d stale reviews", reviewsDismissed)
	}

	// Append dismiss errors to message if any occurred
	if len(dismissErrors) > 0 {
		if len(dismissErrors) == 1 {
			message = fmt.Sprintf("%s (failed to dismiss 1: %s)", message, dismissErrors[0])
		} else {
			message = fmt.Sprintf("%s (failed to dismiss %d reviews: %s, ...)", message, len(dismissErrors), dismissErrors[0])
		}
	}

	// Determine success: fail if dismiss was requested but all attempts failed
	success := true
	if input.DismissStale && len(dismissErrors) > 0 && reviewsDismissed == 0 {
		success = false
		message = fmt.Sprintf("All dismiss attempts failed: %s", dismissErrors[0])
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: message},
			},
		}, RequestRereviewOutput{
			Success:          success,
			ReviewsDismissed: reviewsDismissed,
			ReviewsRequested: reviewsRequested,
			Message:          message,
		}, nil
}

// Helper functions for converting domain types to output types

func annotationToOutput(ann domain.Annotation) AnnotationOutput {
	start, end := ann.LineRange()
	return AnnotationOutput{
		CheckRunID: ann.CheckRunID,
		Index:      ann.Index,
		Path:       ann.Path,
		StartLine:  start,
		EndLine:    end,
		Level:      string(ann.Level),
		Message:    ann.Message,
		Title:      ann.Title,
	}
}

// computeFindingsSummary calculates triage progress statistics from findings.
func computeFindingsSummary(findings []domain.PRFinding) *FindingsSummary {
	if len(findings) == 0 {
		return nil
	}

	summary := &FindingsSummary{
		Total:      len(findings),
		BySeverity: make(map[string]int),
	}

	for _, f := range findings {
		if f.HasReply {
			summary.Replied++
		} else {
			summary.Unreplied++
		}

		// Track by severity (use "unknown" for empty)
		sev := f.Severity
		if sev == "" {
			sev = "unknown"
		}
		summary.BySeverity[sev]++
	}

	// Calculate triage percentage (replied / total * 100)
	if summary.Total > 0 {
		summary.TriagePercent = float64(summary.Replied) / float64(summary.Total) * 100
	}

	return summary
}

func findingToOutput(f domain.PRFinding) PRFindingOutput {
	output := PRFindingOutput{
		CommentID:    f.CommentID,
		Fingerprint:  f.Fingerprint,
		Path:         f.Path,
		Line:         f.Line,
		Severity:     f.Severity,
		Category:     f.Category,
		Body:         f.Body,
		Author:       f.Author,
		IsResolved:   f.IsResolved,
		ReplyCount:   f.ReplyCount,
		HasReply:     f.HasReply,
		LastReplyBy:  f.LastReplyBy,
		ThreadStatus: f.ThreadStatus(),
	}

	// Format LastReplyAt as RFC3339 if present and non-zero
	// (could be zero if all replies had unparseable timestamps)
	if f.LastReplyAt != nil && !f.LastReplyAt.IsZero() {
		formatted := f.LastReplyAt.Format(time.RFC3339)
		output.LastReplyAt = &formatted
	}

	return output
}
