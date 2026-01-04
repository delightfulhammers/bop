# Code Quality Improvements - Technical Design

**Version**: v0.1.3
**Date**: 2025-10-22
**Status**: Implementation

## Problem Statement

Code review feedback from AI providers identified two code quality issues:

1. **Duplicated JSON Parsing Logic** - All 4 LLM clients (OpenAI, Anthropic, Gemini, Ollama) duplicate JSON extraction and parsing logic
2. **Duplicated ID Generation** - ID generation functions appear in both `internal/store/util.go` and `internal/usecase/review/store_helpers.go`

## Investigation Results

### 1. JSON Parsing Duplication ✅ VALID ISSUE

**Current state**: Each LLM client has its own `parseReviewJSON` and `extractJSONFromMarkdown` functions:

**OpenAI** (`internal/adapter/llm/openai/client.go:376-385`):
```go
func extractJSONFromMarkdown(text string) string {
    re := regexp.MustCompile("(?s)```(?:json)?\\s*({.*?})\\s*```")
    matches := re.FindStringSubmatch(text)
    if len(matches) > 1 {
        return matches[1]
    }
    return ""
}
```

**Anthropic** (`internal/adapter/llm/anthropic/client.go:386`):
```go
jsonPattern := regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)```")
matches := jsonPattern.FindStringSubmatch(text)
```

**Similar implementations in**: Gemini, Ollama

**Problems**:
- Code duplication (DRY principle violation)
- Slightly different regex patterns (potential inconsistency)
- Each client compiles its own regexp (inefficiency)
- Harder to maintain (fixes need to be applied 4 times)

**Solution**: Extract to `internal/adapter/llm/http/json.go`

### 2. ID Generation Duplication ❌ FALSE POSITIVE

**Investigation findings**:

```go
// internal/store/util.go
func GenerateRunID(timestamp time.Time, baseRef, targetRef string) string

// internal/usecase/review/store_helpers.go
func generateRunID(timestamp time.Time, baseRef, targetRef string) string
```

**Why duplication exists** (from code comment):
```go
// generateFindingHash creates a deterministic hash for a finding.
// Duplicates the implementation from store package to avoid circular dependency.
```

**Reason**: Clean Architecture / Dependency Rule
- `internal/store` is an adapter layer
- `internal/usecase/review` is a use case layer
- Use case layer CANNOT import adapter layer (violates dependency direction)
- Store adapter implements interfaces defined in use case layer
- If use case imported store utils → circular dependency

**Verdict**: **This is intentional and correct design**, not a problem to fix.

**Recommendation**: Add test to ensure implementations stay in sync.

## Design

### Solution: Extract Shared JSON Parsing

**New file**: `internal/adapter/llm/http/json.go`

```go
package http

import (
    "encoding/json"
    "fmt"
    "regexp"
    "strings"
    "sync"

    "github.com/delightfulhammers/bop/internal/domain"
)

var (
    // Compile regex once and reuse (thread-safe)
    jsonBlockRegex     = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")
    jsonBlockRegexOnce sync.Once
)

// ExtractJSONFromMarkdown extracts JSON from markdown code blocks.
// Supports both ```json and ``` code blocks.
// Returns extracted JSON or original text if no code block found.
func ExtractJSONFromMarkdown(text string) string {
    matches := jsonBlockRegex.FindStringSubmatch(text)
    if len(matches) > 1 {
        return strings.TrimSpace(matches[1])
    }
    // No code block found, return original text (might be raw JSON)
    return strings.TrimSpace(text)
}

// ParseReviewResponse parses JSON into a structured review response.
// Handles both markdown-wrapped and raw JSON responses.
func ParseReviewResponse(text string) (summary string, findings []domain.Finding, err error) {
    // Extract JSON from markdown if present
    jsonText := ExtractJSONFromMarkdown(text)

    // Parse into intermediate structure
    var result struct {
        Summary  string           `json:"summary"`
        Findings []domain.Finding `json:"findings"`
    }

    if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
        return "", nil, fmt.Errorf("failed to parse JSON review: %w", err)
    }

    return result.Summary, result.Findings, nil
}
```

### Migration Plan

**Phase 1: Create Shared Utilities** (TDD)
1. Write tests for `ExtractJSONFromMarkdown`
   - Test with ```json code block
   - Test with ``` code block
   - Test with raw JSON (no code block)
   - Test with malformed markdown
   - Test with empty string
   - Test with multiple code blocks (should take first)

2. Write tests for `ParseReviewResponse`
   - Test with valid JSON in markdown
   - Test with raw JSON
   - Test with invalid JSON (should error)
   - Test with missing fields
   - Test with empty findings array

3. Implement functions (Green phase)

**Phase 2: Update Clients** (One at a time)
1. Update OpenAI client
   - Replace `extractJSONFromMarkdown` with `http.ExtractJSONFromMarkdown`
   - Simplify `parseReviewJSON` to use shared function
   - Run OpenAI tests → should pass

2. Update Anthropic client
3. Update Gemini client
4. Update Ollama client

**Phase 3: Cleanup**
- Remove old `extractJSONFromMarkdown` functions from each client
- Remove old `parseReviewJSON` implementations
- Verify all tests pass

### ID Generation - Documentation Only

**Action**: Add comment and test to document intentional duplication

**New test**: `internal/usecase/review/store_helpers_test.go`
```go
func TestIDGenerationMatchesStorePackage(t *testing.T) {
    // This test ensures review package ID generation stays in sync with store package.
    // NOTE: These functions are intentionally duplicated to avoid circular dependencies.
    // The store package is an adapter implementing interfaces from the review package,
    // so review cannot import store utilities.

    ts := time.Date(2025, 10, 22, 10, 30, 0, 0, time.UTC)

    // Generate IDs using review package (private functions via SaveReviewToStore)
    runID1 := generateRunID(ts, "main", "feature")

    // Generate IDs using store package (public functions)
    runID2 := store.GenerateRunID(ts, "main", "feature")

    // They should be identical
    assert.Equal(t, runID1, runID2, "generateRunID implementations must match")

    // Test review ID generation
    reviewID1 := generateReviewID(runID1, "openai")
    reviewID2 := store.GenerateReviewID(runID2, "openai")
    assert.Equal(t, reviewID1, reviewID2, "generateReviewID implementations must match")

    // Test finding ID generation
    findingID1 := generateFindingID(reviewID1, 5)
    findingID2 := store.GenerateFindingID(reviewID2, 5)
    assert.Equal(t, findingID1, findingID2, "generateFindingID implementations must match")
}
```

## Testing Strategy

### JSON Parsing Tests
- Unit tests for extraction with various markdown formats
- Unit tests for parsing with valid/invalid JSON
- Integration tests in each LLM client (should still pass)
- Edge case: Multiple code blocks (take first)
- Edge case: Nested code blocks
- Edge case: Unicode in JSON

### ID Generation Test
- Sync test to catch accidental divergence
- Run with CI to ensure implementations don't drift

## Benefits

### JSON Parsing Extraction
✅ **DRY Principle** - Single implementation, maintained in one place
✅ **Consistency** - All clients use identical parsing logic
✅ **Performance** - Regex compiled once, reused by all clients
✅ **Maintainability** - Bug fixes apply to all clients automatically
✅ **Testability** - Shared code has comprehensive test coverage

### ID Generation Documentation
✅ **Clarity** - Documents why duplication exists (not a mistake)
✅ **Safety** - Test prevents accidental divergence
✅ **Architecture** - Reinforces clean architecture principles

## Non-Goals

- NOT fixing ID generation "duplication" (it's intentional)
- NOT changing response parsing structure (only extraction logic)
- NOT breaking existing client APIs

## Risks

**Low risk**:
- Changes are isolated to JSON parsing logic
- Each client will be updated and tested independently
- Existing tests will catch any regressions
- Shared code is simpler than individual implementations

## Rollback Plan

If issues arise:
1. Revert individual client changes (one at a time)
2. Shared utilities remain (no harm if unused)
3. Clients can fall back to old implementation

## Time Estimate

- Create shared utilities with tests: 1 hour
- Update 4 clients (one at a time): 1.5 hours
- ID generation test and documentation: 30 minutes
- **Total**: ~3 hours

## Success Criteria

✅ All existing tests pass
✅ New shared JSON parsing tests pass (10+ tests)
✅ ID generation sync test passes
✅ All 4 clients use shared parsing utilities
✅ Zero code duplication for JSON parsing
✅ ID generation duplication is documented as intentional
✅ Zero regressions
