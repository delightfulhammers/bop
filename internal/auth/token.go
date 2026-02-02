package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CurrentStorageVersion is the current auth.json schema version.
const CurrentStorageVersion = 1

// ErrNotLoggedIn is returned when no auth file exists.
var ErrNotLoggedIn = errors.New("not logged in")

// TokenStore handles local storage of authentication state.
type TokenStore struct {
	path string
}

// NewTokenStore creates a token store at the default location (~/.config/bop/auth.json).
func NewTokenStore() (*TokenStore, error) {
	path, err := defaultAuthPath()
	if err != nil {
		return nil, err
	}
	return &TokenStore{path: path}, nil
}

// NewTokenStoreAt creates a token store at a custom path (for testing).
func NewTokenStoreAt(path string) *TokenStore {
	return &TokenStore{path: path}
}

// Path returns the path to the auth file.
func (t *TokenStore) Path() string {
	return t.path
}

// Load reads the stored authentication state.
// Returns ErrNotLoggedIn if the file doesn't exist.
func (t *TokenStore) Load() (*StoredAuth, error) {
	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotLoggedIn
		}
		return nil, fmt.Errorf("read auth file: %w", err)
	}

	var auth StoredAuth
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("parse auth file: %w", err)
	}

	// Check version compatibility
	if auth.Version > CurrentStorageVersion {
		return nil, fmt.Errorf("auth file version %d is newer than supported version %d - please upgrade bop", auth.Version, CurrentStorageVersion)
	}

	// Validate required fields to detect corrupt/incomplete auth files
	if err := auth.Validate(); err != nil {
		return nil, fmt.Errorf("invalid auth file: %w - try 'bop auth logout' to reset", err)
	}

	return &auth, nil
}

// Save writes the authentication state to disk.
func (t *TokenStore) Save(auth *StoredAuth) error {
	if auth == nil {
		return errors.New("auth is nil")
	}

	// Use flow-aware validation: device flow requires more fields than OIDC
	if auth.IsOIDCFlow() {
		if err := auth.Validate(); err != nil {
			return fmt.Errorf("invalid auth: %w", err)
		}
	} else {
		if err := auth.ValidateForDeviceFlow(); err != nil {
			return fmt.Errorf("invalid auth: %w", err)
		}
	}

	// Ensure version is set
	auth.Version = CurrentStorageVersion

	// Ensure directory exists
	dir := filepath.Dir(t.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create auth directory: %w", err)
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth: %w", err)
	}

	// Write with restrictive permissions (user-only read/write)
	if err := os.WriteFile(t.path, data, 0600); err != nil {
		return fmt.Errorf("write auth file: %w", err)
	}

	return nil
}

// Clear removes the stored authentication state.
func (t *TokenStore) Clear() error {
	err := os.Remove(t.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove auth file: %w", err)
	}
	return nil
}

// Exists returns true if an auth file exists (may be expired).
func (t *TokenStore) Exists() bool {
	_, err := os.Stat(t.path)
	return err == nil
}

// defaultAuthPath returns the default auth file path.
func defaultAuthPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".config", "bop", "auth.json"), nil
}
