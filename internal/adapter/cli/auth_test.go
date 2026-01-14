package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/delightfulhammers/bop/internal/auth"
)

func TestAuthDependencies_RequireAuth(t *testing.T) {
	t.Run("legacy mode returns nil checker and nil error", func(t *testing.T) {
		deps := AuthDependencies{
			TokenStore: nil, // Legacy mode - no token store
		}

		checker, err := deps.RequireAuth()
		assert.NoError(t, err)
		assert.Nil(t, checker)
	})

	t.Run("not logged in returns error", func(t *testing.T) {
		// Create temp dir for token store
		tmpDir := t.TempDir()
		tokenStore := auth.NewTokenStoreAt(filepath.Join(tmpDir, "auth.json"))

		deps := AuthDependencies{
			TokenStore: tokenStore,
		}

		checker, err := deps.RequireAuth()
		require.Error(t, err)
		assert.Nil(t, checker)
		assert.Contains(t, err.Error(), "not authenticated")
		assert.Contains(t, err.Error(), "bop auth login")
	})

	t.Run("expired token returns error", func(t *testing.T) {
		// Create temp dir for token store
		tmpDir := t.TempDir()
		authFile := filepath.Join(tmpDir, "auth.json")
		tokenStore := auth.NewTokenStoreAt(authFile)

		// Save expired auth
		expiredAuth := &auth.StoredAuth{
			Version:      1,
			AccessToken:  "expired-token",
			RefreshToken: "refresh-token",
			User: auth.UserInfo{
				ID:          "user-123",
				GitHubLogin: "testuser",
				Email:       "test@example.com",
			},
			TenantID:  "tenant-123",
			ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired
		}
		require.NoError(t, tokenStore.Save(expiredAuth))

		deps := AuthDependencies{
			TokenStore: tokenStore,
		}

		checker, err := deps.RequireAuth()
		require.Error(t, err)
		assert.Nil(t, checker)
		assert.Contains(t, err.Error(), "expired")
	})

	t.Run("valid auth returns checker", func(t *testing.T) {
		// Create temp dir for token store
		tmpDir := t.TempDir()
		authFile := filepath.Join(tmpDir, "auth.json")
		tokenStore := auth.NewTokenStoreAt(authFile)

		// Save valid auth with entitlements
		validAuth := &auth.StoredAuth{
			Version:      1,
			AccessToken:  "valid-token",
			RefreshToken: "refresh-token",
			User: auth.UserInfo{
				ID:          "user-123",
				GitHubLogin: "testuser",
				Email:       "test@example.com",
			},
			TenantID:     "tenant-123",
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			Entitlements: []string{"code-review"},
			Plan:         "pro",
		}
		require.NoError(t, tokenStore.Save(validAuth))

		deps := AuthDependencies{
			TokenStore: tokenStore,
		}

		checker, err := deps.RequireAuth()
		require.NoError(t, err)
		require.NotNil(t, checker)
		assert.True(t, checker.CanReviewCode())
	})

	t.Run("valid auth with empty entitlements grants all (graceful fallback)", func(t *testing.T) {
		// Create temp dir for token store
		tmpDir := t.TempDir()
		authFile := filepath.Join(tmpDir, "auth.json")
		tokenStore := auth.NewTokenStoreAt(authFile)

		// Save valid auth with empty entitlements (platform not enforcing)
		validAuth := &auth.StoredAuth{
			Version:      1,
			AccessToken:  "valid-token",
			RefreshToken: "refresh-token",
			User: auth.UserInfo{
				ID:          "user-123",
				GitHubLogin: "testuser",
				Email:       "test@example.com",
			},
			TenantID:     "tenant-123",
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			Entitlements: []string{}, // Empty = all granted
			Plan:         "",
		}
		require.NoError(t, tokenStore.Save(validAuth))

		deps := AuthDependencies{
			TokenStore: tokenStore,
		}

		checker, err := deps.RequireAuth()
		require.NoError(t, err)
		require.NotNil(t, checker)
		// Graceful fallback: empty entitlements grants all
		assert.True(t, checker.CanReviewCode())
		assert.True(t, checker.HasUnlimitedReviews())
	})

	t.Run("corrupt auth file returns error", func(t *testing.T) {
		// Create temp dir for token store
		tmpDir := t.TempDir()
		authFile := filepath.Join(tmpDir, "auth.json")

		// Write corrupt data
		require.NoError(t, os.WriteFile(authFile, []byte("{not valid json"), 0600))

		tokenStore := auth.NewTokenStoreAt(authFile)

		deps := AuthDependencies{
			TokenStore: tokenStore,
		}

		checker, err := deps.RequireAuth()
		require.Error(t, err)
		assert.Nil(t, checker)
		assert.Contains(t, err.Error(), "auth error")
	})
}
