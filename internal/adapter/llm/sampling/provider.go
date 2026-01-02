// Package sampling provides an LLM provider that uses MCP sampling.
// Instead of making direct API calls, it requests the connected MCP client
// (e.g., Claude Code) to perform completions on its behalf.
//
// This enables zero-configuration usage when the user doesn't have direct
// API keys but is using the tool through an MCP-compatible assistant.
package sampling

import (
	"context"
	"fmt"

	"github.com/bkyoung/code-reviewer/internal/adapter/llm"
	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/review"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const providerName = "sampling"

// Session defines the subset of mcp.ServerSession needed for sampling.
// This interface allows testing without a real MCP connection.
type Session interface {
	CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error)
}

// SessionProvider is a function that returns the current MCP session.
// This allows the provider to be created once but get the session per-request,
// since sessions are request-scoped in MCP tool handlers.
type SessionProvider func() Session

// Provider implements review.Provider using MCP sampling.
// Instead of making direct LLM API calls, it requests the MCP client
// to perform completions on its behalf via CreateMessage.
type Provider struct {
	getSession SessionProvider
}

// NewProvider creates a sampling-based review provider.
// The sessionProvider function is called for each Review request to get
// the current session, allowing session-per-request semantics.
func NewProvider(sessionProvider SessionProvider) *Provider {
	return &Provider{
		getSession: sessionProvider,
	}
}

// Review performs a code review by requesting the MCP client to sample.
// It sends the prompt to the connected client via CreateMessage and parses
// the JSON response using the same parser as other providers.
func (p *Provider) Review(ctx context.Context, req review.ProviderRequest) (domain.Review, error) {
	session := p.getSession()
	if session == nil {
		return domain.Review{}, fmt.Errorf("no MCP session available for sampling")
	}

	// Build the sampling request
	// The prompt is passed as a user message; max tokens controls output size
	samplingReq := &mcp.CreateMessageParams{
		Messages: []*mcp.SamplingMessage{
			{
				Role:    "user", // MCP Role is a string type
				Content: &mcp.TextContent{Text: req.Prompt},
			},
		},
		MaxTokens: int64(req.MaxSize),
	}

	// Request completion from MCP client
	result, err := session.CreateMessage(ctx, samplingReq)
	if err != nil {
		return domain.Review{}, fmt.Errorf("sampling request failed: %w", err)
	}

	// Extract text content from result
	textContent, ok := result.Content.(*mcp.TextContent)
	if !ok {
		return domain.Review{}, fmt.Errorf("unexpected content type: %T (expected TextContent)", result.Content)
	}

	// Parse the review response using shared JSON parser
	// This handles markdown-wrapped JSON and flexible field names (camelCase/snake_case)
	summary, findings, err := llmhttp.ParseReviewResponse(textContent.Text)
	if err != nil {
		return domain.Review{}, fmt.Errorf("parse sampling response: %w", err)
	}

	return domain.Review{
		ProviderName: providerName,
		ModelName:    result.Model,
		Summary:      summary,
		Findings:     findings,
		// Note: MCP sampling doesn't provide token counts or cost information
		// These remain zero, which is acceptable for the fallback use case
	}, nil
}

// EstimateTokens returns an estimated token count using tiktoken.
// Uses the shared estimation logic that approximates Claude's tokenizer.
func (p *Provider) EstimateTokens(text string) int {
	return llm.EstimateTokens(text)
}
