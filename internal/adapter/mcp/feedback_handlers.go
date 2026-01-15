package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/delightfulhammers/bop/internal/adapter/feedback"
	platformfeedback "github.com/delightfulhammers/platform/contracts/feedback"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FeedbackClient defines the interface for submitting feedback.
type FeedbackClient interface {
	Submit(ctx context.Context, req feedback.SubmitRequest) (*feedback.SubmitResponse, error)
}

// SubmitFeedbackInput is the input for the submit_feedback tool.
type SubmitFeedbackInput struct {
	Category    string `json:"category" jsonschema:"Feedback category: bug, feature, usability, other"`
	Title       string `json:"title" jsonschema:"Brief summary of the feedback"`
	Description string `json:"description" jsonschema:"Detailed description of the feedback"`
}

// SubmitFeedbackOutput is the output for the submit_feedback tool.
type SubmitFeedbackOutput struct {
	Success bool   `json:"success"`
	ID      string `json:"id,omitempty"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message"`
}

// registerSubmitFeedbackTool registers the submit_feedback tool.
func (s *Server) registerSubmitFeedbackTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "submit_feedback",
		Description: `Submit feedback about bop to help us improve.

Categories:
- bug: Report a bug or issue
- feature: Request a new feature
- usability: Report usability issues
- other: General feedback

Requires authentication (bop auth login).`,
	}, s.handleSubmitFeedback)
}

func (s *Server) handleSubmitFeedback(ctx context.Context, req *mcp.CallToolRequest, input SubmitFeedbackInput) (*mcp.CallToolResult, SubmitFeedbackOutput, error) {
	// Check if feedback client is available
	if s.deps.FeedbackClient == nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Feedback service not configured"},
			},
		}, SubmitFeedbackOutput{}, nil
	}

	// Require authentication
	if err := s.RequireAuth(); err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: err.Error()},
			},
		}, SubmitFeedbackOutput{}, nil
	}

	// Validate category
	category := platformfeedback.Category(strings.ToUpper(input.Category))
	if !category.IsValid() {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Invalid category %q - must be one of: bug, feature, usability, other", input.Category)},
			},
		}, SubmitFeedbackOutput{}, nil
	}

	// Validate required fields
	if input.Title == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Title is required"},
			},
		}, SubmitFeedbackOutput{}, nil
	}
	if input.Description == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Description is required"},
			},
		}, SubmitFeedbackOutput{}, nil
	}

	// Submit feedback
	resp, err := s.deps.FeedbackClient.Submit(ctx, feedback.SubmitRequest{
		Category:    category,
		Title:       input.Title,
		Description: input.Description,
		ClientType:  platformfeedback.ClientTypeMCP,
	})
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Failed to submit feedback: %v", err)},
			},
		}, SubmitFeedbackOutput{}, nil
	}

	output := SubmitFeedbackOutput{
		Success: true,
		ID:      resp.ID.String(),
		Status:  string(resp.Status),
		Message: "Feedback submitted successfully. Thank you!",
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: output.Message},
		},
	}, output, nil
}
