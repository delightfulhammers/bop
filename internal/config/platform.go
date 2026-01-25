// Package config handles bop configuration loading and management.
package config

import (
	"os"
	"strings"
)

// DefaultPlatformURL is the hardcoded URL for the Delightful Hammers platform.
// This is used when BOP_PLATFORM_URL is not set. Users don't need to configure
// this unless they're running a private platform instance (Enterprise).
const DefaultPlatformURL = "https://api.delightfulhammers.com"

// PlatformURLEnvVar is the environment variable for overriding the platform URL.
// Set to empty string ("") to use legacy mode (no platform).
const PlatformURLEnvVar = "BOP_PLATFORM_URL"

// GetPlatformURL returns the effective platform URL.
// Priority: 1. BOP_PLATFORM_URL env var, 2. DefaultPlatformURL constant.
// Returns empty string if BOP_PLATFORM_URL is explicitly set to empty (legacy mode).
func GetPlatformURL() string {
	// Check if BOP_PLATFORM_URL is explicitly set (including to empty string)
	if val, exists := os.LookupEnv(PlatformURLEnvVar); exists {
		return strings.TrimSpace(val)
	}
	return DefaultPlatformURL
}

// IsLegacyEscapeHatch returns true if BOP_PLATFORM_URL is explicitly set to empty.
// This is the legacy mode escape hatch for users who want to use local config only.
func IsLegacyEscapeHatch() bool {
	val, exists := os.LookupEnv(PlatformURLEnvVar)
	return exists && strings.TrimSpace(val) == ""
}

// OperationalFlags are flags that control how bop operates (not what it reviews).
// These are allowed without authentication and don't require entitlements.
type OperationalFlags struct {
	// LogLevel controls logging verbosity (trace, debug, info, error).
	LogLevel string

	// Verbose enables verbose output.
	Verbose bool

	// Debug enables debug mode with additional diagnostics.
	Debug bool

	// Version prints version and exits.
	Version bool

	// Help prints help and exits.
	Help bool
}

// OperationalEnvVars are environment variables that control bop operation.
// These are always respected, regardless of authentication or entitlements.
var OperationalEnvVars = []string{
	"BOP_PLATFORM_URL", // Platform URL override (empty = legacy mode)
	"BOP_LOG_LEVEL",    // Logging level override
}

// ConfigEnvVars are environment variables that configure bop behavior.
// These require local-bop-config entitlement when platform mode is active.
var ConfigEnvVars = []string{
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"GEMINI_API_KEY",
	"GITHUB_TOKEN",
	"OLLAMA_HOST",
	// BOP_REVIEW_*, BOP_PROVIDERS_*, etc. are also config vars
}

// IsOperationalEnvVar returns true if the given env var is operational (always allowed).
func IsOperationalEnvVar(name string) bool {
	for _, v := range OperationalEnvVars {
		if strings.EqualFold(v, name) {
			return true
		}
	}
	return false
}

// IsConfigEnvVar returns true if the given env var is a config var (requires entitlement).
func IsConfigEnvVar(name string) bool {
	// Explicit config vars
	for _, v := range ConfigEnvVars {
		if strings.EqualFold(v, name) {
			return true
		}
	}
	// BOP_* vars (except operational ones) are config vars
	if strings.HasPrefix(strings.ToUpper(name), "BOP_") && !IsOperationalEnvVar(name) {
		return true
	}
	return false
}
