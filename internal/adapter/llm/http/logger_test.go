package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultLogger(t *testing.T) {
	logger := http.NewDefaultLogger(http.LogLevelInfo, http.LogFormatHuman, true)
	assert.NotNil(t, logger)
}

func TestDefaultLogger_RedactAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "full key",
			key:      "sk-1234567890abcdef",
			expected: "[REDACTED-cdef]",
		},
		{
			name:     "anthropic key",
			key:      "sk-ant-1234567890abcdef",
			expected: "[REDACTED-cdef]",
		},
		{
			name:     "short key",
			key:      "abc",
			expected: "[REDACTED]",
		},
		{
			name:     "empty key",
			key:      "",
			expected: "[REDACTED]",
		},
		{
			name:     "4 char key",
			key:      "abcd",
			expected: "[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := http.NewDefaultLogger(http.LogLevelDebug, http.LogFormatHuman, true)
			result := logger.RedactAPIKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultLogger_LogRequest_DebugLevel(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelDebug, http.LogFormatHuman, true)
	logger.LogRequest(context.Background(), http.RequestLog{
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		Timestamp:   time.Now(),
		PromptChars: 1000,
		APIKey:      "sk-1234567890abcdef",
	})

	output := buf.String()
	assert.Contains(t, output, "[DEBUG]")
	assert.Contains(t, output, "openai")
	assert.Contains(t, output, "gpt-4o-mini")
	assert.Contains(t, output, "1000")
	assert.Contains(t, output, "[REDACTED-cdef]")
	assert.NotContains(t, output, "sk-1234567890abcdef")
}

func TestDefaultLogger_LogRequest_InfoLevel_Skipped(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelInfo, http.LogFormatHuman, true)
	logger.LogRequest(context.Background(), http.RequestLog{
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		Timestamp:   time.Now(),
		PromptChars: 1000,
		APIKey:      "sk-1234567890abcdef",
	})

	output := buf.String()
	assert.Empty(t, output, "Should not log at Info level")
}

func TestDefaultLogger_LogRequest_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelDebug, http.LogFormatJSON, true)
	now := time.Now()
	logger.LogRequest(context.Background(), http.RequestLog{
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		Timestamp:   now,
		PromptChars: 1000,
		APIKey:      "sk-1234567890abcdef",
	})

	output := buf.String()

	// Extract JSON from log output (skip log prefix)
	jsonStart := strings.Index(output, "{")
	require.NotEqual(t, -1, jsonStart, "Should contain JSON")

	var logData map[string]interface{}
	err := json.Unmarshal([]byte(output[jsonStart:]), &logData)
	require.NoError(t, err)

	assert.Equal(t, "debug", logData["level"])
	assert.Equal(t, "request", logData["type"])
	assert.Equal(t, "openai", logData["provider"])
	assert.Equal(t, "gpt-4o-mini", logData["model"])
	assert.Equal(t, float64(1000), logData["prompt_chars"])
	assert.Equal(t, "[REDACTED-cdef]", logData["api_key"])
}

func TestDefaultLogger_LogResponse(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelInfo, http.LogFormatHuman, true)
	logger.LogResponse(context.Background(), http.ResponseLog{
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		Timestamp:    time.Now(),
		Duration:     2500 * time.Millisecond,
		TokensIn:     100,
		TokensOut:    50,
		Cost:         0.0015,
		StatusCode:   200,
		FinishReason: "stop",
	})

	output := buf.String()
	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "openai")
	assert.Contains(t, output, "gpt-4o-mini")
	assert.Contains(t, output, "2.5")
	assert.Contains(t, output, "100")
	assert.Contains(t, output, "50")
	assert.Contains(t, output, "0.0015")
}

func TestDefaultLogger_LogResponse_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelInfo, http.LogFormatJSON, true)
	logger.LogResponse(context.Background(), http.ResponseLog{
		Provider:     "anthropic",
		Model:        "claude-3-5-sonnet-20241022",
		Timestamp:    time.Now(),
		Duration:     3200 * time.Millisecond,
		TokensIn:     200,
		TokensOut:    150,
		Cost:         0.0028,
		StatusCode:   200,
		FinishReason: "end_turn",
	})

	output := buf.String()
	jsonStart := strings.Index(output, "{")
	require.NotEqual(t, -1, jsonStart)

	var logData map[string]interface{}
	err := json.Unmarshal([]byte(output[jsonStart:]), &logData)
	require.NoError(t, err)

	assert.Equal(t, "info", logData["level"])
	assert.Equal(t, "response", logData["type"])
	assert.Equal(t, "anthropic", logData["provider"])
	assert.Equal(t, float64(200), logData["tokens_in"])
	assert.Equal(t, float64(150), logData["tokens_out"])
	assert.Equal(t, 0.0028, logData["cost"])
}

func TestDefaultLogger_LogError(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelError, http.LogFormatHuman, true)

	err := &http.Error{
		Type:       http.ErrTypeRateLimit,
		Message:    "Rate limit exceeded",
		StatusCode: 429,
		Retryable:  true,
		Provider:   "openai",
	}

	logger.LogError(context.Background(), http.ErrorLog{
		Provider:   "openai",
		Model:      "gpt-4o-mini",
		Timestamp:  time.Now(),
		Duration:   1500 * time.Millisecond,
		Error:      err,
		ErrorType:  http.ErrTypeRateLimit,
		StatusCode: 429,
		Retryable:  true,
	})

	output := buf.String()
	assert.Contains(t, output, "[ERROR]")
	assert.Contains(t, output, "openai")
	assert.Contains(t, output, "gpt-4o-mini")
	assert.Contains(t, output, "429")
	assert.Contains(t, output, "retryable")
}

func TestDefaultLogger_LogError_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelError, http.LogFormatJSON, true)

	err := &http.Error{
		Type:       http.ErrTypeAuthentication,
		Message:    "Invalid API key",
		StatusCode: 401,
		Retryable:  false,
		Provider:   "gemini",
	}

	logger.LogError(context.Background(), http.ErrorLog{
		Provider:   "gemini",
		Model:      "gemini-1.5-pro",
		Timestamp:  time.Now(),
		Duration:   500 * time.Millisecond,
		Error:      err,
		ErrorType:  http.ErrTypeAuthentication,
		StatusCode: 401,
		Retryable:  false,
	})

	output := buf.String()
	jsonStart := strings.Index(output, "{")
	require.NotEqual(t, -1, jsonStart)

	var logData map[string]interface{}
	err2 := json.Unmarshal([]byte(output[jsonStart:]), &logData)
	require.NoError(t, err2)

	assert.Equal(t, "error", logData["level"])
	assert.Equal(t, "error", logData["type"])
	assert.Equal(t, "gemini", logData["provider"])
	assert.Equal(t, float64(401), logData["status_code"])
	assert.Equal(t, false, logData["retryable"])
}

func TestDefaultLogger_NoRedaction_WhenDisabled(t *testing.T) {
	logger := http.NewDefaultLogger(http.LogLevelDebug, http.LogFormatHuman, true)
	logger.SetRedaction(false)

	result := logger.RedactAPIKey("sk-1234567890abcdef")
	assert.Equal(t, "sk-1234567890abcdef", result, "Should not redact when disabled")
}

func TestDefaultLogger_LogWarning_JSON(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelInfo, http.LogFormatJSON, true)
	logger.LogWarning(context.Background(), "failed to save review", map[string]interface{}{
		"runID":    "run-123",
		"provider": "openai",
		"error":    "database connection failed",
	})

	output := buf.String()
	jsonStart := strings.Index(output, "{")
	require.NotEqual(t, -1, jsonStart, "Should contain JSON")

	var logData map[string]interface{}
	err := json.Unmarshal([]byte(output[jsonStart:]), &logData)
	require.NoError(t, err)

	assert.Equal(t, "warning", logData["level"])
	assert.Equal(t, "failed to save review", logData["message"])
	assert.Equal(t, "run-123", logData["runID"])
	assert.Equal(t, "openai", logData["provider"])
	assert.Equal(t, "database connection failed", logData["error"])
	assert.Contains(t, logData, "timestamp")
}

func TestDefaultLogger_LogInfo_JSON(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelInfo, http.LogFormatJSON, true)
	logger.LogInfo(context.Background(), "review completed successfully", map[string]interface{}{
		"runID":     "run-456",
		"provider":  "anthropic",
		"totalCost": 0.05,
	})

	output := buf.String()
	jsonStart := strings.Index(output, "{")
	require.NotEqual(t, -1, jsonStart, "Should contain JSON")

	var logData map[string]interface{}
	err := json.Unmarshal([]byte(output[jsonStart:]), &logData)
	require.NoError(t, err)

	assert.Equal(t, "info", logData["level"])
	assert.Equal(t, "review completed successfully", logData["message"])
	assert.Equal(t, "run-456", logData["runID"])
	assert.Equal(t, "anthropic", logData["provider"])
	assert.Equal(t, 0.05, logData["totalCost"])
	assert.Contains(t, logData, "timestamp")
}

func TestDefaultLogger_LogWarning_RespectLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		logLevel  http.LogLevel
		shouldLog bool
	}{
		{"Debug level logs warnings", http.LogLevelDebug, true},
		{"Info level logs warnings", http.LogLevelInfo, true},
		{"Error level skips warnings", http.LogLevelError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(os.Stderr)

			logger := http.NewDefaultLogger(tt.logLevel, http.LogFormatHuman, true)
			logger.LogWarning(context.Background(), "test warning", map[string]interface{}{"key": "value"})

			output := buf.String()
			if tt.shouldLog {
				assert.Contains(t, output, "test warning")
			} else {
				assert.Empty(t, output, "Should not log warnings at Error level")
			}
		})
	}
}

func TestDefaultLogger_LogInfo_RespectLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		logLevel  http.LogLevel
		shouldLog bool
	}{
		{"Debug level logs info", http.LogLevelDebug, true},
		{"Info level logs info", http.LogLevelInfo, true},
		{"Error level skips info", http.LogLevelError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf)
			defer log.SetOutput(os.Stderr)

			logger := http.NewDefaultLogger(tt.logLevel, http.LogFormatHuman, true)
			logger.LogInfo(context.Background(), "test info", map[string]interface{}{"key": "value"})

			output := buf.String()
			if tt.shouldLog {
				assert.Contains(t, output, "test info")
			} else {
				assert.Empty(t, output, "Should not log info at Error level")
			}
		})
	}
}

func TestDefaultLogger_LogWarning_Human(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelInfo, http.LogFormatHuman, true)
	logger.LogWarning(context.Background(), "failed to save review", map[string]interface{}{
		"runID":    "run-123",
		"provider": "openai",
		"error":    "database connection failed",
	})

	output := buf.String()
	assert.Contains(t, output, "[WARN]")
	assert.Contains(t, output, "failed to save review")
	assert.Contains(t, output, "runID=run-123")
	assert.Contains(t, output, "provider=openai")
	assert.Contains(t, output, "error=database connection failed")
}

func TestDefaultLogger_LogInfo_Human(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelInfo, http.LogFormatHuman, true)
	logger.LogInfo(context.Background(), "review completed successfully", map[string]interface{}{
		"runID":     "run-456",
		"provider":  "anthropic",
		"totalCost": 0.05,
	})

	output := buf.String()
	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "review completed successfully")
	assert.Contains(t, output, "runID=run-456")
	assert.Contains(t, output, "provider=anthropic")
	assert.Contains(t, output, "totalCost=0.05")
}

func TestDefaultLogger_LogWarning_Human_EmptyFields(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelInfo, http.LogFormatHuman, true)
	logger.LogWarning(context.Background(), "simple warning", map[string]interface{}{})

	output := buf.String()
	assert.Contains(t, output, "[WARN]")
	assert.Contains(t, output, "simple warning")
	// Should not have extra key=value pairs
	assert.NotContains(t, output, "=")
}

func TestDefaultLogger_LogInfo_Human_MultipleFields(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logger := http.NewDefaultLogger(http.LogLevelInfo, http.LogFormatHuman, true)
	logger.LogInfo(context.Background(), "operation completed", map[string]interface{}{
		"duration": "2.5s",
		"items":    42,
		"status":   "success",
	})

	output := buf.String()
	assert.Contains(t, output, "[INFO]")
	assert.Contains(t, output, "operation completed")
	// Should contain all fields (order may vary due to map iteration)
	assert.Contains(t, output, "duration=2.5s")
	assert.Contains(t, output, "items=42")
	assert.Contains(t, output, "status=success")
}
