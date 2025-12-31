package http

// Pricing calculates API costs based on token usage.
type Pricing interface {
	// GetCost calculates cost for a given model and token usage
	GetCost(provider, model string, tokensIn, tokensOut int) float64
}

// ModelPricing contains pricing information for a model.
type ModelPricing struct {
	InputPer1M  float64 // Cost per 1M input tokens in USD
	OutputPer1M float64 // Cost per 1M output tokens in USD
}

// DefaultPricing provides cost calculation based on provider pricing.
type DefaultPricing struct {
	prices map[string]map[string]ModelPricing
}

// NewDefaultPricing creates a pricing calculator with current rates.
func NewDefaultPricing() *DefaultPricing {
	return &DefaultPricing{
		prices: buildPricingTable(),
	}
}

// GetCost calculates the cost for a given request.
func (p *DefaultPricing) GetCost(provider, model string, tokensIn, tokensOut int) float64 {
	providerPrices, ok := p.prices[provider]
	if !ok {
		return 0.0
	}

	modelPrice, ok := providerPrices[model]
	if !ok {
		return 0.0
	}

	inputCost := float64(tokensIn) / 1_000_000.0 * modelPrice.InputPer1M
	outputCost := float64(tokensOut) / 1_000_000.0 * modelPrice.OutputPer1M

	return inputCost + outputCost
}

// buildPricingTable returns pricing data for all models.
// Pricing as of: 2025-12-27
// Sources:
// - OpenAI: https://openai.com/api/pricing/
// - Anthropic: https://claude.com/pricing
// - Gemini: https://ai.google.dev/gemini-api/docs/pricing
// - Ollama: Free (local)
func buildPricingTable() map[string]map[string]ModelPricing {
	return map[string]map[string]ModelPricing{
		"openai": {
			// GPT-5.2 family (December 2025)
			// Short aliases (commonly used in config)
			"gpt-5.2": {
				InputPer1M:  1.75,
				OutputPer1M: 14.00,
			},
			"gpt-5.2-pro": {
				InputPer1M:  21.00,
				OutputPer1M: 168.00,
			},
			// Full versioned names (returned by API)
			"gpt-5.2-2025-12-11": {
				InputPer1M:  1.75,
				OutputPer1M: 14.00,
			},
			"gpt-5.2-pro-2025-12-11": {
				InputPer1M:  21.00,
				OutputPer1M: 168.00,
			},
			"gpt-5.2-codex": {
				InputPer1M:  1.75,
				OutputPer1M: 14.00,
			},
			// GPT-4o family (still available)
			"gpt-4o": {
				InputPer1M:  2.50,
				OutputPer1M: 10.00,
			},
			"gpt-4o-mini": {
				InputPer1M:  0.15,
				OutputPer1M: 0.60,
			},
			// o-series reasoning models
			"o1": {
				InputPer1M:  15.00,
				OutputPer1M: 60.00,
			},
			"o1-mini": {
				InputPer1M:  3.00,
				OutputPer1M: 12.00,
			},
			"o3-mini": {
				InputPer1M:  1.10,
				OutputPer1M: 4.40,
			},
			"o4-mini": {
				InputPer1M:  1.10,
				OutputPer1M: 4.40,
			},
		},
		"anthropic": {
			// Claude 4.5 family (2025)
			// Short aliases (commonly used in config)
			"claude-opus-4-5": {
				InputPer1M:  5.00,
				OutputPer1M: 25.00,
			},
			"claude-sonnet-4-5": {
				InputPer1M:  3.00,
				OutputPer1M: 15.00,
			},
			"claude-haiku-4-5": {
				InputPer1M:  1.00,
				OutputPer1M: 5.00,
			},
			// Full versioned names (returned by API)
			"claude-opus-4-5-20251101": {
				InputPer1M:  5.00,
				OutputPer1M: 25.00,
			},
			"claude-sonnet-4-5-20250929": {
				InputPer1M:  3.00,
				OutputPer1M: 15.00,
			},
			// Legacy Claude 3.5 family (still available)
			"claude-3-5-sonnet-20241022": {
				InputPer1M:  3.00,
				OutputPer1M: 15.00,
			},
			"claude-3-5-haiku-20241022": {
				InputPer1M:  0.80,
				OutputPer1M: 4.00,
			},
		},
		"gemini": {
			// Gemini 3 family (December 2025)
			"gemini-3-pro-preview": {
				InputPer1M:  2.00,
				OutputPer1M: 12.00,
			},
			"gemini-3-flash-preview": {
				InputPer1M:  0.50,
				OutputPer1M: 3.00,
			},
			// Gemini 2.5 family
			"gemini-2.5-pro": {
				InputPer1M:  1.25,
				OutputPer1M: 10.00,
			},
			"gemini-2.5-flash": {
				InputPer1M:  0.15,
				OutputPer1M: 0.60,
			},
			// Legacy Gemini 1.5 family
			"gemini-1.5-pro": {
				InputPer1M:  1.25,
				OutputPer1M: 5.00,
			},
			"gemini-1.5-flash": {
				InputPer1M:  0.075,
				OutputPer1M: 0.30,
			},
		},
		"ollama": {
			// All Ollama models are free (local execution)
			// We use a wildcard approach - any model returns $0
		},
	}
}
