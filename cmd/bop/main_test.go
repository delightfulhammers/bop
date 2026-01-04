package main

import (
	"context"
	"testing"

	"github.com/delightfulhammers/bop/internal/config"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// boolPtr is a helper to create *bool values in tests.
func boolPtr(b bool) *bool { return &b }

// mockProvider is a simple mock for testing
type mockProvider struct {
	name  string
	model string
}

func (m *mockProvider) Review(ctx context.Context, request review.ProviderRequest) (domain.Review, error) {
	return domain.Review{}, nil
}

func (m *mockProvider) EstimateTokens(text string) int {
	return len(text) / 4 // Simple estimate for testing
}

func TestCreatePlanningProvider(t *testing.T) {
	tests := []struct {
		name             string
		cfg              *config.Config
		providers        map[string]review.Provider
		obs              observabilityComponents
		wantProvider     bool
		wantProviderType string // "openai", "anthropic", "gemini", "ollama", or "reused"
	}{
		{
			name: "planning enabled with specific OpenAI model - creates dedicated provider",
			cfg: &config.Config{
				Planning: config.PlanningConfig{
					Enabled:  true,
					Provider: "openai",
					Model:    "gpt-4o-mini",
				},
				Providers: map[string]config.ProviderConfig{
					"openai": {
						APIKey: "test-key",
					},
				},
				HTTP: config.HTTPConfig{},
			},
			providers:        map[string]review.Provider{},
			obs:              observabilityComponents{},
			wantProvider:     true,
			wantProviderType: "openai",
		},
		{
			name: "planning enabled with specific Anthropic model - creates dedicated provider",
			cfg: &config.Config{
				Planning: config.PlanningConfig{
					Enabled:  true,
					Provider: "anthropic",
					Model:    "claude-3-haiku-20240307",
				},
				Providers: map[string]config.ProviderConfig{
					"anthropic": {
						APIKey: "test-key",
					},
				},
				HTTP: config.HTTPConfig{},
			},
			providers:        map[string]review.Provider{},
			obs:              observabilityComponents{},
			wantProvider:     true,
			wantProviderType: "anthropic",
		},
		{
			name: "planning enabled with specific Gemini model - creates dedicated provider",
			cfg: &config.Config{
				Planning: config.PlanningConfig{
					Enabled:  true,
					Provider: "gemini",
					Model:    "gemini-2.0-flash-thinking-exp-01-21",
				},
				Providers: map[string]config.ProviderConfig{
					"gemini": {
						APIKey: "test-key",
					},
				},
				HTTP: config.HTTPConfig{},
			},
			providers:        map[string]review.Provider{},
			obs:              observabilityComponents{},
			wantProvider:     true,
			wantProviderType: "gemini",
		},
		{
			name: "planning enabled, no specific model - reuses existing provider",
			cfg: &config.Config{
				Planning: config.PlanningConfig{
					Enabled:  true,
					Provider: "openai",
					Model:    "", // No specific model
				},
				Providers: map[string]config.ProviderConfig{
					"openai": {
						APIKey: "test-key",
					},
				},
				HTTP: config.HTTPConfig{},
			},
			providers: map[string]review.Provider{
				"openai": &mockProvider{name: "openai", model: "gpt-4"},
			},
			obs:              observabilityComponents{},
			wantProvider:     true,
			wantProviderType: "reused",
		},
		{
			name: "planning enabled but provider not found - returns nil",
			cfg: &config.Config{
				Planning: config.PlanningConfig{
					Enabled:  true,
					Provider: "nonexistent",
					Model:    "",
				},
				Providers: map[string]config.ProviderConfig{},
				HTTP:      config.HTTPConfig{},
			},
			providers:        map[string]review.Provider{},
			obs:              observabilityComponents{},
			wantProvider:     false,
			wantProviderType: "",
		},
		{
			name: "planning enabled but API key missing - returns nil",
			cfg: &config.Config{
				Planning: config.PlanningConfig{
					Enabled:  true,
					Provider: "openai",
					Model:    "gpt-4o-mini",
				},
				Providers: map[string]config.ProviderConfig{
					"openai": {
						APIKey: "", // Missing API key
					},
				},
				HTTP: config.HTTPConfig{},
			},
			providers:        map[string]review.Provider{},
			obs:              observabilityComponents{},
			wantProvider:     false,
			wantProviderType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createPlanningProvider(tt.cfg, tt.providers, tt.obs)

			if tt.wantProvider && got == nil {
				t.Errorf("createPlanningProvider() = nil, want provider")
			}
			if !tt.wantProvider && got != nil {
				t.Errorf("createPlanningProvider() = %v, want nil", got)
			}

			// For reused provider case, verify it's the same instance
			if tt.wantProviderType == "reused" && got != nil {
				expectedProvider := tt.providers[tt.cfg.Planning.Provider]
				if got != expectedProvider {
					t.Errorf("createPlanningProvider() returned different provider instance, want same instance")
				}
			}
		})
	}
}

func TestIsProviderEnabled(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.ProviderConfig
		want    bool
		comment string
	}{
		// Enabled = nil (not set in config)
		{
			name:    "nil Enabled, no APIKey",
			cfg:     config.ProviderConfig{Enabled: nil, APIKey: ""},
			want:    false,
			comment: "no API key means not usable",
		},
		{
			name:    "nil Enabled, with APIKey",
			cfg:     config.ProviderConfig{Enabled: nil, APIKey: "sk-test"},
			want:    true,
			comment: "backward compat: API key presence enables provider",
		},

		// Enabled = false (explicitly disabled)
		{
			name:    "explicit false, no APIKey",
			cfg:     config.ProviderConfig{Enabled: boolPtr(false), APIKey: ""},
			want:    false,
			comment: "explicitly disabled",
		},
		{
			name:    "explicit false, with APIKey",
			cfg:     config.ProviderConfig{Enabled: boolPtr(false), APIKey: "sk-test"},
			want:    false,
			comment: "explicit disable wins over API key presence",
		},

		// Enabled = true (explicitly enabled)
		{
			name:    "explicit true, no APIKey",
			cfg:     config.ProviderConfig{Enabled: boolPtr(true), APIKey: ""},
			want:    true,
			comment: "explicitly enabled (for keyless providers like Ollama)",
		},
		{
			name:    "explicit true, with APIKey",
			cfg:     config.ProviderConfig{Enabled: boolPtr(true), APIKey: "sk-test"},
			want:    true,
			comment: "explicitly enabled with API key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProviderEnabled(tt.cfg)
			if got != tt.want {
				t.Errorf("isProviderEnabled() = %v, want %v (%s)", got, tt.want, tt.comment)
			}
		})
	}
}

func TestIsProviderUsable(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config.ProviderConfig
		exists bool
		want   bool
	}{
		{
			name:   "provider does not exist",
			cfg:    config.ProviderConfig{APIKey: "sk-test"},
			exists: false,
			want:   false,
		},
		{
			name:   "provider exists and enabled",
			cfg:    config.ProviderConfig{Enabled: boolPtr(true), APIKey: "sk-test"},
			exists: true,
			want:   true,
		},
		{
			name:   "provider exists but disabled",
			cfg:    config.ProviderConfig{Enabled: boolPtr(false), APIKey: "sk-test"},
			exists: true,
			want:   false,
		},
		{
			name:   "provider exists, nil enabled, has API key",
			cfg:    config.ProviderConfig{Enabled: nil, APIKey: "sk-test"},
			exists: true,
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProviderUsable(tt.cfg, tt.exists)
			if got != tt.want {
				t.Errorf("isProviderUsable() = %v, want %v", got, tt.want)
			}
		})
	}
}
