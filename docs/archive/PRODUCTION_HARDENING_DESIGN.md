# Production Hardening Sprint - Technical Design

**Version**: 1.0
**Date**: 2025-10-22
**Status**: In Progress
**Target Release**: v0.1.1

## Overview

This sprint focuses on addressing high-priority technical debt and quick wins identified through AI code reviews. The goal is to improve production reliability, observability, and code quality before adding major new features.

## Objectives

1. **Prevent resource leaks** in HTTP client retry logic
2. **Improve observability** with structured logging throughout
3. **Fix edge cases** in retry mechanism
4. **Polish code quality** with quick wins (constants, validation, formatting)

## Success Criteria

- All HTTP clients pass `-race` detector with no warnings
- All `fmt.Printf` calls replaced with structured logging
- RetryWithBackoff handles all edge cases correctly
- Code quality improvements applied consistently
- All existing tests pass
- New tests added for edge cases
- Zero regression in functionality

---

## Task 1: Response Body Leak Prevention

### Problem Statement

HTTP response bodies must be properly closed on all code paths to prevent resource leaks. Complex retry logic with multiple error paths could potentially miss closing response bodies, leading to memory leaks in high-volume scenarios.

### Current State

All LLM clients (OpenAI, Anthropic, Gemini, Ollama) have retry logic that creates new requests on each attempt. Need to verify that response bodies are closed on:
- Successful responses
- Error responses (4xx, 5xx)
- Network failures
- Context cancellations
- JSON parsing failures

### Design

**Approach**: Defensive audit with verification

1. **Audit Phase**: Review all HTTP client retry closures
   - Check every `c.client.Do(req)` call
   - Verify `defer resp.Body.Close()` placement
   - Identify any error paths that skip closing

2. **Verification Phase**: Use Go's race detector
   - Run all tests with `-race` flag
   - Run integration tests if available
   - Check for resource leak warnings

3. **Fix Phase** (if issues found):
   - Add `defer resp.Body.Close()` after successful `Do()` calls
   - Ensure early returns don't skip deferred closes
   - Consider adding explicit close on error paths for clarity

### Implementation Plan

**Files to Review**:
- `internal/adapter/llm/openai/client.go` - Operation closure starting ~line 163
- `internal/adapter/llm/anthropic/client.go` - Operation closure starting ~line 164
- `internal/adapter/llm/gemini/client.go` - Similar pattern
- `internal/adapter/llm/ollama/client.go` - Similar pattern

**Pattern to Verify**:
```go
operation := func(ctx context.Context) error {
    // Create request
    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
    if err != nil {
        return err  // OK - no response body yet
    }

    resp, err := c.client.Do(req)
    if err != nil {
        return err  // OK - no response body yet
    }
    defer resp.Body.Close()  // MUST be immediately after successful Do()

    // Read body
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return err  // OK - deferred close will happen
    }

    // Check status
    if resp.StatusCode != http.StatusOK {
        return handleError(resp.StatusCode, body)  // OK - deferred close
    }

    // Parse response
    // ... any error here is OK - deferred close

    return nil
}
```

**Testing Strategy**:
```bash
# Run all LLM client tests with race detector
go test -race ./internal/adapter/llm/...

# Run full test suite with race detector
go test -race ./...

# Look for warnings about unclosed bodies
```

**Expected Outcome**: No changes needed (verification task), or minor fixes with deferred closes.

**Time Estimate**: 1 hour

---

## Task 2: Structured Logging Throughout

### Problem Statement

The codebase uses a mix of structured logging (via observability logger) and unstructured logging (`fmt.Printf`, `log.Printf`). This inconsistency makes log aggregation, filtering, and debugging harder in production.

### Current State

**Unstructured logging locations**:
1. `cmd/bop/main.go:88` - Store initialization warning
2. `cmd/bop/main.go:93` - Store initialization warning (else branch)
3. `internal/usecase/review/orchestrator.go:244` - Failed to create run warning
4. `internal/usecase/review/orchestrator.go:349` - Failed to save review warning
5. `internal/usecase/review/orchestrator.go:398` - Failed to update run cost warning

**Current observability infrastructure**:
- `internal/adapter/llm/http/logger.go` - Logger interface and implementation
- Supports JSON and human-readable formats
- Includes structured fields (timestamp, provider, model, etc.)
- Already integrated into all LLM clients

### Design

**Approach**: Add optional logger to components, graceful fallback

#### 2.1. Extend Orchestrator Dependencies

Add optional logger to `OrchestratorDeps`:

```go
// internal/usecase/review/orchestrator.go
type OrchestratorDeps struct {
    Git       GitClient
    Providers map[string]Provider
    Store     Store
    Logger    Logger  // NEW: Optional structured logger
}
```

Define minimal logger interface:

```go
// internal/usecase/review/logger.go (NEW FILE)
package review

import "context"

// Logger provides structured logging for the review use case.
type Logger interface {
    LogWarning(ctx context.Context, message string, fields map[string]interface{})
    LogInfo(ctx context.Context, message string, fields map[string]interface{})
}
```

#### 2.2. Update Orchestrator Logging

Replace `fmt.Printf` with conditional structured logging:

```go
// Example: Store initialization failure
if err := o.deps.Store.CreateRun(ctx, run); err != nil {
    if o.deps.Logger != nil {
        o.deps.Logger.LogWarning(ctx, "failed to create run record", map[string]interface{}{
            "runID": runID,
            "error": err.Error(),
        })
    } else {
        log.Printf("warning: failed to create run record: %v\n", err)
    }
}
```

#### 2.3. Wire Logger from Main

Create adapter to bridge `llmhttp.Logger` to `review.Logger`:

```go
// internal/adapter/observability/logger.go (NEW FILE)
package observability

import (
    "context"
    llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
    "github.com/delightfulhammers/bop/internal/usecase/review"
)

// ReviewLogger adapts llmhttp.Logger to review.Logger interface.
type ReviewLogger struct {
    logger llmhttp.Logger
}

func NewReviewLogger(logger llmhttp.Logger) review.Logger {
    return &ReviewLogger{logger: logger}
}

func (l *ReviewLogger) LogWarning(ctx context.Context, message string, fields map[string]interface{}) {
    // Use logger's existing methods or add new generic logging methods
}

func (l *ReviewLogger) LogInfo(ctx context.Context, message string, fields map[string]interface{}) {
    // Similar implementation
}
```

Update `cmd/bop/main.go` to wire logger:

```go
// Create observability logger
obsLogger := llmhttp.NewDefaultLogger(logLevel, logFormat, redact)

// Create review logger adapter
reviewLogger := observability.NewReviewLogger(obsLogger)

// Wire into orchestrator
orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
    Git:       gitClient,
    Providers: providers,
    Store:     reviewStore,
    Logger:    reviewLogger,  // NEW
})
```

### Implementation Plan

**Phase 1**: Define interfaces and adapters (30 min)
- Create `internal/usecase/review/logger.go` with interface
- Create `internal/adapter/observability/logger.go` with adapter
- Add Logger field to OrchestratorDeps

**Phase 2**: Update orchestrator (45 min)
- Replace all `fmt.Printf` in orchestrator with conditional logging
- Add structured fields (runID, reviewID, error, etc.)
- Fallback to `log.Printf` when logger is nil

**Phase 3**: Wire from main (15 min)
- Create ReviewLogger adapter in main
- Wire into orchestrator dependencies
- Test with both JSON and human formats

**Phase 4**: Update main.go store warnings (15 min)
- Replace `log.Printf` in store initialization
- Use same structured logging approach

**Testing Strategy**:
1. Unit tests for ReviewLogger adapter
2. Integration test verifying log output in JSON format
3. Test logger=nil fallback still works
4. Verify all warning messages appear in logs

**Time Estimate**: 2 hours

---

## Task 3: RetryWithBackoff Edge Case

### Problem Statement

If context is cancelled before the first operation attempt in `RetryWithBackoff`, `lastErr` remains `nil`, leading to `return nil` which incorrectly indicates success when the context was actually cancelled.

### Current Implementation

```go
// internal/adapter/llm/http/retry.go (lines 71-77)
func RetryWithBackoff(ctx context.Context, operation RetryableOperation, cfg RetryConfig) error {
    var lastErr error  // Starts as nil

    for attempt := 0; attempt < cfg.MaxRetries; attempt++ {
        // Check context before operation
        if ctx.Err() != nil {
            return lastErr  // BUG: Could return nil if cancelled before first attempt
        }

        lastErr = operation(ctx)
        if lastErr == nil {
            return nil  // Success
        }

        // ... retry logic
    }

    return lastErr
}
```

### Design

**Approach**: Initialize lastErr and handle context cancellation explicitly

#### Option A: Initialize lastErr (Preferred)

```go
func RetryWithBackoff(ctx context.Context, operation RetryableOperation, cfg RetryConfig) error {
    // Initialize to non-nil so we never return nil on context cancellation
    lastErr := ctx.Err()
    if lastErr != nil {
        return lastErr  // Already cancelled
    }

    for attempt := 0; attempt < cfg.MaxRetries; attempt++ {
        // Check context
        if err := ctx.Err(); err != nil {
            // Context cancelled during retries
            if lastErr != nil {
                return lastErr  // Return last operation error
            }
            return err  // Return context error if no operation ran
        }

        lastErr = operation(ctx)
        if lastErr == nil {
            return nil  // Success
        }

        // ... retry logic
    }

    return lastErr
}
```

#### Option B: Explicit Context Check (Alternative)

```go
func RetryWithBackoff(ctx context.Context, operation RetryableOperation, cfg RetryConfig) error {
    var lastErr error

    for attempt := 0; attempt < cfg.MaxRetries; attempt++ {
        // Check context with explicit handling
        if ctx.Err() != nil {
            if lastErr == nil {
                return ctx.Err()  // Never ran, return context error
            }
            return lastErr  // Return last attempt error
        }

        // ... rest of logic
    }

    return lastErr
}
```

### Implementation Plan

**Phase 1**: Add test case for edge condition (20 min)
```go
// internal/adapter/llm/http/retry_test.go
func TestRetryWithBackoff_ContextCancelledBeforeFirstAttempt(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cancel()  // Cancel before retry starts

    operation := func(ctx context.Context) error {
        t.Fatal("operation should not be called")
        return nil
    }

    cfg := RetryConfig{MaxRetries: 3, InitialBackoff: 10 * time.Millisecond}
    err := RetryWithBackoff(ctx, operation, cfg)

    require.Error(t, err)
    assert.ErrorIs(t, err, context.Canceled)
}
```

**Phase 2**: Implement fix (10 min)
- Choose Option A (cleaner, more explicit)
- Update RetryWithBackoff implementation
- Ensure backwards compatibility

**Phase 3**: Verify all existing tests pass (5 min)

**Testing Strategy**:
- New test: Context cancelled before first attempt
- New test: Context cancelled between retries
- Verify all existing retry tests still pass
- Test with deadline exceeded (similar scenario)

**Time Estimate**: 45 minutes

---

## Task 4: Quick Wins

These are small, low-risk improvements that polish code quality.

### 4.1. Magic Number to Named Constant

**Location**: `internal/determinism/seed.go:23-25`

**Current**:
```go
// Mask to ensure positive int64 (clear sign bit)
seed = seed & 0x7FFFFFFFFFFFFFFF
```

**Improved**:
```go
const maxInt64Mask = 0x7FFFFFFFFFFFFFFF // Ensures result fits in positive int64 range

// Mask to ensure positive int64 (clear sign bit)
seed = seed & maxInt64Mask
```

**Time**: 5 minutes

### 4.2. SARIF Cost Validation

**Location**: `internal/adapter/output/sarif/writer.go:110-115`

**Current**:
```go
properties["cost"] = artifact.Review.Cost
```

**Improved**:
```go
import "math"

// Only include cost if it's a valid number
if !math.IsNaN(artifact.Review.Cost) && !math.IsInf(artifact.Review.Cost, 0) {
    properties["cost"] = artifact.Review.Cost
}
```

**Test Case**:
```go
func TestSARIFWriter_HandlesInvalidCost(t *testing.T) {
    tests := []struct{
        name string
        cost float64
        shouldInclude bool
    }{
        {"valid cost", 1.23, true},
        {"zero cost", 0.0, true},
        {"NaN", math.NaN(), false},
        {"positive infinity", math.Inf(1), false},
        {"negative infinity", math.Inf(-1), false},
    }
    // ... test implementation
}
```

**Time**: 15 minutes

### 4.3. API Key Redaction Format

**Location**: `internal/adapter/llm/http/logger.go:157-166`

**Current**:
```go
if len(key) > 4 {
    return "****" + key[len(key)-4:]
}
return "****"
```

**Improved**:
```go
if len(key) > 4 {
    return "[REDACTED-" + key[len(key)-4:] + "]"
}
return "[REDACTED]"
```

**Test Updates**:
```go
func TestRedactAPIKey(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"sk-1234567890abcdef", "[REDACTED-cdef]"},
        {"abc", "[REDACTED]"},
        {"", "[REDACTED]"},
    }
    // ... test implementation
}
```

**Time**: 15 minutes

---

## Testing Strategy

### Unit Tests

Each task includes specific unit tests:
1. **Response Body Leak**: Race detector on existing tests
2. **Structured Logging**: Logger adapter tests, fallback tests
3. **Retry Edge Case**: Context cancellation tests
4. **Quick Wins**: Individual validation tests

### Integration Tests

- Run full test suite with `-race` flag
- Test orchestrator with and without logger
- Verify log output format (JSON and human)

### Manual Testing

```bash
# Test with race detector
go test -race ./...

# Test structured logging output
./bop review branch HEAD --base main 2>&1 | jq .  # JSON format

# Verify no regressions
mage ci
```

---

## Implementation Order

1. **Task 4: Quick Wins** (35 min) - Easy confidence builders
   - 4.1: Magic number constant
   - 4.2: SARIF cost validation
   - 4.3: API key redaction format

2. **Task 3: RetryWithBackoff Edge Case** (45 min) - Focused, well-defined

3. **Task 1: Response Body Leak Prevention** (1 hour) - Verification task

4. **Task 2: Structured Logging** (2 hours) - Most complex, benefits from momentum

**Total Time**: ~4.5 hours

---

## Success Metrics

- ✅ All tests pass with `-race` flag (no data races, no leaks)
- ✅ Zero `fmt.Printf` in orchestrator and main (excluding debug/dev code)
- ✅ RetryWithBackoff handles context cancellation correctly
- ✅ All magic numbers replaced with named constants
- ✅ SARIF handles invalid cost values gracefully
- ✅ API key redaction format is explicit and clear
- ✅ No regression in functionality
- ✅ Code coverage maintained or improved

---

## Rollback Plan

All changes are non-breaking:
- Structured logging falls back to existing behavior if logger is nil
- Edge case fix only affects error conditions
- Quick wins are internal improvements

If issues arise:
1. Individual commits allow selective revert
2. Tests will catch any regressions immediately
3. No external API changes, so no user impact

---

## Documentation Updates

After completion:
1. Update ROADMAP.md - Move completed items to "Recently Fixed"
2. Update this design doc with "Completed" status
3. Archive to `docs/archive/PRODUCTION_HARDENING_DESIGN.md`
4. Add entry to CHANGELOG.md for v0.1.1

---

## Future Considerations

After this sprint:
- Consider extending structured logging to Git adapter
- Monitor for any resource leaks in production
- Evaluate if more retry edge cases exist
- Consider adding OpenTelemetry integration for richer observability
