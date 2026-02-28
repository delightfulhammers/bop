package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_InitiateDeviceFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/auth/device" {
			t.Errorf("expected /auth/device, got %s", r.URL.Path)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["product_id"] != "bop" {
			t.Errorf("product_id: got %q, want %q", body["product_id"], "bop")
		}
		if body["provider_type"] != "github" {
			t.Errorf("provider_type: got %q, want %q", body["provider_type"], "github")
		}

		resp := DeviceFlowResponse{
			DeviceCode:      "device-123",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://example.com/verify",
			ExpiresIn:       900,
			Interval:        5,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	resp, err := client.InitiateDeviceFlow(context.Background(), "bop", "github")
	if err != nil {
		t.Fatalf("InitiateDeviceFlow: %v", err)
	}

	if resp.DeviceCode != "device-123" {
		t.Errorf("DeviceCode: got %q, want %q", resp.DeviceCode, "device-123")
	}
	if resp.UserCode != "ABCD-1234" {
		t.Errorf("UserCode: got %q, want %q", resp.UserCode, "ABCD-1234")
	}
	if resp.VerificationURI != "https://example.com/verify" {
		t.Errorf("VerificationURI: got %q, want %q", resp.VerificationURI, "https://example.com/verify")
	}
}

func TestClient_PollDeviceToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TokenResponse{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			TokenType:    "bearer",
			ExpiresIn:    3600,
			UserID:       "user-1",
			Username:     "testuser",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	resp, err := client.PollDeviceToken(context.Background(), "bop", "device-123")
	if err != nil {
		t.Fatalf("PollDeviceToken: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
		return
	}
	if resp.AccessToken != "access-token" {
		t.Errorf("AccessToken: got %q, want %q", resp.AccessToken, "access-token")
	}
}

func TestClient_PollDeviceToken_AuthorizationPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(DeviceFlowError{
			Error:            "authorization_pending",
			ErrorDescription: "user has not yet authorized",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	resp, err := client.PollDeviceToken(context.Background(), "bop", "device-123")
	if err != nil {
		t.Fatalf("expected no error for authorization_pending, got: %v", err)
	}
	if resp != nil {
		t.Fatalf("expected nil response for authorization_pending, got %+v", resp)
	}
}

func TestClient_PollDeviceToken_ExpiredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(DeviceFlowError{
			Error:            "expired_token",
			ErrorDescription: "device code has expired",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	_, err := client.PollDeviceToken(context.Background(), "bop", "device-123")
	if err == nil {
		t.Fatal("expected error for expired_token")
	}
}

func TestClient_PollDeviceToken_SlowDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(DeviceFlowError{
			Error:            "slow_down",
			ErrorDescription: "polling too fast",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	_, err := client.PollDeviceToken(context.Background(), "bop", "device-123")
	if !errors.Is(err, ErrSlowDown) {
		t.Fatalf("expected ErrSlowDown, got: %v", err)
	}
}

func TestClient_PollDeviceToken_Malformed400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	resp, err := client.PollDeviceToken(context.Background(), "bop", "device-123")
	if err == nil {
		t.Fatal("expected error for malformed 400 response")
	}
	if resp != nil {
		t.Error("expected nil response for malformed 400")
	}
	if !strings.Contains(err.Error(), "unexpected status 400") {
		t.Errorf("expected 'unexpected status 400' in error, got: %v", err)
	}
}

func TestClient_ErrorBodyTruncation(t *testing.T) {
	// Generate a response body larger than maxErrorBodyBytes
	largeBody := strings.Repeat("x", 2048)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(largeBody))
	}))
	defer server.Close()

	creds := &Credentials{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	client := NewClient(server.URL, creds)

	_, err := client.GetUserInfo(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	// Error message should be truncated (1024 + "..." = 1027 chars max in body portion)
	if len(err.Error()) >= 2048 {
		t.Errorf("error body should be truncated, got length %d", len(err.Error()))
	}
	if !strings.HasSuffix(err.Error(), "...") {
		t.Error("truncated error should end with '...'")
	}
}

func TestClient_GetUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/auth/me" {
			t.Errorf("expected /auth/me, got %s", r.URL.Path)
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			t.Errorf("Authorization header: got %q, want %q", authHeader, "Bearer test-token")
		}

		resp := UserInfoResponse{
			UserID:       "user-1",
			Username:     "testuser",
			Email:        "test@example.com",
			PlanID:       "pro",
			Entitlements: []string{"config.read", "config.write"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds := &Credentials{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	client := NewClient(server.URL, creds)
	resp, err := client.GetUserInfo(context.Background())
	if err != nil {
		t.Fatalf("GetUserInfo: %v", err)
	}

	if resp.Username != "testuser" {
		t.Errorf("Username: got %q, want %q", resp.Username, "testuser")
	}
	if resp.PlanID != "pro" {
		t.Errorf("PlanID: got %q, want %q", resp.PlanID, "pro")
	}
}

func TestClient_ListTeams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/teams" {
			t.Errorf("expected /v1/teams, got %s", r.URL.Path)
		}

		resp := ListTeamsResponse{
			Teams: []TeamResponse{
				{ID: "team-1", Name: "Engineering", Tier: "pro", Status: "active"},
				{ID: "team-2", Name: "Design", Tier: "free", Status: "active"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds := &Credentials{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	client := NewClient(server.URL, creds)
	teams, err := client.ListTeams(context.Background())
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}

	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
	if teams[0].Name != "Engineering" {
		t.Errorf("team[0].Name: got %q, want %q", teams[0].Name, "Engineering")
	}
}

func TestClient_GetConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products/bop/config" {
			t.Errorf("expected /products/bop/config, got %s", r.URL.Path)
		}

		resp := ConfigResponse{
			Config: map[string]any{
				"reviewers": []any{"security", "maintainability"},
				"model":     "claude-sonnet-4-6",
			},
			EditableFields: []string{"reviewers", "model"},
			Tier:           "pro",
			IsReadOnly:     false,
			Version:        3,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds := &Credentials{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	client := NewClient(server.URL, creds)
	resp, err := client.GetConfig(context.Background(), "bop")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}

	if resp.Tier != "pro" {
		t.Errorf("Tier: got %q, want %q", resp.Tier, "pro")
	}
	if resp.Version != 3 {
		t.Errorf("Version: got %d, want %d", resp.Version, 3)
	}
}

func TestClient_AutoRefresh(t *testing.T) {
	var refreshCalled atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/refresh":
			refreshCalled.Add(1)
			resp := TokenResponse{
				AccessToken:  "new-access-token",
				RefreshToken: "new-refresh-token",
				ExpiresIn:    3600,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case "/auth/me":
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer new-access-token" {
				t.Errorf("expected new token in Authorization header, got %q", authHeader)
			}
			resp := UserInfoResponse{UserID: "user-1", Username: "testuser"}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	// Credentials that expire in 1 minute (within refresh grace period)
	creds := &Credentials{
		AccessToken:  "old-access-token",
		RefreshToken: "old-refresh-token",
		TenantID:     "tenant-1",
		ExpiresAt:    time.Now().Add(time.Minute),
		PlatformURL:  server.URL,
	}

	var refreshedCreds *Credentials
	client := NewClient(server.URL, creds).WithOnRefresh(func(c *Credentials) {
		refreshedCreds = c
	})

	// The GET should trigger auto-refresh first, then proceed
	_, err := client.GetUserInfo(context.Background())
	if err != nil {
		t.Fatalf("GetUserInfo with auto-refresh: %v", err)
	}

	if refreshCalled.Load() != 1 {
		t.Errorf("expected refresh to be called once, got %d", refreshCalled.Load())
	}
	if refreshedCreds == nil {
		t.Fatal("expected onRefresh callback to be called")
	}
	if refreshedCreds.AccessToken != "new-access-token" {
		t.Errorf("refreshed AccessToken: got %q, want %q", refreshedCreds.AccessToken, "new-access-token")
	}
}

func TestClient_NoRefreshWhenFresh(t *testing.T) {
	var refreshCalled atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/refresh" {
			refreshCalled.Add(1)
		}
		resp := UserInfoResponse{UserID: "user-1"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Token valid for 10 minutes - should NOT trigger refresh
	creds := &Credentials{
		AccessToken: "valid-token",
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}
	client := NewClient(server.URL, creds)

	_, err := client.GetUserInfo(context.Background())
	if err != nil {
		t.Fatalf("GetUserInfo: %v", err)
	}

	if refreshCalled.Load() != 0 {
		t.Error("expected no refresh call when token is fresh")
	}
}

func TestClient_RevokeToken(t *testing.T) {
	var revokeBody map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/revoke" {
			_ = json.NewDecoder(r.Body).Decode(&revokeBody)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	creds := &Credentials{
		AccessToken: "token-to-revoke",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	client := NewClient(server.URL, creds)

	if err := client.RevokeToken(context.Background()); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	if revokeBody["token"] != "token-to-revoke" {
		t.Errorf("revoke body token: got %q, want %q", revokeBody["token"], "token-to-revoke")
	}
}

func TestClient_RevokeToken_NilCreds(t *testing.T) {
	client := NewClient("https://example.com", nil)
	if err := client.RevokeToken(context.Background()); err != nil {
		t.Fatalf("expected no error for nil creds, got: %v", err)
	}
}

func TestClient_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	creds := &Credentials{
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	client := NewClient(server.URL, creds)

	_, err := client.GetUserInfo(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestNewClient_DefaultBaseURL(t *testing.T) {
	client := NewClient("", nil)
	if client.baseURL != defaultBaseURL {
		t.Errorf("baseURL: got %q, want %q", client.baseURL, defaultBaseURL)
	}
}
