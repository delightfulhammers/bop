package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	original := &Credentials{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TenantID:     "tenant-789",
		ExpiresAt:    time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		UserID:       "user-001",
		Username:     "testuser",
		PlatformURL:  "https://api.example.com",
	}

	if err := saveCredentialsToPath(path, original); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat credentials file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600 permissions, got %o", perm)
	}

	loaded, err := loadCredentialsFromPath(path)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}

	if loaded.AccessToken != original.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", loaded.AccessToken, original.AccessToken)
	}
	if loaded.RefreshToken != original.RefreshToken {
		t.Errorf("RefreshToken: got %q, want %q", loaded.RefreshToken, original.RefreshToken)
	}
	if loaded.TenantID != original.TenantID {
		t.Errorf("TenantID: got %q, want %q", loaded.TenantID, original.TenantID)
	}
	if !loaded.ExpiresAt.Equal(original.ExpiresAt) {
		t.Errorf("ExpiresAt: got %v, want %v", loaded.ExpiresAt, original.ExpiresAt)
	}
	if loaded.UserID != original.UserID {
		t.Errorf("UserID: got %q, want %q", loaded.UserID, original.UserID)
	}
	if loaded.Username != original.Username {
		t.Errorf("Username: got %q, want %q", loaded.Username, original.Username)
	}
	if loaded.PlatformURL != original.PlatformURL {
		t.Errorf("PlatformURL: got %q, want %q", loaded.PlatformURL, original.PlatformURL)
	}
}

func TestLoadCredentials_FileNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")

	creds, err := loadCredentialsFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds != nil {
		t.Fatalf("expected nil credentials for missing file, got %+v", creds)
	}
}

func TestLoadCredentials_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := loadCredentialsFromPath(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestClearCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := clearCredentialsAtPath(path); err != nil {
		t.Fatalf("ClearCredentials: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected credentials file to be removed")
	}
}

func TestClearCredentials_FileNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	if err := clearCredentialsAtPath(path); err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
}

func TestCredentials_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"expired", time.Now().Add(-time.Hour), true},
		{"not expired", time.Now().Add(time.Hour), false},
		{"just expired", time.Now().Add(-time.Millisecond), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Credentials{ExpiresAt: tt.expiresAt}
			if got := c.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCredentials_NeedsRefresh(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"within grace period", time.Now().Add(time.Minute), true},
		{"well before expiry", time.Now().Add(10 * time.Minute), false},
		{"expired", time.Now().Add(-time.Hour), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Credentials{ExpiresAt: tt.expiresAt}
			if got := c.NeedsRefresh(); got != tt.want {
				t.Errorf("NeedsRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveCredentials_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "dir", "credentials.json")

	creds := &Credentials{AccessToken: "test"}
	if err := saveCredentialsToPath(path, creds); err != nil {
		t.Fatalf("SaveCredentials to nested dir: %v", err)
	}

	loaded, err := loadCredentialsFromPath(path)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if loaded.AccessToken != "test" {
		t.Errorf("AccessToken: got %q, want %q", loaded.AccessToken, "test")
	}
}
