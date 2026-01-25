package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

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

// IsValid returns true if the cached config is valid for the given tenant.
func (c *CachedConfig) IsValid(tenantID string) bool {
	if c == nil {
		return false
	}
	if c.TenantID != tenantID {
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
// Returns nil if no cache exists or cache is expired.
func (c *ConfigCache) Load(tenantID string) (*CachedConfig, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No cache exists, not an error
		}
		return nil, err
	}

	var cached CachedConfig
	if err := json.Unmarshal(data, &cached); err != nil {
		// Corrupt cache, treat as missing
		return nil, nil
	}

	// Check if cache is valid for this tenant
	if !cached.IsValid(tenantID) {
		return nil, nil
	}

	return &cached, nil
}

// Save writes the config to cache.
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
	if err := os.MkdirAll(filepath.Dir(c.path), 0755); err != nil {
		return err
	}

	// Write with restricted permissions (user-only)
	return os.WriteFile(c.path, data, 0600)
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
