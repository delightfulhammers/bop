package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ErrSlowDown is returned by PollDeviceToken when the server requests the client
// to increase its polling interval (RFC 8628 §3.5).
var ErrSlowDown = errors.New("slow_down: server requested slower polling")

const (
	defaultTimeout    = 30 * time.Second
	defaultBaseURL    = "https://api.delightfulhammers.com"
	maxErrorBodyBytes = 1024
)

// Client communicates with the bop platform API.
// It handles automatic token refresh when credentials are near expiry.
// All credential mutations are protected by a mutex for concurrent use.
type Client struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.Mutex
	creds      *Credentials
	onRefresh  func(*Credentials) // Called after successful refresh to persist updated credentials
}

// NewClient creates a platform client. The credentials may be nil for unauthenticated flows
// (e.g., device flow initiation).
func NewClient(baseURL string, creds *Credentials) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
		creds:      creds,
	}
}

// WithOnRefresh sets a callback invoked after a successful token refresh.
// This is typically used to persist the updated credentials to disk.
// Must be called during setup before any concurrent API operations.
func (c *Client) WithOnRefresh(fn func(*Credentials)) *Client {
	c.onRefresh = fn
	return c
}

// InitiateDeviceFlow starts the device authorization flow.
func (c *Client) InitiateDeviceFlow(ctx context.Context, productID, providerType string) (*DeviceFlowResponse, error) {
	body := map[string]string{
		"product_id":    productID,
		"provider_type": providerType,
	}
	var resp DeviceFlowResponse
	if err := c.post(ctx, "/auth/device", body, &resp, false); err != nil {
		return nil, fmt.Errorf("initiate device flow: %w", err)
	}
	return &resp, nil
}

// PollDeviceToken polls for the device flow token.
// Returns nil, nil when the server says "authorization_pending" (caller should retry).
// Returns a DeviceFlowError for terminal errors (expired_token, access_denied).
func (c *Client) PollDeviceToken(ctx context.Context, productID, deviceCode string) (*TokenResponse, error) {
	body := map[string]string{
		"product_id":  productID,
		"device_code": deviceCode,
		"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/device/token", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("poll device token: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode == http.StatusOK {
		var tokenResp TokenResponse
		if err := json.Unmarshal(respBody, &tokenResp); err != nil {
			return nil, fmt.Errorf("parse token response: %w", err)
		}
		return &tokenResp, nil
	}

	// Check for known device flow error responses
	var flowErr DeviceFlowError
	if err := json.Unmarshal(respBody, &flowErr); err == nil && flowErr.Error != "" {
		switch flowErr.Error {
		case "authorization_pending":
			return nil, nil // Caller should retry
		case "slow_down":
			return nil, ErrSlowDown // Caller should increase interval (RFC 8628 §3.5)
		default:
			return nil, fmt.Errorf("%s: %s", flowErr.Error, flowErr.ErrorDescription)
		}
	}

	// Malformed 400 response without a valid error field — don't return nil,nil
	// which would cause infinite polling
	bodySnippet := truncateBody(respBody)
	return nil, fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode, bodySnippet)
}

// RefreshToken refreshes the access token using the stored refresh token.
func (c *Client) RefreshToken(ctx context.Context) (*TokenResponse, error) {
	if c.creds == nil || c.creds.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}
	body := map[string]string{
		"refresh_token": c.creds.RefreshToken,
		"tenant_id":     c.creds.TenantID,
	}
	var resp TokenResponse
	if err := c.post(ctx, "/auth/refresh", body, &resp, false); err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	return &resp, nil
}

// RevokeToken revokes the current access token.
func (c *Client) RevokeToken(ctx context.Context) error {
	if c.creds == nil || c.creds.AccessToken == "" {
		return nil
	}
	body := map[string]string{
		"token": c.creds.AccessToken,
	}
	return c.post(ctx, "/auth/revoke", body, nil, true)
}

// GetUserInfo retrieves the authenticated user's information.
func (c *Client) GetUserInfo(ctx context.Context) (*UserInfoResponse, error) {
	var resp UserInfoResponse
	if err := c.get(ctx, "/auth/me", &resp); err != nil {
		return nil, fmt.Errorf("get user info: %w", err)
	}
	return &resp, nil
}

// ListTeams retrieves the authenticated user's team memberships.
func (c *Client) ListTeams(ctx context.Context) ([]TeamResponse, error) {
	var resp ListTeamsResponse
	if err := c.get(ctx, "/v1/teams", &resp); err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	return resp.Teams, nil
}

// GetConfig retrieves the product configuration for the authenticated user.
func (c *Client) GetConfig(ctx context.Context, productID string) (*ConfigResponse, error) {
	var resp ConfigResponse
	if err := c.get(ctx, "/products/"+productID+"/config", &resp); err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}
	return &resp, nil
}

// ensureFreshToken refreshes the token if it's near expiry.
// The mutex protects credential state (AccessToken, RefreshToken, ExpiresAt)
// during refresh. Concurrent HTTP requests via http.Client are safe.
// The onRefresh callback is called under the lock, so it must not re-enter Client methods.
func (c *Client) ensureFreshToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.creds == nil || !c.creds.NeedsRefresh() {
		return nil
	}
	tokenResp, err := c.RefreshToken(ctx)
	if err != nil {
		return err
	}
	c.creds.AccessToken = tokenResp.AccessToken
	c.creds.RefreshToken = tokenResp.RefreshToken
	c.creds.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if c.onRefresh != nil {
		c.onRefresh(c.creds)
	}
	return nil
}

// get performs an authenticated GET request.
func (c *Client) get(ctx context.Context, path string, result any) error {
	if err := c.ensureFreshToken(ctx); err != nil {
		return fmt.Errorf("auto-refresh: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if c.creds != nil && c.creds.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.creds.AccessToken)
	}

	return c.doJSON(req, result)
}

// post performs a POST request, optionally authenticated.
func (c *Client) post(ctx context.Context, path string, body any, result any, authenticated bool) error {
	if authenticated {
		if err := c.ensureFreshToken(ctx); err != nil {
			return fmt.Errorf("auto-refresh: %w", err)
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authenticated && c.creds != nil && c.creds.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.creds.AccessToken)
	}

	return c.doJSON(req, result)
}

// doJSON executes the request and unmarshals the JSON response.
func (c *Client) doJSON(req *http.Request, result any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateBody(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}
	}
	return nil
}

// truncateBody returns the body as a string, truncated to maxErrorBodyBytes.
func truncateBody(body []byte) string {
	if len(body) <= maxErrorBodyBytes {
		return string(body)
	}
	return string(body[:maxErrorBodyBytes]) + "..."
}
