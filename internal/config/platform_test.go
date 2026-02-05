package config

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestGetPlatformURL(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		envSet   bool
		want     string
	}{
		{
			name:   "no env var returns default",
			envSet: false,
			want:   DefaultPlatformURL,
		},
		{
			name:     "env var set returns env value",
			envValue: "https://custom.example.com",
			envSet:   true,
			want:     "https://custom.example.com",
		},
		{
			name:     "empty env var returns empty (legacy mode)",
			envValue: "",
			envSet:   true,
			want:     "",
		},
		{
			name:     "whitespace env var returns empty",
			envValue: "   ",
			envSet:   true,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env var
			oldVal, oldExists := os.LookupEnv(PlatformURLEnvVar)
			defer func() {
				if oldExists {
					_ = os.Setenv(PlatformURLEnvVar, oldVal)
				} else {
					_ = os.Unsetenv(PlatformURLEnvVar)
				}
			}()

			if tt.envSet {
				_ = os.Setenv(PlatformURLEnvVar, tt.envValue)
			} else {
				_ = os.Unsetenv(PlatformURLEnvVar)
			}

			got := GetPlatformURL()
			if got != tt.want {
				t.Errorf("GetPlatformURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetConfigServiceURL(t *testing.T) {
	tests := []struct {
		name                string
		platformURL         string
		platformURLSet      bool
		configServiceURL    string
		configServiceURLSet bool
		want                string
	}{
		{
			name: "no env vars returns default",
			want: DefaultConfigServiceURL,
		},
		{
			name:                "explicit config service URL wins",
			configServiceURL:    "https://custom-config.example.com",
			configServiceURLSet: true,
			want:                "https://custom-config.example.com",
		},
		{
			name:           "legacy mode returns empty",
			platformURL:    "",
			platformURLSet: true,
			want:           "",
		},
		{
			name:           "legacy mode with whitespace returns empty",
			platformURL:    "   ",
			platformURLSet: true,
			want:           "",
		},
		{
			name:           "custom platform URL without config service URL returns empty (security)",
			platformURL:    "https://enterprise.example.com",
			platformURLSet: true,
			want:           "",
		},
		{
			name:                "custom platform URL with config service URL works",
			platformURL:         "https://enterprise.example.com",
			platformURLSet:      true,
			configServiceURL:    "https://enterprise-config.example.com",
			configServiceURLSet: true,
			want:                "https://enterprise-config.example.com",
		},
		{
			name:           "default platform URL without config service URL returns default",
			platformURL:    DefaultPlatformURL,
			platformURLSet: true,
			want:           DefaultConfigServiceURL,
		},
		{
			name:                "config service URL trimmed",
			configServiceURL:    "  https://config.example.com  ",
			configServiceURLSet: true,
			want:                "https://config.example.com",
		},
		{
			name:           "default platform URL with trailing slash returns default config URL",
			platformURL:    DefaultPlatformURL + "/",
			platformURLSet: true,
			want:           DefaultConfigServiceURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env vars
			oldPlatformVal, oldPlatformExists := os.LookupEnv(PlatformURLEnvVar)
			oldConfigVal, oldConfigExists := os.LookupEnv(ConfigServiceURLEnvVar)
			defer func() {
				if oldPlatformExists {
					_ = os.Setenv(PlatformURLEnvVar, oldPlatformVal)
				} else {
					_ = os.Unsetenv(PlatformURLEnvVar)
				}
				if oldConfigExists {
					_ = os.Setenv(ConfigServiceURLEnvVar, oldConfigVal)
				} else {
					_ = os.Unsetenv(ConfigServiceURLEnvVar)
				}
			}()

			// Set up test environment
			if tt.platformURLSet {
				_ = os.Setenv(PlatformURLEnvVar, tt.platformURL)
			} else {
				_ = os.Unsetenv(PlatformURLEnvVar)
			}
			if tt.configServiceURLSet {
				_ = os.Setenv(ConfigServiceURLEnvVar, tt.configServiceURL)
			} else {
				_ = os.Unsetenv(ConfigServiceURLEnvVar)
			}

			got := GetConfigServiceURL()
			if got != tt.want {
				t.Errorf("GetConfigServiceURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsLegacyEscapeHatch(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		envSet   bool
		want     bool
	}{
		{
			name:   "no env var is not legacy",
			envSet: false,
			want:   false,
		},
		{
			name:     "env var set to value is not legacy",
			envValue: "https://example.com",
			envSet:   true,
			want:     false,
		},
		{
			name:     "env var set to empty is legacy",
			envValue: "",
			envSet:   true,
			want:     true,
		},
		{
			name:     "env var set to whitespace is legacy",
			envValue: "   ",
			envSet:   true,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env var
			oldVal, oldExists := os.LookupEnv(PlatformURLEnvVar)
			defer func() {
				if oldExists {
					_ = os.Setenv(PlatformURLEnvVar, oldVal)
				} else {
					_ = os.Unsetenv(PlatformURLEnvVar)
				}
			}()

			if tt.envSet {
				_ = os.Setenv(PlatformURLEnvVar, tt.envValue)
			} else {
				_ = os.Unsetenv(PlatformURLEnvVar)
			}

			got := IsLegacyEscapeHatch()
			if got != tt.want {
				t.Errorf("IsLegacyEscapeHatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsOperationalEnvVar(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"BOP_PLATFORM_URL", true},
		{"BOP_LOG_LEVEL", true},
		{"bop_platform_url", true}, // case insensitive
		{"ANTHROPIC_API_KEY", false},
		{"OPENAI_API_KEY", false},
		{"BOP_REVIEW_ENABLED", false},
		{"RANDOM_VAR", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsOperationalEnvVar(tt.name)
			if got != tt.want {
				t.Errorf("IsOperationalEnvVar(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsConfigEnvVar(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"ANTHROPIC_API_KEY", true},
		{"OPENAI_API_KEY", true},
		{"GEMINI_API_KEY", true},
		{"GITHUB_TOKEN", true},
		{"BOP_REVIEW_ENABLED", true}, // BOP_* except operational
		{"BOP_PLATFORM_URL", false},  // operational
		{"BOP_LOG_LEVEL", false},     // operational
		{"RANDOM_VAR", false},        // not a bop config var
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsConfigEnvVar(tt.name)
			if got != tt.want {
				t.Errorf("IsConfigEnvVar(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// mockConfigFetcher is a test double for ConfigFetcher.
type mockConfigFetcher struct {
	response *ProductConfigResponse
	err      error
	calls    int
}

func (m *mockConfigFetcher) FetchProductConfig(_ context.Context, _ string) (*ProductConfigResponse, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestPlatformConfigClient_FetchConfig(t *testing.T) {
	tests := []struct {
		name       string
		response   *ProductConfigResponse
		fetchErr   error
		wantErr    bool
		wantErrMsg string
		wantTier   string
	}{
		{
			name: "success returns config and tier",
			response: &ProductConfigResponse{
				Config: map[string]any{
					// Platform config format: reviewers is a list of active reviewer names
					"reviewers": []any{"security", "performance"},
					// weights maps reviewer name to weight
					"weights": map[string]any{"security": 1.5, "performance": 1.0},
					// model sets the default model
					"model": "claude-sonnet-4-5",
				},
				Tier:       "pro",
				IsReadOnly: false,
			},
			wantTier: "pro",
		},
		{
			name:       "fetch error propagates",
			fetchErr:   errors.New("network error"),
			wantErr:    true,
			wantErrMsg: "fetch platform config",
		},
		{
			name: "invalid config returns validation error",
			response: &ProductConfigResponse{
				Config: map[string]any{
					// Missing "reviewers" field entirely - validation fails
					"weights": map[string]any{"security": 1.5},
				},
				Tier: "solo",
			},
			wantErr:    true,
			wantErrMsg: "invalid platform config",
		},
		{
			name:       "nil response returns error",
			response:   nil,
			wantErr:    true,
			wantErrMsg: "platform returned nil config response",
		},
		{
			name: "nil config field returns error",
			response: &ProductConfigResponse{
				Config: nil,
				Tier:   "pro",
			},
			wantErr:    true,
			wantErrMsg: "platform config response missing config field",
		},
		{
			name: "empty tier field returns error",
			response: &ProductConfigResponse{
				Config: map[string]any{
					"reviewers": []any{"security"},
				},
				Tier: "",
			},
			wantErr:    true,
			wantErrMsg: "platform config response missing tier field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := &mockConfigFetcher{
				response: tt.response,
				err:      tt.fetchErr,
			}

			client := NewPlatformConfigClient(PlatformConfigClientConfig{
				Fetcher:  fetcher,
				CacheTTL: time.Minute,
			})

			cfg, tier, err := client.FetchConfig(context.Background(), "test-token")

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrMsg != "" && !contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tier != tt.wantTier {
				t.Errorf("tier = %q, want %q", tier, tt.wantTier)
			}
			if cfg == nil {
				t.Error("config should not be nil")
			}
		})
	}
}

func TestPlatformConfigClient_Caching(t *testing.T) {
	response := &ProductConfigResponse{
		Config: map[string]any{
			"reviewers": []any{"security"},
		},
		Tier: "pro",
	}

	fetcher := &mockConfigFetcher{response: response}
	client := NewPlatformConfigClient(PlatformConfigClientConfig{
		Fetcher:  fetcher,
		CacheTTL: time.Hour, // Long TTL so it won't expire during test
	})

	// First call should fetch
	_, _, err := client.FetchConfig(context.Background(), "token")
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	if fetcher.calls != 1 {
		t.Errorf("expected 1 call, got %d", fetcher.calls)
	}

	// Second call should use cache
	_, _, err = client.FetchConfig(context.Background(), "token")
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if fetcher.calls != 1 {
		t.Errorf("expected still 1 call (cached), got %d", fetcher.calls)
	}

	// Verify cache is valid
	if !client.IsCacheValid() {
		t.Error("cache should be valid")
	}

	// Invalidate cache
	client.InvalidateCache()
	if client.IsCacheValid() {
		t.Error("cache should be invalid after InvalidateCache")
	}

	// Third call should fetch again
	_, _, err = client.FetchConfig(context.Background(), "token")
	if err != nil {
		t.Fatalf("third fetch failed: %v", err)
	}
	if fetcher.calls != 2 {
		t.Errorf("expected 2 calls after invalidation, got %d", fetcher.calls)
	}
}

func TestPlatformConfigClient_CacheExpiry(t *testing.T) {
	response := &ProductConfigResponse{
		Config: map[string]any{
			"reviewers": []any{"security"},
		},
		Tier: "solo",
	}

	fetcher := &mockConfigFetcher{response: response}
	client := NewPlatformConfigClient(PlatformConfigClientConfig{
		Fetcher:  fetcher,
		CacheTTL: 10 * time.Millisecond, // Very short TTL
	})

	// First call fetches
	_, _, _ = client.FetchConfig(context.Background(), "token")
	if fetcher.calls != 1 {
		t.Errorf("expected 1 call, got %d", fetcher.calls)
	}

	// Wait for cache to expire
	time.Sleep(20 * time.Millisecond)

	// Cache should be expired
	if client.IsCacheValid() {
		t.Error("cache should be expired")
	}

	// Next call should fetch again
	_, _, _ = client.FetchConfig(context.Background(), "token")
	if fetcher.calls != 2 {
		t.Errorf("expected 2 calls after expiry, got %d", fetcher.calls)
	}
}

func TestPlatformConfigClient_FetchAndMerge(t *testing.T) {
	platformResponse := &ProductConfigResponse{
		Config: map[string]any{
			"reviewers": []any{"security", "performance"},
			"weights":   map[string]any{"security": 1.5, "performance": 1.0},
			"model":     "claude-sonnet-4-5",
		},
		Tier: "pro",
	}

	fetcher := &mockConfigFetcher{response: platformResponse}
	client := NewPlatformConfigClient(PlatformConfigClientConfig{
		Fetcher:  fetcher,
		CacheTTL: time.Minute,
	})

	// Local config with additional settings
	localConfig := Config{
		Reviewers: map[string]ReviewerConfig{
			"security": {
				Weight:  1.0,                         // Will be overridden by platform weight
				Persona: "You are a security expert", // Local persona preserved
			},
		},
	}

	merged, tier, err := client.FetchAndMerge(context.Background(), "token", localConfig)
	if err != nil {
		t.Fatalf("FetchAndMerge failed: %v", err)
	}

	if tier != "pro" {
		t.Errorf("tier = %q, want %q", tier, "pro")
	}
	if merged == nil {
		t.Fatal("merged config should not be nil")
	}
}

func TestPlatformConfigClient_DefaultCacheTTL(t *testing.T) {
	client := NewPlatformConfigClient(PlatformConfigClientConfig{
		Fetcher: &mockConfigFetcher{},
		// No CacheTTL specified - should default to 5 minutes
	})

	// The default TTL is 5 minutes, which we can verify via the struct
	if client.cacheTTL != 5*time.Minute {
		t.Errorf("default cacheTTL = %v, want 5m", client.cacheTTL)
	}
}

func TestPlatformConfigClient_CacheMutationPrevention(t *testing.T) {
	response := &ProductConfigResponse{
		Config: map[string]any{
			"reviewers": []any{"security", "performance"},
			"weights":   map[string]any{"security": 1.5, "performance": 1.0},
		},
		Tier: "pro",
	}

	fetcher := &mockConfigFetcher{response: response}
	client := NewPlatformConfigClient(PlatformConfigClientConfig{
		Fetcher:  fetcher,
		CacheTTL: time.Hour,
	})

	// First fetch
	cfg1, _, err := client.FetchConfig(context.Background(), "token")
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}

	// Mutate the returned config
	cfg1.Merge.Weights["security"] = 999.0
	cfg1.Reviewers = map[string]ReviewerConfig{
		"mutated": {Weight: 100.0},
	}

	// Second fetch should return cached config unaffected by mutation
	cfg2, _, err := client.FetchConfig(context.Background(), "token")
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}

	// Verify the cached config was not mutated
	if cfg2.Merge.Weights["security"] != 1.5 {
		t.Errorf("cache was mutated: security weight = %v, want 1.5", cfg2.Merge.Weights["security"])
	}
	if _, exists := cfg2.Reviewers["mutated"]; exists {
		t.Error("cache was mutated: 'mutated' reviewer should not exist")
	}

	// Verify only one fetch occurred (both calls used cache)
	if fetcher.calls != 1 {
		t.Errorf("expected 1 fetch call, got %d", fetcher.calls)
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
