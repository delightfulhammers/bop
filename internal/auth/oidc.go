// Package auth provides platform authentication for bop CLI and MCP server.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// GitHubActionsOIDC provides OIDC token exchange for GitHub Actions authentication.
// This enables keyless authentication from CI/CD pipelines.
type GitHubActionsOIDC struct {
	client     *Client
	httpClient *http.Client
	audience   string
}

// NewGitHubActionsOIDC creates a new OIDC handler for GitHub Actions.
// The audience parameter specifies the intended recipient of the OIDC token
// (typically the platform URL, e.g. "https://api.delightfulhammers.com").
func NewGitHubActionsOIDC(client *Client, audience string) *GitHubActionsOIDC {
	return &GitHubActionsOIDC{
		client:   client,
		audience: audience,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsAvailable returns true if GitHub Actions OIDC is available in the current environment.
// OIDC is available when both ACTIONS_ID_TOKEN_REQUEST_URL and ACTIONS_ID_TOKEN_REQUEST_TOKEN
// are set. These are only present when the workflow has `permissions: id-token: write`.
func IsAvailable() bool {
	return os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL") != "" &&
		os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN") != ""
}

// OIDCAuthResult contains the result of an OIDC authentication.
// Either (StoredAuth, Entitlements) is populated for success, or Skip is populated for skipped reviews.
type OIDCAuthResult struct {
	// StoredAuth contains the authentication state (tokens, entitlements, etc.)
	// Nil when Skip is set.
	StoredAuth *StoredAuth

	// Entitlements provides access to entitlement checking.
	// Nil when Skip is set.
	Entitlements *BopEntitlements

	// Skip contains information when the review should be skipped.
	// Set when the platform returns a skip result instead of tokens.
	// Nil when authentication succeeds with tokens.
	Skip *SkipInfo
}

// Authenticate requests an OIDC token from GitHub Actions and exchanges it
// for platform credentials. Returns the authentication result on success.
//
// The tenantID parameter identifies which tenant to authenticate to.
// When empty, the platform derives the tenant from the OIDC token's
// repository_owner claim. The platform enforces a UNIQUE constraint on
// (product_id, provider, repository_owner), so derivation is unambiguous.
// If no tenant has OIDC trust configured for the repository owner, the
// platform returns 401 (tenant_not_configured).
// When provided, the platform validates that the repository owner matches
// the tenant's ExternalProviderLogin and returns 403 on mismatch.
func (g *GitHubActionsOIDC) Authenticate(ctx context.Context, tenantID string) (*OIDCAuthResult, error) {
	// Request OIDC token from GitHub Actions runtime
	idToken, err := g.requestOIDCToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("request OIDC token: %w", err)
	}

	// Exchange OIDC token for platform credentials
	tokenResp, skipInfo, err := g.client.ExchangeOIDCToken(ctx, OIDCExchangeRequest{
		TenantID:     tenantID,
		IDToken:      idToken,
		ProviderType: "github",
	})
	if err != nil {
		return nil, fmt.Errorf("exchange OIDC token: %w", err)
	}

	// Handle skip response - the platform returns this when auth succeeds but
	// the actor doesn't have the right entitlements for this operation.
	// Return it as a successful result with Skip set, not an error.
	if skipInfo != nil {
		return &OIDCAuthResult{
			Skip: skipInfo,
		}, nil
	}

	// Build StoredAuth from OIDC response (includes user info, no need to call /auth/me)
	stored := &StoredAuth{
		Version:      1,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		TenantID:     tokenResp.TenantID,
		Entitlements: tokenResp.Entitlements,
		Plan:         tokenResp.PlanID,
		User: UserInfo{
			ID:          tokenResp.UserID,
			GitHubLogin: tokenResp.Username,
		},
	}

	// Create entitlements checker
	entitlements := NewBopEntitlements(stored, os.Stderr)

	return &OIDCAuthResult{
		StoredAuth:   stored,
		Entitlements: entitlements,
	}, nil
}

// requestOIDCToken requests an OIDC token from the GitHub Actions runtime.
// This uses the ACTIONS_ID_TOKEN_REQUEST_URL and ACTIONS_ID_TOKEN_REQUEST_TOKEN
// environment variables that GitHub provides when `permissions: id-token: write` is set.
func (g *GitHubActionsOIDC) requestOIDCToken(ctx context.Context) (string, error) {
	requestURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

	if requestURL == "" || requestToken == "" {
		return "", fmt.Errorf("OIDC not available: missing ACTIONS_ID_TOKEN_REQUEST_URL or ACTIONS_ID_TOKEN_REQUEST_TOKEN; " +
			"ensure workflow has 'permissions: id-token: write'")
	}

	// Add audience parameter - tells GitHub who this token is for.
	// This prevents token reuse against unintended services.
	parsed, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("parse OIDC request URL: %w", err)
	}
	q := parsed.Query()
	q.Set("audience", g.audience)
	parsed.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}

	// GitHub requires the request token as bearer auth
	req.Header.Set("Authorization", "bearer "+requestToken)
	req.Header.Set("Accept", "application/json; api-version=2.0")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request OIDC token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Limit response size to prevent memory exhaustion
	const maxTokenResponseSize = 64 * 1024 // 64KB - JWT tokens are typically ~2-3KB
	limitedReader := io.LimitReader(resp.Body, maxTokenResponseSize)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(limitedReader)
		return "", fmt.Errorf("OIDC token request failed (%d): %s", resp.StatusCode, string(body))
	}

	// GitHub returns {"value": "eyJ..."}
	var tokenResp struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(limitedReader).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode OIDC token response: %w", err)
	}

	if tokenResp.Value == "" {
		return "", fmt.Errorf("OIDC token response missing 'value' field")
	}

	return tokenResp.Value, nil
}
