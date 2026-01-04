package http_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/stretchr/testify/assert"
)

func TestError_Error(t *testing.T) {
	err := &llmhttp.Error{
		Type:       llmhttp.ErrTypeAuthentication,
		Message:    "invalid API key",
		StatusCode: 401,
		Provider:   "openai",
	}

	expected := "openai: authentication error: invalid API key (status: 401)"
	assert.Equal(t, expected, err.Error())
}

func TestError_Is(t *testing.T) {
	err1 := &llmhttp.Error{Type: llmhttp.ErrTypeRateLimit, Message: "rate limited"}
	err2 := &llmhttp.Error{Type: llmhttp.ErrTypeRateLimit, Message: "different message"}
	err3 := &llmhttp.Error{Type: llmhttp.ErrTypeAuthentication, Message: "auth failed"}

	// Same type should match
	assert.True(t, errors.Is(err1, err2))

	// Different type should not match
	assert.False(t, errors.Is(err1, err3))
}

func TestError_Retryable(t *testing.T) {
	tests := []struct {
		name      string
		errType   llmhttp.ErrorType
		retryable bool
	}{
		{"rate limit is retryable", llmhttp.ErrTypeRateLimit, true},
		{"service unavailable is retryable", llmhttp.ErrTypeServiceUnavailable, true},
		{"timeout is retryable", llmhttp.ErrTypeTimeout, true},
		{"authentication is not retryable", llmhttp.ErrTypeAuthentication, false},
		{"invalid request is not retryable", llmhttp.ErrTypeInvalidRequest, false},
		{"content filtered is not retryable", llmhttp.ErrTypeContentFiltered, false},
		{"model not found is not retryable", llmhttp.ErrTypeModelNotFound, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &llmhttp.Error{
				Type:      tt.errType,
				Message:   "test error",
				Retryable: tt.retryable,
			}
			assert.Equal(t, tt.retryable, err.IsRetryable())
		})
	}
}

func TestNewAuthenticationError(t *testing.T) {
	err := llmhttp.NewAuthenticationError("openai", "invalid API key")

	assert.Equal(t, llmhttp.ErrTypeAuthentication, err.Type)
	assert.Equal(t, "invalid API key", err.Message)
	assert.Equal(t, "openai", err.Provider)
	assert.Equal(t, 401, err.StatusCode)
	assert.False(t, err.IsRetryable())
}

func TestNewRateLimitError(t *testing.T) {
	err := llmhttp.NewRateLimitError("anthropic", "too many requests")

	assert.Equal(t, llmhttp.ErrTypeRateLimit, err.Type)
	assert.Equal(t, "too many requests", err.Message)
	assert.Equal(t, "anthropic", err.Provider)
	assert.Equal(t, 429, err.StatusCode)
	assert.True(t, err.IsRetryable())
}

func TestNewServiceUnavailableError(t *testing.T) {
	err := llmhttp.NewServiceUnavailableError("gemini", "server overloaded")

	assert.Equal(t, llmhttp.ErrTypeServiceUnavailable, err.Type)
	assert.Equal(t, "server overloaded", err.Message)
	assert.Equal(t, "gemini", err.Provider)
	assert.Equal(t, 503, err.StatusCode)
	assert.True(t, err.IsRetryable())
}

func TestNewInvalidRequestError(t *testing.T) {
	err := llmhttp.NewInvalidRequestError("openai", "missing required field")

	assert.Equal(t, llmhttp.ErrTypeInvalidRequest, err.Type)
	assert.Equal(t, "missing required field", err.Message)
	assert.Equal(t, "openai", err.Provider)
	assert.Equal(t, 400, err.StatusCode)
	assert.False(t, err.IsRetryable())
}

func TestNewTimeoutError(t *testing.T) {
	err := llmhttp.NewTimeoutError("ollama", "request timed out after 60s")

	assert.Equal(t, llmhttp.ErrTypeTimeout, err.Type)
	assert.Equal(t, "request timed out after 60s", err.Message)
	assert.Equal(t, "ollama", err.Provider)
	assert.Equal(t, 0, err.StatusCode)
	assert.True(t, err.IsRetryable())
}

func TestNewModelNotFoundError(t *testing.T) {
	err := llmhttp.NewModelNotFoundError("ollama", "model 'codellama' not found")

	assert.Equal(t, llmhttp.ErrTypeModelNotFound, err.Type)
	assert.Equal(t, "model 'codellama' not found", err.Message)
	assert.Equal(t, "ollama", err.Provider)
	assert.Equal(t, 404, err.StatusCode)
	assert.False(t, err.IsRetryable())
}

func TestNewContentFilteredError(t *testing.T) {
	err := llmhttp.NewContentFilteredError("gemini", "content blocked by safety filters")

	assert.Equal(t, llmhttp.ErrTypeContentFiltered, err.Type)
	assert.Equal(t, "content blocked by safety filters", err.Message)
	assert.Equal(t, "gemini", err.Provider)
	assert.Equal(t, 400, err.StatusCode)
	assert.False(t, err.IsRetryable())
}

func TestErrorTypeString(t *testing.T) {
	tests := []struct {
		errType  llmhttp.ErrorType
		expected string
	}{
		{llmhttp.ErrTypeAuthentication, "authentication error"},
		{llmhttp.ErrTypeRateLimit, "rate limit exceeded"},
		{llmhttp.ErrTypeServiceUnavailable, "service unavailable"},
		{llmhttp.ErrTypeInvalidRequest, "invalid request"},
		{llmhttp.ErrTypeTimeout, "timeout"},
		{llmhttp.ErrTypeNetwork, "network error"},
		{llmhttp.ErrTypeModelNotFound, "model not found"},
		{llmhttp.ErrTypeContentFiltered, "content filtered"},
		{llmhttp.ErrTypeUnknown, "unknown error"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.errType.String())
		})
	}
}

func TestNewNetworkError(t *testing.T) {
	err := llmhttp.NewNetworkError("openai", "connection refused")

	assert.Equal(t, llmhttp.ErrTypeNetwork, err.Type)
	assert.Equal(t, "connection refused", err.Message)
	assert.Equal(t, "openai", err.Provider)
	assert.Equal(t, 0, err.StatusCode)
	assert.True(t, err.IsRetryable())
}

// mockTimeoutErr implements net.Error with Timeout() returning true
type mockTimeoutErr struct {
	msg string
}

func (e *mockTimeoutErr) Error() string   { return e.msg }
func (e *mockTimeoutErr) Timeout() bool   { return true }
func (e *mockTimeoutErr) Temporary() bool { return true }

// mockNetworkErr is a plain error that doesn't implement net.Error timeout
type mockNetworkErr struct {
	msg string
}

func (e *mockNetworkErr) Error() string { return e.msg }

func TestClassifyNetworkError_ContextDeadlineExceeded(t *testing.T) {
	// Create a context that's already timed out
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond) // Ensure deadline passes

	genericErr := errors.New("some error")
	result := llmhttp.ClassifyNetworkError("openai", genericErr, ctx)

	assert.Equal(t, llmhttp.ErrTypeTimeout, result.Type)
	assert.Equal(t, "request timed out", result.Message)
	assert.Equal(t, "openai", result.Provider)
	assert.True(t, result.IsRetryable())
}

func TestClassifyNetworkError_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	genericErr := errors.New("some error")
	result := llmhttp.ClassifyNetworkError("anthropic", genericErr, ctx)

	assert.Equal(t, llmhttp.ErrTypeUnknown, result.Type)
	assert.Equal(t, "request canceled", result.Message)
	assert.Equal(t, "anthropic", result.Provider)
	assert.False(t, result.IsRetryable())
}

func TestClassifyNetworkError_NetErrorTimeout(t *testing.T) {
	ctx := context.Background()
	timeoutErr := &mockTimeoutErr{msg: "i/o timeout"}

	result := llmhttp.ClassifyNetworkError("gemini", timeoutErr, ctx)

	assert.Equal(t, llmhttp.ErrTypeTimeout, result.Type)
	assert.Equal(t, "i/o timeout", result.Message)
	assert.Equal(t, "gemini", result.Provider)
	assert.True(t, result.IsRetryable())
}

func TestClassifyNetworkError_GenericNetworkError(t *testing.T) {
	ctx := context.Background()
	networkErr := &mockNetworkErr{msg: "connection refused"}

	result := llmhttp.ClassifyNetworkError("ollama", networkErr, ctx)

	assert.Equal(t, llmhttp.ErrTypeNetwork, result.Type)
	assert.Equal(t, "connection refused", result.Message)
	assert.Equal(t, "ollama", result.Provider)
	assert.True(t, result.IsRetryable())
}

func TestClassifyNetworkError_DNSError(t *testing.T) {
	ctx := context.Background()
	// DNS errors are common network errors
	dnsErr := &net.DNSError{
		Err:  "no such host",
		Name: "api.example.com",
	}

	result := llmhttp.ClassifyNetworkError("openai", dnsErr, ctx)

	assert.Equal(t, llmhttp.ErrTypeNetwork, result.Type)
	assert.Contains(t, result.Message, "no such host")
	assert.Equal(t, "openai", result.Provider)
	assert.True(t, result.IsRetryable())
}
