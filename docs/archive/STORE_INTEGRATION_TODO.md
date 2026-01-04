# Store Integration Implementation Checklist

Status: ✅ COMPLETE (manual testing optional)
Started: 2025-10-21
Completed: 2025-10-21

## Goal

Integrate the SQLite persistence layer with the review orchestrator so that all review runs, provider reviews, and findings are automatically persisted to the database.

## Phase 1: Utility Functions (TDD) ✅ COMPLETE

### 1.1 Run ID Generation ✅
- [x] Write tests for GenerateRunID (util_test.go)
  - [x] Test ID format (run-TIMESTAMP-HASH)
  - [x] Test uniqueness for same refs at different times
  - [x] Test sortability by timestamp
  - [x] Test hash determinism
- [x] Implement GenerateRunID (util.go)

### 1.2 Finding Hash Generation ✅
- [x] Write tests for GenerateFindingHash (util_test.go)
  - [x] Test same finding produces same hash
  - [x] Test case insensitivity in description
  - [x] Test whitespace normalization
  - [x] Test different findings produce different hashes
- [x] Implement GenerateFindingHash (util.go)

### 1.3 Review ID Generation ✅
- [x] Write tests for GenerateReviewID (util_test.go)
  - [x] Test format (review-RUNID-PROVIDER)
  - [x] Test uniqueness per run+provider
- [x] Implement GenerateReviewID (util.go)

### 1.4 Finding ID Generation ✅
- [x] Write tests for GenerateFindingID (util_test.go)
  - [x] Test format (finding-REVIEWID-INDEX)
  - [x] Test index padding
- [x] Implement GenerateFindingID (util.go)

### 1.5 Config Hash Generation ✅
- [x] Write tests for CalculateConfigHash (util_test.go)
  - [x] Test determinism (same input = same hash)
  - [x] Test sensitivity (different input = different hash)
  - [x] Test all fields included
- [x] Implement CalculateConfigHash (util.go)

## Phase 2: Configuration Updates (TDD) ✅ COMPLETE

### 2.1 Config Structure ✅
- [x] Add StoreConfig struct to internal/config (config.go)
- [x] Add Store field to Config struct (config.go)
- [x] Write tests for config loading with store options (config_test.go)

### 2.2 Default Configuration ✅
- [x] Implement default store path ~/.config/bop/reviews.db (loader.go)
- [x] Add store defaults to setDefaults() (loader.go)
- [x] Write tests for defaults (config_test.go)

### 2.3 Environment Variables ✅
- [x] Test CR_STORE_ENABLED override (config_test.go)
- [x] Test CR_STORE_PATH override (config_test.go)

## Phase 3: Orchestrator Integration (TDD) ✅ COMPLETE

### 3.1 Dependencies Update ✅
- [x] Add Store to OrchestratorDeps (orchestrator.go)
- [x] Update validateDependencies (store is optional) (orchestrator.go)
- [x] Write test for orchestrator with nil store (orchestrator_test.go)

### 3.2 Helper Methods ✅
- [x] Write tests for saveReviewToStore (orchestrator_test.go)
  - [x] Test review record creation
  - [x] Test finding records creation
  - [x] Test finding hash generation
  - [x] Test error handling
- [x] Implement saveReviewToStore (orchestrator.go)

### 3.3 Integration Points ✅
- [x] Write test for run creation before review (orchestrator_test.go)
  - [x] Test run ID generation
  - [x] Test config hash calculation
  - [x] Test timestamp recording
- [x] Implement run creation in ReviewBranch() (orchestrator.go)
- [x] Write test for review saving after provider completes (orchestrator_test.go)
- [x] Integrate saveReviewToStore in provider goroutine (orchestrator.go)
- [x] Write test for merged review saving (orchestrator_test.go)
- [x] Integrate saveReviewToStore for merged review (orchestrator.go)

### 3.4 Error Handling ✅
- [x] Write test for store creation failure (orchestrator_test.go)
- [x] Write test for save failure (orchestrator_test.go - logs but continues)
- [x] Implement warning logs for store failures (orchestrator.go)

## Phase 4: Main Function Updates ✅ COMPLETE

### 4.1 Store Initialization ✅
- [x] Add store initialization in cmd/bop/main.go (main.go)
- [x] Add directory creation with error handling (main.go)
- [x] Add defer reviewStore.Close() (main.go)
- [x] Wire store into orchestrator deps (main.go)

### 4.2 Config Loading ✅
- [x] Load store config from file (main.go)
- [x] Handle missing config gracefully (main.go)
- [x] Test with store enabled (verified via tests)
- [x] Test with store disabled (verified via tests)

## Phase 5: Integration Testing ✅ COMPLETE

### 5.1 End-to-End Tests ✅
- [x] Write test: review with store enabled (orchestrator_test.go)
  - [x] Verify run created
  - [x] Verify all provider reviews saved
  - [x] Verify all findings saved
  - [x] Verify merged review saved
  - [x] Verify finding hashes correct
  - [x] Verify timestamps recorded
- [x] Write test: review without store - backward compat (orchestrator_test.go)
- [x] Write test: review with store disabled in config (orchestrator_test.go)
- [x] Write test: review with store initialization failure (orchestrator_test.go)

### 5.2 Concurrent Review Tests ✅
- [x] Write test: multiple concurrent reviews (orchestrator_test.go)
- [x] Verify no database locks (SQLite handles concurrent reads)
- [x] Verify all runs persisted correctly (orchestrator_test.go)

## Phase 6: Documentation & Verification ✅ COMPLETE

### 6.1 Documentation ✅
- [x] Update MAIN_INTEGRATION_CHECKLIST.md with store integration status
- [x] Update CONFIGURATION.md with store configuration examples
- [x] Add example bop.yaml with store config (in docs)
- [x] Document environment variable overrides (CONFIGURATION.md)

### 6.2 Verification ✅
- [x] Run full test suite (mage ci) - all tests passing
- [x] Verify all existing tests still pass
- [x] Verify new tests pass
- [ ] Manual testing: Test with real review (create DB file)
- [ ] Manual testing: Inspect DB with sqlite3 CLI to verify data
- [ ] Manual testing: Test with store disabled (ensure backward compat)

## Acceptance Criteria

- [x] All utility functions implemented and tested
- [x] Store configuration working (file + env vars)
- [x] Reviews automatically persisted when store enabled
- [x] Reviews work without store (backward compatible)
- [x] Store failures log warnings but don't break reviews
- [x] Finding hashes enable de-duplication
- [x] Run IDs are sortable and unique
- [x] All tests passing (existing + new)
- [x] Database created at default path automatically
- [x] Documentation updated
- [ ] Manual verification with real database (optional)

## Testing Commands

```bash
# Run all tests
mage ci

# Run specific tests
go test ./internal/store/... -v
go test ./internal/usecase/review/... -v
go test ./cmd/bop/... -v

# Test with real review
./bop review branch main --target feature --output ./reviews

# Inspect database
sqlite3 ~/.config/bop/reviews.db
> .tables
> SELECT * FROM runs;
> SELECT * FROM reviews;
> SELECT * FROM findings;
> .quit
```

## Notes

- Store is optional - orchestrator must work without it
- All store operations should fail gracefully with warnings
- Finding hashes use normalized descriptions for better de-duplication
- Run IDs include timestamp for sorting
- Config hash helps track which settings produced which results
- Use in-memory SQLite (:memory:) for all tests
