package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLogin_Success(t *testing.T) {
	var pollCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/auth/device":
			_ = json.NewEncoder(w).Encode(DeviceFlowResponse{
				DeviceCode:      "device-123",
				UserCode:        "ABCD-1234",
				VerificationURI: "https://example.com/verify",
				ExpiresIn:       300,
				Interval:        1, // 1 second for test speed
			})

		case "/auth/device/token":
			count := pollCount.Add(1)
			if count < 2 {
				// First poll: authorization_pending
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(DeviceFlowError{
					Error:            "authorization_pending",
					ErrorDescription: "user has not yet authorized",
				})
				return
			}
			// Second poll: success
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken:  "access-token",
				RefreshToken: "refresh-token",
				TokenType:    "bearer",
				ExpiresIn:    3600,
				UserID:       "user-1",
				Username:     "testuser",
			})

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	var out bytes.Buffer

	creds, err := Login(context.Background(), client, "bop", &out)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if creds.AccessToken != "access-token" {
		t.Errorf("AccessToken: got %q, want %q", creds.AccessToken, "access-token")
	}
	if creds.Username != "testuser" {
		t.Errorf("Username: got %q, want %q", creds.Username, "testuser")
	}
	if creds.PlatformURL != server.URL {
		t.Errorf("PlatformURL: got %q, want %q", creds.PlatformURL, server.URL)
	}

	// Verify output contains the user code
	output := out.String()
	if !strings.Contains(output, "ABCD-1234") {
		t.Errorf("output should contain user code, got: %s", output)
	}
	if !strings.Contains(output, "done!") {
		t.Errorf("output should contain 'done!', got: %s", output)
	}
}

func TestLogin_DeviceFlowInitFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	var out bytes.Buffer

	_, err := Login(context.Background(), client, "bop", &out)
	if err == nil {
		t.Fatal("expected error for failed device flow init")
	}
}

func TestLogin_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/device" {
			_ = json.NewEncoder(w).Encode(DeviceFlowResponse{
				DeviceCode:      "device-123",
				UserCode:        "ABCD-1234",
				VerificationURI: "https://example.com/verify",
				ExpiresIn:       300,
				Interval:        1,
			})
			return
		}
		// Always return pending
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(DeviceFlowError{
			Error:            "authorization_pending",
			ErrorDescription: "user has not yet authorized",
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	client := NewClient(server.URL, nil)
	var out bytes.Buffer

	_, err := Login(ctx, client, "bop", &out)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestLogin_SlowDown(t *testing.T) {
	var pollCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/auth/device":
			_ = json.NewEncoder(w).Encode(DeviceFlowResponse{
				DeviceCode:      "device-123",
				UserCode:        "ABCD-1234",
				VerificationURI: "https://example.com/verify",
				ExpiresIn:       300,
				Interval:        1,
			})

		case "/auth/device/token":
			count := pollCount.Add(1)
			if count == 1 {
				// First poll: slow_down
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(DeviceFlowError{
					Error:            "slow_down",
					ErrorDescription: "polling too fast",
				})
				return
			}
			// Second poll: success
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken:  "access-token",
				RefreshToken: "refresh-token",
				TokenType:    "bearer",
				ExpiresIn:    3600,
				UserID:       "user-1",
				Username:     "testuser",
			})

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	var out bytes.Buffer

	creds, err := Login(context.Background(), client, "bop", &out)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if creds.AccessToken != "access-token" {
		t.Errorf("AccessToken: got %q, want %q", creds.AccessToken, "access-token")
	}
	// Verify we polled at least twice (first slow_down, then success)
	if pollCount.Load() < 2 {
		t.Errorf("expected at least 2 polls (slow_down + success), got %d", pollCount.Load())
	}
}

func TestLogin_TokenDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/device" {
			_ = json.NewEncoder(w).Encode(DeviceFlowResponse{
				DeviceCode:      "device-123",
				UserCode:        "ABCD-1234",
				VerificationURI: "https://example.com/verify",
				ExpiresIn:       300,
				Interval:        1,
			})
			return
		}
		// Return access_denied
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(DeviceFlowError{
			Error:            "access_denied",
			ErrorDescription: "user denied the request",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	var out bytes.Buffer

	_, err := Login(context.Background(), client, "bop", &out)
	if err == nil {
		t.Fatal("expected error for access_denied")
	}
	if !strings.Contains(err.Error(), "access_denied") {
		t.Errorf("error should contain 'access_denied', got: %v", err)
	}
}
