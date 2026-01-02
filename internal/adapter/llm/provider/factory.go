// Package provider provides a factory for creating LLM providers.
// It handles building providers from environment variables and configuration,
// and provides a centralized fallback mechanism for MCP sampling.
package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/bkyoung/code-reviewer/internal/adapter/llm/anthropic"
	"github.com/bkyoung/code-reviewer/internal/adapter/llm/gemini"
	"github.com/bkyoung/code-reviewer/internal/adapter/llm/ollama"
	"github.com/bkyoung/code-reviewer/internal/adapter/llm/openai"
	"github.com/bkyoung/code-reviewer/internal/adapter/llm/sampling"
	"github.com/bkyoung/code-reviewer/internal/config"
	"github.com/bkyoung/code-reviewer/internal/usecase/review"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SamplingSession defines the subset of mcp.ServerSession needed for sampling.
// This interface allows the factory to work with MCP sessions without
// depending on the full mcp.ServerSession type.
type SamplingSession interface {
	CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error)
	InitializeParams() *mcp.InitializeParams
}

// providerMeta holds metadata about each supported LLM provider.
// This enables provider-specific behavior like keyless authentication.
type providerMeta struct {
	requiresAPIKey bool
	defaultModel   string
}

// providerMetadata defines characteristics for each supported provider.
// Keyless providers (like Ollama) can be enabled without an API key.
var providerMetadata = map[string]providerMeta{
	"openai":    {requiresAPIKey: true, defaultModel: "gpt-5.2"},
	"anthropic": {requiresAPIKey: true, defaultModel: "claude-sonnet-4-5"},
	"gemini":    {requiresAPIKey: true, defaultModel: "gemini-3-pro-preview"},
	"ollama":    {requiresAPIKey: false, defaultModel: "codellama"},
}

// FactoryOptions configures the provider factory.
type FactoryOptions struct {
	// Config provides provider-specific settings (models, timeouts, etc.)
	Config *config.Config
}

// Factory creates and manages LLM providers.
// It handles building providers from environment variables and configuration,
// and provides a centralized fallback mechanism for MCP sampling.
type Factory struct {
	config          *config.Config
	directProviders map[string]review.Provider
}

// NewFactory creates a new provider factory.
// Direct providers are built immediately from environment variables.
func NewFactory(opts FactoryOptions) *Factory {
	cfg := opts.Config
	if cfg == nil {
		cfg = &config.Config{}
	}

	f := &Factory{
		config:          cfg,
		directProviders: make(map[string]review.Provider),
	}

	f.buildDirectProviders()

	return f
}

// buildDirectProviders creates providers from configuration.
// Provider API keys in config support ${VAR} environment variable expansion,
// which is handled by the config loader before reaching this method.
func (f *Factory) buildDirectProviders() {
	if f.config == nil || f.config.Providers == nil {
		return
	}

	// OpenAI provider
	if cfg, ok := f.config.Providers["openai"]; ok && isProviderEnabled("openai", cfg) {
		model := cfg.GetDefaultModel()
		if model == "" {
			model = providerMetadata["openai"].defaultModel
		}
		apiKey := cfg.APIKey
		if apiKey != "" {
			client := openai.NewHTTPClient(apiKey, model, cfg, f.config.HTTP)
			f.directProviders["openai"] = openai.NewProvider(model, client)
		}
	}

	// Anthropic provider
	if cfg, ok := f.config.Providers["anthropic"]; ok && isProviderEnabled("anthropic", cfg) {
		model := cfg.GetDefaultModel()
		if model == "" {
			model = providerMetadata["anthropic"].defaultModel
		}
		apiKey := cfg.APIKey
		if apiKey != "" {
			client := anthropic.NewHTTPClient(apiKey, model, cfg, f.config.HTTP)
			f.directProviders["anthropic"] = anthropic.NewProvider(model, client)
		}
	}

	// Gemini provider
	if cfg, ok := f.config.Providers["gemini"]; ok && isProviderEnabled("gemini", cfg) {
		model := cfg.GetDefaultModel()
		if model == "" {
			model = providerMetadata["gemini"].defaultModel
		}
		apiKey := cfg.APIKey
		if apiKey != "" {
			client := gemini.NewHTTPClient(apiKey, model, cfg, f.config.HTTP)
			f.directProviders["gemini"] = gemini.NewProvider(model, client)
		}
	}

	// Ollama provider (local LLM, no API key required)
	if cfg, ok := f.config.Providers["ollama"]; ok && isProviderEnabled("ollama", cfg) {
		model := cfg.GetDefaultModel()
		if model == "" {
			model = providerMetadata["ollama"].defaultModel
		}
		host := os.Getenv("OLLAMA_HOST")
		if host == "" {
			host = "http://localhost:11434"
		}
		client := ollama.NewHTTPClient(host, model, cfg, f.config.HTTP)
		f.directProviders["ollama"] = ollama.NewProvider(model, client)
	}
}

// isProviderEnabled checks if a provider should be enabled based on its config
// and provider metadata. A provider is enabled if:
//   - Enabled is explicitly true, OR
//   - Enabled is nil (not set) AND:
//   - For providers requiring API keys: APIKey is non-empty
//   - For keyless providers (e.g., Ollama): always enabled when in config
func isProviderEnabled(providerName string, cfg config.ProviderConfig) bool {
	// Explicit enabled/disabled takes precedence
	if cfg.Enabled != nil {
		return *cfg.Enabled
	}

	// Check if this provider requires an API key
	meta, known := providerMetadata[providerName]
	if !known {
		// Unknown providers require API key by default (safe fallback)
		return cfg.APIKey != ""
	}

	if meta.requiresAPIKey {
		return cfg.APIKey != ""
	}

	// Keyless provider: enabled by presence in config
	return true
}

// DirectProviders returns a copy of the providers built from API keys.
// Returns an empty map if no API keys were configured.
// The returned map is a defensive copy safe for external modification.
func (f *Factory) DirectProviders() map[string]review.Provider {
	result := make(map[string]review.Provider, len(f.directProviders))
	for k, v := range f.directProviders {
		result[k] = v
	}
	return result
}

// HasDirectProviders returns true if any direct providers are available.
func (f *Factory) HasDirectProviders() bool {
	return len(f.directProviders) > 0
}

// CreateSamplingProvider creates a provider that uses MCP sampling.
// The session must support sampling (check with ClientSupportsSampling first).
// Returns an error if the session is nil or doesn't support sampling.
func (f *Factory) CreateSamplingProvider(session SamplingSession) (review.Provider, error) {
	if session == nil {
		return nil, fmt.Errorf("cannot create sampling provider: session is nil")
	}

	if !ClientSupportsSampling(session) {
		return nil, fmt.Errorf("cannot create sampling provider: client does not support sampling")
	}

	// Create a session provider that returns the session.
	// The session is captured in this closure.
	sessionProvider := func() sampling.Session {
		return &samplingSessionAdapter{session}
	}

	return sampling.NewProvider(sessionProvider), nil
}

// EffectiveProviders returns the providers to use for a review request.
// Priority:
//  1. Direct providers (from API keys) - if available, these are always used
//  2. Sampling provider (from MCP session) - used as fallback when no API keys
//
// Returns an error if no providers are available (no API keys and no sampling support).
// The returned map is a defensive copy safe for external modification.
func (f *Factory) EffectiveProviders(session SamplingSession) (map[string]review.Provider, error) {
	// Prefer direct providers
	if f.HasDirectProviders() {
		return f.DirectProviders(), nil
	}

	// Fall back to sampling
	samplingProvider, err := f.CreateSamplingProvider(session)
	if err != nil {
		return nil, fmt.Errorf("no providers available: no API keys configured and %w", err)
	}

	return map[string]review.Provider{"sampling": samplingProvider}, nil
}

// ClientSupportsSampling checks if the connected MCP client supports sampling.
// Sampling is indicated by the presence of the Sampling capability in client info.
func ClientSupportsSampling(session SamplingSession) bool {
	if session == nil {
		return false
	}

	params := session.InitializeParams()
	if params == nil {
		return false
	}

	if params.Capabilities == nil {
		return false
	}

	return params.Capabilities.Sampling != nil
}

// samplingSessionAdapter adapts SamplingSession to sampling.Session interface.
type samplingSessionAdapter struct {
	session SamplingSession
}

func (a *samplingSessionAdapter) CreateMessage(ctx context.Context, params *mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
	return a.session.CreateMessage(ctx, params)
}
