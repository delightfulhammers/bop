# HTTP API Client Implementation Checklist

Status: ✅ COMPLETE - All Providers Implemented, Tested, with Full Observability
Started: 2025-10-21
Completed: 2025-10-21
Updated: 2025-10-21

## Goal
Replace stub/static LLM clients with real HTTP implementations for OpenAI, Anthropic, Gemini, and Ollama, enabling actual code review functionality with production LLM APIs.

## Overview

Currently all providers use static/stub clients:
- OpenAI: Uses `openai.NewStaticClient()` returning canned responses
- Anthropic: Uses `nil` client
- Gemini: Uses `nil` client
- Ollama: Uses `nil` client

This batch implements production HTTP clients with proper error handling, retries, and rate limiting.

## Priority Order

1. **OpenAI** (most common, well-documented API)
2. **Anthropic** (Claude - popular for code review)
3. **Ollama** (local, no API key required)
4. **Gemini** (Google)

## Week 1: OpenAI HTTP Client ✅ COMPLETE

### 1.1 HTTP Client Infrastructure (TDD) ✅
- [x] Create `internal/adapter/llm/openai/client.go`
- [x] Define HTTPClient interface for testing
- [x] Write tests for NewHTTPClient with API key
- [x] Implement NewHTTPClient constructor
- [x] Write tests for API key validation
- [x] Implement API key validation
- [x] Add timeout configuration (default 60s)

### 1.2 Request/Response Types (TDD) ✅
- [x] Define ChatCompletionRequest struct (matches OpenAI API)
- [x] Define ChatCompletionResponse struct
- [x] Define Message struct (role, content)
- [x] Write tests for JSON marshaling/unmarshaling
- [x] Add validation for required fields
- [x] Test error response handling

### 1.3 HTTP Implementation (TDD) ✅
- [x] Write test for successful API call
- [x] Implement Review() method calling OpenAI Chat Completion API
- [x] Write tests for error responses (401, 429, 500, 503)
- [x] Implement error handling with typed errors
- [x] Write tests for timeout scenarios
- [x] Implement timeout handling
- [x] Write tests for malformed responses
- [x] Implement response validation

### 1.4 Retry Logic (TDD) ✅
- [x] Write tests for 429 rate limit with exponential backoff
- [x] Implement exponential backoff (2s, 4s, 8s, 16s, 32s)
- [x] Write tests for 503 service unavailable retry
- [x] Implement 503 retry logic (max 3 retries)
- [x] Write tests for non-retryable errors (400, 401, 403)
- [x] Implement retry decision logic
- [x] Add configurable max retries

### 1.5 Response Parsing (TDD) ✅
- [x] Write tests for parsing review from completion text
- [x] Implement JSON extraction from markdown code blocks
- [x] Write tests for handling partial/malformed JSON
- [x] Implement graceful degradation (return text summary)
- [x] Write tests for finding extraction
- [x] Implement finding parsing with validation
- [x] Test with various response formats

### 1.6 Integration ✅
- [x] Update main.go to use HTTPClient instead of StaticClient
- [x] Add API key loading from config/env
- [x] Write integration test with mock HTTP server
- [x] Test with real OpenAI API (manual - verified working)
- [x] Update configuration documentation
- [x] Add environment variable expansion (${VAR} syntax)
- [x] Fix seed generation to work with OpenAI int64 limits

## Week 2: Anthropic (Claude) HTTP Client ✅ COMPLETE

### 2.1 Anthropic API Client (TDD) ✅
- [x] Create `internal/adapter/llm/anthropic/client.go`
- [x] Define MessagesRequest struct (Anthropic API format)
- [x] Define MessagesResponse struct
- [x] Write tests for API authentication (x-api-key header)
- [x] Implement NewHTTPClient with proper headers
- [x] Write tests for successful message creation
- [x] Implement Review() calling Messages API

### 2.2 Anthropic-Specific Features (TDD) ✅
- [x] Write tests for system prompt vs user message
- [x] Implement system prompt handling
- [x] Write tests for streaming disabled
- [x] Ensure non-streaming responses only
- [x] Write tests for content block handling
- [x] Implement content block extraction
- [x] Test with claude-3-5-sonnet model

### 2.3 Error Handling (TDD) ✅
- [x] Write tests for Anthropic error response format
- [x] Implement Anthropic error parsing
- [x] Write tests for rate limit handling (429)
- [x] Implement rate limit retry logic
- [x] Write tests for overloaded_error (529)
- [x] Implement overloaded retry logic
- [x] Test content policy violations (400)

### 2.4 Response Parsing (TDD) ✅
- [x] Write tests for text content extraction
- [x] Implement content[0].text parsing
- [x] Write tests for handling multiple content blocks
- [x] Implement multi-block concatenation
- [x] Write tests for JSON extraction from responses
- [x] Reuse OpenAI JSON parsing logic
- [x] Test finding extraction

### 2.5 Integration ✅
- [x] Update main.go to create real Anthropic client
- [x] Add API key loading (ANTHROPIC_API_KEY)
- [x] Write integration test with mock server (13 comprehensive tests)
- [x] Test with real Anthropic API (manual - verified working)
- [x] Claude models supported: claude-3-5-sonnet-20241022, claude-3-5-haiku, etc.

## Week 3: Ollama & Gemini Clients ✅ COMPLETE

### 3.1 Ollama HTTP Client (TDD) ✅
- [x] Create `internal/adapter/llm/ollama/client.go`
- [x] Define GenerateRequest struct (Ollama format)
- [x] Define GenerateResponse struct
- [x] Write tests for local connection (http://localhost:11434)
- [x] Implement NewHTTPClient with localhost default
- [x] Write tests for /api/generate endpoint
- [x] Implement Review() calling generate API
- [x] Write tests for connection refused error
- [x] Implement friendly "Ollama not running" error message

### 3.2 Ollama Features (TDD) ✅
- [x] Write tests for streaming disabled
- [x] Ensure stream: false in request
- [x] Write tests for temperature and seed options
- [x] Implement options handling
- [x] Test with codellama model

### 3.3 Gemini HTTP Client (TDD) ✅
- [x] Create `internal/adapter/llm/gemini/client.go`
- [x] Define GenerateContentRequest struct (Gemini format)
- [x] Define GenerateContentResponse struct
- [x] Write tests for API key in URL params
- [x] Implement NewHTTPClient with URL construction
- [x] Write tests for generateContent endpoint
- [x] Implement Review() calling Gemini API
- [x] Write tests for parts[] content handling

### 3.4 Gemini Features (TDD) ✅
- [x] Write tests for safety settings
- [x] Implement safety settings for code review context
- [x] Write tests for generation config
- [x] Implement temperature and candidate count settings
- [x] Write tests for content filtering responses
- [x] Handle SAFETY finish reason
- [x] Test with gemini-1.5-pro model

### 3.5 Integration ✅
- [x] Update main.go for Ollama with localhost URL
- [x] Update main.go for Gemini with API key
- [x] Add OLLAMA_HOST env var support
- [x] Add GEMINI_API_KEY env var support (via config ${VAR} expansion)
- [x] Write integration tests for both (15 Ollama + 16 Gemini = 31 tests)
- [x] Test with real Ollama (manual - verified working)
- [x] Test with real Gemini API (manual - verified working)
- [x] Create technical design document (WEEK3_OLLAMA_GEMINI_DESIGN.md)

## Week 4: Polish & Production Readiness

### 4.1 Shared HTTP Infrastructure ✅ COMPLETE
- [x] Create `internal/adapter/llm/http/` package
- [x] Extract common retry logic (retry.go with exponential backoff)
- [x] Extract common error handling (error.go with typed errors)
- [x] Extract common JSON parsing (used across all clients)
- [x] Write tests for shared utilities (error_test.go, retry_test.go)
- [x] Refactor all clients to use shared code
- [x] Reduce code duplication

### 4.2 Observability (TDD) ✅ COMPLETE
- [x] Write tests for request logging (logger_test.go - TestDefaultLogger_LogRequest_*)
- [x] Implement structured logging for all requests (logger.go)
- [x] Write tests for response logging (logger_test.go - TestDefaultLogger_LogResponse_*)
- [x] Implement response logging with API key redaction (logger.go - RedactAPIKey)
- [x] Write tests for duration tracking (metrics_test.go)
- [x] Add request timing metrics (metrics.go - RecordDuration)
- [x] Write tests for token usage tracking (metrics_test.go)
- [x] Implement token counting for cost estimation (metrics.go - RecordTokens)

### 4.3 Configuration ✅ COMPLETE
- [x] Add observability config options (observability.logging.*, observability.metrics.*)
- [x] Observability configuration fully implemented in config.go
- [x] Write tests for config loading (config_test.go)
- [x] Add validation for config values
- [x] Document all observability config options (CONFIGURATION.md, OBSERVABILITY.md)
- [ ] Add http.timeout config option (future enhancement)
- [ ] Add http.maxRetries config option (future enhancement)

### 4.4 Error Handling & Resilience ✅ COMPLETE
- [x] Create typed error hierarchy (error.go - ErrTypeRateLimit, ErrTypeTimeout, etc.)
- [x] Write tests for error types (error_test.go)
- [x] Write tests for request cancellation via context (client tests use context)
- [x] Ensure proper context propagation (all client methods accept context)
- [ ] Write tests for circuit breaker pattern (future enhancement - not currently implemented)
- [ ] Implement circuit breaker (future enhancement - not currently implemented)
- [ ] Write tests for graceful shutdown (future enhancement)

### 4.5 Testing Infrastructure ✅ COMPLETE
- [x] Integration tests using httptest.Server for all providers
- [x] OpenAI mock server (client_test.go - 15+ test scenarios)
- [x] Anthropic mock server (client_test.go - 13+ test scenarios)
- [x] Ollama mock server (client_test.go - 15+ test scenarios)
- [x] Gemini mock server (client_test.go - 16+ test scenarios)
- [x] Test utilities for common scenarios (retry, timeout, error handling)

### 4.6 Security ✅ COMPLETE
- [x] Write tests for API key redaction (logger_test.go - TestDefaultLogger_RedactAPIKey)
- [x] Implement API key masking showing only last 4 chars (logger.go - RedactAPIKey)
- [x] All clients use HTTPS by default (http:// only for Ollama localhost)
- [x] TLS certificate validation (default Go http.Client behavior)
- [x] Security documented in OBSERVABILITY.md and CONFIGURATION.md

### 4.7 Documentation ✅ COMPLETE
- [x] Update ARCHITECTURE.md with HTTP client layer
- [x] Create HTTP_CLIENT_DESIGN.md with API details
- [x] Document all supported models per provider (README.md, CONFIGURATION.md)
- [x] Add troubleshooting guide (CONFIGURATION.md - troubleshooting section)
- [x] Create examples for each provider (README.md, CONFIGURATION.md)
- [x] Document cost calculation (COST_TRACKING.md)
- [x] Add observability guide (OBSERVABILITY.md)

### 4.8 Cost Tracking ✅ COMPLETE
- [x] Write tests for token counting (pricing_test.go)
- [x] Implement token counting from API responses (all clients return TokensIn/TokensOut)
- [x] Write tests for cost calculation (pricing_test.go - all providers)
- [x] Implement cost calculation per provider (pricing.go with provider-specific rates)
- [x] Add cost tracking to Review response (domain.Review.Cost field)
- [x] Cost displayed in all output formats (markdown, JSON, SARIF)
- [x] Document pricing as of implementation date (COST_TRACKING.md with data sources)

## Dependencies to Add

```bash
# No new dependencies needed - using stdlib net/http
# All providers use REST APIs compatible with net/http
```

## Testing Commands

```bash
# Unit tests (with mocks)
go test ./internal/adapter/llm/openai/...
go test ./internal/adapter/llm/anthropic/...
go test ./internal/adapter/llm/ollama/...
go test ./internal/adapter/llm/gemini/...

# Integration tests (with mock HTTP server)
go test -tags=integration ./internal/adapter/llm/...

# Manual tests (requires API keys and real services)
# OpenAI
OPENAI_API_KEY=sk-... ./bop review branch main --target HEAD

# Anthropic
ANTHROPIC_API_KEY=sk-ant-... ./bop review branch main --target HEAD

# Ollama (requires Ollama running locally)
ollama serve &
./bop review branch main --target HEAD

# Gemini
GEMINI_API_KEY=... ./bop review branch main --target HEAD
```

## Completion Criteria

- [x] All unit tests passing
- [x] All integration tests passing
- [x] Manual tests successful with real APIs
- [x] Code coverage >80% for HTTP client packages
- [x] Error handling comprehensive (timeouts, rate limits, auth failures)
- [x] Retry logic tested and working
- [x] Response parsing handles all known formats
- [x] API keys loaded from config and environment
- [x] Documentation complete
- [x] Security review passed (no API keys in logs)

## Success Metrics

- **Reliability**: <1% failure rate for transient errors (should retry successfully)
- **Performance**: 95th percentile response time <30 seconds for typical review
- **Error Handling**: All error types have clear, actionable messages
- **Cost Tracking**: Accurate token counting within 5% of provider billing
- **Documentation**: Each provider has working example in docs

## Notes

- Start with OpenAI (most mature API, best docs)
- Test thoroughly with mock servers before hitting real APIs
- Implement rate limit handling from day 1 (429 responses are common)
- Use context.Context throughout for cancellation support
- Keep API-specific code isolated to each provider package
- Extract common logic to shared http utilities package
- Never log full API keys (mask to last 4 chars only)
- Consider implementing request/response recording for debugging
- Add instrumentation for observability (duration, tokens, cost)

## Risk Mitigation

**API Changes**: Version lock known-good API versions in docs
**Rate Limits**: Implement exponential backoff and retry logic
**Costs**: Add token counting and cost estimation (Phase 4 budget enforcement)
**Security**: Never log credentials, use environment variables
**Reliability**: Circuit breaker pattern for repeated failures
**Testing**: Comprehensive mock server for CI without API keys
