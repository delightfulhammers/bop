// Package provider provides a factory for creating LLM providers.
// It handles building providers from environment variables and configuration,
// and provides a centralized fallback mechanism for MCP sampling.
package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/bkyoung/code-reviewer/internal/adapter/llm/anthropic"
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

// buildDirectProviders creates providers from environment variables.
func (f *Factory) buildDirectProviders() {
	// Anthropic provider
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		model := "claude-sonnet-4-20250514" // default
		providerCfg := config.ProviderConfig{}
		if f.config != nil {
			if pc, ok := f.config.Providers["anthropic"]; ok {
				providerCfg = pc
				if pc.Model != "" {
					model = pc.Model
				}
			}
		}
		client := anthropic.NewHTTPClient(key, model, providerCfg, f.config.HTTP)
		f.directProviders["anthropic"] = anthropic.NewProvider(model, client)
	}

	// OpenAI provider
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		model := "gpt-4o" // default
		providerCfg := config.ProviderConfig{}
		if f.config != nil {
			if pc, ok := f.config.Providers["openai"]; ok {
				providerCfg = pc
				if pc.Model != "" {
					model = pc.Model
				}
			}
		}
		client := openai.NewHTTPClient(key, model, providerCfg, f.config.HTTP)
		f.directProviders["openai"] = openai.NewProvider(model, client)
	}
}

// DirectProviders returns the providers built from API keys.
// Returns an empty map if no API keys were configured.
func (f *Factory) DirectProviders() map[string]review.Provider {
	return f.directProviders
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
func (f *Factory) EffectiveProviders(session SamplingSession) (map[string]review.Provider, error) {
	// Prefer direct providers
	if f.HasDirectProviders() {
		return f.directProviders, nil
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
