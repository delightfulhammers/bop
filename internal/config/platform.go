// Package config handles bop configuration loading and management.
package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// DefaultPlatformURL is the hardcoded URL for the Delightful Hammers platform.
// This is used when BOP_PLATFORM_URL is not set. Users don't need to configure
// this unless they're running a private platform instance (Enterprise).
const DefaultPlatformURL = "https://api.delightfulhammers.com"

// DefaultConfigServiceURL is the hardcoded URL for the config service.
// This is separate from the auth service (platform URL) because config
// is hosted on its own service for clean architecture separation.
const DefaultConfigServiceURL = "https://config.delightfulhammers.com"

// PlatformURLEnvVar is the environment variable for overriding the platform URL.
// Set to empty string ("") to use legacy mode (no platform).
const PlatformURLEnvVar = "BOP_PLATFORM_URL"

// ConfigServiceURLEnvVar is the environment variable for overriding the config service URL.
const ConfigServiceURLEnvVar = "BOP_CONFIG_SERVICE_URL"

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

// GetConfigServiceURL returns the effective config service URL.
// Priority: 1. BOP_CONFIG_SERVICE_URL env var, 2. DefaultConfigServiceURL constant.
// Returns empty string if legacy mode is active (BOP_PLATFORM_URL set to empty).
//
// Security: If a custom platform URL is configured (not the default), the config
// service URL must be explicitly configured to prevent accidentally sending tokens
// to the public service when using a private/enterprise platform instance.
func GetConfigServiceURL() string {
	// Legacy mode disables all platform services
	if IsLegacyEscapeHatch() {
		return ""
	}

	// Explicit config service URL always wins
	if val, exists := os.LookupEnv(ConfigServiceURLEnvVar); exists {
		return strings.TrimSpace(val)
	}

	// Security check: if custom platform URL is set, require explicit config service URL
	// to prevent token leakage to public service when using private platform
	if val, exists := os.LookupEnv(PlatformURLEnvVar); exists {
		platformURL := strings.TrimSpace(val)
		if platformURL != "" && platformURL != DefaultPlatformURL {
			// Custom platform URL without explicit config service URL - return empty
			// to force the caller to handle this case (error or skip config fetch)
			return ""
		}
	}

	return DefaultConfigServiceURL
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

// ─────────────────────────────────────────────────────────────────────────────
// Platform Config Client
// ─────────────────────────────────────────────────────────────────────────────

// ConfigFetcher is the interface for fetching product config from the platform.
// This matches the FetchProductConfig method on auth.Client.
type ConfigFetcher interface {
	FetchProductConfig(ctx context.Context, accessToken string) (*ProductConfigResponse, error)
}

// ProductConfigResponse is the response from the platform config API.
// Duplicated here to avoid import cycle with auth package.
type ProductConfigResponse struct {
	Config         map[string]any `json:"config"`
	EditableFields []string       `json:"editable_fields"`
	Tier           string         `json:"tier"`
	IsReadOnly     bool           `json:"is_read_only"`
	Schema         map[string]any `json:"schema,omitempty"`
}

// PlatformConfigClient fetches and caches configuration from the platform.
type PlatformConfigClient struct {
	fetcher  ConfigFetcher
	logger   *slog.Logger
	cacheTTL time.Duration

	mu          sync.RWMutex
	cachedCfg   *Config
	cachedTier  string
	cachedAt    time.Time
	cacheExpiry time.Time
}

// PlatformConfigClientConfig configures the platform config client.
type PlatformConfigClientConfig struct {
	// Fetcher is the interface for fetching config (typically auth.Client).
	Fetcher ConfigFetcher

	// Logger is the structured logger.
	Logger *slog.Logger

	// CacheTTL is how long to cache fetched config. Default: 5 minutes.
	CacheTTL time.Duration
}

// NewPlatformConfigClient creates a new platform config client.
// Panics if Fetcher is nil (programmer error).
func NewPlatformConfigClient(cfg PlatformConfigClientConfig) *PlatformConfigClient {
	if cfg.Fetcher == nil {
		panic("PlatformConfigClient: Fetcher is required")
	}

	ttl := cfg.CacheTTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &PlatformConfigClient{
		fetcher:  cfg.Fetcher,
		logger:   logger,
		cacheTTL: ttl,
	}
}

// FetchConfig fetches configuration from the platform and returns a merged Config.
// If platform fetch fails, returns an error (caller can decide to use fallback).
// The accessToken must be a valid JWT for the authenticated user.
func (c *PlatformConfigClient) FetchConfig(ctx context.Context, accessToken string) (*Config, string, error) {
	// Check cache first
	c.mu.RLock()
	if c.cachedCfg != nil && time.Now().Before(c.cacheExpiry) {
		// Return a copy to prevent callers from mutating the cached config
		cfg := copyConfig(c.cachedCfg)
		tier := c.cachedTier
		cacheAge := time.Since(c.cachedAt)
		c.mu.RUnlock()
		c.logger.Debug("using cached platform config",
			slog.String("tier", tier),
			slog.Duration("cache_age", cacheAge),
		)
		return cfg, tier, nil
	}
	c.mu.RUnlock()

	// Fetch from platform
	c.logger.Debug("fetching config from platform")
	resp, err := c.fetcher.FetchProductConfig(ctx, accessToken)
	if err != nil {
		return nil, "", fmt.Errorf("fetch platform config: %w", err)
	}

	// Validate response structure before accessing fields
	if resp == nil {
		return nil, "", fmt.Errorf("platform returned nil config response")
	}
	if resp.Config == nil {
		return nil, "", fmt.Errorf("platform config response missing config field")
	}
	if resp.Tier == "" {
		return nil, "", fmt.Errorf("platform config response missing tier field")
	}

	// Validate platform config contents
	if err := ValidatePlatformConfig(resp.Config); err != nil {
		return nil, "", fmt.Errorf("invalid platform config: %w", err)
	}

	// Convert to bop Config
	cfg := ConvertPlatformConfig(resp.Config, resp.Tier)

	// Cache the result
	c.mu.Lock()
	c.cachedCfg = &cfg
	c.cachedTier = resp.Tier
	c.cachedAt = time.Now()
	c.cacheExpiry = c.cachedAt.Add(c.cacheTTL)
	c.mu.Unlock()

	c.logger.Info("fetched platform config",
		slog.String("tier", resp.Tier),
		slog.Bool("is_read_only", resp.IsReadOnly),
		slog.Int("editable_fields", len(resp.EditableFields)),
	)

	// Return a copy to prevent callers from mutating the cached config
	return copyConfig(&cfg), resp.Tier, nil
}

// FetchAndMerge fetches platform config and merges it with local config.
// Uses standard overlay semantics: local config wins for overlapping fields.
//
// Intended usage pattern:
//   - Platform config sets: DefaultReviewers (which reviewers), Merge.Weights, model
//   - Local config provides: Reviewers map (personas/definitions), provider settings
//
// For overlapping keys, local config takes precedence (overlay semantics).
func (c *PlatformConfigClient) FetchAndMerge(ctx context.Context, accessToken string, localConfig Config) (*Config, string, error) {
	platformCfg, tier, err := c.FetchConfig(ctx, accessToken)
	if err != nil {
		return nil, "", err
	}

	// Merge with overlay semantics: platform is base, local overlays (local wins on conflicts)
	merged, err := MergePlatformConfig(*platformCfg, localConfig)
	if err != nil {
		return nil, "", fmt.Errorf("merge platform config: %w", err)
	}

	return &merged, tier, nil
}

// InvalidateCache clears the cached config, forcing a fresh fetch.
func (c *PlatformConfigClient) InvalidateCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cachedCfg = nil
	c.cachedTier = ""
	c.cachedAt = time.Time{}
	c.cacheExpiry = time.Time{}
}

// IsCacheValid returns true if there's a valid cached config.
func (c *PlatformConfigClient) IsCacheValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cachedCfg != nil && time.Now().Before(c.cacheExpiry)
}

// copyConfig creates a deep copy of a Config to prevent cache mutation.
// This copies all map fields to ensure callers cannot corrupt the cached config.
func copyConfig(src *Config) *Config {
	if src == nil {
		return nil
	}

	// Start with a shallow copy of the struct
	dst := *src

	// Deep copy Providers map
	if src.Providers != nil {
		dst.Providers = make(map[string]ProviderConfig, len(src.Providers))
		for k, v := range src.Providers {
			dst.Providers[k] = v
		}
	}

	// Deep copy Reviewers map
	if src.Reviewers != nil {
		dst.Reviewers = make(map[string]ReviewerConfig, len(src.Reviewers))
		for k, v := range src.Reviewers {
			dst.Reviewers[k] = v
		}
	}

	// Deep copy Merge.Weights map
	if src.Merge.Weights != nil {
		dst.Merge.Weights = make(map[string]float64, len(src.Merge.Weights))
		for k, v := range src.Merge.Weights {
			dst.Merge.Weights[k] = v
		}
	}

	// Deep copy SizeGuards.Providers map
	if src.SizeGuards.Providers != nil {
		dst.SizeGuards.Providers = make(map[string]ProviderSizeConfig, len(src.SizeGuards.Providers))
		for k, v := range src.SizeGuards.Providers {
			dst.SizeGuards.Providers[k] = v
		}
	}

	return &dst
}
