# Phase 3 Implementation Checklist

Status: Week 1 ✅ COMPLETE, Weeks 2-4 🔄 DEFERRED
Started: 2025-10-21
Updated: 2025-10-21

## Week 1: SQLite Store Implementation ✅ COMPLETE

### 1.1 Store Package Structure ✅
- [x] Create `internal/store/store.go` with interface definitions
- [x] Define all data structures (Run, ReviewRecord, FindingRecord, Feedback, PrecisionPrior)
- [x] Add method for calculating precision from Beta distribution

### 1.2 SQLite Adapter ✅
- [x] Create `internal/adapter/store/sqlite/` package
- [x] Implement schema creation with all tables and indexes
- [ ] Add migration support for schema versioning (deferred to later)

### 1.3 CRUD Operations (TDD) ✅
- [x] Write tests for CreateRun / GetRun / ListRuns
- [x] Implement CreateRun / GetRun / ListRuns
- [x] Write tests for SaveReview / GetReviewsByRun
- [x] Implement SaveReview / GetReviewsByRun / GetReview
- [x] Write tests for SaveFindings / GetFindingsByReview
- [x] Implement SaveFindings / GetFindingsByReview / GetFinding
- [x] Write tests for RecordFeedback / GetFeedbackForFinding
- [x] Implement RecordFeedback / GetFeedbackForFinding

### 1.4 Precision Priors (TDD) ✅
- [x] Write tests for GetPrecisionPriors
- [x] Implement GetPrecisionPriors
- [x] Write tests for UpdatePrecisionPrior with Beta distribution
- [x] Implement UpdatePrecisionPrior
- [x] Write test for Precision() calculation (α / (α + β))
- [x] Verify uniform prior starts at 0.5 (α=1, β=1)

### 1.5 Store Integration ✅ COMPLETE
- [x] Add Store to OrchestratorDeps (orchestrator.go)
- [x] Update orchestrator to save runs after review completion (orchestrator.go)
- [x] Update orchestrator to save reviews and findings (orchestrator.go - saveReviewToStore)
- [x] Add config options for store (config.go - StoreConfig)
- [x] Update default config to enable store at ~/.config/bop/reviews.db (loader.go)
- [x] Wire store into main.go (main.go - store initialization and adapter)

### 1.6 Utility Functions ✅ COMPLETE
- [x] Implement GenerateRunID() using timestamp + hash (util.go)
- [x] Implement CalculateConfigHash() for deterministic hashing (util.go)
- [x] Implement finding hash generation (util.go - GenerateFindingHash)
- [x] Implement GenerateReviewID() (util.go)
- [x] Implement GenerateFindingID() (util.go)
- [x] Add Close() method with proper cleanup (sqlite/store.go)

## Week 2: Basic TUI Implementation

### 2.1 TUI Package Setup
- [ ] Add Bubble Tea, Bubbles, and Lipgloss dependencies to go.mod
- [ ] Create `internal/adapter/tui/` package structure
- [ ] Define Model struct with state machine
- [ ] Define all message types (reviewsLoadedMsg, findingsLoadedMsg, etc.)

### 2.2 Review List View (TDD)
- [ ] Write tests for review list model initialization
- [ ] Implement review list model using bubbles/list
- [ ] Write tests for loading runs from store
- [ ] Implement async loading of runs
- [ ] Write tests for navigation (up/down, enter to select)
- [ ] Implement navigation handlers
- [ ] Add styling with lipgloss

### 2.3 Finding List View (TDD)
- [ ] Write tests for finding list model
- [ ] Implement finding list view
- [ ] Write tests for loading findings for a selected run
- [ ] Implement async loading of findings
- [ ] Write tests for finding item rendering (show severity, category, file)
- [ ] Implement finding item rendering
- [ ] Add color coding by severity (red=critical, yellow=medium, etc.)

### 2.4 Finding Detail View (TDD)
- [ ] Write tests for detail view model
- [ ] Implement viewport for scrollable content
- [ ] Write tests for rendering finding details
- [ ] Implement detail rendering (description, suggestion, evidence)
- [ ] Add syntax highlighting for code snippets (if possible)

### 2.5 TUI Navigation & State
- [ ] Implement state transitions (list → detail → back)
- [ ] Write tests for key binding handlers (j/k, enter, q, esc)
- [ ] Implement all key bindings
- [ ] Add help view (press '?' to show)
- [ ] Handle window resize events

### 2.6 CLI Integration
- [ ] Add `tui` command to CLI (cmd/bop/main.go)
- [ ] Wire up store dependency
- [ ] Add --db-path flag to override default location
- [ ] Test TUI launches and displays data

## Week 3: Feedback & Intelligence

### 3.1 Feedback Capture in TUI (TDD)
- [ ] Write tests for 'a' (accept) key binding
- [ ] Implement accept feedback handler
- [ ] Write tests for 'r' (reject) key binding
- [ ] Implement reject feedback handler
- [ ] Write tests for feedback recording flow
- [ ] Show confirmation message after feedback
- [ ] Update finding display to show feedback status
- [ ] Test that feedback persists to database

### 3.2 Feedback Processor Use Case (TDD)
- [ ] Create `internal/usecase/feedback/` package
- [ ] Write tests for ProcessFeedback
- [ ] Implement ProcessFeedback (record + update priors)
- [ ] Write tests for extracting provider/category from finding
- [ ] Verify precision prior updates correctly (α+1 for accept, β+1 for reject)

### 3.3 Statistics View (TDD)
- [ ] Write tests for statistics view model
- [ ] Implement statistics view layout
- [ ] Write tests for loading precision priors
- [ ] Display precision priors by provider and category
- [ ] Calculate and display overall provider accuracy
- [ ] Show total feedback count per provider
- [ ] Add 's' key binding to open statistics view

### 3.4 Intelligent Merger v2 (TDD)
- [ ] Create `internal/usecase/merge/intelligent.go`
- [ ] Write tests for loading precision priors
- [ ] Implement priors loading in Merge()
- [ ] Write tests for scoring algorithm
- [ ] Implement calculateScore() with weighted formula
- [ ] Write tests for finding selection (highest precision wins)
- [ ] Implement selectBestFinding()
- [ ] Write tests for sorting by score
- [ ] Update merger to use intelligent strategy when configured

### 3.5 Merger Configuration
- [ ] Add merge.strategy config option ("consensus" or "intelligent")
- [ ] Add merge.weights config for score tuning
- [ ] Update orchestrator to use intelligent merger when enabled
- [ ] Wire store dependency into merger
- [ ] Add tests for merger with mock store

### 3.6 Integration Testing
- [ ] Write end-to-end test: review → save → load TUI → feedback
- [ ] Verify precision priors update after feedback
- [ ] Verify intelligent merger uses updated priors
- [ ] Test with multiple feedback cycles

## Week 4: Enhanced Redaction & Polish

### 4.1 Entropy-Based Detection (TDD)
- [ ] Create `internal/redaction/entropy.go`
- [ ] Write tests for shannonEntropy() with known values
- [ ] Implement shannonEntropy() calculation
- [ ] Write tests for IsHighEntropy() with threshold
- [ ] Implement IsHighEntropy()
- [ ] Write tests for ScanLine() finding high-entropy words
- [ ] Implement ScanLine()
- [ ] Test with real secret examples (API keys, tokens)

### 4.2 Redaction Engine v2 (TDD)
- [ ] Add EntropyDetector to Engine struct
- [ ] Write tests for entropy detection integration
- [ ] Integrate entropy scanning into Redact() method
- [ ] Add config options (entropyThreshold, entropyMinLength)
- [ ] Write tests for combined regex + entropy detection
- [ ] Ensure stable placeholder generation
- [ ] Test redaction performance with large diffs

### 4.3 Configuration Updates
- [ ] Add store.enabled and store.path config options
- [ ] Add tui.enabled and tui.defaultView config
- [ ] Add redaction.entropyThreshold and entropyMinLength
- [ ] Update config loader with new defaults
- [ ] Write tests for config loading
- [ ] Document all new config options

### 4.4 Documentation
- [ ] Update ARCHITECTURE.md with Phase 3 components
- [ ] Update IMPLEMENTATION_PLAN.md marking Phase 3 complete
- [ ] Create TUI_USAGE.md with screenshots/examples
- [ ] Update README.md with TUI command
- [ ] Document precision prior system
- [ ] Add examples of feedback workflow
- [ ] Document entropy detection configuration

### 4.5 Testing & Quality
- [ ] Run full test suite (`mage ci`)
- [ ] Verify >80% code coverage for new packages
- [ ] Test with real repositories of various sizes
- [ ] Performance test: ensure <100ms for TUI navigation
- [ ] Memory test: check for leaks in long-running TUI sessions
- [ ] Test cross-platform (macOS, Linux)

### 4.6 Edge Cases & Error Handling
- [ ] Handle corrupted/missing database gracefully
- [ ] Handle empty review history in TUI
- [ ] Handle concurrent access to SQLite (write locks)
- [ ] Test with very large finding lists (1000+ findings)
- [ ] Test with missing precision priors (use defaults)
- [ ] Add proper error messages for TUI failures

### 4.7 Polish & UX
- [ ] Add loading spinners for async operations
- [ ] Improve color scheme and contrast
- [ ] Add pagination for large lists
- [ ] Implement search/filter in TUI
- [ ] Add keyboard shortcut reference footer
- [ ] Smooth scrolling in viewport
- [ ] Add provider icons/badges in displays

## Dependencies to Add

```bash
go get github.com/mattn/go-sqlite3@v1.14.18
go get github.com/charmbracelet/bubbletea@v0.25.0
go get github.com/charmbracelet/bubbles@v0.18.0
go get github.com/charmbracelet/lipgloss@v0.9.1
```

## Testing Commands

```bash
# Run all tests
mage test

# Run specific package tests
go test ./internal/store/...
go test ./internal/adapter/store/sqlite/...
go test ./internal/adapter/tui/...
go test ./internal/usecase/feedback/...
go test ./internal/redaction/...

# Run with coverage
go test -cover ./...

# Run integration tests
go test -tags=integration ./...
```

## Completion Criteria

- [ ] All tests passing (`mage ci`)
- [ ] Code coverage >80% for new packages
- [ ] TUI launches without errors
- [ ] Can navigate all views smoothly
- [ ] Feedback correctly updates precision priors
- [ ] Intelligent merger uses priors for ranking
- [ ] Entropy detection catches test secrets
- [ ] Documentation complete
- [ ] No memory leaks in long-running sessions
- [ ] Performance targets met (<100ms TUI response, <2min review)

## Notes

- Use in-memory SQLite (`:memory:`) for tests
- Consider cgo-free alternative (modernc.org/sqlite) for easier cross-compilation
- Start with α=1, β=1 uniform prior (precision=0.5)
- Entropy threshold 4.5 is a good starting point (tune based on testing)
- Implement minimal TUI first, enhance incrementally
- Keep TUI code in adapter layer (Clean Architecture)
