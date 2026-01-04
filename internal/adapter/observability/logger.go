package observability

import (
	"context"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// ReviewLogger adapts llmhttp.Logger to review.Logger interface.
// This allows the review orchestrator to use the same structured logging
// infrastructure as the LLM HTTP clients.
type ReviewLogger struct {
	logger llmhttp.Logger
}

// NewReviewLogger creates a new review logger adapter.
func NewReviewLogger(logger llmhttp.Logger) review.Logger {
	return &ReviewLogger{logger: logger}
}

// LogWarning logs a warning message with structured fields.
// Delegates to the underlying llmhttp.Logger for consistent structured logging.
func (l *ReviewLogger) LogWarning(ctx context.Context, message string, fields map[string]interface{}) {
	l.logger.LogWarning(ctx, message, fields)
}

// LogInfo logs an informational message with structured fields.
// Delegates to the underlying llmhttp.Logger for consistent structured logging.
func (l *ReviewLogger) LogInfo(ctx context.Context, message string, fields map[string]interface{}) {
	l.logger.LogInfo(ctx, message, fields)
}
