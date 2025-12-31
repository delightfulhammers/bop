package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// Logger provides structured logging for LLM API calls and generic application logging.
type Logger interface {
	// LogRequest logs an outgoing API request (API key redacted)
	LogRequest(ctx context.Context, req RequestLog)

	// LogResponse logs an API response with timing and token info
	LogResponse(ctx context.Context, resp ResponseLog)

	// LogError logs an API error
	LogError(ctx context.Context, err ErrorLog)

	// LogWarning logs a warning message with structured fields
	LogWarning(ctx context.Context, message string, fields map[string]interface{})

	// LogInfo logs an informational message with structured fields
	LogInfo(ctx context.Context, message string, fields map[string]interface{})
}

// RequestLog contains request information for logging.
type RequestLog struct {
	Provider      string
	Model         string
	Timestamp     time.Time
	PromptChars   int    // Character count of prompt
	APIKey        string // Will be redacted to last 4 chars
	PromptContent string // Full prompt content (for trace level logging)
}

// ResponseLog contains response information for logging.
type ResponseLog struct {
	Provider        string
	Model           string
	Timestamp       time.Time
	Duration        time.Duration
	TokensIn        int
	TokensOut       int
	Cost            float64
	StatusCode      int
	FinishReason    string
	ResponseContent string // Full response content (for trace level logging)
}

// ErrorLog contains error information for logging.
type ErrorLog struct {
	Provider   string
	Model      string
	Timestamp  time.Time
	Duration   time.Duration
	Error      error
	ErrorType  ErrorType
	StatusCode int
	Retryable  bool
}

// LogLevel defines the logging verbosity level.
type LogLevel int

const (
	LogLevelTrace LogLevel = iota // Full prompts/responses (with redaction)
	LogLevelDebug                 // Metadata + truncated content
	LogLevelInfo                  // Response summaries only
	LogLevelError                 // Errors only
)

// LogFormat defines the output format for logs.
type LogFormat int

const (
	LogFormatHuman LogFormat = iota
	LogFormatJSON
)

// DefaultLogger writes logs in structured format to stdout.
type DefaultLogger struct {
	level           LogLevel
	redactKeys      bool
	format          LogFormat
	maxContentBytes int // Max content size for trace logging (0 = unlimited)
}

// NewDefaultLogger creates a logger with the specified config.
func NewDefaultLogger(level LogLevel, format LogFormat, redactKeys bool) *DefaultLogger {
	return &DefaultLogger{
		level:           level,
		redactKeys:      redactKeys,
		format:          format,
		maxContentBytes: 51200, // Default 50KB
	}
}

// WithMaxContentBytes sets the maximum content size for trace logging.
// Content exceeding this limit will be truncated with a marker.
func (l *DefaultLogger) WithMaxContentBytes(maxBytes int) *DefaultLogger {
	l.maxContentBytes = maxBytes
	return l
}

// truncateContent limits content size for logging to prevent log explosion.
func (l *DefaultLogger) truncateContent(content string) string {
	if l.maxContentBytes <= 0 || len(content) <= l.maxContentBytes {
		return content
	}
	return content[:l.maxContentBytes] + "\n... [TRUNCATED - exceeded " + fmt.Sprintf("%d", l.maxContentBytes) + " bytes]"
}

// SetRedaction enables or disables API key redaction.
func (l *DefaultLogger) SetRedaction(enabled bool) {
	l.redactKeys = enabled
}

// LogRequest logs an API request.
func (l *DefaultLogger) LogRequest(ctx context.Context, req RequestLog) {
	if l.level > LogLevelDebug {
		return
	}

	// Redact API key to last 4 characters
	redacted := l.RedactAPIKey(req.APIKey)

	if l.format == LogFormatJSON {
		// JSON format for machine parsing
		if l.level == LogLevelTrace && req.PromptContent != "" {
			// Trace level: include prompt content (truncated if needed)
			truncatedPrompt := l.truncateContent(req.PromptContent)
			log.Printf(`{"level":"trace","type":"request","provider":"%s","model":"%s","timestamp":"%s","prompt_chars":%d,"api_key":"%s","prompt_content":%s}`,
				req.Provider, req.Model, req.Timestamp.Format(time.RFC3339),
				req.PromptChars, redacted, jsonEscapeString(truncatedPrompt))
		} else {
			log.Printf(`{"level":"debug","type":"request","provider":"%s","model":"%s","timestamp":"%s","prompt_chars":%d,"api_key":"%s"}`,
				req.Provider, req.Model, req.Timestamp.Format(time.RFC3339),
				req.PromptChars, redacted)
		}
	} else {
		// Human-readable format
		log.Printf("[DEBUG] %s/%s: Request sent (prompt=%d chars, key=%s)",
			req.Provider, req.Model, req.PromptChars, redacted)

		// Trace level: print prompt content (truncated if needed)
		if l.level == LogLevelTrace && req.PromptContent != "" {
			truncatedPrompt := l.truncateContent(req.PromptContent)
			log.Printf("[TRACE] %s/%s: Prompt content:\n%s",
				req.Provider, req.Model, truncatedPrompt)
		}
	}
}

// LogResponse logs an API response.
func (l *DefaultLogger) LogResponse(ctx context.Context, resp ResponseLog) {
	if l.level > LogLevelInfo {
		return
	}

	if l.format == LogFormatJSON {
		// JSON format for machine parsing
		if l.level == LogLevelTrace && resp.ResponseContent != "" {
			// Trace level: include response content (truncated if needed)
			truncatedResponse := l.truncateContent(resp.ResponseContent)
			log.Printf(`{"level":"trace","type":"response","provider":"%s","model":"%s","timestamp":"%s","duration_ms":%d,"tokens_in":%d,"tokens_out":%d,"cost":%.6f,"status_code":%d,"finish_reason":"%s","response_content":%s}`,
				resp.Provider, resp.Model, resp.Timestamp.Format(time.RFC3339),
				resp.Duration.Milliseconds(), resp.TokensIn, resp.TokensOut,
				resp.Cost, resp.StatusCode, resp.FinishReason, jsonEscapeString(truncatedResponse))
		} else {
			log.Printf(`{"level":"info","type":"response","provider":"%s","model":"%s","timestamp":"%s","duration_ms":%d,"tokens_in":%d,"tokens_out":%d,"cost":%.6f,"status_code":%d,"finish_reason":"%s"}`,
				resp.Provider, resp.Model, resp.Timestamp.Format(time.RFC3339),
				resp.Duration.Milliseconds(), resp.TokensIn, resp.TokensOut,
				resp.Cost, resp.StatusCode, resp.FinishReason)
		}
	} else {
		// Human-readable format
		log.Printf("[INFO] %s/%s: Response received (duration=%.1fs, tokens=%d/%d, cost=$%.4f)",
			resp.Provider, resp.Model, resp.Duration.Seconds(),
			resp.TokensIn, resp.TokensOut, resp.Cost)

		// Trace level: print response content (truncated if needed)
		if l.level == LogLevelTrace && resp.ResponseContent != "" {
			truncatedResponse := l.truncateContent(resp.ResponseContent)
			log.Printf("[TRACE] %s/%s: Response content:\n%s",
				resp.Provider, resp.Model, truncatedResponse)
		}
	}
}

// jsonEscapeString properly escapes a string for JSON output.
func jsonEscapeString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// LogError logs an API error.
func (l *DefaultLogger) LogError(ctx context.Context, err ErrorLog) {
	if l.level > LogLevelError {
		return
	}

	// Redact API keys from URLs in error messages
	errorMsg := RedactURLSecrets(err.Error.Error())

	retryableStr := "non-retryable"
	if err.Retryable {
		retryableStr = "retryable"
	}

	if l.format == LogFormatJSON {
		// JSON format for machine parsing
		log.Printf(`{"level":"error","type":"error","provider":"%s","model":"%s","timestamp":"%s","duration_ms":%d,"error":"%s","error_type":%d,"status_code":%d,"retryable":%t}`,
			err.Provider, err.Model, err.Timestamp.Format(time.RFC3339),
			err.Duration.Milliseconds(), errorMsg, err.ErrorType,
			err.StatusCode, err.Retryable)
	} else {
		// Human-readable format
		log.Printf("[ERROR] %s/%s: API call failed (status=%d, %s): %s",
			err.Provider, err.Model, err.StatusCode, retryableStr, errorMsg)
	}
}

// LogWarning logs a warning message with structured fields.
func (l *DefaultLogger) LogWarning(ctx context.Context, message string, fields map[string]interface{}) {
	if l.level > LogLevelInfo {
		return // Skip warnings if log level is Error
	}

	timestamp := time.Now().UTC()

	if l.format == LogFormatJSON {
		l.logWarningJSON(timestamp, message, fields)
	} else {
		l.logWarningHuman(timestamp, message, fields)
	}
}

// LogInfo logs an informational message with structured fields.
func (l *DefaultLogger) LogInfo(ctx context.Context, message string, fields map[string]interface{}) {
	if l.level > LogLevelInfo {
		return // Skip info if log level is Error
	}

	timestamp := time.Now().UTC()

	if l.format == LogFormatJSON {
		l.logInfoJSON(timestamp, message, fields)
	} else {
		l.logInfoHuman(timestamp, message, fields)
	}
}

// logWarningJSON logs a warning in JSON format
func (l *DefaultLogger) logWarningJSON(timestamp time.Time, message string, fields map[string]interface{}) {
	logEntry := map[string]interface{}{
		"level":     "warning",
		"timestamp": timestamp.Format(time.RFC3339),
		"message":   message,
	}

	// Merge custom fields
	for k, v := range fields {
		logEntry[k] = v
	}

	// Marshal and log
	if jsonBytes, err := json.Marshal(logEntry); err == nil {
		log.Println(string(jsonBytes))
	}
}

// logInfoJSON logs an info message in JSON format
func (l *DefaultLogger) logInfoJSON(timestamp time.Time, message string, fields map[string]interface{}) {
	logEntry := map[string]interface{}{
		"level":     "info",
		"timestamp": timestamp.Format(time.RFC3339),
		"message":   message,
	}

	// Merge custom fields
	for k, v := range fields {
		logEntry[k] = v
	}

	// Marshal and log
	if jsonBytes, err := json.Marshal(logEntry); err == nil {
		log.Println(string(jsonBytes))
	}
}

// logWarningHuman logs a warning in human-readable format
func (l *DefaultLogger) logWarningHuman(timestamp time.Time, message string, fields map[string]interface{}) {
	// Build key=value pairs from fields
	var pairs []string
	for k, v := range fields {
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
	}

	if len(pairs) > 0 {
		log.Printf("[WARN] %s %s %s", timestamp.Format(time.RFC3339), message, strings.Join(pairs, " "))
	} else {
		log.Printf("[WARN] %s %s", timestamp.Format(time.RFC3339), message)
	}
}

// logInfoHuman logs an info message in human-readable format
func (l *DefaultLogger) logInfoHuman(timestamp time.Time, message string, fields map[string]interface{}) {
	// Build key=value pairs from fields
	var pairs []string
	for k, v := range fields {
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
	}

	if len(pairs) > 0 {
		log.Printf("[INFO] %s %s %s", timestamp.Format(time.RFC3339), message, strings.Join(pairs, " "))
	} else {
		log.Printf("[INFO] %s %s", timestamp.Format(time.RFC3339), message)
	}
}

// RedactAPIKey shows only the last 4 characters of an API key with explicit redaction markers.
func (l *DefaultLogger) RedactAPIKey(key string) string {
	if !l.redactKeys {
		return key
	}
	if len(key) <= 4 {
		return "[REDACTED]"
	}
	return fmt.Sprintf("[REDACTED-%s]", key[len(key)-4:])
}
