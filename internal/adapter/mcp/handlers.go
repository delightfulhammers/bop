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

// registerStartSessionTool registers the start_triage_session tool.
func (s *Server) registerStartSessionTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "start_triage_session",
		Description: "Start a new triage session for a pull request. Loads findings from the most recent code review.",
	}, s.handleStartSession)
}

func (s *Server) handleStartSession(ctx context.Context, req *mcp.CallToolRequest, input StartSessionInput) (*mcp.CallToolResult, StartSessionOutput, error) {
	session, err := s.deps.TriageService.StartSession(ctx, input.Repository, input.PRNumber)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool not yet implemented (M2 scope)"},
				},
			}, StartSessionOutput{Message: "Not implemented"}, nil
		}
		return nil, StartSessionOutput{}, fmt.Errorf("failed to start session: %w", err)
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Started triage session %s with %d findings", session.ID, len(session.Findings))},
			},
		}, StartSessionOutput{
			SessionID:    session.ID,
			FindingCount: len(session.Findings),
			Message:      "Session started successfully",
		}, nil
}

// registerGetCurrentFindingTool registers the get_current_finding tool.
func (s *Server) registerGetCurrentFindingTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_current_finding",
		Description: "Get the current finding to triage in the session.",
	}, s.handleGetCurrentFinding)
}

func (s *Server) handleGetCurrentFinding(ctx context.Context, req *mcp.CallToolRequest, input GetCurrentFindingInput) (*mcp.CallToolResult, FindingOutput, error) {
	finding, err := s.deps.TriageService.GetCurrentFinding(ctx, input.SessionID)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool not yet implemented (M2 scope)"},
				},
			}, FindingOutput{}, nil
		}
		return nil, FindingOutput{}, fmt.Errorf("failed to get current finding: %w", err)
	}

	if finding == nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "No more findings to triage"},
			},
		}, FindingOutput{}, nil
	}

	output := FindingOutput{
		ID:           finding.ID,
		File:         finding.File,
		LineStart:    finding.LineStart,
		LineEnd:      finding.LineEnd,
		Severity:     finding.Severity,
		Category:     finding.Category,
		Description:  finding.Description,
		Suggestion:   finding.Suggestion,
		TriageStatus: string(finding.TriageStatus),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Finding in %s (lines %d-%d): %s", finding.File, finding.LineStart, finding.LineEnd, finding.Description)},
		},
	}, output, nil
}

// registerGetFindingContextTool registers the get_finding_context tool.
func (s *Server) registerGetFindingContextTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_finding_context",
		Description: "Get contextual information about a finding, including PR details, file content, and thread history.",
	}, s.handleGetFindingContext)
}

func (s *Server) handleGetFindingContext(ctx context.Context, req *mcp.CallToolRequest, input GetFindingContextInput) (*mcp.CallToolResult, ContextOutput, error) {
	triageCtx, err := s.deps.TriageService.GetFindingContext(ctx, input.SessionID, input.FindingID)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool not yet implemented (M2 scope)"},
				},
			}, ContextOutput{}, nil
		}
		return nil, ContextOutput{}, fmt.Errorf("failed to get finding context: %w", err)
	}

	output := ContextOutput{
		PRTitle:       triageCtx.PRTitle,
		PRDescription: triageCtx.PRDescription,
		PRAuthor:      triageCtx.PRAuthor,
	}

	// Convert thread history
	for _, comment := range triageCtx.ThreadHistory {
		output.ThreadHistory = append(output.ThreadHistory, CommentOutput{
			Author:    comment.Author,
			Body:      comment.Body,
			CreatedAt: comment.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Context for PR: %s by %s", triageCtx.PRTitle, triageCtx.PRAuthor)},
		},
	}, output, nil
}

// registerListFindingsTool registers the list_findings tool.
func (s *Server) registerListFindingsTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "list_findings",
		Description: "List all findings in the triage session, optionally filtered by status.",
	}, s.handleListFindings)
}

func (s *Server) handleListFindings(ctx context.Context, req *mcp.CallToolRequest, input ListFindingsInput) (*mcp.CallToolResult, ListFindingsOutput, error) {
	// Convert status filter if provided
	var statusFilter *domain.TriageStatus
	if input.StatusFilter != nil {
		status := domain.TriageStatus(*input.StatusFilter)
		statusFilter = &status
	}

	findings, err := s.deps.TriageService.ListFindings(ctx, input.SessionID, statusFilter)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool not yet implemented (M2 scope)"},
				},
			}, ListFindingsOutput{}, nil
		}
		return nil, ListFindingsOutput{}, fmt.Errorf("failed to list findings: %w", err)
	}

	output := ListFindingsOutput{
		Findings: make([]FindingOutput, len(findings)),
		Total:    len(findings),
	}

	for i, f := range findings {
		output.Findings[i] = FindingOutput{
			ID:           f.ID,
			File:         f.File,
			LineStart:    f.LineStart,
			LineEnd:      f.LineEnd,
			Severity:     f.Severity,
			Category:     f.Category,
			Description:  f.Description,
			Suggestion:   f.Suggestion,
			TriageStatus: string(f.TriageStatus),
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Found %d findings", len(findings))},
		},
	}, output, nil
}

// registerGetProgressTool registers the get_progress tool.
func (s *Server) registerGetProgressTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "get_triage_progress",
		Description: "Get the current triage progress for a session.",
	}, s.handleGetProgress)
}

func (s *Server) handleGetProgress(ctx context.Context, req *mcp.CallToolRequest, input GetProgressInput) (*mcp.CallToolResult, GetProgressOutput, error) {
	triaged, total, err := s.deps.TriageService.GetProgress(ctx, input.SessionID)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool not yet implemented (M2 scope)"},
				},
			}, GetProgressOutput{}, nil
		}
		return nil, GetProgressOutput{}, fmt.Errorf("failed to get progress: %w", err)
	}

	percent := 0
	if total > 0 {
		percent = (triaged * 100) / total
	}

	message := fmt.Sprintf("Triaged %d of %d findings (%d%%)", triaged, total, percent)

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: message},
			},
		}, GetProgressOutput{
			Triaged: triaged,
			Total:   total,
			Percent: percent,
			Message: message,
		}, nil
}

// registerTriageFindingTool registers the triage_finding tool.
func (s *Server) registerTriageFindingTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "triage_finding",
		Description: "Apply a triage decision to a finding (accept, dispute, question, resolve, or won't fix).",
	}, s.handleTriageFinding)
}

func (s *Server) handleTriageFinding(ctx context.Context, req *mcp.CallToolRequest, input TriageFindingInput) (*mcp.CallToolResult, TriageFindingOutput, error) {
	decision := domain.TriageDecision{
		FindingID: input.FindingID,
		Status:    domain.TriageStatus(input.Status),
		Reason:    input.Reason,
		TriagedAt: time.Now(),
	}

	err := s.deps.TriageService.TriageFinding(ctx, input.SessionID, input.FindingID, decision)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool not yet implemented (M3 scope)"},
				},
			}, TriageFindingOutput{Success: false, Message: "Not implemented"}, nil
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
	err := s.deps.TriageService.PostComment(ctx, input.SessionID, input.FindingID, input.Body)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool not yet implemented (M3 scope)"},
				},
			}, PostCommentOutput{Success: false, Message: "Not implemented"}, nil
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
	err := s.deps.TriageService.ReplyToFinding(ctx, input.SessionID, input.FindingID, input.Body)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool not yet implemented (M3 scope)"},
				},
			}, ReplyToThreadOutput{Success: false, Message: "Not implemented"}, nil
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
	err := s.deps.TriageService.ResolveFinding(ctx, input.SessionID, input.FindingID)
	if err != nil {
		if errors.Is(err, triage.ErrNotImplemented) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Tool not yet implemented (M3 scope)"},
				},
			}, ResolveThreadOutput{Success: false, Message: "Not implemented"}, nil
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
