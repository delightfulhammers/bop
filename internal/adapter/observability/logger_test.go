package observability_test

import (
	"bytes"
	"context"
	"log"
	"os"
	"testing"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/delightfulhammers/bop/internal/adapter/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReviewLogger(t *testing.T) {
	llmLogger := llmhttp.NewDefaultLogger(llmhttp.LogLevelInfo, llmhttp.LogFormatHuman, true)
	reviewLogger := observability.NewReviewLogger(llmLogger)

	require.NotNil(t, reviewLogger)
}

func TestReviewLogger_LogWarning(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	llmLogger := llmhttp.NewDefaultLogger(llmhttp.LogLevelInfo, llmhttp.LogFormatHuman, true)
	reviewLogger := observability.NewReviewLogger(llmLogger)

	ctx := context.Background()
	reviewLogger.LogWarning(ctx, "failed to save review", map[string]interface{}{
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

func TestReviewLogger_LogInfo(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	llmLogger := llmhttp.NewDefaultLogger(llmhttp.LogLevelInfo, llmhttp.LogFormatHuman, true)
	reviewLogger := observability.NewReviewLogger(llmLogger)

	ctx := context.Background()
	reviewLogger.LogInfo(ctx, "review completed successfully", map[string]interface{}{
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
