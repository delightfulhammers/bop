package store_test

import (
	"testing"

	"github.com/delightfulhammers/bop/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestPrecisionPrior_Precision(t *testing.T) {
	tests := []struct {
		name     string
		prior    store.PrecisionPrior
		expected float64
	}{
		{
			name: "uniform prior (α=1, β=1)",
			prior: store.PrecisionPrior{
				Provider: "openai",
				Category: "security",
				Alpha:    1.0,
				Beta:     1.0,
			},
			expected: 0.5,
		},
		{
			name: "perfect precision (no rejections)",
			prior: store.PrecisionPrior{
				Provider: "anthropic",
				Category: "performance",
				Alpha:    10.0,
				Beta:     1.0,
			},
			expected: 10.0 / 11.0, // ~0.909
		},
		{
			name: "low precision (many rejections)",
			prior: store.PrecisionPrior{
				Provider: "gemini",
				Category: "style",
				Alpha:    2.0,
				Beta:     8.0,
			},
			expected: 2.0 / 10.0, // 0.2
		},
		{
			name: "zero counts (edge case)",
			prior: store.PrecisionPrior{
				Provider: "ollama",
				Category: "security",
				Alpha:    0.0,
				Beta:     0.0,
			},
			expected: 0.5, // Should return uniform prior
		},
		{
			name: "high confidence high precision",
			prior: store.PrecisionPrior{
				Provider: "openai",
				Category: "security",
				Alpha:    95.0,
				Beta:     5.0,
			},
			expected: 0.95,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.prior.Precision()
			assert.InDelta(t, tt.expected, actual, 0.001, "precision calculation mismatch")
		})
	}
}
