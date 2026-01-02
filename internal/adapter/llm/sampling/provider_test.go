package sampling

import (
	"context"
	"errors"
	"testing"

	"github.com/bkyoung/code-reviewer/internal/usecase/review"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSession implements the subset of ServerSession we need for testing.
type mockSession struct {
	createMessageFunc func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error)
}

func (m *mockSession) CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
	if m.createMessageFunc != nil {
		return m.createMessageFunc(ctx, params)
	}
	return nil, errors.New("not implemented")
}

func TestProvider_Review_Success(t *testing.T) {
	// Arrange: mock session returns valid JSON review
	session := &mockSession{
		createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: `{
					"summary": "Found 1 issue",
					"findings": [
						{
							"file": "main.go",
							"lineStart": 10,
							"lineEnd": 12,
							"severity": "high",
							"category": "bug",
							"description": "Nil pointer dereference"
						}
					]
				}`},
				Model:      "claude-sonnet-4-20250514",
				Role:       "assistant",
				StopReason: "end_turn",
			}, nil
		},
	}

	provider := NewProvider(func() Session { return session })

	// Act
	result, err := provider.Review(context.Background(), review.ProviderRequest{
		Prompt:       "Review this code...",
		Seed:         12345,
		MaxSize:      8192,
		ReviewerName: "test-reviewer",
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, providerName, result.ProviderName)
	assert.Equal(t, "claude-sonnet-4-20250514", result.ModelName)
	assert.Equal(t, "Found 1 issue", result.Summary)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, "main.go", result.Findings[0].File)
	assert.Equal(t, 10, result.Findings[0].LineStart)
	assert.Equal(t, "high", result.Findings[0].Severity)
	assert.Equal(t, "bug", result.Findings[0].Category)
	assert.Equal(t, "Nil pointer dereference", result.Findings[0].Description)
}

func TestProvider_Review_MarkdownWrappedJSON(t *testing.T) {
	// Test that we can handle JSON wrapped in markdown code blocks
	session := &mockSession{
		createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "```json\n{\"summary\": \"No issues\", \"findings\": []}\n```"},
				Model:   "claude-sonnet-4-20250514",
			}, nil
		},
	}

	provider := NewProvider(func() Session { return session })

	result, err := provider.Review(context.Background(), review.ProviderRequest{
		Prompt: "Review this...",
	})

	require.NoError(t, err)
	assert.Equal(t, "No issues", result.Summary)
	assert.Empty(t, result.Findings)
}

func TestProvider_Review_NoSession(t *testing.T) {
	// Test error when session provider returns nil
	provider := NewProvider(func() Session { return nil })

	_, err := provider.Review(context.Background(), review.ProviderRequest{
		Prompt: "Review this...",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no MCP session available")
}

func TestProvider_Review_SessionError(t *testing.T) {
	// Test error propagation from CreateMessage
	session := &mockSession{
		createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return nil, errors.New("sampling failed: client disconnected")
		},
	}

	provider := NewProvider(func() Session { return session })

	_, err := provider.Review(context.Background(), review.ProviderRequest{
		Prompt: "Review this...",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sampling request failed")
	assert.Contains(t, err.Error(), "client disconnected")
}

func TestProvider_Review_InvalidJSON(t *testing.T) {
	// Test handling of invalid JSON response
	session := &mockSession{
		createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: "This is not valid JSON at all"},
				Model:   "test-model",
			}, nil
		},
	}

	provider := NewProvider(func() Session { return session })

	_, err := provider.Review(context.Background(), review.ProviderRequest{
		Prompt: "Review this...",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestProvider_Review_UnexpectedContentType(t *testing.T) {
	// Test handling of non-text content
	session := &mockSession{
		createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.ImageContent{Data: []byte("base64data"), MIMEType: "image/png"},
				Model:   "test-model",
			}, nil
		},
	}

	provider := NewProvider(func() Session { return session })

	_, err := provider.Review(context.Background(), review.ProviderRequest{
		Prompt: "Review this...",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected content type")
}

func TestProvider_Review_PromptPassthrough(t *testing.T) {
	// Verify the prompt is correctly passed to CreateMessage
	var capturedParams *mcp.CreateMessageParams

	session := &mockSession{
		createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			capturedParams = params
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: `{"summary": "ok", "findings": []}`},
				Model:   "test-model",
			}, nil
		},
	}

	provider := NewProvider(func() Session { return session })

	_, err := provider.Review(context.Background(), review.ProviderRequest{
		Prompt:  "Please review this code:\n\n```go\npackage main\n```",
		MaxSize: 4096,
	})

	require.NoError(t, err)
	require.NotNil(t, capturedParams)

	// Verify messages were constructed correctly
	require.Len(t, capturedParams.Messages, 1)
	msg := capturedParams.Messages[0]
	assert.Equal(t, mcp.Role("user"), msg.Role)

	textContent, ok := msg.Content.(*mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", msg.Content)
	assert.Contains(t, textContent.Text, "Please review this code")

	// Verify max tokens
	assert.Equal(t, int64(4096), capturedParams.MaxTokens)
}

func TestProvider_EstimateTokens(t *testing.T) {
	provider := NewProvider(func() Session { return nil })

	// Test token estimation (should use tiktoken-based estimation)
	tokens := provider.EstimateTokens("Hello, world!")
	assert.Greater(t, tokens, 0)

	// Longer text should have more tokens
	longText := "This is a longer piece of text that should have more tokens than the short one."
	longTokens := provider.EstimateTokens(longText)
	assert.Greater(t, longTokens, tokens)
}

func TestProvider_SnakeCaseJSON(t *testing.T) {
	// Test that snake_case JSON fields are handled (LLMs sometimes ignore schema)
	session := &mockSession{
		createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: `{
					"summary": "Found 1 issue",
					"findings": [
						{
							"file": "main.go",
							"line_start": 10,
							"line_end": 12,
							"severity": "medium",
							"category": "style",
							"description": "Variable naming"
						}
					]
				}`},
				Model: "test-model",
			}, nil
		},
	}

	provider := NewProvider(func() Session { return session })

	result, err := provider.Review(context.Background(), review.ProviderRequest{
		Prompt: "Review this...",
	})

	require.NoError(t, err)
	require.Len(t, result.Findings, 1)
	// The shared parser handles snake_case line_start/line_end
	assert.Equal(t, 10, result.Findings[0].LineStart)
	assert.Equal(t, 12, result.Findings[0].LineEnd)
}

// TestProvider_ImplementsInterface verifies the provider implements review.Provider
func TestProvider_ImplementsInterface(t *testing.T) {
	var _ review.Provider = (*Provider)(nil)
}

func TestProvider_EmptyFindings(t *testing.T) {
	session := &mockSession{
		createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content: &mcp.TextContent{Text: `{"summary": "Code looks great!", "findings": []}`},
				Model:   "claude-sonnet-4-20250514",
			}, nil
		},
	}

	provider := NewProvider(func() Session { return session })

	result, err := provider.Review(context.Background(), review.ProviderRequest{
		Prompt: "Review this...",
	})

	require.NoError(t, err)
	assert.Equal(t, "Code looks great!", result.Summary)
	assert.Empty(t, result.Findings)
}
