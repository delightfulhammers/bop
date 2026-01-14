package auth

import (
	"testing"
	"time"
)

func TestStoredAuth_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		auth     *StoredAuth
		expected bool
	}{
		{
			name:     "nil auth is expired",
			auth:     nil,
			expected: true,
		},
		{
			name: "expired token",
			auth: &StoredAuth{
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			},
			expected: true,
		},
		{
			name: "valid token",
			auth: &StoredAuth{
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
			expected: false,
		},
		{
			name: "just expired",
			auth: &StoredAuth{
				ExpiresAt: time.Now().Add(-1 * time.Second),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.auth.IsExpired()
			if got != tt.expected {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStoredAuth_NeedsRefresh(t *testing.T) {
	tests := []struct {
		name     string
		auth     *StoredAuth
		expected bool
	}{
		{
			name:     "nil auth needs refresh",
			auth:     nil,
			expected: true,
		},
		{
			name: "already expired needs refresh",
			auth: &StoredAuth{
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			},
			expected: true,
		},
		{
			name: "expires in 3 minutes needs refresh",
			auth: &StoredAuth{
				ExpiresAt: time.Now().Add(3 * time.Minute),
			},
			expected: true,
		},
		{
			name: "expires in 10 minutes does not need refresh",
			auth: &StoredAuth{
				ExpiresAt: time.Now().Add(10 * time.Minute),
			},
			expected: false,
		},
		{
			name: "expires in 1 hour does not need refresh",
			auth: &StoredAuth{
				ExpiresAt: time.Now().Add(1 * time.Hour),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.auth.NeedsRefresh()
			if got != tt.expected {
				t.Errorf("NeedsRefresh() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDeviceFlowError_ErrorTypes(t *testing.T) {
	tests := []struct {
		name           string
		error          string
		isAuthPending  bool
		isSlowDown     bool
		isExpired      bool
		isAccessDenied bool
	}{
		{
			name:          "authorization_pending",
			error:         "authorization_pending",
			isAuthPending: true,
		},
		{
			name:       "slow_down",
			error:      "slow_down",
			isSlowDown: true,
		},
		{
			name:      "expired_token",
			error:     "expired_token",
			isExpired: true,
		},
		{
			name:           "access_denied",
			error:          "access_denied",
			isAccessDenied: true,
		},
		{
			name:  "unknown error",
			error: "server_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &DeviceFlowError{Error: tt.error}

			if got := err.IsAuthorizationPending(); got != tt.isAuthPending {
				t.Errorf("IsAuthorizationPending() = %v, want %v", got, tt.isAuthPending)
			}
			if got := err.IsSlowDown(); got != tt.isSlowDown {
				t.Errorf("IsSlowDown() = %v, want %v", got, tt.isSlowDown)
			}
			if got := err.IsExpiredToken(); got != tt.isExpired {
				t.Errorf("IsExpiredToken() = %v, want %v", got, tt.isExpired)
			}
			if got := err.IsAccessDenied(); got != tt.isAccessDenied {
				t.Errorf("IsAccessDenied() = %v, want %v", got, tt.isAccessDenied)
			}
		})
	}
}
