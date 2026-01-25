package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCachedConfig_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "future expiry is not expired",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "past expiry is expired",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &CachedConfig{ExpiresAt: tt.expiresAt}
			if got := cc.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}

	// nil cache is expired
	t.Run("nil cache is expired", func(t *testing.T) {
		var cc *CachedConfig
		if got := cc.IsExpired(); !got {
			t.Errorf("IsExpired() = %v, want true for nil", got)
		}
	})
}

func TestCachedConfig_IsValid(t *testing.T) {
	validTenant := "tenant-123"
	validTier := "pro"

	tests := []struct {
		name     string
		cache    *CachedConfig
		tenantID string
		tier     string
		want     bool
	}{
		{
			name: "valid cache for matching tenant and tier",
			cache: &CachedConfig{
				TenantID:  validTenant,
				Tier:      validTier,
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
			tenantID: validTenant,
			tier:     validTier,
			want:     true,
		},
		{
			name: "invalid cache for different tenant",
			cache: &CachedConfig{
				TenantID:  validTenant,
				Tier:      validTier,
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
			tenantID: "other-tenant",
			tier:     validTier,
			want:     false,
		},
		{
			name: "invalid cache for different tier",
			cache: &CachedConfig{
				TenantID:  validTenant,
				Tier:      validTier,
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
			tenantID: validTenant,
			tier:     "enterprise",
			want:     false,
		},
		{
			name: "expired cache is invalid",
			cache: &CachedConfig{
				TenantID:  validTenant,
				Tier:      validTier,
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			},
			tenantID: validTenant,
			tier:     validTier,
			want:     false,
		},
		{
			name:     "nil cache is invalid",
			cache:    nil,
			tenantID: validTenant,
			tier:     validTier,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cache.IsValid(tt.tenantID, tt.tier); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigCache_SaveAndLoad(t *testing.T) {
	// Create temp directory for test cache
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "config-cache.json")

	cache := NewConfigCacheWithPath(cachePath, 1*time.Hour)

	// Test data
	tenantID := "tenant-123"
	tier := "pro"
	configData := map[string]any{
		"reviewers": []string{"code-reviewer"},
		"model":     "claude-sonnet-4-20250514",
	}

	// Save config
	err := cache.Save(configData, tier, tenantID)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Error("cache file was not created")
	}

	// Load config
	loaded, err := cache.Load(tenantID, tier)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("Load() returned nil")
	}

	// Verify loaded data
	if loaded.Tier != tier {
		t.Errorf("loaded Tier = %q, want %q", loaded.Tier, tier)
	}
	if loaded.TenantID != tenantID {
		t.Errorf("loaded TenantID = %q, want %q", loaded.TenantID, tenantID)
	}
	if loaded.IsExpired() {
		t.Error("loaded cache should not be expired")
	}
}

func TestConfigCache_LoadDifferentTenant(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "config-cache.json")

	cache := NewConfigCacheWithPath(cachePath, 1*time.Hour)

	// Save for tenant-123
	err := cache.Save(map[string]any{"model": "test"}, "pro", "tenant-123")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load for different tenant should return nil
	loaded, err := cache.Load("tenant-456", "pro")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != nil {
		t.Error("Load() should return nil for different tenant")
	}
}

func TestConfigCache_LoadDifferentTier(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "config-cache.json")

	cache := NewConfigCacheWithPath(cachePath, 1*time.Hour)

	// Save for pro tier
	err := cache.Save(map[string]any{"model": "test"}, "pro", "tenant-123")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load for different tier should return nil (prevents stale config after upgrade/downgrade)
	loaded, err := cache.Load("tenant-123", "enterprise")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != nil {
		t.Error("Load() should return nil for different tier")
	}
}

func TestConfigCache_LoadNoCache(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "nonexistent", "cache.json")

	cache := NewConfigCacheWithPath(cachePath, 1*time.Hour)

	// Load should return nil when no cache exists
	loaded, err := cache.Load("any-tenant", "any-tier")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != nil {
		t.Error("Load() should return nil when no cache exists")
	}
}

func TestConfigCache_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "config-cache.json")

	cache := NewConfigCacheWithPath(cachePath, 1*time.Hour)

	// Save config
	err := cache.Save(map[string]any{"model": "test"}, "pro", "tenant-123")
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Clear cache
	err = cache.Clear()
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	// Verify file was removed
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Error("cache file should be removed after Clear()")
	}

	// Clear on non-existent file should not error
	err = cache.Clear()
	if err != nil {
		t.Errorf("Clear() on non-existent file should not error: %v", err)
	}
}
