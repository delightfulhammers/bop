package http_test

import (
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/stretchr/testify/assert"
)

func TestTruncateForLogging_ShortResponse(t *testing.T) {
	short := "This is a short response"
	result := http.TruncateForLogging(short)
	assert.Equal(t, short, result, "Short responses should not be truncated")
}

func TestTruncateForLogging_ExactlyMaxLength(t *testing.T) {
	// Create string exactly MaxLoggedResponseLength
	exact := strings.Repeat("a", http.MaxLoggedResponseLength)
	result := http.TruncateForLogging(exact)
	assert.Equal(t, exact, result, "Response exactly at max length should not be truncated")
}

func TestTruncateForLogging_LongResponse(t *testing.T) {
	long := strings.Repeat("a", 500)
	result := http.TruncateForLogging(long)

	// Should be truncated
	assert.True(t, len(result) < len(long), "Long response should be truncated")
	assert.True(t, strings.HasSuffix(result, "[truncated, length=500]") ||
		strings.Contains(result, "truncated"),
		"Truncated response should indicate truncation")

	// Should start with original content
	assert.True(t, strings.HasPrefix(result, long[:100]),
		"Truncated response should start with original content")
}

func TestTruncateForLogging_EmptyString(t *testing.T) {
	result := http.TruncateForLogging("")
	assert.Equal(t, "", result, "Empty string should remain empty")
}

func TestSafeLogResponse_UsesTruncation(t *testing.T) {
	long := strings.Repeat("sensitive data ", 50)
	result := http.SafeLogResponse(long)

	// Should be truncated (SafeLogResponse wraps TruncateForLogging)
	assert.True(t, len(result) < len(long),
		"SafeLogResponse should truncate long responses")
}

func TestTruncateForLogging_PreventsSensitiveDataLeakage(t *testing.T) {
	// Simulate a response with potential secrets
	sensitiveResponse := `{
		"apiKey": "sk-proj-1234567890abcdefghijklmnopqrstuvwxyz1234567890abcdefghijklmnopqrstuvwxyz",
		"code": "package main\n\nfunc processPayment(creditCard string) {\n\t// Process payment with card: 4111-1111-1111-1111\n}",
		"findings": [...]
	}` + strings.Repeat("\nMore data...", 100)

	result := http.TruncateForLogging(sensitiveResponse)

	// Verify truncation happened
	assert.True(t, len(result) <= http.MaxLoggedResponseLength+100,
		"Should truncate to safe length")

	// The full credit card and API key should not be in the truncated output
	// (if they appear after the truncation point)
	assert.False(t, strings.Contains(result, "4111-1111-1111-1111") &&
		strings.Contains(result, "sk-proj-1234567890abcdefghijklmnopqrstuvwxyz"),
		"Should not log full secrets if they're beyond truncation point")
}

func TestRedactURLSecrets_GeminiAPIKey(t *testing.T) {
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=AIzaSyXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
	result := http.RedactURLSecrets(url)

	assert.NotContains(t, result, "AIzaSyXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", "API key should be redacted")
	assert.Contains(t, result, "key=[REDACTED]", "Should show that key parameter was redacted")
	assert.Contains(t, result, "generativelanguage.googleapis.com", "Domain should still be visible")
}

func TestRedactURLSecrets_MultipleQueryParams(t *testing.T) {
	url := "https://api.example.com/endpoint?key=secret123&foo=bar&apiKey=secret456"
	result := http.RedactURLSecrets(url)

	assert.NotContains(t, result, "secret123", "key parameter should be redacted")
	assert.NotContains(t, result, "secret456", "apiKey parameter should be redacted")
	assert.Contains(t, result, "foo=bar", "Non-sensitive parameters should remain")
	assert.Contains(t, result, "key=[REDACTED]", "Redacted key should be indicated")
	assert.Contains(t, result, "apiKey=[REDACTED]", "Redacted apiKey should be indicated")
}

func TestRedactURLSecrets_NoSecrets(t *testing.T) {
	url := "https://api.example.com/endpoint?foo=bar&baz=qux"
	result := http.RedactURLSecrets(url)

	assert.Equal(t, url, result, "URLs without secrets should remain unchanged")
}

func TestRedactURLSecrets_NoQueryString(t *testing.T) {
	url := "https://api.example.com/endpoint"
	result := http.RedactURLSecrets(url)

	assert.Equal(t, url, result, "URLs without query strings should remain unchanged")
}

func TestRedactURLSecrets_EmptyString(t *testing.T) {
	result := http.RedactURLSecrets("")
	assert.Equal(t, "", result, "Empty string should remain empty")
}

func TestRedactURLSecrets_InErrorMessage(t *testing.T) {
	errMsg := `Post "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=AIzaSyXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX": context canceled`
	result := http.RedactURLSecrets(errMsg)

	assert.NotContains(t, result, "AIzaSyXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", "API key should be redacted from error message")
	assert.Contains(t, result, "key=[REDACTED]", "Should show that key was redacted")
	assert.Contains(t, result, "context canceled", "Error details should remain")
}
