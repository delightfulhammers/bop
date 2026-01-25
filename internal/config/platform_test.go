package config

import (
	"os"
	"testing"
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
