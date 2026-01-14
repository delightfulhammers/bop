package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DeviceFlowErrors for different failure modes.
var (
	ErrDeviceCodeExpired = errors.New("device code expired - please try again")
	ErrAccessDenied      = errors.New("authorization denied by user")
	ErrFlowCanceled      = errors.New("device flow canceled")
)

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
				// Increase polling interval by 5 seconds
				interval += 5 * time.Second
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

		// Success! We have tokens
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
