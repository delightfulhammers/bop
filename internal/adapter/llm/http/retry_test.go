package http_test

import (
	"context"
	"errors"
	"testing"
	"time"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRetryConfig(t *testing.T) {
	config := llmhttp.DefaultRetryConfig()

	assert.Equal(t, 5, config.MaxRetries)
	assert.Equal(t, 2*time.Second, config.InitialBackoff)
	assert.Equal(t, 32*time.Second, config.MaxBackoff)
	assert.Equal(t, 2.0, config.Multiplier)
}

func TestExponentialBackoff(t *testing.T) {
	config := llmhttp.RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 2 * time.Second,
		MaxBackoff:     32 * time.Second,
		Multiplier:     2.0,
	}

	tests := []struct {
		name    string
		attempt int
		minWait time.Duration
		maxWait time.Duration
	}{
		{"attempt 0", 0, 1500 * time.Millisecond, 2500 * time.Millisecond}, // 2s ± 25%
		{"attempt 1", 1, 3 * time.Second, 5 * time.Second},                 // 4s ± 25%
		{"attempt 2", 2, 6 * time.Second, 10 * time.Second},                // 8s ± 25%
		{"attempt 3", 3, 12 * time.Second, 20 * time.Second},               // 16s ± 25%
		{"attempt 4", 4, 24 * time.Second, 32 * time.Second},               // 32s (capped)
		{"attempt 5", 5, 24 * time.Second, 32 * time.Second},               // 32s (capped)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to verify jitter works
			for i := 0; i < 10; i++ {
				backoff := llmhttp.ExponentialBackoff(tt.attempt, config)
				assert.GreaterOrEqual(t, backoff, tt.minWait, "backoff too short")
				assert.LessOrEqual(t, backoff, tt.maxWait, "backoff too long")
			}
		})
	}
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "rate limit error should retry",
			err:  llmhttp.NewRateLimitError("openai", "too many requests"),
			want: true,
		},
		{
			name: "service unavailable should retry",
			err:  llmhttp.NewServiceUnavailableError("anthropic", "overloaded"),
			want: true,
		},
		{
			name: "timeout should retry",
			err:  llmhttp.NewTimeoutError("gemini", "timed out"),
			want: true,
		},
		{
			name: "authentication error should not retry",
			err:  llmhttp.NewAuthenticationError("openai", "invalid key"),
			want: false,
		},
		{
			name: "invalid request should not retry",
			err:  llmhttp.NewInvalidRequestError("openai", "bad request"),
			want: false,
		},
		{
			name: "model not found should not retry",
			err:  llmhttp.NewModelNotFoundError("ollama", "model not found"),
			want: false,
		},
		{
			name: "content filtered should not retry",
			err:  llmhttp.NewContentFilteredError("gemini", "blocked"),
			want: false,
		},
		{
			name: "non-HTTP error should not retry",
			err:  errors.New("generic error"),
			want: false,
		},
		{
			name: "nil error should not retry",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llmhttp.ShouldRetry(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRetryWithBackoff_Success(t *testing.T) {
	attempts := 0
	operation := func(ctx context.Context) error {
		attempts++
		return nil // Success on first try
	}

	config := llmhttp.RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}

	err := llmhttp.RetryWithBackoff(context.Background(), operation, config)
	require.NoError(t, err)
	assert.Equal(t, 1, attempts, "should succeed on first attempt")
}

func TestRetryWithBackoff_RetryableError(t *testing.T) {
	attempts := 0
	operation := func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return llmhttp.NewRateLimitError("test", "rate limited")
		}
		return nil // Success on 3rd attempt
	}

	config := llmhttp.RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}

	start := time.Now()
	err := llmhttp.RetryWithBackoff(context.Background(), operation, config)
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, 3, attempts, "should retry twice then succeed")
	// Should have waited at least 2 backoffs (first + second attempt)
	assert.GreaterOrEqual(t, duration, 10*time.Millisecond, "should have backoff delays")
}

func TestRetryWithBackoff_NonRetryableError(t *testing.T) {
	attempts := 0
	operation := func(ctx context.Context) error {
		attempts++
		return llmhttp.NewAuthenticationError("test", "invalid API key")
	}

	config := llmhttp.RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}

	err := llmhttp.RetryWithBackoff(context.Background(), operation, config)
	require.Error(t, err)
	assert.Equal(t, 1, attempts, "should not retry non-retryable error")
	assert.Contains(t, err.Error(), "invalid API key")
}

func TestRetryWithBackoff_MaxRetriesExceeded(t *testing.T) {
	attempts := 0
	operation := func(ctx context.Context) error {
		attempts++
		return llmhttp.NewRateLimitError("test", "rate limited")
	}

	config := llmhttp.RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}

	err := llmhttp.RetryWithBackoff(context.Background(), operation, config)
	require.Error(t, err)
	assert.Equal(t, 4, attempts, "should try once + 3 retries")
	assert.Contains(t, err.Error(), "rate limited")
}

func TestRetryWithBackoff_ContextCanceled(t *testing.T) {
	attempts := 0
	operation := func(ctx context.Context) error {
		attempts++
		return llmhttp.NewRateLimitError("test", "rate limited")
	}

	config := llmhttp.RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     500 * time.Millisecond,
		Multiplier:     2.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()

	err := llmhttp.RetryWithBackoff(ctx, operation, config)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	// Should have attempted once or twice before context canceled
	assert.LessOrEqual(t, attempts, 3, "should respect context cancellation")
}

func TestRetryWithBackoff_ContextCancelledBeforeFirstAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before retry starts

	operation := func(ctx context.Context) error {
		t.Fatal("operation should not be called when context is already cancelled")
		return nil
	}

	config := llmhttp.RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}

	err := llmhttp.RetryWithBackoff(ctx, operation, config)

	require.Error(t, err, "should return error when context cancelled before first attempt")
	assert.ErrorIs(t, err, context.Canceled, "should return context.Canceled error")
}

func TestRetryWithBackoff_ContextCancelledBetweenRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	operation := func(ctx context.Context) error {
		attempts++
		if attempts == 1 {
			cancel() // Cancel after first attempt
			return llmhttp.NewRateLimitError("test", "rate limited")
		}
		t.Fatal("should not retry after context cancelled")
		return nil
	}

	config := llmhttp.RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
	}

	err := llmhttp.RetryWithBackoff(ctx, operation, config)

	require.Error(t, err)
	assert.Equal(t, 1, attempts, "should stop after context cancelled")
	// Should return either the context error or the last operation error
	// Both are acceptable as the context was cancelled after the operation returned
}

func TestRetryWithBackoff_GenericError(t *testing.T) {
	attempts := 0
	operation := func(ctx context.Context) error {
		attempts++
		return errors.New("generic error")
	}

	config := llmhttp.RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
	}

	err := llmhttp.RetryWithBackoff(context.Background(), operation, config)
	require.Error(t, err)
	assert.Equal(t, 1, attempts, "should not retry generic errors")
	assert.Equal(t, "generic error", err.Error())
}
