package config

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxCacheFileSize limits cache file reads to prevent memory exhaustion from corrupted/malicious files.
const maxCacheFileSize = 1 << 20 // 1MB - same limit as network fetch

// DefaultCacheTTL is how long cached config is valid before re-fetching.
const DefaultCacheTTL = 1 * time.Hour

// CachedConfig represents a cached platform configuration response.
type CachedConfig struct {
	// Config is the rendered configuration from the platform.
	Config map[string]any `json:"config"`

	// Tier is the user's subscription tier when config was fetched.
	Tier string `json:"tier"`

	// FetchedAt is when the config was fetched from the platform.
	FetchedAt time.Time `json:"fetched_at"`

	// ExpiresAt is when this cached config should be refreshed.
	ExpiresAt time.Time `json:"expires_at"`

	// TenantID is the tenant this config belongs to.
	TenantID string `json:"tenant_id"`
}

// IsExpired returns true if the cached config has expired.
func (c *CachedConfig) IsExpired() bool {
	if c == nil {
		return true
	}
	return time.Now().After(c.ExpiresAt)
}

// IsValid returns true if the cached config is valid for the given tenant and tier.
// Both tenant ID and tier must match to prevent using stale config after tier changes.
func (c *CachedConfig) IsValid(tenantID, tier string) bool {
	if c == nil {
		return false
	}
	if c.TenantID != tenantID {
		return false
	}
	// Tier must match to prevent using Enterprise config on downgraded account
	// Case-insensitive to handle platform capitalization variations (e.g., "Pro" vs "pro")
	if !strings.EqualFold(c.Tier, tier) {
		return false
	}
	return !c.IsExpired()
}

// ConfigCache handles caching platform config to disk.
type ConfigCache struct {
	path string
	ttl  time.Duration
}

// NewConfigCache creates a config cache at the default location.
// Default path: ~/.config/bop/config-cache.json
func NewConfigCache() (*ConfigCache, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(home, ".config", "bop", "config-cache.json")
	return &ConfigCache{
		path: path,
		ttl:  DefaultCacheTTL,
	}, nil
}

// NewConfigCacheWithPath creates a config cache at a custom path.
func NewConfigCacheWithPath(path string, ttl time.Duration) *ConfigCache {
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	return &ConfigCache{
		path: path,
		ttl:  ttl,
	}
}

// Load reads the cached config from disk.
// Returns nil if no cache exists, cache is expired, or tenant/tier doesn't match.
// Limits file size to maxCacheFileSize to prevent memory exhaustion from corrupted files.
func (c *ConfigCache) Load(tenantID, tier string) (*CachedConfig, error) {
	f, err := os.Open(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No cache exists, not an error
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// Limit read size to prevent memory exhaustion from corrupted/malicious cache files
	limitedReader := io.LimitReader(f, maxCacheFileSize)

	var cached CachedConfig
	if err := json.NewDecoder(limitedReader).Decode(&cached); err != nil {
		// Corrupt cache, treat as missing
		return nil, nil
	}

	// Check if cache is valid for this tenant and tier
	if !cached.IsValid(tenantID, tier) {
		return nil, nil
	}

	return &cached, nil
}

// Save writes the config to cache using atomic write (temp file + rename).
// This prevents cache corruption when multiple bop instances run concurrently.
func (c *ConfigCache) Save(config map[string]any, tier, tenantID string) error {
	cached := CachedConfig{
		Config:    config,
		Tier:      tier,
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(c.ttl),
		TenantID:  tenantID,
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write to temp file first, then atomically rename
	// This prevents partial reads if another process reads during write
	tmpFile, err := os.CreateTemp(dir, ".config-cache-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	// Write data with restricted permissions
	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	// Atomic rename
	if err := os.Rename(tmpPath, c.path); err != nil {
		return err
	}

	success = true
	return nil
}

// Clear removes the cached config.
func (c *ConfigCache) Clear() error {
	if err := os.Remove(c.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Path returns the cache file path.
func (c *ConfigCache) Path() string {
	return c.path
}
