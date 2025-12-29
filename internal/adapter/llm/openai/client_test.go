package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
	"github.com/bkyoung/code-reviewer/internal/adapter/llm/openai"
	"github.com/bkyoung/code-reviewer/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers for config
func boolPtr(b bool) *bool { return &b }

func testProviderConfig() config.ProviderConfig {
	return config.ProviderConfig{
		Enabled: boolPtr(true),
		Model:   "gpt-4o-mini",
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
	client := openai.NewHTTPClient("test-api-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())

	assert.NotNil(t, client)
}

func TestNewHTTPClient_EmptyAPIKey(t *testing.T) {
	client := openai.NewHTTPClient("", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())

	// Should still create client, but will fail on actual API calls
	assert.NotNil(t, client)
}

func TestHTTPClient_Call_Success(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request body
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "gpt-4o-mini", req.Model)
		assert.Equal(t, 0.0, req.Temperature)
		assert.Len(t, req.Messages, 2)
		assert.Equal(t, "system", req.Messages[0].Role)
		assert.Equal(t, "user", req.Messages[1].Role)

		// Send response
		resp := openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4o-mini",
			Choices: []openai.Choice{
				{
					Index: 0,
					Message: openai.Message{
						Role:    "assistant",
						Content: `{"summary": "Code looks good", "findings": []}`,
					},
					FinishReason: "stop",
				},
			},
			Usage: openai.Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-api-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	response, err := client.Call(context.Background(), "test prompt", openai.CallOptions{
		Temperature: 0.0,
		Seed:        uint64Ptr(12345),
		MaxTokens:   16384,
	})

	require.NoError(t, err)
	assert.Equal(t, `{"summary": "Code looks good", "findings": []}`, response.Text)
	assert.Equal(t, 100, response.TokensIn)
	assert.Equal(t, 50, response.TokensOut)
	assert.Equal(t, "gpt-4o-mini", response.Model)
	assert.Equal(t, "stop", response.FinishReason)
}

func TestHTTPClient_Call_AuthenticationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(openai.ErrorResponse{
			Error: openai.ErrorDetail{
				Message: "Invalid API key",
				Type:    "invalid_request_error",
				Code:    "invalid_api_key",
			},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("invalid-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test prompt", openai.CallOptions{})

	require.Error(t, err)
	var httpErr *llmhttp.Error
	assert.ErrorAs(t, err, &httpErr)
	assert.Equal(t, llmhttp.ErrTypeAuthentication, httpErr.Type)
	assert.Contains(t, httpErr.Message, "Invalid API key")
}

func TestHTTPClient_Call_RateLimitError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			// Return rate limit error first 2 times
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(openai.ErrorResponse{
				Error: openai.ErrorDetail{
					Message: "Rate limit exceeded",
					Type:    "rate_limit_error",
				},
			})
			return
		}

		// Success on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4o-mini",
			Choices: []openai.Choice{
				{
					Index: 0,
					Message: openai.Message{
						Role:    "assistant",
						Content: "success",
					},
					FinishReason: "stop",
				},
			},
			Usage: openai.Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	response, err := client.Call(context.Background(), "test", openai.CallOptions{})

	require.NoError(t, err, "should succeed after retries")
	assert.Equal(t, "success", response.Text)
	assert.Equal(t, 3, attempts, "should have retried twice")
}

func TestHTTPClient_Call_ServiceUnavailable(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Success on 2nd attempt
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4o-mini",
			Choices: []openai.Choice{
				{
					Message:      openai.Message{Role: "assistant", Content: "ok"},
					FinishReason: "stop",
				},
			},
			Usage: openai.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	response, err := client.Call(context.Background(), "test", openai.CallOptions{})

	require.NoError(t, err)
	assert.Equal(t, "ok", response.Text)
	assert.Equal(t, 2, attempts)
}

func TestHTTPClient_Call_InvalidRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(openai.ErrorResponse{
			Error: openai.ErrorDetail{
				Message: "Invalid request",
				Type:    "invalid_request_error",
			},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", openai.CallOptions{})

	require.Error(t, err)
	var httpErr *llmhttp.Error
	assert.ErrorAs(t, err, &httpErr)
	assert.Equal(t, llmhttp.ErrTypeInvalidRequest, httpErr.Type)
	assert.False(t, httpErr.IsRetryable(), "invalid request should not be retryable")
}

func TestHTTPClient_Call_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // Longer than timeout
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)
	client.SetTimeout(50 * time.Millisecond)

	_, err := client.Call(context.Background(), "test", openai.CallOptions{})

	require.Error(t, err)
	var httpErr *llmhttp.Error
	assert.ErrorAs(t, err, &httpErr)
	assert.Equal(t, llmhttp.ErrTypeTimeout, httpErr.Type)
}

func TestHTTPClient_Call_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Call(ctx, "test", openai.CallOptions{})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestHTTPClient_Call_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", openai.CallOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestHTTPClient_Call_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4o-mini",
			Choices: []openai.Choice{}, // Empty choices
			Usage:   openai.Usage{PromptTokens: 10, CompletionTokens: 0, TotalTokens: 10},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	_, err := client.Call(context.Background(), "test", openai.CallOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices in response")
}

func TestHTTPClient_Call_O1Model(t *testing.T) {
	// Test that o1 models use max_completion_tokens and omit unsupported params
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify o1 model uses max_completion_tokens, not max_tokens
		assert.Equal(t, 4000, req.MaxCompletionTokens, "o1 models should use max_completion_tokens")
		assert.Equal(t, 0, req.MaxTokens, "o1 models should not set max_tokens")

		// Verify o1 models don't send temperature or seed
		assert.Equal(t, 0.0, req.Temperature, "o1 models should not set temperature")
		assert.Nil(t, req.Seed, "o1 models should not set seed")

		// Send response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "o1-mini",
			Choices: []openai.Choice{
				{
					Index:        0,
					Message:      openai.Message{Role: "assistant", Content: "test response"},
					FinishReason: "stop",
				},
			},
			Usage: openai.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
		})
	}))
	defer server.Close()

	// Test with o1-mini
	client := openai.NewHTTPClient("test-key", "o1-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	seed := uint64(12345)
	resp, err := client.Call(context.Background(), "test prompt", openai.CallOptions{
		Temperature: 0.7, // Should be ignored for o1
		Seed:        &seed,
		MaxTokens:   4000,
	})

	require.NoError(t, err)
	assert.Equal(t, "test response", resp.Text)
}

func TestHTTPClient_Call_O4Model(t *testing.T) {
	// Test that o4 models (if they exist) also use o1 behavior
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify o4 model uses max_completion_tokens
		assert.Equal(t, 2000, req.MaxCompletionTokens)
		assert.Equal(t, 0, req.MaxTokens)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "o4-mini",
			Choices: []openai.Choice{
				{Index: 0, Message: openai.Message{Role: "assistant", Content: "response"}, FinishReason: "stop"},
			},
			Usage: openai.Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "o4-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test", openai.CallOptions{MaxTokens: 2000})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Text)
}

func TestHTTPClient_Call_O3Model(t *testing.T) {
	// Test that o3 models use max_completion_tokens (like o1/o4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify o3 model uses max_completion_tokens
		assert.Equal(t, 3000, req.MaxCompletionTokens, "o3 models should use max_completion_tokens")
		assert.Equal(t, 0, req.MaxTokens, "o3 models should not set max_tokens")

		// Verify o3 models don't send temperature or seed
		assert.Equal(t, 0.0, req.Temperature, "o3 models should not set temperature")
		assert.Nil(t, req.Seed, "o3 models should not set seed")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "o3-mini",
			Choices: []openai.Choice{
				{Index: 0, Message: openai.Message{Role: "assistant", Content: "o3 response"}, FinishReason: "stop"},
			},
			Usage: openai.Usage{PromptTokens: 8, CompletionTokens: 12, TotalTokens: 20},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "o3-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	seed := uint64(54321)
	resp, err := client.Call(context.Background(), "test prompt", openai.CallOptions{
		Temperature: 0.8,   // Should be ignored for o3
		Seed:        &seed, // Should be ignored for o3
		MaxTokens:   3000,
	})
	require.NoError(t, err)
	assert.Equal(t, "o3 response", resp.Text)
}

func TestHTTPClient_Call_O3ModelExactName(t *testing.T) {
	// Test that exact model name "o3" (no suffix) is also recognized
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify exact "o3" model name uses max_completion_tokens
		assert.Equal(t, 2500, req.MaxCompletionTokens, "o3 exact name should use max_completion_tokens")
		assert.Equal(t, 0, req.MaxTokens, "o3 exact name should not set max_tokens")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "o3",
			Choices: []openai.Choice{
				{Index: 0, Message: openai.Message{Role: "assistant", Content: "o3 exact response"}, FinishReason: "stop"},
			},
			Usage: openai.Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "o3", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test", openai.CallOptions{MaxTokens: 2500})
	require.NoError(t, err)
	assert.Equal(t, "o3 exact response", resp.Text)
}

func TestHTTPClient_Call_RegularModel_UsesTemperatureAndSeed(t *testing.T) {
	// Verify regular models (non-o1) still use temperature, seed, and max_tokens
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Regular models should use max_tokens
		assert.Equal(t, 1000, req.MaxTokens, "regular models should use max_tokens")
		assert.Equal(t, 0, req.MaxCompletionTokens, "regular models should not set max_completion_tokens")

		// Regular models should support temperature and seed
		assert.Equal(t, 0.5, req.Temperature)
		require.NotNil(t, req.Seed)
		assert.Equal(t, uint64(99999), *req.Seed)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4o-mini",
			Choices: []openai.Choice{
				{Index: 0, Message: openai.Message{Role: "assistant", Content: "response"}, FinishReason: "stop"},
			},
			Usage: openai.Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	seed := uint64(99999)
	resp, err := client.Call(context.Background(), "test", openai.CallOptions{
		Temperature: 0.5,
		Seed:        &seed,
		MaxTokens:   1000,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Text)
}

// Helper function
func uint64Ptr(v uint64) *uint64 {
	return &v
}

func TestHTTPClient_WithObservability(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4o-mini",
			Choices: []openai.Choice{
				{Index: 0, Message: openai.Message{Role: "assistant", Content: "test response"}, FinishReason: "stop"},
			},
			Usage: openai.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("sk-test-key-1234567890", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	// Set up observability
	logger := llmhttp.NewDefaultLogger(llmhttp.LogLevelDebug, llmhttp.LogFormatHuman, true)
	metrics := llmhttp.NewDefaultMetrics()
	pricing := llmhttp.NewDefaultPricing()

	client.SetLogger(logger)
	client.SetMetrics(metrics)
	client.SetPricing(pricing)

	resp, err := client.Call(context.Background(), "test prompt", openai.CallOptions{MaxTokens: 1024})

	require.NoError(t, err)
	assert.Equal(t, "test response", resp.Text)
	assert.Equal(t, 100, resp.TokensIn)
	assert.Equal(t, 50, resp.TokensOut)
	assert.Greater(t, resp.Cost, 0.0, "Cost should be calculated")

	// Verify metrics
	stats := metrics.GetStats()
	assert.Equal(t, 1, stats.TotalRequests)
	assert.Equal(t, 100, stats.TotalTokensIn)
	assert.Equal(t, 50, stats.TotalTokensOut)
	assert.Greater(t, stats.TotalCost, 0.0)
	assert.Greater(t, stats.TotalDuration, time.Duration(0))

	// Verify provider-specific stats
	openaiStats := stats.ByProvider["openai"]
	assert.Equal(t, 1, openaiStats.Requests)
	assert.Equal(t, 100, openaiStats.TokensIn)
	assert.Equal(t, 50, openaiStats.TokensOut)
	assert.Greater(t, openaiStats.Cost, 0.0)
}

func TestHTTPClient_WithObservability_Error(t *testing.T) {
	// Mock server returning error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(openai.ErrorResponse{
			Error: openai.ErrorDetail{
				Message: "Rate limit exceeded",
				Type:    "rate_limit_error",
				Code:    "rate_limit_exceeded",
			},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("sk-test-key", "gpt-4o-mini", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	// Set up observability
	metrics := llmhttp.NewDefaultMetrics()
	client.SetMetrics(metrics)

	_, err := client.Call(context.Background(), "test prompt", openai.CallOptions{MaxTokens: 1024})

	require.Error(t, err)

	// Verify error was recorded in metrics
	stats := metrics.GetStats()
	assert.Equal(t, 1, stats.ErrorCount)
	assert.Equal(t, 1, stats.ByProvider["openai"].Errors)
}

func TestHTTPClient_Call_GPT5_UsesMaxCompletionTokens(t *testing.T) {
	// GPT-5 models should use max_completion_tokens (like o1 models) but still support temperature
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify gpt-5 model uses max_completion_tokens, not max_tokens
		assert.Equal(t, 2000, req.MaxCompletionTokens, "gpt-5 models should use max_completion_tokens")
		assert.Equal(t, 0, req.MaxTokens, "gpt-5 models should not set max_tokens")

		// GPT-5 models should still support temperature (unlike o1 models)
		assert.Equal(t, 0.7, req.Temperature)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
			ID:      "chatcmpl-gpt5",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-5.2",
			Choices: []openai.Choice{
				{Index: 0, Message: openai.Message{Role: "assistant", Content: "gpt-5 response"}, FinishReason: "stop"},
			},
			Usage: openai.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		})
	}))
	defer server.Close()

	client := openai.NewHTTPClient("test-key", "gpt-5.2", testProviderConfig(), testHTTPConfig())
	client.SetBaseURL(server.URL)

	resp, err := client.Call(context.Background(), "test", openai.CallOptions{MaxTokens: 2000, Temperature: 0.7})
	require.NoError(t, err)
	assert.Equal(t, "gpt-5 response", resp.Text)
}
