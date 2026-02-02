// Package auth provides platform authentication for bop CLI and MCP server.
// It handles device flow OAuth, token storage, refresh, and entitlement checking.
package auth

import (
	"errors"
	"time"
)

// StoredAuth represents the locally persisted authentication state.
// This is saved to ~/.config/bop/auth.json after successful login.
type StoredAuth struct {
	// Version tracks the schema version for forward compatibility.
	Version int `json:"version"`

	// AccessToken is the JWT access token for API requests.
	AccessToken string `json:"access_token"`

	// RefreshToken is used to obtain new access tokens.
	RefreshToken string `json:"refresh_token"`

	// ExpiresAt is when the access token expires.
	ExpiresAt time.Time `json:"expires_at"`

	// User contains the authenticated user's information.
	User UserInfo `json:"user"`

	// TenantID is the user's tenant identifier.
	TenantID string `json:"tenant_id"`

	// Entitlements are the features the user has access to.
	// Empty slice means no permissions (default-deny).
	Entitlements []string `json:"entitlements"`

	// Plan is the user's subscription plan (e.g., "free", "individual", "business").
	Plan string `json:"plan"`
}

// UserInfo contains basic user information.
type UserInfo struct {
	ID          string `json:"id"`
	GitHubLogin string `json:"github_login"`
	Email       string `json:"email"`
}

// IsExpired returns true if the access token has expired.
func (s *StoredAuth) IsExpired() bool {
	if s == nil {
		return true
	}
	return time.Now().After(s.ExpiresAt)
}

// NeedsRefresh returns true if the access token should be refreshed.
// This returns true if the token expires within 5 minutes.
func (s *StoredAuth) NeedsRefresh() bool {
	if s == nil {
		return true
	}
	return time.Now().Add(5 * time.Minute).After(s.ExpiresAt)
}

// IsOIDCFlow returns true if this auth state is from OIDC flow.
// OIDC flow is stateless (no refresh token) and tenant-less (actor-based).
func (s *StoredAuth) IsOIDCFlow() bool {
	return s.RefreshToken == ""
}

// Validate checks that required fields are present and valid.
// Returns an error if the auth state is incomplete or corrupt.
// This is the common validation that applies to all auth flows.
func (s *StoredAuth) Validate() error {
	if s.AccessToken == "" {
		return errors.New("missing access_token")
	}
	if s.ExpiresAt.IsZero() {
		return errors.New("missing expires_at")
	}
	// Validate user identity fields
	if s.User.ID == "" {
		return errors.New("missing user.id")
	}
	if s.User.GitHubLogin == "" {
		return errors.New("missing user.github_login")
	}
	return nil
}

// ValidateForDeviceFlow checks that all fields required for device flow are present.
// Device flow requires RefreshToken and TenantID for token refresh operations.
func (s *StoredAuth) ValidateForDeviceFlow() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.RefreshToken == "" {
		return errors.New("missing refresh_token (required for device flow)")
	}
	if s.TenantID == "" {
		return errors.New("missing tenant_id (required for device flow)")
	}
	return nil
}

// DeviceFlowResponse is returned when initiating device flow.
type DeviceFlowResponse struct {
	// DeviceCode is the code used for polling.
	DeviceCode string `json:"device_code"`

	// UserCode is displayed to the user to enter at the verification URI.
	UserCode string `json:"user_code"`

	// VerificationURI is where the user enters the code.
	VerificationURI string `json:"verification_uri"`

	// ExpiresIn is how long the device code is valid (seconds).
	ExpiresIn int `json:"expires_in"`

	// Interval is the minimum polling interval (seconds).
	Interval int `json:"interval"`
}

// TokenResponse is returned when polling succeeds.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`

	// User info fields (populated in OIDC flow so client doesn't need to call /auth/me)
	UserID       string   `json:"user_id,omitempty"`
	Username     string   `json:"username,omitempty"`
	TenantID     string   `json:"tenant_id,omitempty"`
	PlanID       string   `json:"plan_id,omitempty"`
	Entitlements []string `json:"entitlements,omitempty"`
}

// DeviceFlowError represents an error during device flow polling.
type DeviceFlowError struct {
	// Error is the error code (e.g., "authorization_pending", "slow_down").
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

// IsAuthorizationPending returns true if the user hasn't authorized yet.
func (e *DeviceFlowError) IsAuthorizationPending() bool {
	return e.Error == "authorization_pending"
}

// IsSlowDown returns true if we're polling too fast.
func (e *DeviceFlowError) IsSlowDown() bool {
	return e.Error == "slow_down"
}

// IsExpiredToken returns true if the device code has expired.
func (e *DeviceFlowError) IsExpiredToken() bool {
	return e.Error == "expired_token"
}

// IsAccessDenied returns true if the user denied authorization.
func (e *DeviceFlowError) IsAccessDenied() bool {
	return e.Error == "access_denied"
}

// CurrentUserResponse is the response from the /auth/me endpoint.
type CurrentUserResponse struct {
	UserID       string   `json:"user_id"`
	Email        string   `json:"email"`
	Username     string   `json:"username"`
	ProviderType string   `json:"provider_type"`
	ProviderID   string   `json:"provider_id"`
	ProductID    string   `json:"product_id"`
	TenantID     string   `json:"tenant_id"`
	PlanID       string   `json:"plan_id"`
	PlanStatus   string   `json:"plan_status"`
	Entitlements []string `json:"entitlements"`
}
