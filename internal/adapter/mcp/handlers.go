package mcp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

	findings, err := s.deps.PRService.ListFindings(ctx, input.Owner, input.Repo, input.PRNumber, input.Severity, input.Category)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M2 - CommentReader"), ListFindingsOutput{}, nil
		}
		return nil, ListFindingsOutput{}, fmt.Errorf("list findings: %w", err)
	}

	output := ListFindingsOutput{
		Findings: make([]PRFindingOutput, len(findings)),
		Total:    len(findings),
	}

	for i, f := range findings {
		output.Findings[i] = findingToOutput(f)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d findings", len(findings))},
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
					&mcp.TextContent{Text: fmt.Sprintf("Finding not found: %s", input.FindingID)},
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

	// Safely truncate body for display
	bodyPreview := finding.Body
	if len(bodyPreview) > 100 {
		bodyPreview = bodyPreview[:100] + "..."
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
					&mcp.TextContent{Text: "No structured suggestion found in finding"},
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
					&mcp.TextContent{Text: fmt.Sprintf("File not found: %s", input.File)},
				},
			}, GetCodeContextOutput{}, nil
		}
		if errors.Is(err, triage.ErrInvalidLineRange) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Invalid line range: %d-%d", input.StartLine, input.EndLine)},
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

// M3 Write Tool Handlers (session-based, for future implementation)

// registerTriageFindingTool registers the triage_finding tool.
func (s *Server) registerTriageFindingTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "triage_finding",
		Description: "Apply a triage decision to a finding (accept, dispute, question, resolve, or won't fix).",
	}, s.handleTriageFinding)
}

func (s *Server) handleTriageFinding(ctx context.Context, req *mcp.CallToolRequest, input TriageFindingInput) (*mcp.CallToolResult, TriageFindingOutput, error) {
	if s.deps.TriageService == nil {
		return notImplementedResult("M3"), TriageFindingOutput{Success: false, Message: "Not implemented"}, nil
	}

	// Validate status before creating decision
	status := domain.TriageStatus(input.Status)
	if !status.IsValid() {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("invalid triage status: %s (valid: pending, accepted, disputed, question, resolved, wont_fix)", input.Status)},
			},
		}, TriageFindingOutput{Success: false, Message: "Invalid status"}, nil
	}

	decision := domain.TriageDecision{
		FindingID: input.FindingID,
		Status:    status,
		Reason:    input.Reason,
		TriagedAt: time.Now(),
	}

	err := s.deps.TriageService.TriageFinding(ctx, input.SessionID, input.FindingID, decision)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M3"), TriageFindingOutput{Success: false, Message: "Not implemented"}, nil
		}
		return nil, TriageFindingOutput{}, fmt.Errorf("failed to triage finding: %w", err)
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Finding %s marked as %s", input.FindingID, input.Status)},
			},
		}, TriageFindingOutput{
			Success: true,
			Message: fmt.Sprintf("Finding triaged as %s", input.Status),
		}, nil
}

// registerPostCommentTool registers the post_comment tool.
func (s *Server) registerPostCommentTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "post_review_comment",
		Description: "Post a new review comment on GitHub for a finding.",
	}, s.handlePostComment)
}

func (s *Server) handlePostComment(ctx context.Context, req *mcp.CallToolRequest, input PostCommentInput) (*mcp.CallToolResult, PostCommentOutput, error) {
	if s.deps.TriageService == nil {
		return notImplementedResult("M3"), PostCommentOutput{Success: false, Message: "Not implemented"}, nil
	}

	err := s.deps.TriageService.PostComment(ctx, input.SessionID, input.FindingID, input.Body)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M3"), PostCommentOutput{Success: false, Message: "Not implemented"}, nil
		}
		return nil, PostCommentOutput{}, fmt.Errorf("failed to post comment: %w", err)
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Comment posted successfully"},
			},
		}, PostCommentOutput{
			Success: true,
			Message: "Comment posted",
		}, nil
}

// registerReplyToThreadTool registers the reply_to_thread tool.
func (s *Server) registerReplyToThreadTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "reply_to_thread",
		Description: "Reply to an existing review thread on GitHub.",
	}, s.handleReplyToThread)
}

func (s *Server) handleReplyToThread(ctx context.Context, req *mcp.CallToolRequest, input ReplyToThreadInput) (*mcp.CallToolResult, ReplyToThreadOutput, error) {
	if s.deps.TriageService == nil {
		return notImplementedResult("M3"), ReplyToThreadOutput{Success: false, Message: "Not implemented"}, nil
	}

	err := s.deps.TriageService.ReplyToFinding(ctx, input.SessionID, input.FindingID, input.Body)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M3"), ReplyToThreadOutput{Success: false, Message: "Not implemented"}, nil
		}
		return nil, ReplyToThreadOutput{}, fmt.Errorf("failed to reply to thread: %w", err)
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Reply posted successfully"},
			},
		}, ReplyToThreadOutput{
			Success: true,
			Message: "Reply posted",
		}, nil
}

// registerResolveThreadTool registers the resolve_thread tool.
func (s *Server) registerResolveThreadTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "resolve_thread",
		Description: "Mark a review thread as resolved on GitHub.",
	}, s.handleResolveThread)
}

func (s *Server) handleResolveThread(ctx context.Context, req *mcp.CallToolRequest, input ResolveThreadInput) (*mcp.CallToolResult, ResolveThreadOutput, error) {
	if s.deps.TriageService == nil {
		return notImplementedResult("M3"), ResolveThreadOutput{Success: false, Message: "Not implemented"}, nil
	}

	err := s.deps.TriageService.ResolveFinding(ctx, input.SessionID, input.FindingID)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return notImplementedResult("M3"), ResolveThreadOutput{Success: false, Message: "Not implemented"}, nil
		}
		return nil, ResolveThreadOutput{}, fmt.Errorf("failed to resolve thread: %w", err)
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Thread resolved successfully"},
			},
		}, ResolveThreadOutput{
			Success: true,
			Message: "Thread resolved",
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

func findingToOutput(f domain.PRFinding) PRFindingOutput {
	return PRFindingOutput{
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
		ThreadStatus: f.ThreadStatus(),
	}
}
