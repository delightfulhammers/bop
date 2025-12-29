package anthropic_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bkyoung/code-reviewer/internal/adapter/llm/anthropic"
	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
	"github.com/bkyoung/code-reviewer/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers for config
func boolPtr(b bool) *bool { return &b }

func testProviderConfig() config.ProviderConfig {
	return config.ProviderConfig{
		Enabled: boolPtr(true),
		Model:   "claude-3-5-sonnet-20241022",
	}
}

func testHTTPConfig() config.HTTPConfig {
	return config.HTTPConfig{
		Timeout:           "60s",
		MaxRetries:        5,
		InitialBackoff:    "2s",
		MaxBackoff:        "32s",
		BackoffMultiplier: 2.0,
	}
}

func TestNewHTTPClient(t *testing.T) {
	client := anthropic.NewHTTPClient("test-api-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())

	assert.NotNil(t, client)
}

func TestNewHTTPClient_EmptyAPIKey(t *testing.T) {
	client := anthropic.NewHTTPClient("", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())

	// Should still create client, but will fail on actual API calls
	assert.NotNil(t, client)
}

func TestHTTPClient_Call_Success(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))

		// Parse request body
		var req anthropic.MessagesRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "claude-3-5-sonnet-20241022", req.Model)
		assert.Equal(t, 4096, req.MaxTokens)
		assert.Len(t, req.Messages, 1)
		assert.Equal(t, "user", req.Messages[0].Role)

		// Send response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(anthropic.MessagesResponse{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []anthropic.ContentBlock{
				{
					Type: "text",
					Text: "test response",
				},
			},
			Model:      "claude-3-5-sonnet-20241022",
			StopReason: "end_turn",
			Usage: anthropic.Usage{
				InputTokens:  10,
				OutputTokens: 20,
			},
		})
	}))
	defer server.Close()

	client := anthropic.NewHTTPClient("test-api-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test prompt", anthropic.CallOptions{
		MaxTokens: 4096,
	})

	require.NoError(t, err)
	assert.Equal(t, "test response", resp.Text)
	assert.Equal(t, 10, resp.TokensIn)
	assert.Equal(t, 20, resp.TokensOut)
	assert.Equal(t, "claude-3-5-sonnet-20241022", resp.Model)
	assert.Equal(t, "end_turn", resp.StopReason)
}

func TestHTTPClient_Call_AuthenticationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(anthropic.ErrorResponse{
			Type: "error",
			Error: anthropic.ErrorDetail{
				Type:    "authentication_error",
				Message: "invalid x-api-key",
			},
		})
	}))
	defer server.Close()

	client := anthropic.NewHTTPClient("invalid-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", anthropic.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	var httpErr *llmhttp.Error
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, llmhttp.ErrTypeAuthentication, httpErr.Type)
	assert.Equal(t, http.StatusUnauthorized, httpErr.StatusCode)
	assert.False(t, httpErr.Retryable)
}

func TestHTTPClient_Call_RateLimitError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(anthropic.ErrorResponse{
				Type: "error",
				Error: anthropic.ErrorDetail{
					Type:    "rate_limit_error",
					Message: "rate limit exceeded",
				},
			})
			return
		}
		// Success on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(anthropic.MessagesResponse{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []anthropic.ContentBlock{
				{Type: "text", Text: "success after retry"},
			},
			Model:      "claude-3-5-sonnet-20241022",
			StopReason: "end_turn",
			Usage:      anthropic.Usage{InputTokens: 5, OutputTokens: 10},
		})
	}))
	defer server.Close()

	client := anthropic.NewHTTPClient("test-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test", anthropic.CallOptions{MaxTokens: 1024})

	require.NoError(t, err)
	assert.Equal(t, "success after retry", resp.Text)
	assert.Equal(t, 3, callCount, "should have retried twice")
}

func TestHTTPClient_Call_OverloadedError(t *testing.T) {
	// Anthropic returns 529 status code when overloaded
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(529) // Anthropic-specific overloaded status
			_ = json.NewEncoder(w).Encode(anthropic.ErrorResponse{
				Type: "error",
				Error: anthropic.ErrorDetail{
					Type:    "overloaded_error",
					Message: "service is overloaded",
				},
			})
			return
		}
		// Success on 2nd attempt
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(anthropic.MessagesResponse{
			ID:      "msg_123",
			Type:    "message",
			Role:    "assistant",
			Content: []anthropic.ContentBlock{{Type: "text", Text: "recovered"}},
			Model:   "claude-3-5-sonnet-20241022",
			Usage:   anthropic.Usage{InputTokens: 5, OutputTokens: 5},
		})
	}))
	defer server.Close()

	client := anthropic.NewHTTPClient("test-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test", anthropic.CallOptions{MaxTokens: 1024})

	require.NoError(t, err)
	assert.Equal(t, "recovered", resp.Text)
	assert.Equal(t, 2, callCount)
}

func TestHTTPClient_Call_InvalidRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(anthropic.ErrorResponse{
			Type: "error",
			Error: anthropic.ErrorDetail{
				Type:    "invalid_request_error",
				Message: "max_tokens is required",
			},
		})
	}))
	defer server.Close()

	client := anthropic.NewHTTPClient("test-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", anthropic.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	var httpErr *llmhttp.Error
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, llmhttp.ErrTypeInvalidRequest, httpErr.Type)
	assert.False(t, httpErr.Retryable)
}

func TestHTTPClient_Call_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := anthropic.NewHTTPClient("test-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)
	client.SetTimeout(50 * time.Millisecond)

	_, err := client.Call(context.Background(), "test", anthropic.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

func TestHTTPClient_Call_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := anthropic.NewHTTPClient("test-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.Call(ctx, "test", anthropic.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestHTTPClient_Call_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"invalid json`))
	}))
	defer server.Close()

	client := anthropic.NewHTTPClient("test-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", anthropic.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestHTTPClient_Call_EmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(anthropic.MessagesResponse{
			ID:         "msg_123",
			Type:       "message",
			Role:       "assistant",
			Content:    []anthropic.ContentBlock{}, // Empty content
			Model:      "claude-3-5-sonnet-20241022",
			StopReason: "end_turn",
			Usage:      anthropic.Usage{InputTokens: 10, OutputTokens: 0},
		})
	}))
	defer server.Close()

	client := anthropic.NewHTTPClient("test-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", anthropic.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no content in response")
}

func TestHTTPClient_Call_MultipleContentBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(anthropic.MessagesResponse{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []anthropic.ContentBlock{
				{Type: "text", Text: "First block. "},
				{Type: "text", Text: "Second block."},
			},
			Model:      "claude-3-5-sonnet-20241022",
			StopReason: "end_turn",
			Usage:      anthropic.Usage{InputTokens: 10, OutputTokens: 20},
		})
	}))
	defer server.Close()

	client := anthropic.NewHTTPClient("test-key", "claude-3-5-sonnet-20241022", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test", anthropic.CallOptions{MaxTokens: 1024})

	require.NoError(t, err)
	assert.Equal(t, "First block. Second block.", resp.Text)
}
