package http_test

import (
	"testing"
	"time"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/delightfulhammers/bop/internal/config"
	"github.com/stretchr/testify/assert"
)

// Helper to create string pointers
func stringPtr(s string) *string {
	return &s
}

// Helper to create int pointers
func intPtr(i int) *int {
	return &i
}

func TestParseTimeout_ProviderOverrideTakesPrecedence(t *testing.T) {
	override := stringPtr("10s")
	global := "20s"
	defaultVal := 30 * time.Second

	result := llmhttp.ParseTimeout(override, global, defaultVal)

	assert.Equal(t, 10*time.Second, result, "Provider override should take precedence")
}

func TestParseTimeout_GlobalFallback(t *testing.T) {
	var override *string = nil
	global := "20s"
	defaultVal := 30 * time.Second

	result := llmhttp.ParseTimeout(override, global, defaultVal)

	assert.Equal(t, 20*time.Second, result, "Should use global config when no provider override")
}

func TestParseTimeout_DefaultFallback(t *testing.T) {
	var override *string = nil
	global := ""
	defaultVal := 30 * time.Second

	result := llmhttp.ParseTimeout(override, global, defaultVal)

	assert.Equal(t, 30*time.Second, result, "Should use default when no override or global")
}

func TestParseTimeout_InvalidProviderOverrideFallsBackToGlobal(t *testing.T) {
	override := stringPtr("invalid")
	global := "20s"
	defaultVal := 30 * time.Second

	result := llmhttp.ParseTimeout(override, global, defaultVal)

	assert.Equal(t, 20*time.Second, result, "Invalid provider override should fall back to global")
}

func TestParseTimeout_InvalidGlobalFallsBackToDefault(t *testing.T) {
	var override *string = nil
	global := "not-a-duration"
	defaultVal := 30 * time.Second

	result := llmhttp.ParseTimeout(override, global, defaultVal)

	assert.Equal(t, 30*time.Second, result, "Invalid global should fall back to default")
}

func TestParseTimeout_EmptyStringProviderOverrideFallsBackToGlobal(t *testing.T) {
	override := stringPtr("")
	global := "20s"
	defaultVal := 30 * time.Second

	result := llmhttp.ParseTimeout(override, global, defaultVal)

	assert.Equal(t, 20*time.Second, result, "Empty string override should fall back to global")
}

func TestParseTimeout_ZeroValue(t *testing.T) {
	override := stringPtr("0s")
	global := "20s"
	defaultVal := 30 * time.Second

	result := llmhttp.ParseTimeout(override, global, defaultVal)

	assert.Equal(t, 0*time.Second, result, "Zero duration should be valid and returned")
}

func TestParseTimeout_NegativeValueRejected(t *testing.T) {
	// Negative values should be rejected and fall back to global
	override := stringPtr("-10s")
	global := "20s"
	defaultVal := 30 * time.Second

	result := llmhttp.ParseTimeout(override, global, defaultVal)

	assert.Equal(t, 20*time.Second, result, "Negative provider override should fall back to global")
}

func TestParseTimeout_NegativeGlobalFallsBackToDefault(t *testing.T) {
	// Negative global should fall back to default
	var override *string = nil
	global := "-20s"
	defaultVal := 30 * time.Second

	result := llmhttp.ParseTimeout(override, global, defaultVal)

	assert.Equal(t, 30*time.Second, result, "Negative global should fall back to default")
}

func TestParseTimeout_NegativeDefaultUsesSafeFallback(t *testing.T) {
	// If somehow defaultVal is negative, use safe fallback
	var override *string = nil
	global := ""
	defaultVal := -10 * time.Second

	result := llmhttp.ParseTimeout(override, global, defaultVal)

	assert.Equal(t, 60*time.Second, result, "Negative default should use 60s safe fallback")
}

func TestBuildRetryConfig_AllProviderOverrides(t *testing.T) {
	providerCfg := config.ProviderConfig{
		MaxRetries:     intPtr(3),
		InitialBackoff: stringPtr("1s"),
		MaxBackoff:     stringPtr("10s"),
	}

	httpCfg := config.HTTPConfig{
		MaxRetries:        5,
		InitialBackoff:    "2s",
		MaxBackoff:        "32s",
		BackoffMultiplier: 2.5,
	}

	result := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	assert.Equal(t, 3, result.MaxRetries, "Should use provider max retries")
	assert.Equal(t, 1*time.Second, result.InitialBackoff, "Should use provider initial backoff")
	assert.Equal(t, 10*time.Second, result.MaxBackoff, "Should use provider max backoff")
	assert.Equal(t, 2.5, result.Multiplier, "Should use global multiplier")
}

func TestBuildRetryConfig_GlobalFallbacks(t *testing.T) {
	providerCfg := config.ProviderConfig{
		// No overrides
	}

	httpCfg := config.HTTPConfig{
		MaxRetries:        5,
		InitialBackoff:    "3s",
		MaxBackoff:        "40s",
		BackoffMultiplier: 3.0,
	}

	result := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	assert.Equal(t, 5, result.MaxRetries, "Should use global max retries")
	assert.Equal(t, 3*time.Second, result.InitialBackoff, "Should use global initial backoff")
	assert.Equal(t, 40*time.Second, result.MaxBackoff, "Should use global max backoff")
	assert.Equal(t, 3.0, result.Multiplier, "Should use global multiplier")
}

func TestBuildRetryConfig_DefaultFallbacks(t *testing.T) {
	providerCfg := config.ProviderConfig{
		// No overrides
	}

	httpCfg := config.HTTPConfig{
		MaxRetries:        5,
		InitialBackoff:    "", // Empty, should use default
		MaxBackoff:        "", // Empty, should use default
		BackoffMultiplier: 2.0,
	}

	result := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	assert.Equal(t, 5, result.MaxRetries, "Should use global max retries")
	assert.Equal(t, 2*time.Second, result.InitialBackoff, "Should use default initial backoff (2s)")
	assert.Equal(t, 32*time.Second, result.MaxBackoff, "Should use default max backoff (32s)")
	assert.Equal(t, 2.0, result.Multiplier, "Should use global multiplier")
}

func TestBuildRetryConfig_InvalidProviderValuesFallBackToGlobal(t *testing.T) {
	providerCfg := config.ProviderConfig{
		InitialBackoff: stringPtr("invalid-duration"),
		MaxBackoff:     stringPtr("also-invalid"),
	}

	httpCfg := config.HTTPConfig{
		MaxRetries:        5,
		InitialBackoff:    "3s",
		MaxBackoff:        "40s",
		BackoffMultiplier: 2.0,
	}

	result := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	assert.Equal(t, 3*time.Second, result.InitialBackoff, "Invalid provider should fall back to global")
	assert.Equal(t, 40*time.Second, result.MaxBackoff, "Invalid provider should fall back to global")
}

func TestBuildRetryConfig_ZeroMaxRetries(t *testing.T) {
	providerCfg := config.ProviderConfig{
		MaxRetries: intPtr(0),
	}

	httpCfg := config.HTTPConfig{
		MaxRetries:        5,
		InitialBackoff:    "2s",
		MaxBackoff:        "32s",
		BackoffMultiplier: 2.0,
	}

	result := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	assert.Equal(t, 0, result.MaxRetries, "Zero max retries should be valid (disables retries)")
}

func TestBuildRetryConfig_MixedOverridesAndFallbacks(t *testing.T) {
	providerCfg := config.ProviderConfig{
		MaxRetries:     intPtr(10),
		InitialBackoff: stringPtr("5s"),
		// MaxBackoff not overridden
	}

	httpCfg := config.HTTPConfig{
		MaxRetries:        3,
		InitialBackoff:    "2s",
		MaxBackoff:        "60s",
		BackoffMultiplier: 2.5,
	}

	result := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	assert.Equal(t, 10, result.MaxRetries, "Should use provider max retries")
	assert.Equal(t, 5*time.Second, result.InitialBackoff, "Should use provider initial backoff")
	assert.Equal(t, 60*time.Second, result.MaxBackoff, "Should fall back to global max backoff")
	assert.Equal(t, 2.5, result.Multiplier, "Should use global multiplier")
}

func TestBuildRetryConfig_EmptyStringProviderOverrides(t *testing.T) {
	providerCfg := config.ProviderConfig{
		InitialBackoff: stringPtr(""),
		MaxBackoff:     stringPtr(""),
	}

	httpCfg := config.HTTPConfig{
		MaxRetries:        5,
		InitialBackoff:    "3s",
		MaxBackoff:        "40s",
		BackoffMultiplier: 2.0,
	}

	result := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	assert.Equal(t, 3*time.Second, result.InitialBackoff, "Empty string override should fall back to global")
	assert.Equal(t, 40*time.Second, result.MaxBackoff, "Empty string override should fall back to global")
}
