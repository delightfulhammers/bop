package http_test

import (
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/stretchr/testify/assert"
)

func TestNewDefaultPricing(t *testing.T) {
	pricing := http.NewDefaultPricing()
	assert.NotNil(t, pricing)
}

func TestDefaultPricing_OpenAI_GPT4oMini(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// gpt-4o-mini: $0.15 per 1M input tokens, $0.60 per 1M output tokens
	// 100 input tokens = $0.000015
	// 50 output tokens = $0.000030
	// Total = $0.000045
	cost := pricing.GetCost("openai", "gpt-4o-mini", 100, 50)
	assert.InDelta(t, 0.000045, cost, 0.000001)
}

func TestDefaultPricing_OpenAI_GPT4o(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// gpt-4o: $2.50 per 1M input tokens, $10.00 per 1M output tokens
	// 1000 input tokens = $0.0025
	// 500 output tokens = $0.0050
	// Total = $0.0075
	cost := pricing.GetCost("openai", "gpt-4o", 1000, 500)
	assert.InDelta(t, 0.0075, cost, 0.0001)
}

func TestDefaultPricing_OpenAI_GPT54(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// gpt-5.4: $2.50 per 1M input tokens, $15.00 per 1M output tokens
	// 1000 input tokens = $0.0025
	// 500 output tokens = $0.0075
	// Total = $0.0100
	cost := pricing.GetCost("openai", "gpt-5.4", 1000, 500)
	assert.InDelta(t, 0.0100, cost, 0.0001)
}

func TestDefaultPricing_OpenAI_O1(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// o1: $15.00 per 1M input tokens, $60.00 per 1M output tokens
	// 1000 input tokens = $0.015
	// 500 output tokens = $0.030
	// Total = $0.045
	cost := pricing.GetCost("openai", "o1", 1000, 500)
	assert.InDelta(t, 0.045, cost, 0.001)
}

func TestDefaultPricing_Anthropic_Claude46Sonnet(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// claude-sonnet-4-6: $3.00 per 1M input, $15.00 per 1M output
	// 1000 input tokens = $0.003
	// 500 output tokens = $0.0075
	// Total = $0.0105
	cost := pricing.GetCost("anthropic", "claude-sonnet-4-6", 1000, 500)
	assert.InDelta(t, 0.0105, cost, 0.0001)
}

func TestDefaultPricing_Anthropic_Claude46Opus(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// claude-opus-4-6: $5.00 per 1M input, $25.00 per 1M output
	// 1000 input tokens = $0.005
	// 500 output tokens = $0.0125
	// Total = $0.0175
	cost := pricing.GetCost("anthropic", "claude-opus-4-6", 1000, 500)
	assert.InDelta(t, 0.0175, cost, 0.0001)
}

func TestDefaultPricing_Anthropic_Claude35Sonnet(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// claude-3-5-sonnet-20241022: $3.00 per 1M input, $15.00 per 1M output
	// 1000 input tokens = $0.003
	// 500 output tokens = $0.0075
	// Total = $0.0105
	cost := pricing.GetCost("anthropic", "claude-3-5-sonnet-20241022", 1000, 500)
	assert.InDelta(t, 0.0105, cost, 0.0001)
}

func TestDefaultPricing_Anthropic_Claude35Haiku(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// claude-3-5-haiku-20241022: $0.80 per 1M input, $4.00 per 1M output
	// 1000 input tokens = $0.0008
	// 500 output tokens = $0.0020
	// Total = $0.0028
	cost := pricing.GetCost("anthropic", "claude-3-5-haiku-20241022", 1000, 500)
	assert.InDelta(t, 0.0028, cost, 0.0001)
}

func TestDefaultPricing_Gemini_31ProPreview(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// gemini-3.1-pro-preview: $2.00 per 1M input, $12.00 per 1M output
	// 1000 input tokens = $0.002
	// 500 output tokens = $0.006
	// Total = $0.008
	cost := pricing.GetCost("gemini", "gemini-3.1-pro-preview", 1000, 500)
	assert.InDelta(t, 0.008, cost, 0.0001)
}

func TestDefaultPricing_Gemini_31FlashPreview(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// gemini-3.1-flash-preview: $0.25 per 1M input, $1.50 per 1M output
	// 1000 input tokens = $0.00025
	// 500 output tokens = $0.00075
	// Total = $0.00100
	cost := pricing.GetCost("gemini", "gemini-3.1-flash-preview", 1000, 500)
	assert.InDelta(t, 0.00100, cost, 0.00001)
}

func TestDefaultPricing_Gemini_15Pro(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// gemini-1.5-pro: $1.25 per 1M input, $5.00 per 1M output
	// 1000 input tokens = $0.00125
	// 500 output tokens = $0.00250
	// Total = $0.00375
	cost := pricing.GetCost("gemini", "gemini-1.5-pro", 1000, 500)
	assert.InDelta(t, 0.00375, cost, 0.00001)
}

func TestDefaultPricing_Gemini_15Flash(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// gemini-1.5-flash: $0.075 per 1M input, $0.30 per 1M output
	// 1000 input tokens = $0.000075
	// 500 output tokens = $0.000150
	// Total = $0.000225
	cost := pricing.GetCost("gemini", "gemini-1.5-flash", 1000, 500)
	assert.InDelta(t, 0.000225, cost, 0.000001)
}

func TestDefaultPricing_Ollama_Free(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// All Ollama models are free (local)
	cost := pricing.GetCost("ollama", "codellama", 1000, 500)
	assert.Equal(t, 0.0, cost)

	cost = pricing.GetCost("ollama", "qwen2.5-coder", 1000, 500)
	assert.Equal(t, 0.0, cost)

	cost = pricing.GetCost("ollama", "deepseek-coder", 1000, 500)
	assert.Equal(t, 0.0, cost)
}

func TestDefaultPricing_UnknownProvider(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// Unknown provider should return 0
	cost := pricing.GetCost("unknown", "model", 1000, 500)
	assert.Equal(t, 0.0, cost)
}

func TestDefaultPricing_UnknownModel(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// Known provider but unknown model should return 0
	cost := pricing.GetCost("openai", "unknown-model", 1000, 500)
	assert.Equal(t, 0.0, cost)
}

func TestDefaultPricing_ZeroTokens(t *testing.T) {
	pricing := http.NewDefaultPricing()

	cost := pricing.GetCost("openai", "gpt-4o-mini", 0, 0)
	assert.Equal(t, 0.0, cost)
}

func TestDefaultPricing_LargeTokenCounts(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// Test with larger token counts to verify precision
	// gpt-4o-mini: $0.15 per 1M input, $0.60 per 1M output
	// 100,000 input tokens = $0.015
	// 50,000 output tokens = $0.030
	// Total = $0.045
	cost := pricing.GetCost("openai", "gpt-4o-mini", 100000, 50000)
	assert.InDelta(t, 0.045, cost, 0.001)
}

func TestDefaultPricing_InputOnly(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// Test with only input tokens (no output)
	// gpt-4o-mini: $0.15 per 1M input tokens
	// 1000 input tokens = $0.00015
	cost := pricing.GetCost("openai", "gpt-4o-mini", 1000, 0)
	assert.InDelta(t, 0.00015, cost, 0.00001)
}

func TestDefaultPricing_OutputOnly(t *testing.T) {
	pricing := http.NewDefaultPricing()

	// Test with only output tokens (no input)
	// gpt-4o-mini: $0.60 per 1M output tokens
	// 1000 output tokens = $0.00060
	cost := pricing.GetCost("openai", "gpt-4o-mini", 0, 1000)
	assert.InDelta(t, 0.00060, cost, 0.00001)
}

func TestDefaultPricing_AllProviders(t *testing.T) {
	pricing := http.NewDefaultPricing()

	tests := []struct {
		provider string
		model    string
		tokensIn int
		minCost  float64 // Minimum expected cost (should be > 0 except Ollama)
	}{
		{"openai", "gpt-5.4", 1000, 0.001},
		{"openai", "gpt-5.2", 1000, 0.001},
		{"openai", "gpt-4o-mini", 1000, 0.0001},
		{"openai", "gpt-4o", 1000, 0.001},
		{"openai", "o1", 1000, 0.01},
		{"anthropic", "claude-opus-4-6", 1000, 0.001},
		{"anthropic", "claude-sonnet-4-6", 1000, 0.001},
		{"anthropic", "claude-opus-4-5", 1000, 0.001},
		{"anthropic", "claude-sonnet-4-5", 1000, 0.001},
		{"anthropic", "claude-haiku-4-5", 1000, 0.0005},
		{"anthropic", "claude-3-5-sonnet-20241022", 1000, 0.001},
		{"anthropic", "claude-3-5-haiku-20241022", 1000, 0.0005},
		{"gemini", "gemini-3.1-pro-preview", 1000, 0.001},
		{"gemini", "gemini-3.1-flash-preview", 1000, 0.0001},
		{"gemini", "gemini-3-pro-preview", 1000, 0.001},
		{"gemini", "gemini-3-flash-preview", 1000, 0.0001},
		{"gemini", "gemini-2.5-pro", 1000, 0.001},
		{"gemini", "gemini-2.5-flash", 1000, 0.0001},
		{"gemini", "gemini-1.5-pro", 1000, 0.001},
		{"gemini", "gemini-1.5-flash", 1000, 0.00005},
		{"ollama", "codellama", 1000, 0.0}, // Free
	}

	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.model, func(t *testing.T) {
			cost := pricing.GetCost(tt.provider, tt.model, tt.tokensIn, 0)
			assert.GreaterOrEqual(t, cost, tt.minCost)
		})
	}
}
