# Observability & Cost Tracking Technical Design

Version: 1.0
Date: 2025-10-21
Status: Draft

## 1. Overview

This document describes the technical design for adding observability (logging, metrics, duration tracking) and cost tracking to the HTTP LLM clients. The design maintains clean architecture principles while providing visibility into API usage, performance, and costs.

## 2. Goals

### Primary Goals
- **Visibility**: Log all API requests and responses for debugging
- **Performance Tracking**: Measure and record API call duration
- **Cost Awareness**: Calculate and track costs per provider and per review
- **Security**: Never log full API keys, only last 4 characters
- **Configurability**: Allow users to control logging verbosity

### Non-Goals
- Real-time metrics export (Prometheus, OpenTelemetry) - future enhancement
- Log aggregation/indexing - users can use external tools
- Cost budgeting/limits - Phase 4 feature
- Performance profiling - use Go's built-in profiler

## 3. Architecture

### 3.1 Component Overview

```
┌─────────────────────────────────────────────────┐
│           Orchestrator (usecase)                │
│  - Aggregates costs from all providers          │
│  - Includes total cost in review output         │
└──────────────┬──────────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────┐
│         Provider Interface (domain)             │
│  - CreateReview(req) -> (resp, error)           │
│  - Response includes Cost field                 │
└──────────────┬──────────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────────┐
│      HTTP Client (adapter/llm/*/client.go)      │
│  - Logs requests/responses via Logger           │
│  - Tracks duration with Metrics                 │
│  - Calculates cost with Pricing                 │
└──────┬──────────────┬──────────────┬────────────┘
       │              │              │
       ▼              ▼              ▼
┌──────────┐   ┌──────────┐   ┌──────────┐
│  Logger  │   │ Metrics  │   │ Pricing  │
│ (http)   │   │ (http)   │   │ (http)   │
└──────────┘   └──────────┘   └──────────┘
```

### 3.2 Package Structure

```
internal/adapter/llm/http/
├── errors.go          # Existing: typed errors
├── retry.go           # Existing: retry logic
├── logger.go          # NEW: logging infrastructure
├── metrics.go         # NEW: metrics tracking
└── pricing.go         # NEW: cost calculation

internal/domain/
└── types.go           # UPDATE: Add Cost field to Review

internal/adapter/llm/openai/
├── client.go          # UPDATE: Add logging, metrics, pricing
└── client_test.go     # UPDATE: Test logging, metrics, pricing

(Similar updates for anthropic/, ollama/, gemini/)
```

## 4. Detailed Design

### 4.1 Logging Infrastructure

#### Logger Interface

```go
// internal/adapter/llm/http/logger.go

package llmhttp

import (
    "context"
    "time"
)

// Logger provides structured logging for LLM API calls.
type Logger interface {
    // LogRequest logs an outgoing API request (API key redacted)
    LogRequest(ctx context.Context, req RequestLog)

    // LogResponse logs an API response with timing and token info
    LogResponse(ctx context.Context, resp ResponseLog)

    // LogError logs an API error
    LogError(ctx context.Context, err ErrorLog)
}

// RequestLog contains request information for logging.
type RequestLog struct {
    Provider    string
    Model       string
    Timestamp   time.Time
    PromptChars int    // Character count of prompt
    APIKey      string // Will be redacted to last 4 chars
}

// ResponseLog contains response information for logging.
type ResponseLog struct {
    Provider     string
    Model        string
    Timestamp    time.Time
    Duration     time.Duration
    TokensIn     int
    TokensOut    int
    Cost         float64
    StatusCode   int
    FinishReason string
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
```

#### Default Logger Implementation

```go
// DefaultLogger writes logs in structured format to stdout.
type DefaultLogger struct {
    level      LogLevel
    redactKeys bool
    format     LogFormat
}

type LogLevel int

const (
    LogLevelDebug LogLevel = iota
    LogLevelInfo
    LogLevelError
)

type LogFormat int

const (
    LogFormatJSON LogFormat = iota
    LogFormatHuman
)

// NewDefaultLogger creates a logger with the specified config.
func NewDefaultLogger(level LogLevel, format LogFormat) *DefaultLogger {
    return &DefaultLogger{
        level:      level,
        redactKeys: true,
        format:     format,
    }
}

// LogRequest logs an API request.
func (l *DefaultLogger) LogRequest(ctx context.Context, req RequestLog) {
    if l.level > LogLevelDebug {
        return
    }

    // Redact API key to last 4 characters
    redacted := l.redactAPIKey(req.APIKey)

    if l.format == LogFormatJSON {
        // JSON format for machine parsing
        log.Printf(`{"level":"debug","type":"request","provider":"%s","model":"%s","timestamp":"%s","prompt_chars":%d,"api_key":"%s"}`,
            req.Provider, req.Model, req.Timestamp.Format(time.RFC3339),
            req.PromptChars, redacted)
    } else {
        // Human-readable format
        log.Printf("[DEBUG] %s/%s: Request sent (prompt=%d chars, key=%s)",
            req.Provider, req.Model, req.PromptChars, redacted)
    }
}

// redactAPIKey shows only the last 4 characters.
func (l *DefaultLogger) redactAPIKey(key string) string {
    if !l.redactKeys || len(key) <= 4 {
        return "****"
    }
    return "****" + key[len(key)-4:]
}
```

### 4.2 Metrics Infrastructure

#### Metrics Interface

```go
// internal/adapter/llm/http/metrics.go

package llmhttp

import (
    "sync"
    "time"
)

// Metrics tracks aggregate statistics for API calls.
type Metrics interface {
    // RecordRequest records an API request
    RecordRequest(provider, model string)

    // RecordDuration records request duration
    RecordDuration(provider, model string, duration time.Duration)

    // RecordTokens records token usage
    RecordTokens(provider, model string, tokensIn, tokensOut int)

    // RecordCost records API cost
    RecordCost(provider, model string, cost float64)

    // RecordError records an error
    RecordError(provider, model string, errType ErrorType)

    // GetStats returns current statistics
    GetStats() Stats
}

// Stats contains aggregate statistics.
type Stats struct {
    TotalRequests   int
    TotalTokensIn   int
    TotalTokensOut  int
    TotalCost       float64
    TotalDuration   time.Duration
    ErrorCount      int
    ByProvider      map[string]ProviderStats
}

// ProviderStats contains per-provider statistics.
type ProviderStats struct {
    Requests   int
    TokensIn   int
    TokensOut  int
    Cost       float64
    Duration   time.Duration
    Errors     int
}
```

#### Default Metrics Implementation

```go
// DefaultMetrics provides in-memory metrics tracking.
type DefaultMetrics struct {
    mu    sync.RWMutex
    stats Stats
}

// NewDefaultMetrics creates a metrics tracker.
func NewDefaultMetrics() *DefaultMetrics {
    return &DefaultMetrics{
        stats: Stats{
            ByProvider: make(map[string]ProviderStats),
        },
    }
}

// RecordRequest increments request counter.
func (m *DefaultMetrics) RecordRequest(provider, model string) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.stats.TotalRequests++

    ps := m.stats.ByProvider[provider]
    ps.Requests++
    m.stats.ByProvider[provider] = ps
}

// RecordDuration records API call duration.
func (m *DefaultMetrics) RecordDuration(provider, model string, duration time.Duration) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.stats.TotalDuration += duration

    ps := m.stats.ByProvider[provider]
    ps.Duration += duration
    m.stats.ByProvider[provider] = ps
}

// GetStats returns a copy of current statistics.
func (m *DefaultMetrics) GetStats() Stats {
    m.mu.RLock()
    defer m.mu.RUnlock()

    // Deep copy to avoid race conditions
    statsCopy := m.stats
    statsCopy.ByProvider = make(map[string]ProviderStats)
    for k, v := range m.stats.ByProvider {
        statsCopy.ByProvider[k] = v
    }
    return statsCopy
}
```

### 4.3 Cost Calculation

#### Pricing Interface

```go
// internal/adapter/llm/http/pricing.go

package llmhttp

// Pricing calculates API costs based on token usage.
type Pricing interface {
    // GetCost calculates cost for a given model and token usage
    GetCost(provider, model string, tokensIn, tokensOut int) float64
}

// ModelPricing contains pricing information for a model.
type ModelPricing struct {
    InputPer1M  float64 // Cost per 1M input tokens
    OutputPer1M float64 // Cost per 1M output tokens
}

// DefaultPricing provides cost calculation based on provider pricing.
type DefaultPricing struct {
    prices map[string]map[string]ModelPricing
}

// NewDefaultPricing creates a pricing calculator with current rates.
func NewDefaultPricing() *DefaultPricing {
    return &DefaultPricing{
        prices: buildPricingTable(),
    }
}

// GetCost calculates the cost for a given request.
func (p *DefaultPricing) GetCost(provider, model string, tokensIn, tokensOut int) float64 {
    providerPrices, ok := p.prices[provider]
    if !ok {
        return 0.0
    }

    modelPrice, ok := providerPrices[model]
    if !ok {
        return 0.0
    }

    inputCost := float64(tokensIn) / 1_000_000.0 * modelPrice.InputPer1M
    outputCost := float64(tokensOut) / 1_000_000.0 * modelPrice.OutputPer1M

    return inputCost + outputCost
}
```

#### Pricing Table

```go
// buildPricingTable returns pricing data for all models.
// Pricing as of: 2025-10-21
// Sources:
// - OpenAI: https://openai.com/api/pricing/
// - Anthropic: https://www.anthropic.com/pricing
// - Gemini: https://ai.google.dev/pricing
// - Ollama: Free (local)
func buildPricingTable() map[string]map[string]ModelPricing {
    return map[string]map[string]ModelPricing{
        "openai": {
            "gpt-4o": {
                InputPer1M:  2.50,
                OutputPer1M: 10.00,
            },
            "gpt-4o-mini": {
                InputPer1M:  0.15,
                OutputPer1M: 0.60,
            },
            "o1": {
                InputPer1M:  15.00,
                OutputPer1M: 60.00,
            },
            "o1-mini": {
                InputPer1M:  3.00,
                OutputPer1M: 12.00,
            },
            "o1-preview": {
                InputPer1M:  15.00,
                OutputPer1M: 60.00,
            },
        },
        "anthropic": {
            "claude-3-5-sonnet-20241022": {
                InputPer1M:  3.00,
                OutputPer1M: 15.00,
            },
            "claude-3-5-sonnet-20240620": {
                InputPer1M:  3.00,
                OutputPer1M: 15.00,
            },
            "claude-3-5-haiku-20241022": {
                InputPer1M:  0.80,
                OutputPer1M: 4.00,
            },
            "claude-3-opus-20240229": {
                InputPer1M:  15.00,
                OutputPer1M: 75.00,
            },
        },
        "gemini": {
            "gemini-1.5-pro": {
                InputPer1M:  1.25,
                OutputPer1M: 5.00,
            },
            "gemini-1.5-flash": {
                InputPer1M:  0.075,
                OutputPer1M: 0.30,
            },
            "gemini-2.0-flash-exp": {
                InputPer1M:  0.00, // Free during preview
                OutputPer1M: 0.00,
            },
        },
        "ollama": {
            // All Ollama models are free (local)
            "codellama": {
                InputPer1M:  0.00,
                OutputPer1M: 0.00,
            },
            "qwen2.5-coder": {
                InputPer1M:  0.00,
                OutputPer1M: 0.00,
            },
            "deepseek-coder": {
                InputPer1M:  0.00,
                OutputPer1M: 0.00,
            },
        },
    }
}
```

### 4.4 Integration with HTTP Clients

#### OpenAI Client Update

```go
// internal/adapter/llm/openai/client.go

type HTTPClient struct {
    apiKey  string
    model   string
    baseURL string
    timeout time.Duration
    client  *http.Client

    // NEW: Observability components
    logger  llmhttp.Logger
    metrics llmhttp.Metrics
    pricing llmhttp.Pricing
}

// SetLogger sets the logger for this client.
func (c *HTTPClient) SetLogger(logger llmhttp.Logger) {
    c.logger = logger
}

// SetMetrics sets the metrics tracker for this client.
func (c *HTTPClient) SetMetrics(metrics llmhttp.Metrics) {
    c.metrics = metrics
}

// SetPricing sets the pricing calculator for this client.
func (c *HTTPClient) SetPricing(pricing llmhttp.Pricing) {
    c.pricing = pricing
}

// Call makes a request with logging, metrics, and cost tracking.
func (c *HTTPClient) Call(ctx context.Context, prompt string, options CallOptions) (*APIResponse, error) {
    startTime := time.Now()

    // Log request (if logger configured)
    if c.logger != nil {
        c.logger.LogRequest(ctx, llmhttp.RequestLog{
            Provider:    "openai",
            Model:       c.model,
            Timestamp:   startTime,
            PromptChars: len(prompt),
            APIKey:      c.apiKey,
        })
    }

    // Record request metric
    if c.metrics != nil {
        c.metrics.RecordRequest("openai", c.model)
    }

    // Make API call (existing logic)
    resp, err := c.makeAPICall(ctx, prompt, options)

    duration := time.Since(startTime)

    if err != nil {
        // Log error
        if c.logger != nil {
            var httpErr *llmhttp.Error
            if errors.As(err, &httpErr) {
                c.logger.LogError(ctx, llmhttp.ErrorLog{
                    Provider:   "openai",
                    Model:      c.model,
                    Timestamp:  time.Now(),
                    Duration:   duration,
                    Error:      err,
                    ErrorType:  httpErr.Type,
                    StatusCode: httpErr.StatusCode,
                    Retryable:  httpErr.Retryable,
                })
            }
        }
        // Record error metric
        if c.metrics != nil {
            var httpErr *llmhttp.Error
            if errors.As(err, &httpErr) {
                c.metrics.RecordError("openai", c.model, httpErr.Type)
            }
        }
        return nil, err
    }

    // Calculate cost
    var cost float64
    if c.pricing != nil {
        cost = c.pricing.GetCost("openai", c.model, resp.TokensIn, resp.TokensOut)
    }
    resp.Cost = cost

    // Log response
    if c.logger != nil {
        c.logger.LogResponse(ctx, llmhttp.ResponseLog{
            Provider:     "openai",
            Model:        c.model,
            Timestamp:    time.Now(),
            Duration:     duration,
            TokensIn:     resp.TokensIn,
            TokensOut:    resp.TokensOut,
            Cost:         cost,
            StatusCode:   200,
            FinishReason: resp.FinishReason,
        })
    }

    // Record metrics
    if c.metrics != nil {
        c.metrics.RecordDuration("openai", c.model, duration)
        c.metrics.RecordTokens("openai", c.model, resp.TokensIn, resp.TokensOut)
        c.metrics.RecordCost("openai", c.model, cost)
    }

    return resp, nil
}
```

#### APIResponse Update

```go
// internal/adapter/llm/openai/client.go

type APIResponse struct {
    Text         string
    TokensIn     int
    TokensOut    int
    FinishReason string
    Cost         float64  // NEW: Cost in USD
}
```

### 4.5 Domain Model Update

```go
// internal/domain/types.go

type Review struct {
    Model    string
    Summary  string
    Findings []Finding
    Cost     float64  // NEW: Cost in USD
}
```

### 4.6 Configuration

```yaml
# ~/.config/bop/bop.yaml

observability:
  logging:
    enabled: true
    level: info      # debug, info, error
    format: human    # human, json
    redact_api_keys: true

  metrics:
    enabled: true
```

### 4.7 Output Format Updates

#### Markdown Output

```markdown
# Code Review: feature-branch vs main

**Summary:** ...

**Cost:** $0.0042 (OpenAI: $0.0021, Anthropic: $0.0021, Gemini: $0.0000, Ollama: $0.0000)

## Findings

...
```

#### JSON Output

```json
{
  "model": "merged",
  "summary": "...",
  "cost": 0.0042,
  "cost_breakdown": {
    "openai": 0.0021,
    "anthropic": 0.0021,
    "gemini": 0.0000,
    "ollama": 0.0000
  },
  "findings": [...]
}
```

## 5. Testing Strategy

### 5.1 Unit Tests

```go
// internal/adapter/llm/http/logger_test.go

func TestDefaultLogger_RedactAPIKey(t *testing.T) {
    logger := NewDefaultLogger(LogLevelDebug, LogFormatHuman)

    tests := []struct {
        name     string
        key      string
        expected string
    }{
        {
            name:     "full key",
            key:      "sk-1234567890abcdef",
            expected: "****cdef",
        },
        {
            name:     "short key",
            key:      "abc",
            expected: "****",
        },
        {
            name:     "empty key",
            key:      "",
            expected: "****",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := logger.redactAPIKey(tt.key)
            assert.Equal(t, tt.expected, result)
        })
    }
}

func TestDefaultLogger_LogRequest(t *testing.T) {
    var buf bytes.Buffer
    log.SetOutput(&buf)
    defer log.SetOutput(os.Stderr)

    logger := NewDefaultLogger(LogLevelDebug, LogFormatHuman)
    logger.LogRequest(context.Background(), llmhttp.RequestLog{
        Provider:    "openai",
        Model:       "gpt-4o-mini",
        Timestamp:   time.Now(),
        PromptChars: 1000,
        APIKey:      "sk-1234567890abcdef",
    })

    output := buf.String()
    assert.Contains(t, output, "openai/gpt-4o-mini")
    assert.Contains(t, output, "1000 chars")
    assert.Contains(t, output, "****cdef")
    assert.NotContains(t, output, "sk-1234567890abcdef")
}
```

### 5.2 Integration Tests

```go
// internal/adapter/llm/openai/client_test.go

func TestHTTPClient_Call_WithObservability(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(ChatCompletionResponse{
            Choices: []Choice{{Message: Message{Content: "test"}, FinishReason: "stop"}},
            Usage:   Usage{PromptTokens: 100, CompletionTokens: 50},
        })
    }))
    defer server.Close()

    client := NewHTTPClient("test-key", "gpt-4o-mini")
    client.SetBaseURL(server.URL)

    // Set up observability
    logger := llmhttp.NewDefaultLogger(llmhttp.LogLevelDebug, llmhttp.LogFormatHuman)
    metrics := llmhttp.NewDefaultMetrics()
    pricing := llmhttp.NewDefaultPricing()

    client.SetLogger(logger)
    client.SetMetrics(metrics)
    client.SetPricing(pricing)

    resp, err := client.Call(context.Background(), "test prompt", CallOptions{MaxTokens: 1024})

    require.NoError(t, err)
    assert.Equal(t, "test", resp.Text)
    assert.Greater(t, resp.Cost, 0.0)

    // Verify metrics
    stats := metrics.GetStats()
    assert.Equal(t, 1, stats.TotalRequests)
    assert.Equal(t, 100, stats.TotalTokensIn)
    assert.Equal(t, 50, stats.TotalTokensOut)
    assert.Greater(t, stats.TotalCost, 0.0)
}
```

## 6. Performance Considerations

### Overhead Budget
- Logging: <5ms per request
- Metrics: <1ms per request (in-memory operations)
- Cost calculation: <1ms per request (simple arithmetic)
- **Total overhead: <10ms per request**

### Memory Usage
- Metrics stored in-memory per session
- Estimated: <1KB per request
- For 100 providers × 10 requests = 100KB total

## 7. Security Considerations

### API Key Redaction
- Always redact API keys to last 4 characters
- Apply redaction even in debug mode
- Test redaction thoroughly

### Log Sensitivity
- Don't log full prompts (may contain sensitive code)
- Log character counts instead
- Don't log full responses (may contain sensitive findings)

## 8. Future Enhancements

### Phase 4 Integration
- Budget enforcement: Check cost before calling API
- Cost alerts: Warn when approaching budget limit
- Cost reporting: Generate cost reports by time period

### External Metrics
- Prometheus exporter
- OpenTelemetry integration
- StatsD support

### Advanced Logging
- Distributed tracing (trace IDs)
- Log sampling for high-volume scenarios
- Structured log aggregation

## 9. Migration Path

### Backward Compatibility
- All observability features are opt-in
- Default behavior: minimal logging to avoid spam
- Existing tests continue to pass without changes

### Rollout Plan
1. Add infrastructure (logger, metrics, pricing) with tests
2. Update one client (OpenAI) with observability
3. Verify functionality and performance
4. Update remaining clients (Anthropic, Ollama, Gemini)
5. Update orchestrator to aggregate costs
6. Update output writers (Markdown, JSON, SARIF)
7. Add configuration support
8. Document features

## 10. Success Criteria

- ✅ All unit tests pass (target: 50+ new tests)
- ✅ API keys never logged in full form
- ✅ Duration tracking accurate within 1ms
- ✅ Token counts match API responses exactly
- ✅ Cost calculations within 5% of actual billing
- ✅ Performance overhead <10ms per request
- ✅ Documentation complete with examples
- ✅ Manual testing with all 4 providers successful
