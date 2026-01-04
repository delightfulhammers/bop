package determinism_test

import (
	"math"
	"testing"

	"github.com/delightfulhammers/bop/internal/determinism"
	"github.com/stretchr/testify/assert"
)

func TestGenerateSeed(t *testing.T) {
	t.Run("generates consistent seed for same inputs", func(t *testing.T) {
		baseRef := "main"
		targetRef := "feature-branch"

		seed1 := determinism.GenerateSeed(baseRef, targetRef)
		seed2 := determinism.GenerateSeed(baseRef, targetRef)

		assert.Equal(t, seed1, seed2, "seed should be deterministic for same inputs")
	})

	t.Run("generates different seeds for different inputs", func(t *testing.T) {
		seed1 := determinism.GenerateSeed("main", "feature-1")
		seed2 := determinism.GenerateSeed("main", "feature-2")

		assert.NotEqual(t, seed1, seed2, "different inputs should produce different seeds")
	})

	t.Run("generates different seeds when refs are swapped", func(t *testing.T) {
		seed1 := determinism.GenerateSeed("main", "develop")
		seed2 := determinism.GenerateSeed("develop", "main")

		assert.NotEqual(t, seed1, seed2, "swapped refs should produce different seeds")
	})

	t.Run("handles empty strings", func(t *testing.T) {
		seed1 := determinism.GenerateSeed("", "")
		seed2 := determinism.GenerateSeed("", "")

		assert.Equal(t, seed1, seed2, "empty strings should still produce deterministic seed")
	})

	t.Run("generates non-zero seed", func(t *testing.T) {
		seed := determinism.GenerateSeed("main", "feature")

		assert.NotEqual(t, uint64(0), seed, "seed should not be zero")
	})

	t.Run("seed fits in int64 range for LLM API compatibility", func(t *testing.T) {
		// Test multiple inputs to ensure masking works consistently
		testCases := []struct {
			base   string
			target string
		}{
			{"main", "feature"},
			{"develop", "hotfix"},
			{"release-1.0", "release-2.0"},
			{"", ""},
			{"very-long-branch-name-that-might-produce-large-hash", "another-very-long-branch-name"},
		}

		for _, tc := range testCases {
			seed := determinism.GenerateSeed(tc.base, tc.target)

			// Verify seed is within int64 range [0, math.MaxInt64]
			assert.LessOrEqual(t, seed, uint64(math.MaxInt64),
				"seed must fit in int64 for OpenAI and other LLM APIs (base=%s, target=%s)", tc.base, tc.target)
		}
	})
}
