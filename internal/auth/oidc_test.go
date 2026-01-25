package auth

import (
	"context"
	"encoding/json"
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

func TestGitHubActionsOIDC_Authenticate_MissingTenantID(t *testing.T) {
	client, err := NewClient(ClientConfig{
		BaseURL:   "https://api.example.com",
		ProductID: "bop",
	})
	if err != nil {
		t.Fatal(err)
	}

	oidc := NewGitHubActionsOIDC(client, "https://api.example.com")
	_, authErr := oidc.Authenticate(context.Background(), "")
	if authErr == nil {
		t.Fatal("expected error for empty tenant ID")
	}
	if got := authErr.Error(); got != "tenant_id is required for OIDC authentication (set BOP_TENANT_ID)" {
		t.Errorf("unexpected error: %s", got)
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
