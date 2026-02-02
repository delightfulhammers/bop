package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIError represents an error response from the platform API.
// It carries the HTTP status code so callers can distinguish between
// client errors (4xx) and server errors (5xx) for error handling decisions.
type APIError struct {
	StatusCode int
	ErrorCode  string
	Message    string
}

func (e *APIError) Error() string {
	switch {
	case e.ErrorCode != "" && e.Message != "":
		return fmt.Sprintf("auth-service error (%d) [%s]: %s", e.StatusCode, e.ErrorCode, e.Message)
	case e.Message != "":
		return fmt.Sprintf("auth-service error (%d): %s", e.StatusCode, e.Message)
	case e.ErrorCode != "":
		return fmt.Sprintf("auth-service error (%d): %s", e.StatusCode, e.ErrorCode)
	default:
		return fmt.Sprintf("auth-service error (%d)", e.StatusCode)
	}
}

// IsTenantNotConfigured returns true if the error indicates the platform could
// not find a tenant for the OIDC token's repository owner.
// Matches only when the platform returns HTTP 401 with error code "tenant_not_configured".
// This is the only error condition where soft-fallback to stored auth is safe —
// all other errors (network, 5xx, token validation, tenant mismatch, other 401s)
// should fail hard.
func IsTenantNotConfigured(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) &&
		apiErr.StatusCode == http.StatusUnauthorized &&
		apiErr.ErrorCode == "tenant_not_configured"
}

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
// Returns an error if the BaseURL is invalid, uses an unsupported scheme, or ProductID is empty.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.ProductID == "" {
		return nil, fmt.Errorf("ProductID is required")
	}

	baseURL, err := parseAndValidateBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid auth service URL: %w", err)
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
	}, nil
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

	// Reject URLs with userinfo (user:pass@host) to prevent credential confusion attacks
	if parsed.User != nil {
		return nil, fmt.Errorf("URL must not contain userinfo (credentials): %s", rawURL)
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

// OIDCExchangeRequest contains parameters for exchanging an OIDC token.
type OIDCExchangeRequest struct {
	// TenantID is the tenant to authenticate to (optional).
	// When empty, the platform derives the tenant from the OIDC token's
	// repository_owner claim via ExternalProviderLogin lookup. The platform
	// enforces a UNIQUE constraint so derivation is unambiguous — at most one
	// tenant can be configured for a given (product, provider, repository_owner).
	TenantID string

	// IDToken is the OIDC token from GitHub Actions (required).
	IDToken string

	// ProviderType identifies the OIDC provider (optional, defaults to "github").
	ProviderType string
}

// oidcExchangeResponse is an internal type for parsing the OIDC exchange response.
// The platform returns either tokens or a skip result, both with HTTP 200.
type oidcExchangeResponse struct {
	// Token fields (present when not skipped)
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`

	// User info fields (present in OIDC flow to avoid /auth/me call)
	UserID       string   `json:"user_id"`
	Username     string   `json:"username"`
	TenantID     string   `json:"tenant_id"`
	PlanID       string   `json:"plan_id"`
	Entitlements []string `json:"entitlements"`

	// Skip fields (present when skipped)
	Skipped bool   `json:"skipped"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Comment string `json:"comment"`
}

// ExchangeOIDCToken exchanges a GitHub Actions OIDC token for platform credentials.
// This is used for machine-to-machine authentication from CI/CD pipelines.
//
// Returns:
// - (*TokenResponse, nil, nil): successful authentication with tokens
// - (nil, *SkipInfo, nil): authorization passed but review should be skipped (e.g., solo namespace violation)
// - (nil, nil, error): authentication or network failure
func (c *Client) ExchangeOIDCToken(ctx context.Context, req OIDCExchangeRequest) (*TokenResponse, *SkipInfo, error) {
	// Validate required inputs
	if req.IDToken == "" {
		return nil, nil, fmt.Errorf("id_token is required")
	}

	providerType := req.ProviderType
	if providerType == "" {
		providerType = "github"
	}

	reqBody := map[string]interface{}{
		"product_id":    c.productID,
		"provider_type": providerType,
		"id_token":      req.IDToken,
	}
	if req.TenantID != "" {
		reqBody["tenant_id"] = req.TenantID
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.buildURL("/auth/actions-oidc"), bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("OIDC exchange request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, c.parseError(resp)
	}

	// Limit response body to prevent memory exhaustion
	const maxResponseSize = 64 * 1024 // 64KB
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)

	var result oidcExchangeResponse
	if err := json.NewDecoder(limitedReader).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("decode response: %w", err)
	}

	// Check if this is a skip response
	if result.Skipped {
		return nil, &SkipInfo{
			Reason:  SkipReason(result.Reason),
			Message: result.Message,
			Comment: result.Comment,
		}, nil
	}

	// Normal token response (includes user info for OIDC flow)
	return &TokenResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    result.TokenType,
		ExpiresIn:    result.ExpiresIn,
		UserID:       result.UserID,
		Username:     result.Username,
		TenantID:     result.TenantID,
		PlanID:       result.PlanID,
		Entitlements: result.Entitlements,
	}, nil, nil
}

// ProductConfigResponse is the response from the config API.
// This matches the platform's /products/{product_id}/config endpoint.
type ProductConfigResponse struct {
	// Config is the rendered configuration for the user's tier.
	Config map[string]any `json:"config"`

	// EditableFields lists fields the user can modify (for UI).
	EditableFields []string `json:"editable_fields"`

	// Tier is the user's subscription tier (beta, solo, pro, enterprise).
	Tier string `json:"tier"`

	// IsReadOnly indicates if the user can modify config.
	IsReadOnly bool `json:"is_read_only"`

	// Schema is the JSON schema for editable fields (optional).
	Schema map[string]any `json:"schema,omitempty"`
}

// FetchProductConfig retrieves the user's configuration from the platform.
// The configuration is tier-based: beta/solo users get defaults, pro/enterprise can customize.
func (c *Client) FetchProductConfig(ctx context.Context, accessToken string) (*ProductConfigResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.buildURL("/products/"+c.productID+"/config"), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch config request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	// Limit response body size to prevent memory exhaustion from malicious/misconfigured server
	const maxConfigBodySize = 1 << 20 // 1MB - plenty for config, defensive against abuse
	limitedReader := io.LimitReader(resp.Body, maxConfigBodySize)

	var result ProductConfigResponse
	if err := json.NewDecoder(limitedReader).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode config response: %w", err)
	}

	return &result, nil
}

// parseError parses an error response from the auth-service.
// Returns an *APIError with the HTTP status code so callers can make
// error-handling decisions based on the response type (4xx vs 5xx).
// Limits response body to 4KB to prevent memory exhaustion from large error pages.
func (c *Client) parseError(resp *http.Response) error {
	const maxErrorBodySize = 4 * 1024 // 4KB
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))

	apiErr := &APIError{StatusCode: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}

	if len(body) > 0 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &errResp); err == nil && (errResp.Error != "" || errResp.Message != "") {
			apiErr.ErrorCode = errResp.Error
			if errResp.Message != "" {
				apiErr.Message = errResp.Message
			}
		} else if err != nil {
			// Non-JSON response — use body as message
			bodyStr := string(body)
			if len(body) == maxErrorBodySize {
				bodyStr += "... (truncated)"
			}
			apiErr.Message = bodyStr
		}
		// If JSON parsed but both fields empty, keep the http.StatusText default
	}

	return apiErr
}
