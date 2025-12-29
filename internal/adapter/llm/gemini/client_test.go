package gemini_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bkyoung/code-reviewer/internal/adapter/llm/gemini"
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
		Model:   "gemini-pro",
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
	client := gemini.NewHTTPClient("test-api-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())

	assert.NotNil(t, client)
}

func TestNewHTTPClient_EmptyAPIKey(t *testing.T) {
	client := gemini.NewHTTPClient("", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())

	// Should still create client, but will fail on actual API calls
	assert.NotNil(t, client)
}

func TestHTTPClient_Call_Success(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.True(t, strings.Contains(r.URL.Path, "/v1beta/models/gemini-1.5-pro:generateContent"))
		assert.Equal(t, "test-api-key", r.URL.Query().Get("key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request body
		var req gemini.GenerateContentRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Len(t, req.Contents, 1)
		assert.Len(t, req.Contents[0].Parts, 1)
		assert.NotEmpty(t, req.Contents[0].Parts[0].Text)

		// Send response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gemini.GenerateContentResponse{
			Candidates: []gemini.Candidate{
				{
					Content: gemini.Content{
						Parts: []gemini.Part{
							{Text: "test response from gemini"},
						},
						Role: "model",
					},
					FinishReason: "STOP",
				},
			},
			UsageMetadata: gemini.UsageMetadata{
				PromptTokenCount:     100,
				CandidatesTokenCount: 200,
				TotalTokenCount:      300,
			},
		})
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("test-api-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test prompt", gemini.CallOptions{})

	require.NoError(t, err)
	assert.Equal(t, "test response from gemini", resp.Text)
	assert.Equal(t, 100, resp.TokensIn)
	assert.Equal(t, 200, resp.TokensOut)
	assert.Equal(t, "STOP", resp.FinishReason)
}

func TestHTTPClient_Call_WithTemperature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req gemini.GenerateContentRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Verify temperature was set
		require.NotNil(t, req.GenerationConfig)
		assert.Equal(t, 0.7, req.GenerationConfig.Temperature)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gemini.GenerateContentResponse{
			Candidates: []gemini.Candidate{
				{
					Content:      gemini.Content{Parts: []gemini.Part{{Text: "response"}}},
					FinishReason: "STOP",
				},
			},
			UsageMetadata: gemini.UsageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 10},
		})
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("test-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test", gemini.CallOptions{
		Temperature: 0.7,
		MaxTokens:   1000,
	})

	require.NoError(t, err)
	assert.Equal(t, "response", resp.Text)
}

func TestHTTPClient_Call_AuthenticationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(gemini.ErrorResponse{
			Error: gemini.ErrorDetail{
				Code:    401,
				Message: "API key not valid",
				Status:  "UNAUTHENTICATED",
			},
		})
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("invalid-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", gemini.CallOptions{MaxTokens: 1024})

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
			_ = json.NewEncoder(w).Encode(gemini.ErrorResponse{
				Error: gemini.ErrorDetail{
					Code:    429,
					Message: "Resource exhausted",
					Status:  "RESOURCE_EXHAUSTED",
				},
			})
			return
		}
		// Success on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gemini.GenerateContentResponse{
			Candidates: []gemini.Candidate{
				{
					Content:      gemini.Content{Parts: []gemini.Part{{Text: "success after retry"}}},
					FinishReason: "STOP",
				},
			},
			UsageMetadata: gemini.UsageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 10},
		})
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("test-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test", gemini.CallOptions{MaxTokens: 1024})

	require.NoError(t, err)
	assert.Equal(t, "success after retry", resp.Text)
	assert.Equal(t, 3, callCount, "should have retried twice")
}

func TestHTTPClient_Call_InvalidRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(gemini.ErrorResponse{
			Error: gemini.ErrorDetail{
				Code:    400,
				Message: "Invalid request",
				Status:  "INVALID_ARGUMENT",
			},
		})
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("test-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", gemini.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	var httpErr *llmhttp.Error
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, llmhttp.ErrTypeInvalidRequest, httpErr.Type)
	assert.False(t, httpErr.Retryable)
}

func TestHTTPClient_Call_ContentFiltered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gemini.GenerateContentResponse{
			Candidates: []gemini.Candidate{
				{
					Content:      gemini.Content{Parts: []gemini.Part{}},
					FinishReason: "SAFETY", // Content blocked by safety filters
				},
			},
			UsageMetadata: gemini.UsageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 0},
		})
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("test-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", gemini.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	var httpErr *llmhttp.Error
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, llmhttp.ErrTypeContentFiltered, httpErr.Type)
	assert.Contains(t, err.Error(), "safety filters")
}

func TestHTTPClient_Call_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("test-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)
	client.SetTimeout(50 * time.Millisecond)

	_, err := client.Call(context.Background(), "test", gemini.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

func TestHTTPClient_Call_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("test-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.Call(ctx, "test", gemini.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestHTTPClient_Call_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"invalid json`))
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("test-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", gemini.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestHTTPClient_Call_EmptyCandidates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gemini.GenerateContentResponse{
			Candidates:    []gemini.Candidate{}, // Empty candidates
			UsageMetadata: gemini.UsageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 0},
		})
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("test-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", gemini.CallOptions{MaxTokens: 1024})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no candidates")
}

func TestHTTPClient_Call_MultiplePartsConcatenation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gemini.GenerateContentResponse{
			Candidates: []gemini.Candidate{
				{
					Content: gemini.Content{
						Parts: []gemini.Part{
							{Text: "First part. "},
							{Text: "Second part."},
						},
					},
					FinishReason: "STOP",
				},
			},
			UsageMetadata: gemini.UsageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 20},
		})
	}))
	defer server.Close()

	client := gemini.NewHTTPClient("test-key", "gemini-1.5-pro", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test", gemini.CallOptions{MaxTokens: 1024})

	require.NoError(t, err)
	assert.Equal(t, "First part. Second part.", resp.Text)
}
