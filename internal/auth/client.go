package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is an HTTP client for the platform auth-service.
type Client struct {
	baseURL    *url.URL
	productID  string
	httpClient *http.Client
}

// ClientConfig configures the auth client.
type ClientConfig struct {
	// BaseURL is the auth-service URL (e.g., "https://auth.delightfulhammers.com").
	BaseURL string

	// ProductID identifies this product to the auth-service (e.g., "bop").
	ProductID string

	// Timeout is the HTTP request timeout.
	Timeout time.Duration
}

// NewClient creates a new auth-service client.
// Returns nil if the BaseURL is invalid or uses an unsupported scheme.
func NewClient(cfg ClientConfig) *Client {
	baseURL, err := parseAndValidateBaseURL(cfg.BaseURL)
	if err != nil {
		return nil
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL:   baseURL,
		productID: cfg.ProductID,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// parseAndValidateBaseURL parses and normalizes the base URL.
// Validates scheme is https (or http for local development).
func parseAndValidateBaseURL(rawURL string) (*url.URL, error) {
	// Trim whitespace that could cause issues
	rawURL = strings.TrimSpace(rawURL)

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Validate scheme (https required, http allowed for localhost dev)
	switch parsed.Scheme {
	case "https":
		// OK
	case "http":
		// Allow http only for localhost/127.0.0.1 (development)
		host := parsed.Hostname()
		if host != "localhost" && host != "127.0.0.1" {
			return nil, fmt.Errorf("http scheme only allowed for localhost, got: %s", host)
		}
	default:
		return nil, fmt.Errorf("unsupported URL scheme: %s (must be https)", parsed.Scheme)
	}

	// Ensure host is present
	if parsed.Host == "" {
		return nil, fmt.Errorf("missing host in URL: %s", rawURL)
	}

	// Normalize: strip trailing slash from path
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")

	return parsed, nil
}

// buildURL constructs a full URL by joining the base URL with a path.
func (c *Client) buildURL(path string) string {
	return c.baseURL.JoinPath(path).String()
}

// InitiateDeviceFlow starts the device authorization flow.
func (c *Client) InitiateDeviceFlow(ctx context.Context) (*DeviceFlowResponse, error) {
	reqBody := map[string]interface{}{
		"product_id":    c.productID,
		"provider_type": "github",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.buildURL("/auth/device"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device flow request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result DeviceFlowResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// PollDeviceToken polls for the device token.
// Returns the token response on success, or a DeviceFlowError for known states.
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string) (*TokenResponse, *DeviceFlowError, error) {
	reqBody := map[string]interface{}{
		"product_id":  c.productID,
		"device_code": deviceCode,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.buildURL("/auth/device/token"), bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("poll token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Success - token issued
	if resp.StatusCode == http.StatusOK {
		var result TokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, nil, fmt.Errorf("decode token response: %w", err)
		}
		return &result, nil, nil
	}

	// Check for known device flow errors (400 status)
	if resp.StatusCode == http.StatusBadRequest {
		var dfErr DeviceFlowError
		if err := json.NewDecoder(resp.Body).Decode(&dfErr); err != nil {
			// Fall back to generic error if not a device flow error format
			return nil, nil, fmt.Errorf("poll failed with status %d", resp.StatusCode)
		}
		// Return the device flow error for the caller to handle
		return nil, &dfErr, nil
	}

	return nil, nil, c.parseError(resp)
}

// RefreshToken exchanges a refresh token for a new token pair.
func (c *Client) RefreshToken(ctx context.Context, tenantID, refreshToken string) (*TokenResponse, error) {
	reqBody := map[string]interface{}{
		"tenant_id":     tenantID,
		"refresh_token": refreshToken,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.buildURL("/auth/refresh"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetCurrentUser retrieves the current user's information using an access token.
func (c *Client) GetCurrentUser(ctx context.Context, accessToken string) (*CurrentUserResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.buildURL("/auth/me"), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get user request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result CurrentUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// RevokeToken revokes a refresh token.
func (c *Client) RevokeToken(ctx context.Context, tenantID, refreshToken string) error {
	reqBody := map[string]interface{}{
		"tenant_id":     tenantID,
		"refresh_token": refreshToken,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.buildURL("/auth/revoke"), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("revoke request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Per RFC 7009, revoke returns 200 even for invalid tokens
	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

	return nil
}

// parseError parses an error response from the auth-service.
// Limits response body to 4KB to prevent memory exhaustion from large error pages.
func (c *Client) parseError(resp *http.Response) error {
	const maxErrorBodySize = 4 * 1024 // 4KB
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
	if len(body) > 0 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil {
			if errResp.Message != "" {
				return fmt.Errorf("auth-service error (%d): %s", resp.StatusCode, errResp.Message)
			}
			if errResp.Error != "" {
				return fmt.Errorf("auth-service error (%d): %s", resp.StatusCode, errResp.Error)
			}
		}
		return fmt.Errorf("auth-service error (%d): %s", resp.StatusCode, string(body))
	}
	return fmt.Errorf("auth-service error: %s", resp.Status)
}
