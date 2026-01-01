package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetFileContent(t *testing.T) {
	tests := []struct {
		name           string
		owner          string
		repo           string
		path           string
		ref            string
		serverResponse interface{}
		statusCode     int
		wantContent    string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:  "returns file content",
			owner: "testowner",
			repo:  "testrepo",
			path:  "ARCHITECTURE.md",
			ref:   "main",
			serverResponse: contentAPIResponse{
				Type:     "file",
				Encoding: "base64",
				Content:  base64.StdEncoding.EncodeToString([]byte("# Architecture\n\nThis is the architecture.")),
			},
			statusCode:  http.StatusOK,
			wantContent: "# Architecture\n\nThis is the architecture.",
			wantErr:     false,
		},
		{
			name:  "returns content with specific ref",
			owner: "testowner",
			repo:  "testrepo",
			path:  "README.md",
			ref:   "abc123",
			serverResponse: contentAPIResponse{
				Type:     "file",
				Encoding: "base64",
				Content:  base64.StdEncoding.EncodeToString([]byte("# README")),
			},
			statusCode:  http.StatusOK,
			wantContent: "# README",
			wantErr:     false,
		},
		{
			name:           "returns error for 404",
			owner:          "testowner",
			repo:           "testrepo",
			path:           "nonexistent.md",
			ref:            "main",
			serverResponse: errorResponse{Message: "Not Found"},
			statusCode:     http.StatusNotFound,
			wantErr:        true,
			wantErrContain: "not found",
		},
		{
			name:  "returns error for directory",
			owner: "testowner",
			repo:  "testrepo",
			path:  "src",
			ref:   "main",
			serverResponse: contentAPIResponse{
				Type: "dir",
			},
			statusCode:     http.StatusOK,
			wantErr:        true,
			wantErrContain: "not a file",
		},
		{
			name:           "returns error for empty owner",
			owner:          "",
			repo:           "testrepo",
			path:           "file.md",
			ref:            "main",
			serverResponse: nil,
			statusCode:     0,
			wantErr:        true,
			wantErrContain: "owner",
		},
		{
			name:           "returns error for empty repo",
			owner:          "testowner",
			repo:           "",
			path:           "file.md",
			ref:            "main",
			serverResponse: nil,
			statusCode:     0,
			wantErr:        true,
			wantErrContain: "repo",
		},
		{
			name:           "returns error for empty path",
			owner:          "testowner",
			repo:           "testrepo",
			path:           "",
			ref:            "main",
			serverResponse: nil,
			statusCode:     0,
			wantErr:        true,
			wantErrContain: "path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.serverResponse != nil {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Verify ref query parameter
					if tt.ref != "" {
						assert.Equal(t, tt.ref, r.URL.Query().Get("ref"))
					}
					w.WriteHeader(tt.statusCode)
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}))
				defer server.Close()
			}

			client := NewClient("test-token")
			if server != nil {
				client.SetBaseURL(server.URL)
			}
			client.SetMaxRetries(0)

			content, err := client.GetFileContent(context.Background(), tt.owner, tt.repo, tt.path, tt.ref)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantContent, content)
		})
	}
}

func TestClient_GetFileContent_Base64Decoding(t *testing.T) {
	// Test with multiline content that includes newlines in base64
	originalContent := "Line 1\nLine 2\nLine 3\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GitHub returns base64 with embedded newlines for readability
		encoded := base64.StdEncoding.EncodeToString([]byte(originalContent))
		response := contentAPIResponse{
			Type:     "file",
			Encoding: "base64",
			Content:  encoded,
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.SetBaseURL(server.URL)
	client.SetMaxRetries(0)

	content, err := client.GetFileContent(context.Background(), "owner", "repo", "file.txt", "main")

	require.NoError(t, err)
	assert.Equal(t, originalContent, content)
}

// contentAPIResponse matches GitHub Contents API response
type contentAPIResponse struct {
	Type     string `json:"type"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
	Size     int    `json:"size"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
}
