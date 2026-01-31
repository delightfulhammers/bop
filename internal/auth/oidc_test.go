package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsAvailable(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "both env vars set",
			envVars:  map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": "https://token.example.com", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "test-token"},
			expected: true,
		},
		{
			name:     "only URL set",
			envVars:  map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": "https://token.example.com"},
			expected: false,
		},
		{
			name:     "only token set",
			envVars:  map[string]string{"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "test-token"},
			expected: false,
		},
		{
			name:     "neither set",
			envVars:  map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv sets env var and auto-restores on cleanup.
			// Setting to empty doesn't work for "unset" semantics, so we set
			// both to empty first (which Setenv handles) then override.
			t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
			t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")

			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			if got := IsAvailable(); got != tt.expected {
				t.Errorf("IsAvailable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGitHubActionsOIDC_Authenticate_WithoutTenantID(t *testing.T) {
	// When tenant_id is empty, the platform derives the tenant from the
	// OIDC token's repository_owner claim. Verify that the client sends
	// the request without tenant_id and accepts the response.
	expectedToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test"
	oidcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": expectedToken})
	}))
	defer oidcServer.Close()

	// Use a channel to safely pass the request body from the handler goroutine
	bodyCh := make(chan map[string]any, 1)
	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/actions-oidc":
			var b map[string]any
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			bodyCh <- b
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken:  "access-token-derived",
				RefreshToken: "refresh-token-derived",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
			})
		case "/auth/me":
			_ = json.NewEncoder(w).Encode(CurrentUserResponse{
				UserID:       "user-derived",
				Username:     "deriveduser",
				Email:        "derived@example.com",
				TenantID:     "tenant-derived-from-token",
				PlanID:       "beta",
				Entitlements: []string{"public-repos", "private-repos"},
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer platformServer.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?param=value")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")

	client, err := NewClient(ClientConfig{
		BaseURL:   platformServer.URL,
		ProductID: "bop",
	})
	if err != nil {
		t.Fatal(err)
	}

	oidc := NewGitHubActionsOIDC(client, "https://api.example.com")
	result, authErr := oidc.Authenticate(context.Background(), "")
	if authErr != nil {
		t.Fatalf("Authenticate() error: %v", authErr)
	}

	// Read the captured request body from the handler (channel ensures synchronization)
	receivedBody := <-bodyCh

	// Verify tenant_id was NOT sent in the request body
	if _, hasTenantID := receivedBody["tenant_id"]; hasTenantID {
		t.Errorf("expected no tenant_id in request body, got %v", receivedBody["tenant_id"])
	}

	// Verify result uses the platform-derived tenant
	if result.StoredAuth.TenantID != "tenant-derived-from-token" {
		t.Errorf("unexpected tenant ID: %s", result.StoredAuth.TenantID)
	}
	if result.StoredAuth.AccessToken != "access-token-derived" {
		t.Errorf("unexpected access token: %s", result.StoredAuth.AccessToken)
	}
}

func TestGitHubActionsOIDC_RequestOIDCToken(t *testing.T) {
	// Create a mock GitHub OIDC token endpoint
	expectedToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test"
	oidcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "bearer test-request-token" {
			t.Errorf("unexpected auth header: %s", authHeader)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Verify audience parameter
		audience := r.URL.Query().Get("audience")
		if audience != "https://api.example.com" {
			t.Errorf("unexpected audience: %s", audience)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"value": expectedToken}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer oidcServer.Close()

	// Create a mock platform endpoint
	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/actions-oidc":
			// Verify the request body contains the OIDC token
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request body: %v", err)
			}
			if body["id_token"] != expectedToken {
				t.Errorf("unexpected id_token: %s", body["id_token"])
			}
			if body["tenant_id"] != "tenant-123" {
				t.Errorf("unexpected tenant_id: %s", body["tenant_id"])
			}

			if err := json.NewEncoder(w).Encode(TokenResponse{
				AccessToken:  "access-token-123",
				RefreshToken: "refresh-token-123",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
			}); err != nil {
				t.Errorf("encode token response: %v", err)
			}

		case "/auth/me":
			if err := json.NewEncoder(w).Encode(CurrentUserResponse{
				UserID:       "user-123",
				Username:     "testuser",
				Email:        "test@example.com",
				TenantID:     "tenant-123",
				PlanID:       "beta",
				Entitlements: []string{"public-repos", "private-repos", "any-org"},
			}); err != nil {
				t.Errorf("encode user response: %v", err)
			}

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer platformServer.Close()

	// Set up env vars for OIDC
	// The URL from GitHub has ?foo=bar format, we append &audience=
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?param=value")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")

	// Create OIDC handler
	client, err := NewClient(ClientConfig{
		BaseURL:   platformServer.URL,
		ProductID: "bop",
	})
	if err != nil {
		t.Fatal(err)
	}

	oidc := NewGitHubActionsOIDC(client, "https://api.example.com")
	result, authErr := oidc.Authenticate(context.Background(), "tenant-123")
	if authErr != nil {
		t.Fatalf("Authenticate() error: %v", authErr)
	}

	// Verify result
	if result.StoredAuth.AccessToken != "access-token-123" {
		t.Errorf("unexpected access token: %s", result.StoredAuth.AccessToken)
	}
	if result.StoredAuth.TenantID != "tenant-123" {
		t.Errorf("unexpected tenant ID: %s", result.StoredAuth.TenantID)
	}
	if result.StoredAuth.User.GitHubLogin != "testuser" {
		t.Errorf("unexpected github login: %s", result.StoredAuth.User.GitHubLogin)
	}
	if result.StoredAuth.Plan != "beta" {
		t.Errorf("unexpected plan: %s", result.StoredAuth.Plan)
	}
	if result.Entitlements == nil {
		t.Fatal("expected non-nil entitlements")
	}
}

func TestGitHubActionsOIDC_RequestOIDCToken_AudienceEncoding(t *testing.T) {
	// Verify that the audience parameter is properly URL-encoded
	// (not just string-concatenated) when added to the OIDC request URL.
	var receivedAudience string
	oidcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAudience = r.URL.Query().Get("audience")
		// Also verify existing params are preserved
		if got := r.URL.Query().Get("api-version"); got != "2.0" {
			t.Errorf("existing param lost: api-version = %q, want %q", got, "2.0")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": "test-token"})
	}))
	defer oidcServer.Close()

	// URL already has query params (like GitHub's actual runtime URL)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?api-version=2.0")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/actions-oidc":
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "a", RefreshToken: "r", TokenType: "Bearer", ExpiresIn: 3600,
			})
		case "/auth/me":
			_ = json.NewEncoder(w).Encode(CurrentUserResponse{
				UserID: "u", Username: "u", TenantID: "t", PlanID: "beta",
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer platformServer.Close()

	client, err := NewClient(ClientConfig{BaseURL: platformServer.URL, ProductID: "bop"})
	if err != nil {
		t.Fatal(err)
	}

	// Use a URL with special characters as audience to test encoding
	audience := "https://api.example.com"
	oidc := NewGitHubActionsOIDC(client, audience)
	_, authErr := oidc.Authenticate(context.Background(), "tenant-123")
	if authErr != nil {
		t.Fatalf("Authenticate() error: %v", authErr)
	}

	if receivedAudience != audience {
		t.Errorf("audience = %q, want %q", receivedAudience, audience)
	}
}

func TestGitHubActionsOIDC_RequestOIDCToken_OIDCUnavailable(t *testing.T) {
	// Set env vars to empty to simulate OIDC not available
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")

	client, err := NewClient(ClientConfig{
		BaseURL:   "https://api.example.com",
		ProductID: "bop",
	})
	if err != nil {
		t.Fatal(err)
	}

	oidc := NewGitHubActionsOIDC(client, "https://api.example.com")
	_, authErr := oidc.Authenticate(context.Background(), "tenant-123")
	if authErr == nil {
		t.Fatal("expected error when OIDC is unavailable")
	}
}

func TestGitHubActionsOIDC_RequestOIDCToken_ServerError(t *testing.T) {
	// Create a mock OIDC endpoint that returns an error
	oidcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer oidcServer.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?param=value")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")

	client, err := NewClient(ClientConfig{
		BaseURL:   "https://api.example.com",
		ProductID: "bop",
	})
	if err != nil {
		t.Fatal(err)
	}

	oidc := NewGitHubActionsOIDC(client, "https://api.example.com")
	_, authErr := oidc.Authenticate(context.Background(), "tenant-123")
	if authErr == nil {
		t.Fatal("expected error for server error response")
	}
}

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  APIError
		want string
	}{
		{
			name: "code and message",
			err:  APIError{StatusCode: 401, ErrorCode: "tenant_not_configured", Message: "No tenant configured"},
			want: "auth-service error (401) [tenant_not_configured]: No tenant configured",
		},
		{
			name: "message only",
			err:  APIError{StatusCode: 500, Message: "Internal Server Error"},
			want: "auth-service error (500): Internal Server Error",
		},
		{
			name: "code only",
			err:  APIError{StatusCode: 403, ErrorCode: "oidc_tenant_mismatch"},
			want: "auth-service error (403): oidc_tenant_mismatch",
		},
		{
			name: "no code or message",
			err:  APIError{StatusCode: 502},
			want: "auth-service error (502)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIError_ErrorsAs(t *testing.T) {
	// Verify APIError is properly propagated through error wrapping so that
	// callers (like tryOIDCAuth) can use errors.As to inspect the status code.
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantCode   string
		wantMsg    string
	}{
		{
			name:       "JSON error with code and message",
			statusCode: 401,
			body:       `{"error":"tenant_not_configured","message":"No tenant configured for this repository owner"}`,
			wantCode:   "tenant_not_configured",
			wantMsg:    "No tenant configured for this repository owner",
		},
		{
			name:       "JSON error with code only",
			statusCode: 403,
			body:       `{"error":"oidc_tenant_mismatch"}`,
			wantCode:   "oidc_tenant_mismatch",
		},
		{
			name:       "non-JSON error",
			statusCode: 502,
			body:       "Bad Gateway",
			wantMsg:    "Bad Gateway",
		},
		{
			name:       "empty JSON preserves HTTP status text",
			statusCode: 422,
			body:       `{}`,
			wantMsg:    "Unprocessable Entity",
		},
		{
			name:       "empty body uses HTTP status text",
			statusCode: 503,
			body:       "",
			wantMsg:    "Service Unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/auth/actions-oidc":
					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.body))
				default:
					http.Error(w, "not found", http.StatusNotFound)
				}
			}))
			defer server.Close()

			client, err := NewClient(ClientConfig{BaseURL: server.URL, ProductID: "bop"})
			if err != nil {
				t.Fatal(err)
			}

			_, _, exchangeErr := client.ExchangeOIDCToken(context.Background(), OIDCExchangeRequest{
				IDToken:      "test-token",
				TenantID:     "tenant-123",
				ProviderType: "github",
			})
			if exchangeErr == nil {
				t.Fatal("expected error from exchange")
			}

			// Verify errors.As works through the error chain
			var apiErr *APIError
			if !errors.As(exchangeErr, &apiErr) {
				t.Fatalf("expected errors.As to find *APIError, got: %T: %v", exchangeErr, exchangeErr)
			}
			if apiErr.StatusCode != tt.statusCode {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.statusCode)
			}
			if tt.wantCode != "" && apiErr.ErrorCode != tt.wantCode {
				t.Errorf("ErrorCode = %q, want %q", apiErr.ErrorCode, tt.wantCode)
			}
			if tt.wantMsg != "" && apiErr.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", apiErr.Message, tt.wantMsg)
			}
		})
	}
}

func TestIsTenantNotConfigured(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "401 with tenant_not_configured code",
			err:  &APIError{StatusCode: 401, ErrorCode: "tenant_not_configured"},
			want: true,
		},
		{
			name: "wrapped 401 with tenant_not_configured code",
			err:  fmt.Errorf("exchange OIDC token: %w", &APIError{StatusCode: 401, ErrorCode: "tenant_not_configured"}),
			want: true,
		},
		{
			name: "401 without tenant_not_configured code is not a match",
			err:  &APIError{StatusCode: 401, ErrorCode: "invalid_credentials"},
			want: false,
		},
		{
			name: "401 with no error code is not a match",
			err:  &APIError{StatusCode: 401},
			want: false,
		},
		{
			name: "403 APIError is not tenant not configured",
			err:  &APIError{StatusCode: 403, ErrorCode: "oidc_tenant_mismatch"},
			want: false,
		},
		{
			name: "500 APIError is not tenant not configured",
			err:  &APIError{StatusCode: 500},
			want: false,
		},
		{
			name: "non-APIError is not tenant not configured",
			err:  fmt.Errorf("network error"),
			want: false,
		},
		{
			name: "nil error is not tenant not configured",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTenantNotConfigured(tt.err); got != tt.want {
				t.Errorf("IsTenantNotConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExchangeOIDCToken_SkipResponse(t *testing.T) {
	// Test that ExchangeOIDCToken correctly detects and returns skip responses.
	// The platform returns HTTP 200 with {"skipped": true, ...} instead of tokens.
	tests := []struct {
		name       string
		response   string
		wantReason SkipReason
		wantMsg    string
	}{
		{
			name: "actor_not_member skip",
			response: `{
				"skipped": true,
				"reason": "actor_not_member",
				"message": "actor testuser is not a bop member",
				"comment": "## Code review skipped\n\nActor @testuser is not a bop member."
			}`,
			wantReason: SkipReasonActorNotMember,
			wantMsg:    "actor testuser is not a bop member",
		},
		{
			name: "no_active_entitlement skip",
			response: `{
				"skipped": true,
				"reason": "no_active_entitlement",
				"message": "actor testuser has no active entitlement",
				"comment": "## Code review skipped\n\nActor @testuser does not have an active subscription."
			}`,
			wantReason: SkipReasonNoActiveEntitlement,
			wantMsg:    "actor testuser has no active entitlement",
		},
		{
			name: "solo_namespace_violation skip",
			response: `{
				"skipped": true,
				"reason": "solo_namespace_violation",
				"message": "actor testuser has solo plan but repo owner is orgname",
				"comment": "## Code review skipped\n\nActor @testuser has a Solo plan."
			}`,
			wantReason: SkipReasonSoloNamespaceViolation,
			wantMsg:    "actor testuser has solo plan but repo owner is orgname",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client, err := NewClient(ClientConfig{BaseURL: server.URL, ProductID: "bop"})
			if err != nil {
				t.Fatal(err)
			}

			result, skipInfo, err := client.ExchangeOIDCToken(context.Background(), OIDCExchangeRequest{
				IDToken:      "test-token",
				ProviderType: "github",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != nil {
				t.Errorf("expected nil TokenResponse for skip, got %+v", result)
			}
			if skipInfo == nil {
				t.Fatal("expected non-nil SkipInfo")
			}
			if skipInfo.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", skipInfo.Reason, tt.wantReason)
			}
			if skipInfo.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", skipInfo.Message, tt.wantMsg)
			}
		})
	}
}

func TestExchangeOIDCToken_TokenResponse(t *testing.T) {
	// Verify that normal token responses still work after skip handling is added.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{BaseURL: server.URL, ProductID: "bop"})
	if err != nil {
		t.Fatal(err)
	}

	result, skipInfo, err := client.ExchangeOIDCToken(context.Background(), OIDCExchangeRequest{
		IDToken:      "test-token",
		ProviderType: "github",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skipInfo != nil {
		t.Errorf("expected nil SkipInfo for token response, got %+v", skipInfo)
	}
	if result == nil {
		t.Fatal("expected non-nil TokenResponse")
	}
	if result.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want %q", result.AccessToken, "test-access-token")
	}
}

func TestGitHubActionsOIDC_Authenticate_Skip(t *testing.T) {
	// Test that the OIDC Authenticate flow correctly handles skip responses.
	expectedToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test"
	oidcServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": expectedToken})
	}))
	defer oidcServer.Close()

	platformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a skip response instead of tokens
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"skipped": true,
			"reason":  "solo_namespace_violation",
			"message": "actor testuser has solo plan but repo owner is orgname",
			"comment": "## Code review skipped\n\nActor @testuser has a Solo plan.",
		})
	}))
	defer platformServer.Close()

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", oidcServer.URL+"?param=value")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")

	client, err := NewClient(ClientConfig{
		BaseURL:   platformServer.URL,
		ProductID: "bop",
	})
	if err != nil {
		t.Fatal(err)
	}

	oidc := NewGitHubActionsOIDC(client, "https://api.example.com")
	result, authErr := oidc.Authenticate(context.Background(), "")
	if authErr != nil {
		t.Fatalf("Authenticate() error: %v", authErr)
	}

	// Skip should be returned without error
	if result.StoredAuth != nil {
		t.Errorf("expected nil StoredAuth for skip, got %+v", result.StoredAuth)
	}
	if result.Skip == nil {
		t.Fatal("expected non-nil Skip")
	}
	if result.Skip.Reason != SkipReasonSoloNamespaceViolation {
		t.Errorf("Skip.Reason = %q, want %q", result.Skip.Reason, SkipReasonSoloNamespaceViolation)
	}
}
