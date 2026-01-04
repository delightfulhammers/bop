# Code Review Fixes

This document tracks all fixes applied in response to code review feedback from multiple reviewers (Gemini, Claude Phase 1, Claude Phase 2, and Codex).

## Critical Issues Fixed

### 1. Missing Provider Wiring in main.go
**Issue**: The CLI only recognized OpenAI provider, ignoring Anthropic, Gemini, and Ollama configurations.

**Fix**: Updated `buildProviders()` function in `cmd/bop/main.go` to instantiate all four providers:
- Anthropic provider with default model `claude-3-5-sonnet-20241022`
- Gemini provider with default model `gemini-pro`
- Ollama provider with default model `llama2`
- Static provider for testing

**Impact**: Multi-provider runs now work as designed.

### 2. Missing Orchestrator Dependencies
**Issue**: JSON and SARIF writers, and Redaction engine were not instantiated or wired into the orchestrator, causing "orchestrator dependencies missing" errors.

**Fix**:
- Instantiated `json.NewWriter(nowFunc)` at `cmd/bop/main.go:63`
- Instantiated `sarif.NewWriter(nowFunc)` at `cmd/bop/main.go:64`
- Conditionally instantiated `redaction.NewEngine()` when `cfg.Redaction.Enabled` at `cmd/bop/main.go:72-74`
- Wired all dependencies into orchestrator at `cmd/bop/main.go:76-86`

**Impact**: All output formats (Markdown, JSON, SARIF) now work correctly, and secret redaction is properly enabled when configured.

### 3. Duplicate Seed Generation Logic
**Issue**: Two different seed generation algorithms existed - FNV-1a hash in `main.go` and SHA-256 in `internal/determinism/seed.go`.

**Fix**: Removed duplicate `deterministicSeed()` function from `main.go` and used `determinism.GenerateSeed` directly in orchestrator dependencies.

**Impact**: Consistent deterministic behavior across all reviews.

## Warnings Addressed

### 4. Directory Naming Collision
**Issue**: JSON and SARIF writers used `baseRef` for directory naming, causing reviews of different feature branches to overwrite each other under the same `repo_main` directory.

**Fix**:
- Changed `internal/adapter/output/json/writer.go:25` to use `artifact.TargetRef` instead of `artifact.BaseRef`
- Changed `internal/adapter/output/sarif/writer.go:25` to use `artifact.TargetRef` instead of `artifact.BaseRef`
- Updated tests to reflect new directory structure

**Impact**: Each branch review now gets isolated output in its own directory (e.g., `repo_feature-branch`).

### 5. Weak Dependency Validation
**Issue**: Generic "orchestrator dependencies missing" error didn't indicate which dependency was missing.

**Fix**: Added `validateDependencies()` method in `internal/usecase/review/orchestrator.go:112-139` with specific error messages for each required dependency:
- Git engine
- Providers (at least one)
- Merger
- Markdown writer
- JSON writer
- SARIF writer
- Prompt builder
- Seed generator

**Impact**: Developers can now quickly identify and fix configuration issues.

## Enhancements Implemented

### 6. Provider Panic Recovery
**Issue**: If a provider panicked, it could crash the entire orchestrator without reporting results from other providers.

**Fix**: Added `defer` panic recovery in provider goroutines at `internal/usecase/review/orchestrator.go:187-197`:
```go
defer func() {
    if r := recover(); r != nil {
        resultsChan <- struct{...}{
            err: fmt.Errorf("provider %s panicked: %v", name, r)
        }
    }
    wg.Done()
}()
```

**Impact**: Individual provider failures no longer crash the entire review process.

### 7. Improved Error Aggregation
**Issue**: Error handling only reported the first provider failure, hiding other concurrent failures.

**Fix**: Modified error collection at `internal/usecase/review/orchestrator.go:300-306` to aggregate all errors:
```go
if len(errs) > 0 {
    var errMsgs []string
    for _, err := range errs {
        errMsgs = append(errMsgs, err.Error())
    }
    return Result{}, fmt.Errorf("%d provider(s) failed: %s", len(errs), strings.Join(errMsgs, "; "))
}
```

**Impact**: All provider failures are now visible in a single error message.

## Technical Debt Resolved

### 8. strings.Title() Deprecation
**Issue**: Already fixed in Phase 2 implementation.

**Status**: The markdown writer at `internal/adapter/output/markdown/writer.go:52,68` uses `cases.Title(language.English)` from `golang.org/x/text/cases` instead of deprecated `strings.Title()`.

## Test Updates

Updated tests to reflect the directory naming changes:
- `internal/adapter/output/json/writer_test.go`: Added `TargetRef` field and updated expected path to use `test-repo_feature`
- `internal/adapter/output/sarif/writer_test.go`: Updated expected path from `test-repo_main` to `test-repo_feature`

## Verification

All fixes verified with full CI pipeline:
```bash
mage ci
```

All tests passing as of this commit.

## Summary

- **7 critical/warning issues** resolved
- **2 enhancements** implemented
- **1 technical debt** item confirmed resolved
- **2 test suites** updated
- **All CI checks** passing

The codebase is now ready for production use with all identified issues from code reviews addressed.
