package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenStore_SaveAndLoad(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()
	authPath := filepath.Join(tmpDir, "auth.json")

	store := NewTokenStoreAt(authPath)

	// Test saving
	auth := &StoredAuth{
		Version:      CurrentStorageVersion,
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Truncate(time.Second),
		User: UserInfo{
			ID:          "user-123",
			GitHubLogin: "testuser",
			Email:       "test@example.com",
		},
		TenantID:     "tenant-456",
		Entitlements: []string{"code-review", "private-repos"},
		Plan:         "individual",
	}

	if err := store.Save(auth); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists with correct permissions
	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}

	// Test loading
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify loaded data
	if loaded.Version != auth.Version {
		t.Errorf("Version = %d, want %d", loaded.Version, auth.Version)
	}
	if loaded.AccessToken != auth.AccessToken {
		t.Errorf("AccessToken = %s, want %s", loaded.AccessToken, auth.AccessToken)
	}
	if loaded.RefreshToken != auth.RefreshToken {
		t.Errorf("RefreshToken = %s, want %s", loaded.RefreshToken, auth.RefreshToken)
	}
	if !loaded.ExpiresAt.Equal(auth.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", loaded.ExpiresAt, auth.ExpiresAt)
	}
	if loaded.User.ID != auth.User.ID {
		t.Errorf("User.ID = %s, want %s", loaded.User.ID, auth.User.ID)
	}
	if loaded.User.GitHubLogin != auth.User.GitHubLogin {
		t.Errorf("User.GitHubLogin = %s, want %s", loaded.User.GitHubLogin, auth.User.GitHubLogin)
	}
	if loaded.User.Email != auth.User.Email {
		t.Errorf("User.Email = %s, want %s", loaded.User.Email, auth.User.Email)
	}
	if loaded.TenantID != auth.TenantID {
		t.Errorf("TenantID = %s, want %s", loaded.TenantID, auth.TenantID)
	}
	if loaded.Plan != auth.Plan {
		t.Errorf("Plan = %s, want %s", loaded.Plan, auth.Plan)
	}
	if len(loaded.Entitlements) != len(auth.Entitlements) {
		t.Errorf("Entitlements length = %d, want %d", len(loaded.Entitlements), len(auth.Entitlements))
	}
}

func TestTokenStore_LoadNotLoggedIn(t *testing.T) {
	tmpDir := t.TempDir()
	authPath := filepath.Join(tmpDir, "nonexistent", "auth.json")

	store := NewTokenStoreAt(authPath)

	_, err := store.Load()
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("Load() error = %v, want ErrNotLoggedIn", err)
	}
}

func TestTokenStore_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	authPath := filepath.Join(tmpDir, "auth.json")

	store := NewTokenStoreAt(authPath)

	// Save some auth
	auth := &StoredAuth{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	if err := store.Save(auth); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify it exists
	if !store.Exists() {
		t.Error("Exists() = false, want true after save")
	}

	// Clear it
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	// Verify it's gone
	if store.Exists() {
		t.Error("Exists() = true, want false after clear")
	}

	// Clear again should not error
	if err := store.Clear(); err != nil {
		t.Errorf("Clear() on nonexistent file error = %v, want nil", err)
	}
}

func TestTokenStore_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	authPath := filepath.Join(tmpDir, "auth.json")

	store := NewTokenStoreAt(authPath)

	// Should not exist initially
	if store.Exists() {
		t.Error("Exists() = true for nonexistent file")
	}

	// Create file
	if err := os.WriteFile(authPath, []byte("{}"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Should exist now
	if !store.Exists() {
		t.Error("Exists() = false for existing file")
	}
}

func TestTokenStore_Path(t *testing.T) {
	customPath := "/custom/path/auth.json"
	store := NewTokenStoreAt(customPath)

	if got := store.Path(); got != customPath {
		t.Errorf("Path() = %s, want %s", got, customPath)
	}
}

func TestTokenStore_VersionCheck(t *testing.T) {
	tmpDir := t.TempDir()
	authPath := filepath.Join(tmpDir, "auth.json")

	// Write a file with a future version
	futureJSON := `{"version": 999, "access_token": "test"}`
	if err := os.WriteFile(authPath, []byte(futureJSON), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := NewTokenStoreAt(authPath)

	_, err := store.Load()
	if err == nil {
		t.Error("Load() should error for future version")
	}
}

func TestNewTokenStore(t *testing.T) {
	store, err := NewTokenStore()
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}

	// Path should end with auth.json in .config/bop
	path := store.Path()
	if filepath.Base(path) != "auth.json" {
		t.Errorf("Path() base = %s, want auth.json", filepath.Base(path))
	}
	if filepath.Base(filepath.Dir(path)) != "bop" {
		t.Errorf("Path() parent = %s, want bop", filepath.Base(filepath.Dir(path)))
	}
}
