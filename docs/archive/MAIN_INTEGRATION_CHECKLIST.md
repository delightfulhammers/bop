# Main.go Store Integration Checklist

Status: Phase 1-3 Complete, Phase 4-5 Pending Manual Testing
Date: 2025-10-21
Last Updated: 2025-10-21

## Goal
Wire the SQLite store into the main application so that reviews are automatically persisted when the store is enabled in configuration.

## Implementation Steps

### Phase 1: Store Adapter Bridge ✅

#### 1. Create Store Adapter ✅
- [x] Create internal/adapter/store/bridge.go
- [x] Implement StoreAdapter that wraps sqlite.Store
- [x] Convert between store.* and review.* types
- [x] Write tests for adapter conversions
- [x] Ensure nil-safety

The orchestrator expects `review.Store` interface but we have `store.Store` with different types. We need an adapter.

```go
// internal/adapter/store/bridge.go
type StoreAdapter struct {
    store store.Store
}

func NewStoreAdapter(s store.Store) *StoreAdapter {
    return &StoreAdapter{store: s}
}

func (a *StoreAdapter) CreateRun(ctx context.Context, run review.StoreRun) error {
    storeRun := store.Run{
        RunID:      run.RunID,
        Timestamp:  run.Timestamp,
        // ... convert fields
    }
    return a.store.CreateRun(ctx, storeRun)
}
```

### Phase 2: Main Function Integration ✅

#### 2. Store Initialization in main.go ✅
- [x] Check if store is enabled in config
- [x] Create store directory if doesn't exist
- [x] Initialize SQLite store with config path
- [x] Handle initialization errors gracefully
- [x] Create store adapter
- [x] Add defer store.Close()
- [x] Wire adapter into orchestrator deps

#### 3. Error Handling ✅
- [x] Log clear error messages for store failures
- [x] Application continues if store init fails
- [x] Disabled store doesn't break application

### Phase 3: Integration Testing ✅

#### 4. End-to-End Store Tests ✅
- [x] Test: review with store enabled saves to DB
- [x] Test: verify run record created
- [x] Test: verify provider reviews saved
- [x] Test: verify findings saved with hashes
- [x] Test: verify merged review saved
- [x] Test: review works with store disabled
- [x] Test: review works when store init fails

#### 5. Test Utilities ✅
- [x] Helper to create test database (mockStore)
- [x] Helper to query and verify database content (mockStore fields)
- [x] Helper to count records by type (len() on mockStore slices)
- [x] Cleanup helpers (mockStore.closed flag)

### Phase 4: Verification & Documentation

#### 6. Manual Testing
- [ ] Build the application
- [ ] Run review with store enabled
- [ ] Inspect database with sqlite3
- [ ] Verify table structure
- [ ] Verify data content
- [ ] Test with store disabled

#### 7. Database Inspection Commands
```bash
# Build application
mage build

# Run review
./bop review branch main --target feature

# Inspect database
sqlite3 ~/.config/bop/reviews.db

# Check tables
.tables

# Verify data
SELECT * FROM runs;
SELECT * FROM reviews;
SELECT * FROM findings LIMIT 5;
SELECT * FROM precision_priors;

# Exit
.quit
```

#### 8. Documentation Updates
- [ ] Update STORE_INTEGRATION_TODO.md progress
- [ ] Update PHASE3_TODO.md progress
- [ ] Update IMPLEMENTATION_PLAN.md
- [ ] Add example configuration to README
- [ ] Document database schema
- [ ] Document how to disable store

### Phase 5: Final Verification

#### 9. CI/CD Pipeline ✅
- [x] Run mage ci
- [x] All tests passing
- [x] No regressions
- [x] Build succeeds

#### 10. Code Quality ✅
- [x] Run mage format
- [x] Code properly formatted
- [x] No linting errors
- [x] All imports organized

## Implementation Details

### Store Adapter Pattern

The adapter pattern is needed because:
1. `store` package defines `store.Run`, `store.ReviewRecord`, etc.
2. `orchestrator` package defines `review.StoreRun`, `review.StoreReview`, etc.
3. We can't import `store` in `review` (would create circular dependency)
4. Solution: Create adapter in adapter layer that implements `review.Store`

### Directory Structure
```
internal/
  adapter/
    store/
      bridge.go        # Adapter implementing review.Store
      bridge_test.go   # Adapter tests
      sqlite/
        store.go       # SQLite implementation
  usecase/
    review/
      orchestrator.go  # Defines Store interface
  store/
    store.go          # Domain store types
```

### Configuration Flow
```
Config Loaded
    ↓
Store Enabled? → No → Continue without store
    ↓ Yes
Create Directory
    ↓
Initialize SQLite Store
    ↓
Wrap in Adapter
    ↓
Pass to Orchestrator
    ↓
Reviews Persisted
```

## Acceptance Criteria

- [x] Store initialized from configuration
- [x] Reviews automatically persisted when enabled
- [x] Application works with store disabled
- [x] Store failures don't crash application
- [x] All tests passing
- [ ] Database created at correct path (requires manual testing)
- [ ] Data structure verified in DB (requires manual testing)
- [ ] Documentation complete (in progress)
- [ ] Manual testing successful (pending)

## Error Scenarios to Test

1. Store enabled but directory can't be created
2. Store enabled but database can't be opened
3. Store enabled but write fails
4. Store disabled in config
5. Store config missing entirely
6. Invalid database path
7. Permission denied on database file

## Success Metrics

- [ ] Reviews persist to database successfully
- [ ] Database queries return correct data
- [ ] Finding hashes enable de-duplication
- [ ] Run IDs are sortable by timestamp
- [ ] No performance degradation
- [ ] No memory leaks with store enabled
