package http_test

import (
	"testing"
	"time"

	"github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/stretchr/testify/assert"
)

func TestNewDefaultMetrics(t *testing.T) {
	metrics := http.NewDefaultMetrics()
	assert.NotNil(t, metrics)

	// Verify initial state
	stats := metrics.GetStats()
	assert.Equal(t, 0, stats.TotalRequests)
	assert.Equal(t, 0, stats.TotalTokensIn)
	assert.Equal(t, 0, stats.TotalTokensOut)
	assert.Equal(t, 0.0, stats.TotalCost)
	assert.Equal(t, time.Duration(0), stats.TotalDuration)
	assert.Equal(t, 0, stats.ErrorCount)
	assert.NotNil(t, stats.ByProvider)
	assert.Equal(t, 0, len(stats.ByProvider))
}

func TestDefaultMetrics_RecordRequest(t *testing.T) {
	metrics := http.NewDefaultMetrics()

	metrics.RecordRequest("openai", "gpt-4o-mini")
	metrics.RecordRequest("openai", "gpt-4o-mini")
	metrics.RecordRequest("anthropic", "claude-3-5-sonnet-20241022")

	stats := metrics.GetStats()
	assert.Equal(t, 3, stats.TotalRequests)
	assert.Equal(t, 2, stats.ByProvider["openai"].Requests)
	assert.Equal(t, 1, stats.ByProvider["anthropic"].Requests)
}

func TestDefaultMetrics_RecordDuration(t *testing.T) {
	metrics := http.NewDefaultMetrics()

	metrics.RecordDuration("openai", "gpt-4o-mini", 2*time.Second)
	metrics.RecordDuration("openai", "gpt-4o-mini", 3*time.Second)
	metrics.RecordDuration("anthropic", "claude-3-5-sonnet-20241022", 1*time.Second)

	stats := metrics.GetStats()
	assert.Equal(t, 6*time.Second, stats.TotalDuration)
	assert.Equal(t, 5*time.Second, stats.ByProvider["openai"].Duration)
	assert.Equal(t, 1*time.Second, stats.ByProvider["anthropic"].Duration)
}

func TestDefaultMetrics_RecordTokens(t *testing.T) {
	metrics := http.NewDefaultMetrics()

	metrics.RecordTokens("openai", "gpt-4o-mini", 100, 50)
	metrics.RecordTokens("openai", "gpt-4o-mini", 200, 100)
	metrics.RecordTokens("anthropic", "claude-3-5-sonnet-20241022", 300, 150)

	stats := metrics.GetStats()
	assert.Equal(t, 600, stats.TotalTokensIn)
	assert.Equal(t, 300, stats.TotalTokensOut)
	assert.Equal(t, 300, stats.ByProvider["openai"].TokensIn)
	assert.Equal(t, 150, stats.ByProvider["openai"].TokensOut)
	assert.Equal(t, 300, stats.ByProvider["anthropic"].TokensIn)
	assert.Equal(t, 150, stats.ByProvider["anthropic"].TokensOut)
}

func TestDefaultMetrics_RecordCost(t *testing.T) {
	metrics := http.NewDefaultMetrics()

	metrics.RecordCost("openai", "gpt-4o-mini", 0.0015)
	metrics.RecordCost("openai", "gpt-4o-mini", 0.0020)
	metrics.RecordCost("anthropic", "claude-3-5-sonnet-20241022", 0.0035)

	stats := metrics.GetStats()
	assert.InDelta(t, 0.0070, stats.TotalCost, 0.0001)
	assert.InDelta(t, 0.0035, stats.ByProvider["openai"].Cost, 0.0001)
	assert.InDelta(t, 0.0035, stats.ByProvider["anthropic"].Cost, 0.0001)
}

func TestDefaultMetrics_RecordError(t *testing.T) {
	metrics := http.NewDefaultMetrics()

	metrics.RecordError("openai", "gpt-4o-mini", http.ErrTypeRateLimit)
	metrics.RecordError("openai", "gpt-4o-mini", http.ErrTypeTimeout)
	metrics.RecordError("gemini", "gemini-1.5-pro", http.ErrTypeAuthentication)

	stats := metrics.GetStats()
	assert.Equal(t, 3, stats.ErrorCount)
	assert.Equal(t, 2, stats.ByProvider["openai"].Errors)
	assert.Equal(t, 1, stats.ByProvider["gemini"].Errors)
}

func TestDefaultMetrics_MultipleOperations(t *testing.T) {
	metrics := http.NewDefaultMetrics()

	// Simulate a complete API call lifecycle
	metrics.RecordRequest("openai", "gpt-4o-mini")
	metrics.RecordDuration("openai", "gpt-4o-mini", 2*time.Second)
	metrics.RecordTokens("openai", "gpt-4o-mini", 100, 50)
	metrics.RecordCost("openai", "gpt-4o-mini", 0.0015)

	stats := metrics.GetStats()
	assert.Equal(t, 1, stats.TotalRequests)
	assert.Equal(t, 2*time.Second, stats.TotalDuration)
	assert.Equal(t, 100, stats.TotalTokensIn)
	assert.Equal(t, 50, stats.TotalTokensOut)
	assert.InDelta(t, 0.0015, stats.TotalCost, 0.0001)
	assert.Equal(t, 0, stats.ErrorCount)

	// Check provider-specific stats
	openaiStats := stats.ByProvider["openai"]
	assert.Equal(t, 1, openaiStats.Requests)
	assert.Equal(t, 2*time.Second, openaiStats.Duration)
	assert.Equal(t, 100, openaiStats.TokensIn)
	assert.Equal(t, 50, openaiStats.TokensOut)
	assert.InDelta(t, 0.0015, openaiStats.Cost, 0.0001)
	assert.Equal(t, 0, openaiStats.Errors)
}

func TestDefaultMetrics_MultipleProviders(t *testing.T) {
	metrics := http.NewDefaultMetrics()

	// OpenAI calls
	metrics.RecordRequest("openai", "gpt-4o-mini")
	metrics.RecordTokens("openai", "gpt-4o-mini", 100, 50)
	metrics.RecordCost("openai", "gpt-4o-mini", 0.001)

	// Anthropic calls
	metrics.RecordRequest("anthropic", "claude-3-5-sonnet-20241022")
	metrics.RecordTokens("anthropic", "claude-3-5-sonnet-20241022", 200, 100)
	metrics.RecordCost("anthropic", "claude-3-5-sonnet-20241022", 0.002)

	// Ollama calls (free)
	metrics.RecordRequest("ollama", "codellama")
	metrics.RecordTokens("ollama", "codellama", 300, 150)
	metrics.RecordCost("ollama", "codellama", 0.0)

	stats := metrics.GetStats()
	assert.Equal(t, 3, stats.TotalRequests)
	assert.Equal(t, 600, stats.TotalTokensIn)
	assert.Equal(t, 300, stats.TotalTokensOut)
	assert.InDelta(t, 0.003, stats.TotalCost, 0.0001)

	assert.Equal(t, 3, len(stats.ByProvider))
	assert.Equal(t, 1, stats.ByProvider["openai"].Requests)
	assert.Equal(t, 1, stats.ByProvider["anthropic"].Requests)
	assert.Equal(t, 1, stats.ByProvider["ollama"].Requests)
}

func TestDefaultMetrics_GetStats_ThreadSafe(t *testing.T) {
	metrics := http.NewDefaultMetrics()

	// Record some data
	metrics.RecordRequest("openai", "gpt-4o-mini")
	metrics.RecordTokens("openai", "gpt-4o-mini", 100, 50)

	// Get stats multiple times - should not panic or cause race conditions
	stats1 := metrics.GetStats()
	stats2 := metrics.GetStats()

	// Verify both are independent copies
	assert.Equal(t, stats1.TotalRequests, stats2.TotalRequests)

	// Modify one copy shouldn't affect the other
	stats1.TotalRequests = 999
	assert.NotEqual(t, stats1.TotalRequests, stats2.TotalRequests)
}
