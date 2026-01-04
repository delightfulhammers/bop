package store_test

import (
	"strings"
	"testing"
	"time"

	"github.com/delightfulhammers/bop/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestGenerateRunID(t *testing.T) {
	t.Run("format is correct", func(t *testing.T) {
		ts := time.Date(2025, 10, 21, 14, 30, 45, 0, time.UTC)
		id := store.GenerateRunID(ts, "main", "feature")

		// Should start with "run-"
		assert.True(t, strings.HasPrefix(id, "run-"))

		// Should contain timestamp in ISO format
		assert.Contains(t, id, "20251021T143045Z")

		// Should contain hash (6 characters after final hyphen)
		parts := strings.Split(id, "-")
		assert.Len(t, parts, 3) // run-TIMESTAMP-HASH
		assert.Len(t, parts[2], 6, "hash should be 6 characters")
	})

	t.Run("different times produce unique IDs", func(t *testing.T) {
		ts1 := time.Date(2025, 10, 21, 14, 30, 45, 0, time.UTC)
		ts2 := time.Date(2025, 10, 21, 14, 30, 46, 0, time.UTC)

		id1 := store.GenerateRunID(ts1, "main", "feature")
		id2 := store.GenerateRunID(ts2, "main", "feature")

		assert.NotEqual(t, id1, id2)
	})

	t.Run("different refs produce unique IDs", func(t *testing.T) {
		ts := time.Date(2025, 10, 21, 14, 30, 45, 0, time.UTC)

		id1 := store.GenerateRunID(ts, "main", "feature")
		id2 := store.GenerateRunID(ts, "main", "bugfix")

		assert.NotEqual(t, id1, id2)
	})

	t.Run("IDs are sortable by timestamp", func(t *testing.T) {
		ts1 := time.Date(2025, 10, 21, 14, 30, 45, 0, time.UTC)
		ts2 := time.Date(2025, 10, 21, 15, 30, 45, 0, time.UTC)
		ts3 := time.Date(2025, 10, 22, 14, 30, 45, 0, time.UTC)

		id1 := store.GenerateRunID(ts1, "main", "feature")
		id2 := store.GenerateRunID(ts2, "main", "feature")
		id3 := store.GenerateRunID(ts3, "main", "feature")

		// String comparison should work due to ISO timestamp format
		assert.True(t, id1 < id2)
		assert.True(t, id2 < id3)
	})
}

func TestGenerateFindingHash(t *testing.T) {
	t.Run("same finding produces same hash", func(t *testing.T) {
		hash1 := store.GenerateFindingHash("main.go", 10, 15, "SQL injection vulnerability")
		hash2 := store.GenerateFindingHash("main.go", 10, 15, "SQL injection vulnerability")

		assert.Equal(t, hash1, hash2)
	})

	t.Run("case insensitive description", func(t *testing.T) {
		hash1 := store.GenerateFindingHash("main.go", 10, 15, "SQL Injection Vulnerability")
		hash2 := store.GenerateFindingHash("main.go", 10, 15, "sql injection vulnerability")

		assert.Equal(t, hash1, hash2, "description should be case-insensitive")
	})

	t.Run("whitespace normalization", func(t *testing.T) {
		hash1 := store.GenerateFindingHash("main.go", 10, 15, "SQL injection  vulnerability")
		hash2 := store.GenerateFindingHash("main.go", 10, 15, "  SQL injection vulnerability  ")

		assert.Equal(t, hash1, hash2, "whitespace should be normalized")
	})

	t.Run("different files produce different hashes", func(t *testing.T) {
		hash1 := store.GenerateFindingHash("main.go", 10, 15, "vulnerability")
		hash2 := store.GenerateFindingHash("utils.go", 10, 15, "vulnerability")

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different line ranges produce different hashes", func(t *testing.T) {
		hash1 := store.GenerateFindingHash("main.go", 10, 15, "vulnerability")
		hash2 := store.GenerateFindingHash("main.go", 20, 25, "vulnerability")

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("different descriptions produce different hashes", func(t *testing.T) {
		hash1 := store.GenerateFindingHash("main.go", 10, 15, "SQL injection")
		hash2 := store.GenerateFindingHash("main.go", 10, 15, "XSS vulnerability")

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("hash is hex string", func(t *testing.T) {
		hash := store.GenerateFindingHash("main.go", 10, 15, "test")

		// Should be valid hex
		assert.Regexp(t, "^[0-9a-f]+$", hash)

		// SHA-256 hex is 64 characters
		assert.Len(t, hash, 64)
	})
}

func TestGenerateReviewID(t *testing.T) {
	t.Run("format is correct", func(t *testing.T) {
		id := store.GenerateReviewID("run-123", "openai")

		assert.Equal(t, "review-run-123-openai", id)
	})

	t.Run("unique per run and provider", func(t *testing.T) {
		id1 := store.GenerateReviewID("run-123", "openai")
		id2 := store.GenerateReviewID("run-123", "anthropic")
		id3 := store.GenerateReviewID("run-456", "openai")

		assert.NotEqual(t, id1, id2)
		assert.NotEqual(t, id1, id3)
		assert.NotEqual(t, id2, id3)
	})
}

func TestGenerateFindingID(t *testing.T) {
	t.Run("format is correct", func(t *testing.T) {
		id := store.GenerateFindingID("review-123", 5)

		assert.Equal(t, "finding-review-123-0005", id)
	})

	t.Run("index is zero-padded", func(t *testing.T) {
		id1 := store.GenerateFindingID("review-123", 0)
		id2 := store.GenerateFindingID("review-123", 9)
		id3 := store.GenerateFindingID("review-123", 99)
		id4 := store.GenerateFindingID("review-123", 999)

		assert.Equal(t, "finding-review-123-0000", id1)
		assert.Equal(t, "finding-review-123-0009", id2)
		assert.Equal(t, "finding-review-123-0099", id3)
		assert.Equal(t, "finding-review-123-0999", id4)
	})

	t.Run("IDs are sortable", func(t *testing.T) {
		id1 := store.GenerateFindingID("review-123", 1)
		id2 := store.GenerateFindingID("review-123", 10)
		id3 := store.GenerateFindingID("review-123", 100)

		// String comparison should work due to padding
		assert.True(t, id1 < id2)
		assert.True(t, id2 < id3)
	})
}

func TestCalculateConfigHash(t *testing.T) {
	t.Run("same config produces same hash", func(t *testing.T) {
		config := map[string]interface{}{
			"baseRef":   "main",
			"targetRef": "feature",
			"outputDir": "/tmp/reviews",
		}

		hash1, err := store.CalculateConfigHash(config)
		assert.NoError(t, err)

		hash2, err := store.CalculateConfigHash(config)
		assert.NoError(t, err)

		assert.Equal(t, hash1, hash2, "determinism: same config should produce same hash")
	})

	t.Run("different configs produce different hashes", func(t *testing.T) {
		config1 := map[string]interface{}{
			"baseRef":   "main",
			"targetRef": "feature",
		}

		config2 := map[string]interface{}{
			"baseRef":   "main",
			"targetRef": "bugfix",
		}

		hash1, err := store.CalculateConfigHash(config1)
		assert.NoError(t, err)

		hash2, err := store.CalculateConfigHash(config2)
		assert.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("field order doesn't matter for maps", func(t *testing.T) {
		// Note: Go maps are unordered, but JSON marshaling in Go 1.12+
		// sorts keys, so this should be deterministic
		config1 := map[string]interface{}{
			"a": "value1",
			"b": "value2",
		}

		config2 := map[string]interface{}{
			"b": "value2",
			"a": "value1",
		}

		hash1, err := store.CalculateConfigHash(config1)
		assert.NoError(t, err)

		hash2, err := store.CalculateConfigHash(config2)
		assert.NoError(t, err)

		assert.Equal(t, hash1, hash2, "JSON marshaling should sort keys for determinism")
	})

	t.Run("hash is hex string", func(t *testing.T) {
		config := map[string]interface{}{"test": "value"}

		hash, err := store.CalculateConfigHash(config)
		assert.NoError(t, err)

		// Should be valid hex
		assert.Regexp(t, "^[0-9a-f]+$", hash)

		// SHA-256 hex is 64 characters
		assert.Len(t, hash, 64)
	})

	t.Run("handles complex nested structures", func(t *testing.T) {
		config := map[string]interface{}{
			"providers": map[string]interface{}{
				"openai": map[string]interface{}{
					"enabled": true,
					"model":   "gpt-4o-mini",
				},
			},
			"merge": map[string]interface{}{
				"enabled":  true,
				"strategy": "consensus",
			},
		}

		hash, err := store.CalculateConfigHash(config)
		assert.NoError(t, err)
		assert.NotEmpty(t, hash)
	})
}
