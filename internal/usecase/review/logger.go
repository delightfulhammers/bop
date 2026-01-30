package review

import "context"

// Logger provides structured logging for the review use case.
// This interface allows the orchestrator to log warnings and info messages
// with structured fields for better observability in production.
type Logger interface {
	// LogWarning logs a warning message with structured fields.
	// Fields typically include error details, IDs, and context.
	LogWarning(ctx context.Context, message string, fields map[string]interface{})

	// LogInfo logs an informational message with structured fields.
	// Fields typically include operation details and metadata.
	LogInfo(ctx context.Context, message string, fields map[string]interface{})

	// LogDebug logs a debug message with structured fields.
	// Only visible when log level is debug or trace.
	// Fields typically include detailed metrics and internal state.
	LogDebug(ctx context.Context, message string, fields map[string]interface{})
}
