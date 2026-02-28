package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	credentialsFileName = "credentials.json"
	credentialsDirPerm  = 0700
	credentialsFilePerm = 0600
	refreshGracePeriod  = 2 * time.Minute
)

// Credentials holds the persisted authentication state for the platform.
type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TenantID     string    `json:"tenant_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	UserID       string    `json:"user_id"`
	Username     string    `json:"username"`
	PlatformURL  string    `json:"platform_url"`
}

// IsExpired returns true if the access token has expired.
func (c *Credentials) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// NeedsRefresh returns true if the access token is within the grace period of expiry.
func (c *Credentials) NeedsRefresh() bool {
	return time.Now().After(c.ExpiresAt.Add(-refreshGracePeriod))
}

// credentialsDir returns the directory for bop credentials (~/.config/bop/).
func credentialsDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(configDir, "bop"), nil
}

// credentialsPath returns the full path to the credentials file.
func credentialsPath() (string, error) {
	dir, err := credentialsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, credentialsFileName), nil
}

// LoadCredentials reads credentials from ~/.config/bop/credentials.json.
// Returns nil and no error if the file does not exist (not logged in).
func LoadCredentials() (*Credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	return loadCredentialsFromPath(path)
}

func loadCredentialsFromPath(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	return &creds, nil
}

// SaveCredentials writes credentials to ~/.config/bop/credentials.json atomically.
// Creates the directory with 0700 permissions and the file with 0600 permissions.
func SaveCredentials(creds *Credentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	return saveCredentialsToPath(path, creds)
}

func saveCredentialsToPath(path string, creds *Credentials) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, credentialsDirPerm); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	// Atomic write: write to temp file (with restricted permissions from creation)
	// then rename. Using os.OpenFile ensures the file is never world-readable,
	// even briefly before rename.
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, credentialsFilePerm)
	if err != nil {
		return fmt.Errorf("create credentials temp file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write credentials temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close credentials temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath) // Clean up on rename failure
		return fmt.Errorf("rename credentials file: %w", err)
	}
	return nil
}

// ClearCredentials removes the credentials file.
// Returns nil if the file does not exist.
func ClearCredentials() error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	return clearCredentialsAtPath(path)
}

func clearCredentialsAtPath(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove credentials: %w", err)
	}
	return nil
}
