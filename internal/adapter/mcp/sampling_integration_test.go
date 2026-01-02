package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bkyoung/code-reviewer/internal/adapter/llm/provider"
	"github.com/bkyoung/code-reviewer/internal/config"
	"github.com/bkyoung/code-reviewer/internal/usecase/review"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// =============================================================================
// Mock Implementations for Sampling Integration Tests
// =============================================================================

// mockSamplingSession implements provider.SamplingSession for testing.
type mockSamplingSession struct {
	createMessageFunc func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error)
	supportsSampling  bool
}

func (m *mockSamplingSession) CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
	if m.createMessageFunc != nil {
		return m.createMessageFunc(ctx, params)
	}
	// Default: return a valid review response
	return &mcp.CreateMessageResult{
		Content: &mcp.TextContent{Text: `{"summary": "test review", "findings": [{"file": "main.go", "line_start": 10, "line_end": 10, "severity": "medium", "category": "bug", "description": "Test finding from sampling"}]}`},
		Model:   "claude-3-5-sonnet",
	}, nil
}

func (m *mockSamplingSession) InitializeParams() *mcp.InitializeParams {
	if m.supportsSampling {
		return &mcp.InitializeParams{
			Capabilities: &mcp.ClientCapabilities{
				Sampling: &mcp.SamplingCapabilities{},
			},
		}
	}
	return &mcp.InitializeParams{
		Capabilities: &mcp.ClientCapabilities{},
	}
}

// =============================================================================
// Integration Tests: Sampling Fallback Behavior
// =============================================================================

func TestIntegration_SamplingFallback_FactoryBehavior(t *testing.T) {
	t.Run("creates sampling provider when no API keys", func(t *testing.T) {
		// Temporarily clear API keys
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "")

		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{},
		})

		// Verify factory has no direct providers
		require.Empty(t, factory.DirectProviders(), "expected no direct providers when API keys are cleared")

		// Create a mock session with sampling support
		mockSession := &mockSamplingSession{
			supportsSampling: true,
			createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
				return &mcp.CreateMessageResult{
					Content: &mcp.TextContent{Text: `{"summary": "test", "findings": []}`},
					Model:   "claude-3-5-sonnet",
				}, nil
			},
		}

		// Should be able to create sampling provider
		samplingProvider, err := factory.CreateSamplingProvider(mockSession)
		require.NoError(t, err)
		require.NotNil(t, samplingProvider)

		// EffectiveProviders should return sampling
		providers, err := factory.EffectiveProviders(mockSession)
		require.NoError(t, err)
		assert.Len(t, providers, 1)
		assert.Contains(t, providers, "sampling")
	})

	t.Run("prefers direct providers over sampling when API keys present", func(t *testing.T) {
		// Test the behavior: when HasDirectProviders returns true, sampling is not used
		// Use config with API keys (Factory now uses config, not raw env vars)
		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{
				Providers: map[string]config.ProviderConfig{
					"anthropic": {APIKey: "test-anthropic-key"},
				},
			},
		})

		mockSession := &mockSamplingSession{supportsSampling: true}

		// Since we have API keys in config, EffectiveProviders should return direct providers
		providers, err := factory.EffectiveProviders(mockSession)
		require.NoError(t, err)

		// Should NOT contain sampling when direct providers available
		assert.NotContains(t, providers, "sampling")
		// Should contain the direct provider
		assert.Contains(t, providers, "anthropic")
	})

	t.Run("returns error when no providers available", func(t *testing.T) {
		// Temporarily clear API keys
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "")

		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{},
		})

		// Session without sampling support
		mockSession := &mockSamplingSession{supportsSampling: false}

		_, err := factory.EffectiveProviders(mockSession)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no providers available")
	})

	t.Run("returns error for nil session without direct providers", func(t *testing.T) {
		// Temporarily clear API keys
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "")

		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{},
		})

		_, err := factory.EffectiveProviders(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no providers available")
	})
}

func TestIntegration_CapabilityDetection(t *testing.T) {
	t.Run("detects sampling capability from session", func(t *testing.T) {
		sessionWithSampling := &mockSamplingSession{supportsSampling: true}
		assert.True(t, provider.ClientSupportsSampling(sessionWithSampling))
	})

	t.Run("detects missing sampling capability", func(t *testing.T) {
		sessionWithoutSampling := &mockSamplingSession{supportsSampling: false}
		assert.False(t, provider.ClientSupportsSampling(sessionWithoutSampling))
	})

	t.Run("handles nil session gracefully", func(t *testing.T) {
		assert.False(t, provider.ClientSupportsSampling(nil))
	})
}

func TestIntegration_SamplingProvider_ResponseParsing(t *testing.T) {
	// Save and clear environment for consistent testing
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	t.Run("parses valid JSON review response", func(t *testing.T) {
		mockSession := &mockSamplingSession{
			supportsSampling: true,
			createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
				return &mcp.CreateMessageResult{
					Content: &mcp.TextContent{Text: `{
						"summary": "Found potential issues",
						"findings": [
							{
								"file": "main.go",
								"line_start": 42,
								"line_end": 45,
								"severity": "high",
								"category": "security",
								"description": "SQL injection vulnerability"
							},
							{
								"file": "utils.go",
								"line_start": 10,
								"line_end": 10,
								"severity": "medium",
								"category": "bug",
								"description": "Potential nil pointer"
							}
						]
					}`},
					Model: "claude-3-5-sonnet",
				}, nil
			},
		}

		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{},
		})

		samplingProvider, err := factory.CreateSamplingProvider(mockSession)
		require.NoError(t, err)

		// Perform a review request
		result, err := samplingProvider.Review(context.Background(), review.ProviderRequest{
			Prompt:  "Review this code",
			MaxSize: 4096,
		})
		require.NoError(t, err)

		assert.Equal(t, "sampling", result.ProviderName)
		assert.Equal(t, "claude-3-5-sonnet", result.ModelName)
		assert.Len(t, result.Findings, 2)
		assert.Equal(t, "main.go", result.Findings[0].File)
		assert.Equal(t, "high", result.Findings[0].Severity)
	})

	t.Run("parses markdown-wrapped JSON response", func(t *testing.T) {
		mockSession := &mockSamplingSession{
			supportsSampling: true,
			createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
				return &mcp.CreateMessageResult{
					Content: &mcp.TextContent{Text: "```json\n{\"summary\": \"test\", \"findings\": [{\"file\": \"test.go\", \"line_start\": 1, \"line_end\": 1, \"severity\": \"low\", \"category\": \"style\", \"description\": \"minor issue\"}]}\n```"},
					Model:   "claude-3-5-sonnet",
				}, nil
			},
		}

		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{},
		})

		samplingProvider, err := factory.CreateSamplingProvider(mockSession)
		require.NoError(t, err)

		result, err := samplingProvider.Review(context.Background(), review.ProviderRequest{
			Prompt:  "Review this code",
			MaxSize: 4096,
		})
		require.NoError(t, err)

		assert.Len(t, result.Findings, 1)
		assert.Equal(t, "test.go", result.Findings[0].File)
	})

	t.Run("handles empty findings array", func(t *testing.T) {
		mockSession := &mockSamplingSession{
			supportsSampling: true,
			createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
				return &mcp.CreateMessageResult{
					Content: &mcp.TextContent{Text: `{"summary": "No issues found", "findings": []}`},
					Model:   "claude-3-5-sonnet",
				}, nil
			},
		}

		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{},
		})

		samplingProvider, err := factory.CreateSamplingProvider(mockSession)
		require.NoError(t, err)

		result, err := samplingProvider.Review(context.Background(), review.ProviderRequest{
			Prompt:  "Review this code",
			MaxSize: 4096,
		})
		require.NoError(t, err)

		assert.Empty(t, result.Findings)
		assert.Equal(t, "No issues found", result.Summary)
	})

	t.Run("handles camelCase field names", func(t *testing.T) {
		mockSession := &mockSamplingSession{
			supportsSampling: true,
			createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
				return &mcp.CreateMessageResult{
					Content: &mcp.TextContent{Text: `{"summary": "test", "findings": [{"file": "main.go", "lineStart": 10, "lineEnd": 10, "severity": "high", "category": "bug", "description": "test"}]}`},
					Model:   "claude-3-5-sonnet",
				}, nil
			},
		}

		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{},
		})

		samplingProvider, err := factory.CreateSamplingProvider(mockSession)
		require.NoError(t, err)

		result, err := samplingProvider.Review(context.Background(), review.ProviderRequest{
			Prompt:  "Review this code",
			MaxSize: 4096,
		})
		require.NoError(t, err)

		// The parser should handle both camelCase and snake_case
		assert.Len(t, result.Findings, 1)
		assert.Equal(t, 10, result.Findings[0].LineStart)
	})
}

func TestIntegration_SamplingProvider_ErrorHandling(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	t.Run("returns error for invalid JSON response", func(t *testing.T) {
		mockSession := &mockSamplingSession{
			supportsSampling: true,
			createMessageFunc: func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
				return &mcp.CreateMessageResult{
					Content: &mcp.TextContent{Text: `not valid json`},
					Model:   "claude-3-5-sonnet",
				}, nil
			},
		}

		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{},
		})

		samplingProvider, err := factory.CreateSamplingProvider(mockSession)
		require.NoError(t, err)

		_, err = samplingProvider.Review(context.Background(), review.ProviderRequest{
			Prompt:  "Review this code",
			MaxSize: 4096,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse")
	})
}

func TestIntegration_HandlerErrorMessages(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	t.Run("review_branch shows helpful error when no providers", func(t *testing.T) {
		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{},
		})

		server := NewServer(ServerDeps{
			ProviderFactory: factory,
			// Git, Merger not set - simulates minimal setup
		})

		input := ReviewBranchInput{
			BaseRef:   "main",
			TargetRef: "feature/test", // Explicit target to skip auto-detection (no Git engine)
		}

		// Call handler directly with nil request (no session)
		result, _, err := server.handleReviewBranch(context.Background(), nil, input)

		// The handler should return a result with IsError, not an error
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)

		// Check error message is helpful
		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent.Text, "LLM API keys")
		assert.Contains(t, textContent.Text, "sampling")
	})

	t.Run("review_pr shows helpful error when no providers", func(t *testing.T) {
		factory := provider.NewFactory(provider.FactoryOptions{
			Config: &config.Config{},
		})

		server := NewServer(ServerDeps{
			ProviderFactory: factory,
		})

		input := ReviewPRInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
		}

		result, _, err := server.handleReviewPR(context.Background(), nil, input)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)

		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent.Text, "LLM API keys")
		assert.Contains(t, textContent.Text, "sampling")
	})
}
