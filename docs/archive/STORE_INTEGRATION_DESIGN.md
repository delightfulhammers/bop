# Store Integration Technical Design

Version: 1.0
Date: 2025-10-21
Status: Planning

## 1. Overview

This document details the integration of the SQLite persistence layer with the review orchestrator. After completing the store implementation in Week 1, we need to wire it into the review workflow so that all runs, reviews, and findings are automatically persisted.

## 2. Utility Functions

### 2.1 Run ID Generation

Run IDs must be unique, sortable, and human-readable.

```go
// internal/store/util.go
package store

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "time"
)

// GenerateRunID creates a unique, time-ordered run ID.
// Format: run-<timestamp>-<hash>
// Example: run-20251021T143052Z-a3f9c2
func GenerateRunID(timestamp time.Time, baseRef, targetRef string) string {
    // Use timestamp for ordering
    ts := timestamp.UTC().Format("20060102T150405Z")

    // Create short hash from refs for uniqueness
    input := fmt.Sprintf("%s|%s|%d", baseRef, targetRef, timestamp.UnixNano())
    hash := sha256.Sum256([]byte(input))
    shortHash := hex.EncodeToString(hash[:3]) // 6 character hash

    return fmt.Sprintf("run-%s-%s", ts, shortHash)
}
```

**Benefits:**
- Sortable by timestamp
- Unique even for same refs run at same time
- Human-readable format

### 2.2 Config Hash Generation

Configuration hashing ensures deterministic tracking of settings.

```go
// CalculateConfigHash creates a deterministic hash of the configuration.
// This allows tracking which config was used for each run.
func CalculateConfigHash(cfg interface{}) (string, error) {
    // Serialize config to JSON
    data, err := json.Marshal(cfg)
    if err != nil {
        return "", err
    }

    // Hash the serialized config
    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:]), nil
}
```

### 2.3 Finding Hash Generation

Finding hashes enable de-duplication across providers.

```go
// GenerateFindingHash creates a deterministic hash for a finding.
// Findings with the same hash are considered duplicates.
func GenerateFindingHash(file string, lineStart, lineEnd int, description string) string {
    // Normalize description (lowercase, trim whitespace)
    normalized := strings.ToLower(strings.TrimSpace(description))

    // Create hash input
    input := fmt.Sprintf("%s:%d-%d:%s", file, lineStart, lineEnd, normalized)
    hash := sha256.Sum256([]byte(input))

    return hex.EncodeToString(hash[:])
}
```

### 2.4 Review ID Generation

```go
// GenerateReviewID creates a unique ID for a review.
// Format: review-<run_id>-<provider>
func GenerateReviewID(runID, provider string) string {
    return fmt.Sprintf("review-%s-%s", runID, provider)
}
```

### 2.5 Finding ID Generation

```go
// GenerateFindingID creates a unique ID for a finding.
// Format: finding-<review_id>-<index>
func GenerateFindingID(reviewID string, index int) string {
    return fmt.Sprintf("finding-%s-%04d", reviewID, index)
}
```

## 3. Orchestrator Integration

### 3.1 Updated Dependencies

```go
// internal/usecase/review/orchestrator.go

type OrchestratorDeps struct {
    Git           GitEngine
    Providers     map[string]Provider
    Merger        Merger
    Markdown      MarkdownWriter
    JSON          JSONWriter
    SARIF         SARIFWriter
    Redactor      Redactor
    SeedGenerator SeedFunc
    PromptBuilder PromptBuilder
    Store         store.Store // NEW: Optional store for persistence
}
```

**Note:** Store is optional - the orchestrator works without it for backward compatibility.

### 3.2 Integration Points

The orchestrator will save data at these points:

1. **Before review starts**: Create run record
2. **After each provider completes**: Save review and findings
3. **After merge completes**: Save merged review and findings

```go
func (o *Orchestrator) ReviewBranch(ctx context.Context, req BranchRequest) (Result, error) {
    // ... existing validation and diff retrieval ...

    var runID string

    // 1. Create run record if store available
    if o.deps.Store != nil {
        run := store.Run{
            RunID:      store.GenerateRunID(time.Now(), req.BaseRef, req.TargetRef),
            Timestamp:  time.Now(),
            Scope:      fmt.Sprintf("%s..%s", req.BaseRef, req.TargetRef),
            ConfigHash: calculateConfigHash(req), // utility function
            TotalCost:  0.0, // Will be updated later if cost tracking added
            BaseRef:    req.BaseRef,
            TargetRef:  req.TargetRef,
            Repository: req.Repository,
        }

        if err := o.deps.Store.CreateRun(ctx, run); err != nil {
            // Log warning but don't fail - store is optional
            log.Printf("warning: failed to create run record: %v", err)
        } else {
            runID = run.RunID
        }
    }

    // ... existing parallel provider execution ...

    // 2. After each provider completes, save review and findings
    for name, provider := range o.deps.Providers {
        go func(name string, provider Provider) {
            defer wg.Done()

            review, err := provider.Review(ctx, providerReq)
            if err != nil {
                resultsChan <- ...
                return
            }

            // Save to store if available
            if o.deps.Store != nil && runID != "" {
                if err := o.saveReviewToStore(ctx, runID, review); err != nil {
                    log.Printf("warning: failed to save review: %v", err)
                }
            }

            // ... existing markdown/json/sarif writing ...
        }(name, provider)
    }

    // ... rest of implementation ...
}
```

### 3.3 Helper Method for Saving Reviews

```go
// saveReviewToStore persists a review and its findings to the store.
func (o *Orchestrator) saveReviewToStore(ctx context.Context, runID string, review domain.Review) error {
    // Create review record
    reviewID := store.GenerateReviewID(runID, review.ProviderName)
    reviewRecord := store.ReviewRecord{
        ReviewID:  reviewID,
        RunID:     runID,
        Provider:  review.ProviderName,
        Model:     review.ModelName,
        Summary:   review.Summary,
        CreatedAt: time.Now(),
    }

    if err := o.deps.Store.SaveReview(ctx, reviewRecord); err != nil {
        return err
    }

    // Create finding records
    findings := make([]store.FindingRecord, len(review.Findings))
    for i, f := range review.Findings {
        findings[i] = store.FindingRecord{
            FindingID:   store.GenerateFindingID(reviewID, i),
            ReviewID:    reviewID,
            FindingHash: store.GenerateFindingHash(f.File, f.LineStart, f.LineEnd, f.Description),
            File:        f.File,
            LineStart:   f.LineStart,
            LineEnd:     f.LineEnd,
            Category:    f.Category,
            Severity:    f.Severity,
            Description: f.Description,
            Suggestion:  f.Suggestion,
            Evidence:    f.Evidence,
        }
    }

    if len(findings) > 0 {
        if err := o.deps.Store.SaveFindings(ctx, findings); err != nil {
            return err
        }
    }

    return nil
}
```

### 3.4 Config Hash Helper

```go
// calculateConfigHash creates a hash of the request configuration.
// This is used to track which settings were used for each run.
func calculateConfigHash(req BranchRequest) string {
    // Create a stable representation of the config
    configStr := fmt.Sprintf("%s|%s|%s|%s|%v",
        req.BaseRef,
        req.TargetRef,
        req.OutputDir,
        req.Repository,
        req.IncludeUncommitted,
    )

    hash := sha256.Sum256([]byte(configStr))
    return hex.EncodeToString(hash[:8]) // 16 char hash
}
```

## 4. Configuration Updates

### 4.1 New Config Structure

```go
// internal/config/loader.go

type Config struct {
    // ... existing fields ...
    Store StoreConfig `yaml:"store"`
}

type StoreConfig struct {
    Enabled bool   `yaml:"enabled"`
    Path    string `yaml:"path"`
}
```

### 4.2 Default Configuration

```go
func setDefaults(v *viper.Viper) {
    // ... existing defaults ...

    // Store defaults
    v.SetDefault("store.enabled", true)
    v.SetDefault("store.path", defaultStorePath())
}

func defaultStorePath() string {
    home, err := os.UserHomeDir()
    if err != nil {
        return "./reviews.db"
    }
    return filepath.Join(home, ".config", "bop", "reviews.db")
}
```

### 4.3 Example Configuration File

```yaml
# .bop.yaml
store:
  enabled: true
  path: ~/.config/bop/reviews.db

# Environment variable override:
# CR_STORE_ENABLED=false
# CR_STORE_PATH=/custom/path/reviews.db
```

## 5. Main Function Updates

### 5.1 Wire Store into Orchestrator

```go
// cmd/bop/main.go

func run() error {
    ctx := context.Background()

    cfg, err := config.Load(...)
    // ... existing setup ...

    // Initialize store if enabled
    var reviewStore store.Store
    if cfg.Store.Enabled {
        // Ensure directory exists
        storeDir := filepath.Dir(cfg.Store.Path)
        if err := os.MkdirAll(storeDir, 0755); err != nil {
            return fmt.Errorf("failed to create store directory: %w", err)
        }

        reviewStore, err = sqlite.NewStore(cfg.Store.Path)
        if err != nil {
            // Log warning but continue without store
            log.Printf("warning: failed to initialize store: %v", err)
            reviewStore = nil
        } else {
            defer reviewStore.Close()
        }
    }

    orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
        // ... existing deps ...
        Store: reviewStore, // NEW
    })

    // ... rest of setup ...
}
```

## 6. Testing Strategy

### 6.1 Utility Function Tests

```go
// internal/store/util_test.go

func TestGenerateRunID(t *testing.T) {
    // Test uniqueness
    // Test sortability
    // Test format
}

func TestCalculateConfigHash(t *testing.T) {
    // Test determinism
    // Test different configs produce different hashes
    // Test same config produces same hash
}

func TestGenerateFindingHash(t *testing.T) {
    // Test normalization
    // Test duplicates have same hash
    // Test different findings have different hashes
}
```

### 6.2 Integration Tests

```go
// internal/usecase/review/orchestrator_store_test.go

func TestOrchestrator_ReviewBranch_WithStore(t *testing.T) {
    // Create in-memory store
    // Run review
    // Verify run record created
    // Verify reviews saved
    // Verify findings saved
    // Verify finding hashes correct
}

func TestOrchestrator_ReviewBranch_WithoutStore(t *testing.T) {
    // Run review with nil store
    // Verify it works without errors
    // Verify backward compatibility
}

func TestOrchestrator_ReviewBranch_StoreFailure(t *testing.T) {
    // Simulate store failure
    // Verify review continues
    // Verify results still returned
}
```

## 7. Error Handling

### 7.1 Graceful Degradation

Store failures should never break the review process:

```go
if o.deps.Store != nil {
    if err := o.saveReviewToStore(ctx, runID, review); err != nil {
        // Log warning but don't fail
        log.Printf("warning: failed to save review to store: %v", err)
    }
}
```

### 7.2 Directory Creation

```go
// Ensure store directory exists before opening DB
storeDir := filepath.Dir(cfg.Store.Path)
if err := os.MkdirAll(storeDir, 0755); err != nil {
    return fmt.Errorf("failed to create store directory: %w", err)
}
```

## 8. Success Criteria

- [ ] All utility functions implemented with tests
- [ ] Store integrated into orchestrator
- [ ] Reviews automatically persisted when store enabled
- [ ] Store failures don't break reviews
- [ ] Configuration options working
- [ ] Default store path created automatically
- [ ] All existing tests still passing
- [ ] New integration tests passing
- [ ] Documentation updated

## 9. Implementation Order

1. **Utility functions** (`internal/store/util.go`)
   - GenerateRunID
   - CalculateConfigHash
   - GenerateFindingHash
   - GenerateReviewID
   - GenerateFindingID

2. **Config updates** (`internal/config/loader.go`)
   - Add StoreConfig struct
   - Add defaults
   - Add domain.Config field

3. **Orchestrator integration** (`internal/usecase/review/orchestrator.go`)
   - Add Store to deps
   - Add saveReviewToStore helper
   - Integrate at appropriate points
   - Add error handling

4. **Main function updates** (`cmd/bop/main.go`)
   - Initialize store from config
   - Wire into orchestrator
   - Add cleanup

5. **Tests**
   - Utility function tests
   - Integration tests
   - Backward compatibility tests

## 10. Dependencies

No new dependencies required - everything uses existing packages.

## 11. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Store failures break reviews | Graceful degradation - log warnings, continue |
| Database locks with concurrent reviews | SQLite handles this with timeouts |
| Large databases slow down | Add indexes (already done), consider cleanup commands later |
| Path permissions issues | Create directory with proper permissions, clear error messages |
