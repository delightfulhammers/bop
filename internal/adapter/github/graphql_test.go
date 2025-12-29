package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_ResolveThread(t *testing.T) {
	tests := []struct {
		name         string
		owner        string
		repo         string
		threadID     string
		statusCode   int
		responseBody string
		wantErr      bool
		errContains  string
	}{
		{
			name:       "successful resolve",
			owner:      "owner",
			repo:       "repo",
			threadID:   "PRRT_kwDOtest123",
			statusCode: http.StatusOK,
			responseBody: `{
				"data": {
					"resolveReviewThread": {
						"thread": {
							"id": "PRRT_kwDOtest123",
							"isResolved": true
						}
					}
				}
			}`,
			wantErr: false,
		},
		{
			name:       "thread not found",
			owner:      "owner",
			repo:       "repo",
			threadID:   "PRRT_invalid",
			statusCode: http.StatusOK,
			responseBody: `{
				"data": null,
				"errors": [
					{
						"message": "Could not resolve to a node with the global id of 'PRRT_invalid'",
						"type": "NOT_FOUND"
					}
				]
			}`,
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "empty thread ID",
			owner:       "owner",
			repo:        "repo",
			threadID:    "",
			wantErr:     true,
			errContains: "thread ID cannot be empty",
		},
		{
			name:        "invalid owner",
			owner:       "../hack",
			repo:        "repo",
			threadID:    "PRRT_test",
			wantErr:     true,
			errContains: "invalid owner",
		},
		{
			name:       "permission denied",
			owner:      "owner",
			repo:       "repo",
			threadID:   "PRRT_nopermission",
			statusCode: http.StatusOK,
			responseBody: `{
				"data": null,
				"errors": [
					{
						"message": "Resource not accessible by integration",
						"type": "FORBIDDEN"
					}
				]
			}`,
			wantErr:     true,
			errContains: "not accessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.statusCode != 0 {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Validate it's a POST to /graphql
					if r.Method != http.MethodPost {
						t.Errorf("expected POST, got %s", r.Method)
					}
					if !strings.HasSuffix(r.URL.Path, "/graphql") {
						t.Errorf("expected /graphql path, got %s", r.URL.Path)
					}

					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.responseBody))
				}))
				defer server.Close()
			}

			client := NewClient("test-token")
			if server != nil {
				client.SetBaseURL(server.URL)
			}
			client.SetMaxRetries(1)

			err := client.ResolveThread(context.Background(), tt.owner, tt.repo, tt.threadID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveThread() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

func TestClient_UnresolveThread(t *testing.T) {
	tests := []struct {
		name         string
		owner        string
		repo         string
		threadID     string
		statusCode   int
		responseBody string
		wantErr      bool
		errContains  string
	}{
		{
			name:       "successful unresolve",
			owner:      "owner",
			repo:       "repo",
			threadID:   "PRRT_kwDOtest123",
			statusCode: http.StatusOK,
			responseBody: `{
				"data": {
					"unresolveReviewThread": {
						"thread": {
							"id": "PRRT_kwDOtest123",
							"isResolved": false
						}
					}
				}
			}`,
			wantErr: false,
		},
		{
			name:       "thread not found",
			owner:      "owner",
			repo:       "repo",
			threadID:   "PRRT_invalid",
			statusCode: http.StatusOK,
			responseBody: `{
				"data": null,
				"errors": [
					{
						"message": "Could not resolve to a node with the global id of 'PRRT_invalid'",
						"type": "NOT_FOUND"
					}
				]
			}`,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.statusCode != 0 {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost {
						t.Errorf("expected POST, got %s", r.Method)
					}
					if !strings.HasSuffix(r.URL.Path, "/graphql") {
						t.Errorf("expected /graphql path, got %s", r.URL.Path)
					}

					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.responseBody))
				}))
				defer server.Close()
			}

			client := NewClient("test-token")
			if server != nil {
				client.SetBaseURL(server.URL)
			}
			client.SetMaxRetries(1)

			err := client.UnresolveThread(context.Background(), tt.owner, tt.repo, tt.threadID)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnresolveThread() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.errContains != "" && err != nil {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}
