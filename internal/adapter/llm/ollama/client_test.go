package ollama_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/delightfulhammers/bop/internal/adapter/llm/ollama"
	"github.com/delightfulhammers/bop/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers for config
func boolPtr(b bool) *bool { return &b }

func testProviderConfig() config.ProviderConfig {
	return config.ProviderConfig{
		Enabled: boolPtr(true),
		Model:   "codellama",
	}
}

func testHTTPConfig() config.HTTPConfig {
	return config.HTTPConfig{
		Timeout:           "120s",
		MaxRetries:        5,
		InitialBackoff:    "2s",
		MaxBackoff:        "32s",
		BackoffMultiplier: 2.0,
	}
}

func TestNewHTTPClient(t *testing.T) {
	client := ollama.NewHTTPClient("http://localhost:11434", "codellama", testProviderConfig(), testHTTPConfig())

	assert.NotNil(t, client)
}

func TestHTTPClient_Call_Success(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/generate", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request body
		var req ollama.GenerateRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "codellama", req.Model)
		assert.False(t, req.Stream)
		assert.NotEmpty(t, req.Prompt)

		// Send response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollama.GenerateResponse{
			Model:           "codellama",
			CreatedAt:       "2024-01-01T00:00:00Z",
			Response:        "test response from codellama",
			Done:            true,
			PromptEvalCount: 100,
			EvalCount:       200,
		})
	}))
	defer server.Close()

	client := ollama.NewHTTPClient(server.URL, "codellama", testProviderConfig(), testHTTPConfig())

	resp, err := client.Call(context.Background(), "test prompt", ollama.CallOptions{})

	require.NoError(t, err)
	assert.Equal(t, "test response from codellama", resp.Text)
	assert.Equal(t, "codellama", resp.Model)
	assert.Equal(t, 100, resp.TokensIn)
	assert.Equal(t, 200, resp.TokensOut)
}

func TestHTTPClient_Call_WithTemperature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollama.GenerateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Verify temperature was set in options
		require.NotNil(t, req.Options)
		assert.Equal(t, 0.7, req.Options["temperature"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollama.GenerateResponse{
			Model:    "codellama",
			Response: "response with temperature",
			Done:     true,
		})
	}))
	defer server.Close()

	client := ollama.NewHTTPClient(server.URL, "codellama", testProviderConfig(), testHTTPConfig())

	resp, err := client.Call(context.Background(), "test", ollama.CallOptions{
		Temperature: 0.7,
	})

	require.NoError(t, err)
	assert.Equal(t, "response with temperature", resp.Text)
}

func TestHTTPClient_Call_WithSeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollama.GenerateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Verify seed was set in options
		require.NotNil(t, req.Options)
		// Seed is stored as float64 in JSON
		assert.Equal(t, float64(12345), req.Options["seed"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollama.GenerateResponse{
			Model:    "codellama",
			Response: "deterministic response",
			Done:     true,
		})
	}))
	defer server.Close()

	client := ollama.NewHTTPClient(server.URL, "codellama", testProviderConfig(), testHTTPConfig())

	seed := uint64(12345)
	resp, err := client.Call(context.Background(), "test", ollama.CallOptions{
		Seed: &seed,
	})

	require.NoError(t, err)
	assert.Equal(t, "deterministic response", resp.Text)
}

func TestHTTPClient_Call_ConnectionRefused(t *testing.T) {
	// Use a port that won't have anything running
	client := ollama.NewHTTPClient("http://localhost:9999", "codellama", testProviderConfig(), testHTTPConfig())

	_, err := client.Call(context.Background(), "test", ollama.CallOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	assert.Contains(t, err.Error(), "Is Ollama running")
}

func TestHTTPClient_Call_ModelNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ollama.ErrorResponse{
			Error: "model 'nonexistent' not found",
		})
	}))
	defer server.Close()

	client := ollama.NewHTTPClient(server.URL, "nonexistent", testProviderConfig(), testHTTPConfig())

	_, err := client.Call(context.Background(), "test", ollama.CallOptions{})

	require.Error(t, err)
	var httpErr *llmhttp.Error
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, llmhttp.ErrTypeModelNotFound, httpErr.Type)
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "ollama pull")
}

func TestHTTPClient_Call_ServerError(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(ollama.ErrorResponse{
				Error: "internal server error",
			})
			return
		}
		// Success on 2nd attempt
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollama.GenerateResponse{
			Model:    "codellama",
			Response: "success after retry",
			Done:     true,
		})
	}))
	defer server.Close()

	client := ollama.NewHTTPClient(server.URL, "codellama", testProviderConfig(), testHTTPConfig())

	resp, err := client.Call(context.Background(), "test", ollama.CallOptions{})

	require.NoError(t, err)
	assert.Equal(t, "success after retry", resp.Text)
	assert.Equal(t, 2, callCount, "should have retried once")
}

func TestHTTPClient_Call_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := ollama.NewHTTPClient(server.URL, "codellama", testProviderConfig(), testHTTPConfig())
	client.SetTimeout(50 * time.Millisecond)

	_, err := client.Call(context.Background(), "test", ollama.CallOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

func TestHTTPClient_Call_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := ollama.NewHTTPClient(server.URL, "codellama", testProviderConfig(), testHTTPConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.Call(ctx, "test", ollama.CallOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestHTTPClient_Call_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"invalid json`))
	}))
	defer server.Close()

	client := ollama.NewHTTPClient(server.URL, "codellama", testProviderConfig(), testHTTPConfig())

	_, err := client.Call(context.Background(), "test", ollama.CallOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestHTTPClient_Call_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollama.GenerateResponse{
			Model:    "codellama",
			Response: "", // Empty response
			Done:     true,
		})
	}))
	defer server.Close()

	client := ollama.NewHTTPClient(server.URL, "codellama", testProviderConfig(), testHTTPConfig())

	_, err := client.Call(context.Background(), "test", ollama.CallOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestHTTPClient_Call_NotDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollama.GenerateResponse{
			Model:    "codellama",
			Response: "partial response",
			Done:     false, // Not done yet
		})
	}))
	defer server.Close()

	client := ollama.NewHTTPClient(server.URL, "codellama", testProviderConfig(), testHTTPConfig())

	_, err := client.Call(context.Background(), "test", ollama.CallOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete response")
}
