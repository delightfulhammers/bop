package http

import (
	"time"

	"github.com/delightfulhammers/bop/internal/config"
)

// ParseTimeout parses timeout with fallback chain: provider override > global > default.
// Negative durations are rejected (would cause runtime panic in http.Client.Timeout).
func ParseTimeout(providerOverride *string, globalTimeout string, defaultVal time.Duration) time.Duration {
	// Provider override takes precedence
	if providerOverride != nil && *providerOverride != "" {
		if d, err := time.ParseDuration(*providerOverride); err == nil && d >= 0 {
			return d
		}
	}

	// Try global config
	if globalTimeout != "" {
		if d, err := time.ParseDuration(globalTimeout); err == nil && d >= 0 {
			return d
		}
	}

	// Use default (should always be >= 0)
	if defaultVal < 0 {
		return 60 * time.Second // Fallback to safe default
	}
	return defaultVal
}

// BuildRetryConfig creates RetryConfig from provider + global HTTP config
func BuildRetryConfig(provider config.ProviderConfig, httpCfg config.HTTPConfig) RetryConfig {
	// Max retries: provider override > global
	maxRetries := httpCfg.MaxRetries
	if provider.MaxRetries != nil {
		maxRetries = *provider.MaxRetries
	}

	// Initial backoff: provider override > global > default
	initialBackoff := parseDuration(provider.InitialBackoff, httpCfg.InitialBackoff, 2*time.Second)

	// Max backoff: provider override > global > default
	maxBackoff := parseDuration(provider.MaxBackoff, httpCfg.MaxBackoff, 32*time.Second)

	return RetryConfig{
		MaxRetries:     maxRetries,
		InitialBackoff: initialBackoff,
		MaxBackoff:     maxBackoff,
		Multiplier:     httpCfg.BackoffMultiplier,
	}
}

// parseDuration parses duration with fallback chain.
// Negative durations are rejected to prevent invalid backoff values.
func parseDuration(override *string, global string, defaultVal time.Duration) time.Duration {
	if override != nil && *override != "" {
		if d, err := time.ParseDuration(*override); err == nil && d >= 0 {
			return d
		}
	}

	if global != "" {
		if d, err := time.ParseDuration(global); err == nil && d >= 0 {
			return d
		}
	}

	// Use default (should always be >= 0)
	if defaultVal < 0 {
		return 2 * time.Second // Safe fallback for backoff
	}
	return defaultVal
}
