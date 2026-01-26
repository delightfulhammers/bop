package mcp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/delightfulhammers/bop/internal/auth"
)

func TestServer_RequireAuth(t *testing.T) {
	tests := []struct {
		name         string
		platformMode bool
		auth         *auth.StoredAuth
		wantErr      error
	}{
		{
			name:         "legacy mode allows all",
			platformMode: false,
			auth:         nil,
			wantErr:      nil,
		},
		{
			name:         "platform mode requires auth",
			platformMode: true,
			auth:         nil,
			wantErr:      ErrNotAuthenticated,
		},
		{
			name:         "platform mode with valid auth passes",
			platformMode: true,
			auth:         validAuth(),
			wantErr:      nil,
		},
		{
			name:         "platform mode with expired auth fails",
			platformMode: true,
			auth:         expiredAuth(),
			wantErr:      ErrAuthExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				deps: ServerDeps{
					PlatformMode: tt.platformMode,
				},
				auth: tt.auth,
			}

			err := s.RequireAuth()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestServer_RequireEntitlement(t *testing.T) {
	tests := []struct {
		name        string
		auth        *auth.StoredAuth
		entitlement string
		wantErr     bool
		errContains string
	}{
		{
			name:        "grants with explicit entitlement",
			auth:        authWith([]string{auth.EntitlementPublicRepos}),
			entitlement: auth.EntitlementPublicRepos,
			wantErr:     false,
		},
		{
			name:        "denies without entitlement",
			auth:        authWith([]string{"other-feature"}),
			entitlement: auth.EntitlementPrivateRepos,
			wantErr:     true,
			errContains: "private-repos",
		},
		{
			name:        "default-deny with empty entitlements",
			auth:        authWith([]string{}),
			entitlement: auth.EntitlementPublicRepos,
			wantErr:     true, // Empty = no permissions (default-deny)
			errContains: "public-repos",
		},
		{
			name:        "requires auth first in platform mode",
			auth:        nil,
			entitlement: auth.EntitlementPublicRepos,
			wantErr:     true,
			errContains: "not authenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				deps: ServerDeps{
					PlatformMode: true,
				},
				auth: tt.auth,
			}

			err := s.RequireEntitlement(tt.entitlement)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestServer_RequireEntitlement_LegacyMode(t *testing.T) {
	// In legacy mode (PlatformMode=false), RequireEntitlement should pass
	// even without auth because RequireAuth() returns nil in legacy mode
	s := &Server{
		deps: ServerDeps{
			PlatformMode: false,
		},
		auth: nil,
	}

	err := s.RequireEntitlement(auth.EntitlementPublicRepos)
	assert.NoError(t, err)
}

func TestServer_Entitlements(t *testing.T) {
	tests := []struct {
		name     string
		auth     *auth.StoredAuth
		checkFn  func(*auth.EntitlementChecker) bool
		expected bool
	}{
		{
			name:     "checker with valid auth can always review code (logged in)",
			auth:     authWith([]string{auth.EntitlementPublicRepos}),
			checkFn:  func(c *auth.EntitlementChecker) bool { return c.CanReviewCode() },
			expected: true,
		},
		{
			name:     "logged in user can review code regardless of entitlements",
			auth:     authWith([]string{"other-feature"}),
			checkFn:  func(c *auth.EntitlementChecker) bool { return c.CanReviewCode() },
			expected: true, // New behavior: logged-in users can always review
		},
		{
			name:     "checker with empty entitlements grants all (graceful fallback)",
			auth:     authWith([]string{}),
			checkFn:  func(c *auth.EntitlementChecker) bool { return c.CanReviewCode() },
			expected: true,
		},
		{
			name:     "checker with nil auth grants nothing",
			auth:     nil,
			checkFn:  func(c *auth.EntitlementChecker) bool { return c.CanReviewCode() },
			expected: false,
		},
		{
			name:     "private repos check with entitlement",
			auth:     authWith([]string{auth.EntitlementPrivateRepos}),
			checkFn:  func(c *auth.EntitlementChecker) bool { return c.CanReviewPrivateRepos() },
			expected: true,
		},
		{
			name:     "private repos check without entitlement",
			auth:     authWith([]string{auth.EntitlementPublicRepos}),
			checkFn:  func(c *auth.EntitlementChecker) bool { return c.CanReviewPrivateRepos() },
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				auth: tt.auth,
			}

			checker := s.Entitlements()
			require.NotNil(t, checker)
			assert.Equal(t, tt.expected, tt.checkFn(checker))
		})
	}
}

func TestEntitlementError(t *testing.T) {
	err := &EntitlementError{Entitlement: auth.EntitlementPrivateRepos}
	assert.Equal(t, `feature "private-repos" not available on your plan`, err.Error())
}

// Test helper functions

func validAuth() *auth.StoredAuth {
	return &auth.StoredAuth{
		User: auth.UserInfo{
			ID:          "user-123",
			GitHubLogin: "testuser",
			Email:       "test@example.com",
		},
		TenantID:  "tenant-123",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Entitlements: []string{
			auth.EntitlementPublicRepos,
			auth.EntitlementPrivateRepos,
			auth.EntitlementAnyOrg,
			auth.EntitlementModelSelection,
		},
		Plan: "pro",
	}
}

func expiredAuth() *auth.StoredAuth {
	return &auth.StoredAuth{
		User: auth.UserInfo{
			ID:          "user-123",
			GitHubLogin: "testuser",
			Email:       "test@example.com",
		},
		TenantID:  "tenant-123",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired
		Entitlements: []string{
			auth.EntitlementPublicRepos,
		},
		Plan: "pro",
	}
}

func authWith(entitlements []string) *auth.StoredAuth {
	return &auth.StoredAuth{
		User: auth.UserInfo{
			ID:          "user-123",
			GitHubLogin: "testuser",
			Email:       "test@example.com",
		},
		TenantID:     "tenant-123",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Entitlements: entitlements,
		Plan:         "pro",
	}
}
