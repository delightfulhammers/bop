# Production Hardening Sprint - Implementation Checklist

**Target Release**: v0.1.1
**Started**: 2025-10-22
**Status**: In Progress

## Overview

This checklist tracks the implementation of production hardening improvements identified through AI code reviews. All items follow TDD principles with tests written before implementation.

---

## Task 1: Quick Wins (35 minutes)

Quick, low-risk improvements that polish code quality.

### 1.1. Magic Number to Named Constant ⏱️ 5 min

- [ ] **Read** `internal/determinism/seed.go`
- [ ] **Add** named constant `maxInt64Mask = 0x7FFFFFFFFFFFFFFF`
- [ ] **Replace** magic number with constant
- [ ] **Verify** existing tests still pass
- [ ] **Run** `go test ./internal/determinism/... -v`
- [ ] **Commit** with message

**Files Modified**:
- `internal/determinism/seed.go`

---

### 1.2. SARIF Cost Validation ⏱️ 15 min

- [ ] **Read** `internal/adapter/output/sarif/writer.go`
- [ ] **Write TEST** `TestSARIFWriter_HandlesInvalidCost` in `sarif/writer_test.go`
  - Test with NaN cost
  - Test with +Inf cost
  - Test with -Inf cost
  - Test with valid cost (0.0, 1.23)
- [ ] **Run TEST** - should fail (not implemented yet)
- [ ] **Add** `import "math"`
- [ ] **Implement** validation before adding cost to properties
  ```go
  if !math.IsNaN(artifact.Review.Cost) && !math.IsInf(artifact.Review.Cost, 0) {
      properties["cost"] = artifact.Review.Cost
  }
  ```
- [ ] **Run TEST** - should pass
- [ ] **Run** `go test ./internal/adapter/output/sarif/... -v`
- [ ] **Commit** with message

**Files Modified**:
- `internal/adapter/output/sarif/writer.go`
- `internal/adapter/output/sarif/writer_test.go`

---

### 1.3. API Key Redaction Format ⏱️ 15 min

- [ ] **Read** `internal/adapter/llm/http/logger.go`
- [ ] **Read** `internal/adapter/llm/http/logger_test.go`
- [ ] **Update TEST** `TestRedactAPIKey` with new expected format
  - `"sk-1234567890abcdef"` → `"[REDACTED-cdef]"`
  - `"abc"` → `"[REDACTED]"`
  - `""` → `"[REDACTED]"`
- [ ] **Run TEST** - should fail (old format)
- [ ] **Update** `RedactAPIKey` function with new format
  ```go
  if len(key) > 4 {
      return "[REDACTED-" + key[len(key)-4:] + "]"
  }
  return "[REDACTED]"
  ```
- [ ] **Run TEST** - should pass
- [ ] **Run** `go test ./internal/adapter/llm/http/... -v -run TestRedactAPIKey`
- [ ] **Run** `go test ./internal/adapter/llm/http/... -v` (all tests)
- [ ] **Commit** with message

**Files Modified**:
- `internal/adapter/llm/http/logger.go`
- `internal/adapter/llm/http/logger_test.go`

---

## Task 2: RetryWithBackoff Edge Case (45 minutes)

Fix edge case where context cancellation before first attempt could return nil error.

### 2.1. Write Failing Test ⏱️ 20 min

- [ ] **Read** `internal/adapter/llm/http/retry.go`
- [ ] **Read** `internal/adapter/llm/http/retry_test.go`
- [ ] **Write TEST** `TestRetryWithBackoff_ContextCancelledBeforeFirstAttempt`
  ```go
  func TestRetryWithBackoff_ContextCancelledBeforeFirstAttempt(t *testing.T) {
      ctx, cancel := context.WithCancel(context.Background())
      cancel()  // Cancel before retry starts

      operation := func(ctx context.Context) error {
          t.Fatal("operation should not be called")
          return nil
      }

      cfg := RetryConfig{
          MaxRetries:     3,
          InitialBackoff: 10 * time.Millisecond,
          MaxBackoff:     100 * time.Millisecond,
          Multiplier:     2.0,
      }

      err := RetryWithBackoff(ctx, operation, cfg)

      require.Error(t, err, "should return error when context cancelled")
      assert.ErrorIs(t, err, context.Canceled)
  }
  ```
- [ ] **Write TEST** `TestRetryWithBackoff_ContextCancelledBetweenRetries`
  ```go
  func TestRetryWithBackoff_ContextCancelledBetweenRetries(t *testing.T) {
      ctx, cancel := context.WithCancel(context.Background())
      attempts := 0

      operation := func(ctx context.Context) error {
          attempts++
          if attempts == 1 {
              cancel()  // Cancel after first attempt
              return errors.New("first attempt failed")
          }
          t.Fatal("should not retry after context cancelled")
          return nil
      }

      cfg := RetryConfig{MaxRetries: 3, InitialBackoff: 1 * time.Millisecond}
      err := RetryWithBackoff(ctx, operation, cfg)

      require.Error(t, err)
      assert.Equal(t, 1, attempts, "should stop after context cancelled")
  }
  ```
- [ ] **Run TEST** - should fail (bug exists)
  ```bash
  go test ./internal/adapter/llm/http/... -v -run TestRetryWithBackoff_Context
  ```

**Files Modified**:
- `internal/adapter/llm/http/retry_test.go`

---

### 2.2. Implement Fix ⏱️ 15 min

- [ ] **Update** `RetryWithBackoff` function
  ```go
  func RetryWithBackoff(ctx context.Context, operation RetryableOperation, cfg RetryConfig) error {
      // Check if already cancelled
      if err := ctx.Err(); err != nil {
          return err
      }

      var lastErr error

      for attempt := 0; attempt < cfg.MaxRetries; attempt++ {
          // Check context before each attempt
          if err := ctx.Err(); err != nil {
              if lastErr != nil {
                  return lastErr  // Return last operation error
              }
              return err  // Return context error if no operation ran yet
          }

          lastErr = operation(ctx)
          if lastErr == nil {
              return nil  // Success
          }

          // ... rest of backoff logic
      }

      return lastErr
  }
  ```
- [ ] **Run TEST** - should pass
  ```bash
  go test ./internal/adapter/llm/http/... -v -run TestRetryWithBackoff_Context
  ```
- [ ] **Run** all retry tests
  ```bash
  go test ./internal/adapter/llm/http/... -v -run TestRetry
  ```

**Files Modified**:
- `internal/adapter/llm/http/retry.go`

---

### 2.3. Verify No Regressions ⏱️ 10 min

- [ ] **Run** all HTTP tests
  ```bash
  go test ./internal/adapter/llm/http/... -v
  ```
- [ ] **Run** all LLM adapter tests (uses retry logic)
  ```bash
  go test ./internal/adapter/llm/... -v
  ```
- [ ] **Format** code
  ```bash
  mage format
  ```
- [ ] **Commit** with message

---

## Task 3: Response Body Leak Prevention (1 hour)

Audit and verify that all HTTP response bodies are properly closed.

### 3.1. Audit All HTTP Clients ⏱️ 30 min

- [ ] **Read** `internal/adapter/llm/openai/client.go` (operation closure ~line 163)
  - [ ] Verify `defer resp.Body.Close()` immediately after `Do()`
  - [ ] Check all error return paths
  - [ ] Document any issues found

- [ ] **Read** `internal/adapter/llm/anthropic/client.go` (operation closure ~line 164)
  - [ ] Verify `defer resp.Body.Close()` immediately after `Do()`
  - [ ] Check all error return paths
  - [ ] Document any issues found

- [ ] **Read** `internal/adapter/llm/gemini/client.go`
  - [ ] Verify `defer resp.Body.Close()` immediately after `Do()`
  - [ ] Check all error return paths
  - [ ] Document any issues found

- [ ] **Read** `internal/adapter/llm/ollama/client.go`
  - [ ] Verify `defer resp.Body.Close()` immediately after `Do()`
  - [ ] Check all error return paths
  - [ ] Document any issues found

**Audit Notes Template**:
```
Client: [name]
Location: [file:line]
Status: ✅ Correct | ⚠️ Issue Found
Issue: [description if found]
Fix: [proposed fix if needed]
```

---

### 3.2. Run Race Detector ⏱️ 20 min

- [ ] **Run** all LLM tests with race detector
  ```bash
  go test -race ./internal/adapter/llm/... -v
  ```
- [ ] **Check** output for warnings about:
  - Resource leaks
  - Unclosed response bodies
  - Data races
- [ ] **Document** any warnings found

- [ ] **Run** full test suite with race detector
  ```bash
  go test -race ./... -v
  ```
- [ ] **Verify** no race warnings
- [ ] **Document** results

**Race Detector Results**:
```
Date: [timestamp]
Command: go test -race ./...
Duration: [time]
Warnings: [count]
Details: [any warnings found]
```

---

### 3.3. Apply Fixes (If Needed) ⏱️ 10 min

- [ ] **If issues found**: Fix each identified issue
- [ ] **Add tests** if gaps in coverage found
- [ ] **Re-run** race detector
- [ ] **Verify** all warnings resolved
- [ ] **Commit** fixes

**Files Modified** (if needed):
- [list files that required fixes]

---

## Task 4: Structured Logging Throughout (2 hours)

Replace unstructured logging with structured logging throughout the codebase.

### 4.1. Define Interfaces and Adapters ⏱️ 30 min

- [ ] **Create** `internal/usecase/review/logger.go`
  ```go
  package review

  import "context"

  // Logger provides structured logging for the review use case.
  type Logger interface {
      LogWarning(ctx context.Context, message string, fields map[string]interface{})
      LogInfo(ctx context.Context, message string, fields map[string]interface{})
  }
  ```

- [ ] **Create** `internal/adapter/observability/` directory
- [ ] **Create** `internal/adapter/observability/logger.go`
  ```go
  package observability

  import (
      "context"
      "log"
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
      // Implementation
  }

  func (l *ReviewLogger) LogInfo(ctx context.Context, message string, fields map[string]interface{}) {
      // Implementation
  }
  ```

- [ ] **Write TEST** `internal/adapter/observability/logger_test.go`
  ```go
  func TestReviewLogger_LogWarning(t *testing.T)
  func TestReviewLogger_LogInfo(t *testing.T)
  ```

- [ ] **Run TEST** - should pass
  ```bash
  go test ./internal/adapter/observability/... -v
  ```

**Files Created**:
- `internal/usecase/review/logger.go`
- `internal/adapter/observability/logger.go`
- `internal/adapter/observability/logger_test.go`

---

### 4.2. Update Orchestrator Dependencies ⏱️ 15 min

- [ ] **Read** `internal/usecase/review/orchestrator.go`
- [ ] **Add** Logger field to OrchestratorDeps
  ```go
  type OrchestratorDeps struct {
      Git       GitClient
      Providers map[string]Provider
      Store     Store
      Logger    Logger  // NEW: Optional structured logger
  }
  ```
- [ ] **Verify** existing tests still compile (may need to update)
- [ ] **Run** orchestrator tests
  ```bash
  go test ./internal/usecase/review/... -v
  ```

**Files Modified**:
- `internal/usecase/review/orchestrator.go`
- `internal/usecase/review/orchestrator_test.go` (if needed)

---

### 4.3. Replace fmt.Printf in Orchestrator ⏱️ 45 min

- [ ] **Update** line ~244 (failed to create run)
  ```go
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

- [ ] **Update** line ~349 (failed to save review)
  ```go
  if err := o.SaveReviewToStore(ctx, runID, review); err != nil {
      if o.deps.Logger != nil {
          o.deps.Logger.LogWarning(ctx, "failed to save review to store", map[string]interface{}{
              "runID":    runID,
              "provider": name,
              "error":    err.Error(),
          })
      } else {
          log.Printf("warning: failed to save review to store: %v\n", err)
      }
  }
  ```

- [ ] **Update** line ~398 (failed to update run cost)
  ```go
  if err := o.deps.Store.UpdateRunCost(ctx, runID, totalCost); err != nil {
      if o.deps.Logger != nil {
          o.deps.Logger.LogWarning(ctx, "failed to update run cost", map[string]interface{}{
              "runID":     runID,
              "totalCost": totalCost,
              "error":     err.Error(),
          })
      } else {
          log.Printf("warning: failed to update run cost: %v\n", err)
      }
  }
  ```

- [ ] **Write TEST** for logging behavior
  ```go
  func TestOrchestrator_LogsWarnings(t *testing.T)
  func TestOrchestrator_FallsBackWhenNoLogger(t *testing.T)
  ```

- [ ] **Run TEST** - should pass
  ```bash
  go test ./internal/usecase/review/... -v
  ```

**Files Modified**:
- `internal/usecase/review/orchestrator.go`
- `internal/usecase/review/orchestrator_test.go`

---

### 4.4. Wire Logger from Main ⏱️ 30 min

- [ ] **Read** `cmd/bop/main.go`
- [ ] **Import** observability package
  ```go
  import "github.com/delightfulhammers/bop/internal/adapter/observability"
  ```

- [ ] **Create** ReviewLogger after observability logger setup
  ```go
  // Create observability logger (already exists)
  obsLogger := llmhttp.NewDefaultLogger(logLevel, logFormat, redact)

  // Create review logger adapter
  reviewLogger := observability.NewReviewLogger(obsLogger)
  ```

- [ ] **Wire** into orchestrator
  ```go
  orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
      Git:       gitClient,
      Providers: providers,
      Store:     reviewStore,
      Logger:    reviewLogger,  // NEW
  })
  ```

- [ ] **Replace** store initialization warnings (~line 88, 93)
  - Use reviewLogger for consistency
  - Keep fallback for errors before logger creation

- [ ] **Test** manually with JSON output
  ```bash
  ./bop review branch HEAD --base main --log-format json 2>&1 | jq .
  ```

- [ ] **Test** manually with human output
  ```bash
  ./bop review branch HEAD --base main --log-format human
  ```

- [ ] **Verify** structured fields appear in logs

**Files Modified**:
- `cmd/bop/main.go`

---

## Final Verification (30 minutes)

### Build and Test

- [ ] **Format** all code
  ```bash
  mage format
  ```

- [ ] **Run** full test suite
  ```bash
  mage test
  ```

- [ ] **Run** with race detector
  ```bash
  go test -race ./...
  ```

- [ ] **Build** project
  ```bash
  mage build
  ```

- [ ] **Verify** binary works
  ```bash
  ./bop --version
  ./bop review --help
  ```

---

### Manual Integration Testing

- [ ] **Test** structured logging with JSON format
  ```bash
  ./bop review branch HEAD~5..HEAD --log-format json 2>&1 | jq .
  ```

- [ ] **Test** structured logging with human format
  ```bash
  ./bop review branch HEAD~5..HEAD --log-format human
  ```

- [ ] **Test** retry logic with invalid API key (should retry and fail gracefully)

- [ ] **Test** context cancellation (Ctrl+C during review)

- [ ] **Verify** no resource leaks in logs

---

## Documentation Updates

- [ ] **Update** ROADMAP.md
  - Move completed items from "Known Issues" to "Recently Fixed"
  - Update status dates

- [ ] **Update** PRODUCTION_HARDENING_DESIGN.md
  - Mark as "Completed"
  - Add completion date
  - Document any deviations from design

- [ ] **Create** CHANGELOG.md entry for v0.1.1
  ```markdown
  ## [0.1.1] - 2025-10-22

  ### Fixed
  - Response body leak prevention in all HTTP clients
  - RetryWithBackoff edge case with context cancellation
  - Structured logging throughout orchestrator and main

  ### Improved
  - API key redaction format now more explicit
  - SARIF writer validates cost before serialization
  - Magic numbers replaced with named constants
  ```

- [ ] **Archive** design doc to `docs/archive/`
  ```bash
  mv PRODUCTION_HARDENING_DESIGN.md docs/archive/
  ```

- [ ] **Archive** this checklist to `docs/archive/`
  ```bash
  mv PRODUCTION_HARDENING_CHECKLIST.md docs/archive/
  ```

---

## Commit Strategy

Make atomic commits for each task:

1. ✅ **Commit**: "Add named constant for seed mask"
2. ✅ **Commit**: "Add SARIF cost validation for NaN and Inf"
3. ✅ **Commit**: "Improve API key redaction format"
4. ✅ **Commit**: "Fix RetryWithBackoff context cancellation edge case"
5. ✅ **Commit**: "Audit HTTP clients for response body leaks"
6. ✅ **Commit**: "Add structured logging interfaces and adapters"
7. ✅ **Commit**: "Replace unstructured logging in orchestrator"
8. ✅ **Commit**: "Wire structured logging from main"
9. ✅ **Commit**: "Update documentation for v0.1.1 release"

---

## Success Criteria

- ✅ All tests pass (120+ tests)
- ✅ Race detector shows no warnings
- ✅ No `fmt.Printf` in production code
- ✅ All edge cases have tests
- ✅ Code quality improvements applied consistently
- ✅ Documentation updated
- ✅ Ready for v0.1.1 release

---

## Time Tracking

| Task | Estimated | Actual | Notes |
|------|-----------|--------|-------|
| Quick Wins | 35 min | | |
| Retry Edge Case | 45 min | | |
| Response Body Audit | 1 hour | | |
| Structured Logging | 2 hours | | |
| Verification | 30 min | | |
| **Total** | **4.5 hours** | | |

---

## Notes

Use this section to track any deviations, decisions, or issues encountered during implementation.

-
