package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// DeviceFlowErrors for different failure modes.
var (
	ErrDeviceCodeExpired      = errors.New("device code expired - please try again")
	ErrAccessDenied           = errors.New("authorization denied by user")
	ErrFlowCanceled           = errors.New("device flow canceled")
	ErrInvalidVerificationURI = errors.New("invalid verification URI from auth service")
)

// allowedVerificationPaths defines the trusted URL paths for device flow verification.
// This prevents phishing attacks where a compromised auth-service could direct users
// to malicious sites.
var allowedVerificationPaths = []string{
	"/login/device",
	"/login/oauth/authorize",
}

// validateVerificationURI checks that the verification URI is from a trusted domain.
// Uses url.Parse to prevent path traversal attacks (e.g., /login/device/../../../evil.com).
func validateVerificationURI(uri string) error {
	parsed, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("%w: parse error: %s", ErrInvalidVerificationURI, uri)
	}

	// Validate scheme
	if parsed.Scheme != "https" {
		return fmt.Errorf("%w: must use https: %s", ErrInvalidVerificationURI, uri)
	}

	// Validate host (exact match, no subdomains)
	if parsed.Host != "github.com" {
		return fmt.Errorf("%w: untrusted host: %s", ErrInvalidVerificationURI, uri)
	}

	// Validate path starts with an allowed prefix
	// path.Clean is already applied by url.Parse, preventing traversal
	for _, allowedPath := range allowedVerificationPaths {
		if strings.HasPrefix(parsed.Path, allowedPath) {
			return nil
		}
	}

	return fmt.Errorf("%w: untrusted path: %s", ErrInvalidVerificationURI, uri)
}

// DeviceFlowCallbacks receives notifications during the device flow.
type DeviceFlowCallbacks struct {
	// OnUserCode is called when the user code is available for display.
	OnUserCode func(userCode, verificationURI string)

	// OnPolling is called before each poll attempt.
	OnPolling func(attempt int)

	// OnSlowDown is called when the server requests slower polling.
	OnSlowDown func(newInterval time.Duration)
}

// DeviceFlowResult contains the result of a successful device flow.
type DeviceFlowResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	User         *CurrentUserResponse
}

// RunDeviceFlow executes the complete device flow authentication.
// It initiates the flow, displays the user code, polls for completion,
// and retrieves user information on success.
func RunDeviceFlow(ctx context.Context, client *Client, callbacks DeviceFlowCallbacks) (*DeviceFlowResult, error) {
	// Step 1: Initiate device flow
	initResp, err := client.InitiateDeviceFlow(ctx)
	if err != nil {
		return nil, fmt.Errorf("initiate device flow: %w", err)
	}

	// Validate device flow response fields
	if initResp.DeviceCode == "" || initResp.UserCode == "" {
		return nil, fmt.Errorf("invalid device flow response: missing device_code or user_code")
	}
	if initResp.ExpiresIn <= 0 {
		return nil, fmt.Errorf("invalid device flow response: expires_in must be positive, got %d", initResp.ExpiresIn)
	}

	// Validate verification URI to prevent phishing attacks
	if err := validateVerificationURI(initResp.VerificationURI); err != nil {
		return nil, err
	}

	// Notify callback of user code
	if callbacks.OnUserCode != nil {
		callbacks.OnUserCode(initResp.UserCode, initResp.VerificationURI)
	}

	// Step 2: Poll for token
	interval := time.Duration(initResp.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second // RFC 8628 minimum
	}

	deadline := time.Now().Add(time.Duration(initResp.ExpiresIn) * time.Second)
	attempt := 0

	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ErrFlowCanceled
		default:
		}

		// Check if device code has expired
		if time.Now().After(deadline) {
			return nil, ErrDeviceCodeExpired
		}

		// Wait for interval before polling
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ErrFlowCanceled
		case <-timer.C:
		}

		attempt++
		if callbacks.OnPolling != nil {
			callbacks.OnPolling(attempt)
		}

		// Poll for token
		tokenResp, dfErr, err := client.PollDeviceToken(ctx, initResp.DeviceCode)
		if err != nil {
			return nil, fmt.Errorf("poll for token: %w", err)
		}

		// Handle device flow specific errors
		if dfErr != nil {
			switch {
			case dfErr.IsAuthorizationPending():
				// User hasn't authorized yet, continue polling
				continue

			case dfErr.IsSlowDown():
				// Increase polling interval by 5 seconds, capped at 60 seconds
				interval += 5 * time.Second
				const maxInterval = 60 * time.Second
				if interval > maxInterval {
					interval = maxInterval
				}
				if callbacks.OnSlowDown != nil {
					callbacks.OnSlowDown(interval)
				}
				continue

			case dfErr.IsExpiredToken():
				return nil, ErrDeviceCodeExpired

			case dfErr.IsAccessDenied():
				return nil, ErrAccessDenied

			default:
				return nil, fmt.Errorf("device flow error: %s - %s", dfErr.Error, dfErr.Description)
			}
		}

		// Success! We have tokens - defensive nil check
		if tokenResp == nil {
			return nil, fmt.Errorf("poll for token: empty success response from auth service")
		}

		expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

		// Step 3: Fetch user information
		user, err := client.GetCurrentUser(ctx, tokenResp.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("get user info: %w", err)
		}

		return &DeviceFlowResult{
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			ExpiresAt:    expiresAt,
			User:         user,
		}, nil
	}
}

// StoreDeviceFlowResult saves the device flow result to the token store.
func StoreDeviceFlowResult(store *TokenStore, result *DeviceFlowResult) error {
	// Validate token data from device flow
	if result.AccessToken == "" || result.RefreshToken == "" {
		return fmt.Errorf("incomplete auth response: missing access_token or refresh_token")
	}
	if result.ExpiresAt.IsZero() {
		return fmt.Errorf("incomplete auth response: missing or invalid expires_at")
	}
	if result.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("incomplete auth response: token already expired")
	}

	// Validate user data from auth service
	if result.User == nil {
		return fmt.Errorf("incomplete auth response: missing user data")
	}
	if result.User.UserID == "" || result.User.Username == "" {
		return fmt.Errorf("incomplete auth response: missing user_id or username")
	}
	if result.User.TenantID == "" {
		return fmt.Errorf("incomplete auth response: missing tenant_id")
	}

	auth := &StoredAuth{
		Version:      CurrentStorageVersion,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt,
		User: UserInfo{
			ID:          result.User.UserID,
			GitHubLogin: result.User.Username,
			Email:       result.User.Email,
		},
		TenantID:     result.User.TenantID,
		Entitlements: result.User.Entitlements,
		Plan:         result.User.PlanID,
	}

	return store.Save(auth)
}
