package main

import (
	"context"
	"strings"
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

func TestValidateReviewerProviders(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.Config
		providers   map[string]review.Provider
		wantErr     bool
		errContains string
	}{
		{
			name: "default reviewer has provider available",
			cfg: config.Config{
				DefaultReviewers: []string{"default"},
				Reviewers: map[string]config.ReviewerConfig{
					"default": {Provider: "anthropic"},
				},
			},
			providers: map[string]review.Provider{
				"anthropic": &mockProvider{name: "anthropic"},
			},
			wantErr: false,
		},
		{
			name: "default reviewer missing provider - anthropic",
			cfg: config.Config{
				DefaultReviewers: []string{"default"},
				Reviewers: map[string]config.ReviewerConfig{
					"default": {Provider: "anthropic"},
				},
			},
			providers:   map[string]review.Provider{},
			wantErr:     true,
			errContains: "ANTHROPIC_API_KEY",
		},
		{
			name: "default reviewer missing provider - openai",
			cfg: config.Config{
				DefaultReviewers: []string{"myreviewer"},
				Reviewers: map[string]config.ReviewerConfig{
					"myreviewer": {Provider: "openai"},
				},
			},
			providers:   map[string]review.Provider{},
			wantErr:     true,
			errContains: "OPENAI_API_KEY",
		},
		{
			name: "default reviewer missing provider - gemini",
			cfg: config.Config{
				DefaultReviewers: []string{"myreviewer"},
				Reviewers: map[string]config.ReviewerConfig{
					"myreviewer": {Provider: "gemini"},
				},
			},
			providers:   map[string]review.Provider{},
			wantErr:     true,
			errContains: "GEMINI_API_KEY",
		},
		{
			name: "multiple default reviewers - all available",
			cfg: config.Config{
				DefaultReviewers: []string{"a", "b"},
				Reviewers: map[string]config.ReviewerConfig{
					"a": {Provider: "anthropic"},
					"b": {Provider: "openai"},
				},
			},
			providers: map[string]review.Provider{
				"anthropic": &mockProvider{name: "anthropic"},
				"openai":    &mockProvider{name: "openai"},
			},
			wantErr: false,
		},
		{
			name: "multiple default reviewers - second missing",
			cfg: config.Config{
				DefaultReviewers: []string{"a", "b"},
				Reviewers: map[string]config.ReviewerConfig{
					"a": {Provider: "anthropic"},
					"b": {Provider: "openai"},
				},
			},
			providers: map[string]review.Provider{
				"anthropic": &mockProvider{name: "anthropic"},
			},
			wantErr:     true,
			errContains: "OPENAI_API_KEY",
		},
		{
			name: "empty default reviewers - no error",
			cfg: config.Config{
				DefaultReviewers: []string{},
				Reviewers:        map[string]config.ReviewerConfig{},
			},
			providers: map[string]review.Provider{},
			wantErr:   false,
		},
		{
			name: "default reviewer not in reviewers map - skipped gracefully",
			cfg: config.Config{
				DefaultReviewers: []string{"nonexistent"},
				Reviewers:        map[string]config.ReviewerConfig{},
			},
			providers: map[string]review.Provider{},
			wantErr:   false,
		},
		{
			name: "unknown provider gets generic env var name",
			cfg: config.Config{
				DefaultReviewers: []string{"custom"},
				Reviewers: map[string]config.ReviewerConfig{
					"custom": {Provider: "custom_provider"},
				},
			},
			providers:   map[string]review.Provider{},
			wantErr:     true,
			errContains: "CUSTOM_PROVIDER_API_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReviewerProviders(tt.cfg, tt.providers)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got: %s", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
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

func TestValidateGitHubAPIURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid https URL", url: "https://github.example.com/api/v3", wantErr: false},
		{name: "valid http URL", url: "http://localhost:8080", wantErr: false},
		{name: "missing scheme", url: "github.example.com/api/v3", wantErr: true},
		{name: "missing host", url: "https://", wantErr: true},
		{name: "just slashes", url: "///", wantErr: true},
		{name: "empty after trim would be caught before call", url: "not-a-url", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitHubAPIURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGitHubAPIURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
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
