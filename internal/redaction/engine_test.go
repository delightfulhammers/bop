package redaction_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/redaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_Redact(t *testing.T) {
	t.Run("redacts API keys", func(t *testing.T) {
		engine := redaction.NewEngine()
		input := `const apiKey = "sk-1234567890abcdefghijklmnopqrstuvwxyz12345678"`

		result, err := engine.Redact(input)
		require.NoError(t, err)

		assert.NotContains(t, result, "sk-1234567890abcdefghijklmnopqrstuvwxyz12345678")
		assert.Contains(t, result, "<REDACTED:")
	})

	t.Run("redacts AWS access keys", func(t *testing.T) {
		engine := redaction.NewEngine()
		input := `AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE`

		result, err := engine.Redact(input)
		require.NoError(t, err)

		assert.NotContains(t, result, "AKIAIOSFODNN7EXAMPLE")
		assert.Contains(t, result, "<REDACTED:")
	})

	t.Run("redacts private keys", func(t *testing.T) {
		engine := redaction.NewEngine()
		input := `-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQC1234567890
-----END RSA PRIVATE KEY-----`

		result, err := engine.Redact(input)
		require.NoError(t, err)

		assert.NotContains(t, result, "MIICXAIBAAKBgQC1234567890")
		assert.Contains(t, result, "<REDACTED:")
	})

	t.Run("redacts GitHub tokens", func(t *testing.T) {
		engine := redaction.NewEngine()
		input := `token = "ghp_1234567890abcdefghijklmnopqrstuvwxyz"`

		result, err := engine.Redact(input)
		require.NoError(t, err)

		assert.NotContains(t, result, "ghp_1234567890abcdefghijklmnopqrstuvwxyz")
		assert.Contains(t, result, "<REDACTED:")
	})

	t.Run("redacts JWT tokens", func(t *testing.T) {
		engine := redaction.NewEngine()
		input := `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U`

		result, err := engine.Redact(input)
		require.NoError(t, err)

		assert.NotContains(t, result, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
		assert.Contains(t, result, "<REDACTED:")
	})

	t.Run("leaves non-secret code unchanged", func(t *testing.T) {
		engine := redaction.NewEngine()
		input := `func main() {
	fmt.Println("Hello, World!")
}`

		result, err := engine.Redact(input)
		require.NoError(t, err)

		assert.Equal(t, input, result, "non-secret code should remain unchanged")
	})

	t.Run("uses stable placeholders for same secret", func(t *testing.T) {
		engine := redaction.NewEngine()
		// Use a realistic test key with sufficient length
		testKey := "sk-test1234567890abcdefghijk"
		input := fmt.Sprintf(`key1 = "%s"
key2 = "%s"`, testKey, testKey)

		result, err := engine.Redact(input)
		require.NoError(t, err)

		// Both occurrences of the same secret should be replaced with the same placeholder
		assert.Contains(t, result, "<REDACTED:")
		assert.NotContains(t, result, testKey, "secret should be redacted")

		// Extract the placeholder from the first occurrence
		firstStart := strings.Index(result, `"`) + 1
		firstEnd := strings.Index(result[firstStart:], `"`) + firstStart
		firstPlaceholder := result[firstStart:firstEnd]

		// Extract the placeholder from the second occurrence
		secondKeyStart := strings.Index(result, "key2")
		secondStart := strings.Index(result[secondKeyStart:], `"`) + secondKeyStart + 1
		secondEnd := strings.Index(result[secondStart:], `"`) + secondStart
		secondPlaceholder := result[secondStart:secondEnd]

		assert.Equal(t, firstPlaceholder, secondPlaceholder, "same secret should use same placeholder")
	})

	t.Run("handles empty input", func(t *testing.T) {
		engine := redaction.NewEngine()
		result, err := engine.Redact("")
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})
}

func TestEngine_IsRedacted(t *testing.T) {
	t.Run("detects redacted content", func(t *testing.T) {
		engine := redaction.NewEngine()
		input := `const apiKey = "sk-test1234567890abcdefghijk"`

		redacted, err := engine.Redact(input)
		require.NoError(t, err)

		assert.True(t, engine.IsRedacted(redacted), "should detect redacted content")
	})

	t.Run("returns false for non-redacted content", func(t *testing.T) {
		engine := redaction.NewEngine()
		input := `const message = "Hello, World!"`

		assert.False(t, engine.IsRedacted(input), "should not detect redaction in clean content")
	})
}
