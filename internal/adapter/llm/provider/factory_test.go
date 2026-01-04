package provider_test

import (
	"context"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/llm/provider"
	"github.com/delightfulhammers/bop/internal/config"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSession implements provider.SamplingSession for testing.
type mockSession struct {
	createMessageFunc func(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error)
	initializeParams  *mcp.InitializeParams
	supportsSampling  bool
}

func (m *mockSession) CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
	if m.createMessageFunc != nil {
		return m.createMessageFunc(ctx, params)
	}
	return &mcp.CreateMessageResult{
		Content: &mcp.TextContent{Text: `{"summary": "test", "findings": []}`},
		Model:   "test-model",
	}, nil
}

func (m *mockSession) InitializeParams() *mcp.InitializeParams {
	if m.initializeParams != nil {
		return m.initializeParams
	}
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
// Factory Creation Tests
// =============================================================================

func TestNewFactory_NoProviders(t *testing.T) {
	// Clear any existing API keys
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{},
	})

	assert.NotNil(t, factory)
	assert.Empty(t, factory.DirectProviders())
}

func TestNewFactory_WithAnthropicKey(t *testing.T) {
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"anthropic": {APIKey: "test-anthropic-key"},
			},
		},
	})

	providers := factory.DirectProviders()
	assert.Len(t, providers, 1)
	assert.Contains(t, providers, "anthropic")
}

func TestNewFactory_WithOpenAIKey(t *testing.T) {
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"openai": {APIKey: "test-openai-key"},
			},
		},
	})

	providers := factory.DirectProviders()
	assert.Len(t, providers, 1)
	assert.Contains(t, providers, "openai")
}

func TestNewFactory_WithBothKeys(t *testing.T) {
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"anthropic": {APIKey: "test-anthropic-key"},
				"openai":    {APIKey: "test-openai-key"},
			},
		},
	})

	providers := factory.DirectProviders()
	assert.Len(t, providers, 2)
	assert.Contains(t, providers, "anthropic")
	assert.Contains(t, providers, "openai")
}

// =============================================================================
// Capability Detection Tests
// =============================================================================

func TestClientSupportsSampling_WithSamplingCapability(t *testing.T) {
	session := &mockSession{supportsSampling: true}
	assert.True(t, provider.ClientSupportsSampling(session))
}

func TestClientSupportsSampling_WithoutSamplingCapability(t *testing.T) {
	session := &mockSession{supportsSampling: false}
	assert.False(t, provider.ClientSupportsSampling(session))
}

func TestClientSupportsSampling_NilSession(t *testing.T) {
	assert.False(t, provider.ClientSupportsSampling(nil))
}

func TestClientSupportsSampling_NilCapabilities(t *testing.T) {
	session := &mockSession{
		initializeParams: &mcp.InitializeParams{
			Capabilities: nil,
		},
	}
	assert.False(t, provider.ClientSupportsSampling(session))
}

func TestClientSupportsSampling_NilInitializeParams(t *testing.T) {
	// nilParamsSession returns nil from InitializeParams(), unlike mockSession
	// which returns an empty InitializeParams when initializeParams field is nil
	session := &nilParamsSession{}
	assert.False(t, provider.ClientSupportsSampling(session))
}

// nilParamsSession is a session that returns nil from InitializeParams().
// This is distinct from mockSession which returns an empty InitializeParams.
type nilParamsSession struct{}

func (n *nilParamsSession) CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
	return nil, nil
}

func (n *nilParamsSession) InitializeParams() *mcp.InitializeParams {
	return nil
}

// =============================================================================
// Sampling Provider Creation Tests
// =============================================================================

func TestFactory_CreateSamplingProvider_Success(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{},
	})

	session := &mockSession{supportsSampling: true}
	p, err := factory.CreateSamplingProvider(session)

	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestFactory_CreateSamplingProvider_NilSession(t *testing.T) {
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{},
	})

	_, err := factory.CreateSamplingProvider(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session")
}

func TestFactory_CreateSamplingProvider_NoSamplingSupport(t *testing.T) {
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{},
	})

	session := &mockSession{supportsSampling: false}
	_, err := factory.CreateSamplingProvider(session)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sampling")
}

// =============================================================================
// Effective Providers Tests
// =============================================================================

func TestFactory_EffectiveProviders_DirectProvidersAvailable(t *testing.T) {
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"anthropic": {APIKey: "test-anthropic-key"},
			},
		},
	})

	// Even with sampling support, should prefer direct providers
	session := &mockSession{supportsSampling: true}
	providers, err := factory.EffectiveProviders(session)

	require.NoError(t, err)
	assert.Len(t, providers, 1)
	assert.Contains(t, providers, "anthropic")
	// Should NOT contain sampling since direct is available
	assert.NotContains(t, providers, "sampling")
}

func TestFactory_EffectiveProviders_FallbackToSampling(t *testing.T) {
	// Clear API keys
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{},
	})

	session := &mockSession{supportsSampling: true}
	providers, err := factory.EffectiveProviders(session)

	require.NoError(t, err)
	assert.Len(t, providers, 1)
	assert.Contains(t, providers, "sampling")
}

func TestFactory_EffectiveProviders_NoProvidersAvailable(t *testing.T) {
	// Clear API keys
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{},
	})

	// No sampling support either
	session := &mockSession{supportsSampling: false}
	_, err := factory.EffectiveProviders(session)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no providers available")
}

func TestFactory_EffectiveProviders_NilSession_NoDirectProviders(t *testing.T) {
	// Clear API keys
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{},
	})

	_, err := factory.EffectiveProviders(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no providers available")
}

func TestFactory_EffectiveProviders_NilSession_DirectProvidersAvailable(t *testing.T) {
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"anthropic": {APIKey: "test-anthropic-key"},
			},
		},
	})

	// Nil session but direct providers available - should work
	providers, err := factory.EffectiveProviders(nil)

	require.NoError(t, err)
	assert.Len(t, providers, 1)
	assert.Contains(t, providers, "anthropic")
}

// =============================================================================
// Interface Verification Tests
// =============================================================================

func TestFactory_SamplingProvider_ImplementsInterface(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{},
	})

	session := &mockSession{supportsSampling: true}
	p, err := factory.CreateSamplingProvider(session)

	require.NoError(t, err)
	// Verify it implements review.Provider
	_ = review.Provider(p)
}

// =============================================================================
// Keyless Provider Tests (Issue #212)
// =============================================================================

func TestNewFactory_OllamaWithExplicitEnabled(t *testing.T) {
	enabled := true
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"ollama": {Enabled: &enabled, DefaultModel: "codellama"},
			},
		},
	})

	providers := factory.DirectProviders()
	assert.Len(t, providers, 1)
	assert.Contains(t, providers, "ollama")
}

func TestNewFactory_OllamaWithImplicitEnabled(t *testing.T) {
	// When Ollama config is present without explicit enabled: true,
	// it should still be enabled since Ollama doesn't require an API key.
	// This is the bug fix for issue #212.
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"ollama": {DefaultModel: "codellama"}, // No APIKey, no Enabled
			},
		},
	})

	providers := factory.DirectProviders()
	assert.Len(t, providers, 1, "Ollama should be enabled implicitly when config is present")
	assert.Contains(t, providers, "ollama")
}

func TestNewFactory_OllamaExplicitlyDisabled(t *testing.T) {
	enabled := false
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"ollama": {Enabled: &enabled, DefaultModel: "codellama"},
			},
		},
	})

	providers := factory.DirectProviders()
	assert.Empty(t, providers, "Ollama should not be enabled when explicitly disabled")
}

func TestNewFactory_KeyRequiredProvidersStillNeedKeys(t *testing.T) {
	// Providers that require API keys should still fail without them
	factory := provider.NewFactory(provider.FactoryOptions{
		Config: &config.Config{
			Providers: map[string]config.ProviderConfig{
				"anthropic": {DefaultModel: "claude-sonnet-4-5"}, // No APIKey
				"openai":    {DefaultModel: "gpt-4o"},            // No APIKey
			},
		},
	})

	providers := factory.DirectProviders()
	assert.Empty(t, providers, "Providers requiring API keys should not be enabled without keys")
}
