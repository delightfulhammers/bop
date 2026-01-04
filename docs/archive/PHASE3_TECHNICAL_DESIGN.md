# Phase 3 Technical Design: Intelligence & Feedback Loop

Version: 1.0
Date: 2025-10-21
Status: Planning

## 1. Overview

Phase 3 introduces intelligence and learning capabilities to the code review tool through:
- **Persistent storage** of review history and user feedback
- **Interactive TUI** for rich user experience and feedback capture
- **Precision priors** using Beta distribution to track model accuracy
- **Intelligent merging** that weights findings based on historical accuracy
- **Enhanced redaction** with entropy-based secret detection

## 2. SQLite Persistence Layer

### 2.1 Database Schema

```sql
-- Stores metadata about each review run
CREATE TABLE runs (
    run_id TEXT PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    scope TEXT NOT NULL,              -- e.g., "main..feature-branch"
    config_hash TEXT NOT NULL,        -- Hash of the config used
    total_cost REAL DEFAULT 0.0,
    base_ref TEXT NOT NULL,
    target_ref TEXT NOT NULL,
    repository TEXT NOT NULL
);

-- Individual reviews from each provider
CREATE TABLE reviews (
    review_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    summary TEXT,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (run_id) REFERENCES runs(run_id) ON DELETE CASCADE
);

-- Individual findings from reviews
CREATE TABLE findings (
    finding_id TEXT PRIMARY KEY,
    review_id TEXT NOT NULL,
    finding_hash TEXT NOT NULL,       -- Hash for de-duplication
    file TEXT NOT NULL,
    line_start INTEGER NOT NULL,
    line_end INTEGER NOT NULL,
    category TEXT NOT NULL,
    severity TEXT NOT NULL,
    description TEXT NOT NULL,
    suggestion TEXT,
    evidence BOOLEAN DEFAULT 0,
    FOREIGN KEY (review_id) REFERENCES reviews(review_id) ON DELETE CASCADE
);

-- User feedback on findings
CREATE TABLE feedback (
    feedback_id INTEGER PRIMARY KEY AUTOINCREMENT,
    finding_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('accepted', 'rejected')),
    timestamp INTEGER NOT NULL,
    FOREIGN KEY (finding_id) REFERENCES findings(finding_id) ON DELETE CASCADE
);

-- Precision priors using Beta distribution parameters
CREATE TABLE precision_priors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,
    category TEXT NOT NULL,
    alpha REAL NOT NULL DEFAULT 1.0,  -- Successes (accepted findings)
    beta REAL NOT NULL DEFAULT 1.0,   -- Failures (rejected findings)
    UNIQUE(provider, category)
);

-- Indexes for performance
CREATE INDEX idx_findings_hash ON findings(finding_hash);
CREATE INDEX idx_findings_review ON findings(review_id);
CREATE INDEX idx_feedback_finding ON feedback(finding_id);
CREATE INDEX idx_precision_provider_category ON precision_priors(provider, category);
CREATE INDEX idx_runs_timestamp ON runs(timestamp DESC);
```

### 2.2 Store Interface

```go
// internal/store/store.go
package store

import (
    "context"
    "time"
    "github.com/delightfulhammers/bop/internal/domain"
)

// Store defines the persistence layer interface
type Store interface {
    // Run management
    CreateRun(ctx context.Context, run Run) error
    GetRun(ctx context.Context, runID string) (Run, error)
    ListRuns(ctx context.Context, limit int) ([]Run, error)

    // Review persistence
    SaveReview(ctx context.Context, review ReviewRecord) error
    GetReviewsByRun(ctx context.Context, runID string) ([]ReviewRecord, error)

    // Finding persistence
    SaveFindings(ctx context.Context, findings []FindingRecord) error
    GetFindingsByReview(ctx context.Context, reviewID string) ([]FindingRecord, error)

    // Feedback management
    RecordFeedback(ctx context.Context, feedback Feedback) error
    GetFeedbackForFinding(ctx context.Context, findingID string) ([]Feedback, error)

    // Precision priors
    GetPrecisionPriors(ctx context.Context) (map[string]map[string]PrecisionPrior, error)
    UpdatePrecisionPrior(ctx context.Context, provider, category string, accepted, rejected int) error

    // Utility
    Close() error
}

// Run represents a review execution
type Run struct {
    RunID      string
    Timestamp  time.Time
    Scope      string
    ConfigHash string
    TotalCost  float64
    BaseRef    string
    TargetRef  string
    Repository string
}

// ReviewRecord stores review metadata
type ReviewRecord struct {
    ReviewID  string
    RunID     string
    Provider  string
    Model     string
    Summary   string
    CreatedAt time.Time
}

// FindingRecord stores a single finding with metadata
type FindingRecord struct {
    FindingID   string
    ReviewID    string
    FindingHash string
    File        string
    LineStart   int
    LineEnd     int
    Category    string
    Severity    string
    Description string
    Suggestion  string
    Evidence    bool
}

// Feedback records user's acceptance/rejection
type Feedback struct {
    FeedbackID int
    FindingID  string
    Status     string // "accepted" or "rejected"
    Timestamp  time.Time
}

// PrecisionPrior represents Beta distribution parameters
type PrecisionPrior struct {
    Provider string
    Category string
    Alpha    float64  // Accepted count
    Beta     float64  // Rejected count
}

// Precision calculates the mean of the Beta distribution
func (p PrecisionPrior) Precision() float64 {
    return p.Alpha / (p.Alpha + p.Beta)
}
```

### 2.3 SQLite Implementation

```go
// internal/adapter/store/sqlite/store.go
package sqlite

import (
    "context"
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
    "github.com/delightfulhammers/bop/internal/store"
)

type Store struct {
    db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
    db, err := sql.Open("sqlite3", dbPath)
    if err != nil {
        return nil, err
    }

    // Execute schema creation
    if err := createSchema(db); err != nil {
        return nil, err
    }

    return &Store{db: db}, nil
}
```

## 3. Terminal User Interface (TUI)

### 3.1 TUI Architecture

Using Bubble Tea framework for interactive terminal UI with the following views:

1. **Review List View**: Shows recent review runs
2. **Review Detail View**: Displays findings from a specific run
3. **Finding Detail View**: Shows full details of a finding with accept/reject options
4. **Statistics View**: Shows precision priors and model performance

### 3.2 TUI Models

```go
// internal/adapter/tui/model.go
package tui

import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/bubbles/list"
    "github.com/charmbracelet/bubbles/viewport"
    "github.com/delightfulhammers/bop/internal/store"
)

// Model represents the TUI state machine
type Model struct {
    state       State
    store       store.Store

    // Views
    reviewList  list.Model
    findingList list.Model
    viewport    viewport.Model

    // Data
    currentRun      *store.Run
    currentFindings []store.FindingRecord
    currentIndex    int

    // UI state
    width  int
    height int
    ready  bool
}

type State int

const (
    StateReviewList State = iota
    StateFindingList
    StateFindingDetail
    StateStatistics
)

// Messages
type reviewsLoadedMsg struct {
    runs []store.Run
}

type findingsLoadedMsg struct {
    findings []store.FindingRecord
}

type feedbackRecordedMsg struct {
    finding store.FindingRecord
    status  string
}
```

### 3.3 Key Bindings

- `↑/↓` or `j/k`: Navigate lists
- `Enter`: Select item
- `a`: Accept current finding
- `r`: Reject current finding
- `s`: View statistics
- `q` or `Esc`: Go back / quit
- `?`: Show help

## 4. Precision Priors System

### 4.1 Beta Distribution

The system uses a **Beta distribution** to model the precision of each provider for each category. The Beta distribution is parameterized by α (alpha) and β (beta):

- **α (alpha)**: Count of accepted findings
- **β (beta)**: Count of rejected findings
- **Precision**: α / (α + β) - the mean of the distribution

Starting with α=1, β=1 (uniform prior), the distribution updates as feedback is received.

### 4.2 Update Logic

```go
// internal/usecase/feedback/processor.go
package feedback

type Processor struct {
    store store.Store
}

func (p *Processor) ProcessFeedback(ctx context.Context, findingID, status string) error {
    // 1. Record the feedback
    feedback := store.Feedback{
        FindingID: findingID,
        Status:    status,
        Timestamp: time.Now(),
    }
    if err := p.store.RecordFeedback(ctx, feedback); err != nil {
        return err
    }

    // 2. Get the finding to extract provider and category
    finding, err := p.store.GetFinding(ctx, findingID)
    if err != nil {
        return err
    }

    review, err := p.store.GetReview(ctx, finding.ReviewID)
    if err != nil {
        return err
    }

    // 3. Update precision prior
    accepted := 0
    rejected := 0
    if status == "accepted" {
        accepted = 1
    } else {
        rejected = 1
    }

    return p.store.UpdatePrecisionPrior(ctx, review.Provider, finding.Category, accepted, rejected)
}
```

## 5. Intelligent Merger v2

### 5.1 Enhanced Merging Algorithm

The merger now incorporates precision priors to intelligently weight findings:

```go
// internal/usecase/merge/merger_v2.go
package merge

type IntelligentMerger struct {
    store store.Store
}

func (m *IntelligentMerger) Merge(ctx context.Context, reviews []domain.Review) (domain.Review, error) {
    // 1. Load precision priors
    priors, err := m.store.GetPrecisionPriors(ctx)
    if err != nil {
        return domain.Review{}, err
    }

    // 2. Group findings by hash
    groups := m.groupByHash(reviews)

    // 3. Score each group
    scoredFindings := make([]ScoredFinding, 0, len(groups))
    for hash, findings := range groups {
        score := m.calculateScore(findings, priors)
        scoredFindings = append(scoredFindings, ScoredFinding{
            Finding: m.selectBestFinding(findings, priors),
            Score:   score,
            Count:   len(findings),
        })
    }

    // 4. Sort by score descending
    sort.Slice(scoredFindings, func(i, j int) bool {
        return scoredFindings[i].Score > scoredFindings[j].Score
    })

    // 5. Build merged review
    merged := domain.Review{
        ProviderName: "merged-intelligent",
        ModelName:    "multi-llm-v2",
        Summary:      m.generateSummary(scoredFindings),
        Findings:     extractFindings(scoredFindings),
    }

    return merged, nil
}

func (m *IntelligentMerger) calculateScore(findings []domain.Finding, priors map[string]map[string]store.PrecisionPrior) float64 {
    // Scoring formula:
    // score = (w1 * agreement) + (w2 * avg_severity) + (w3 * max_precision)

    agreement := float64(len(findings)) / 4.0  // Normalize by max providers

    avgSeverity := m.averageSeverityScore(findings)

    maxPrecision := 0.5  // Default
    for _, finding := range findings {
        if prior, ok := priors[finding.ProviderName][finding.Category]; ok {
            precision := prior.Precision()
            if precision > maxPrecision {
                maxPrecision = precision
            }
        }
    }

    // Weights (configurable in future)
    w1, w2, w3 := 0.4, 0.3, 0.3

    return (w1 * agreement) + (w2 * avgSeverity) + (w3 * maxPrecision)
}
```

## 6. Redaction Engine v2: Entropy-Based Detection

### 6.1 Enhanced Redaction

Add entropy analysis to detect high-entropy strings that may be secrets:

```go
// internal/redaction/entropy.go
package redaction

import (
    "math"
    "strings"
)

// EntropyDetector identifies high-entropy strings
type EntropyDetector struct {
    threshold float64  // Default: 4.5
    minLength int      // Default: 20
}

func (d *EntropyDetector) IsHighEntropy(s string) bool {
    if len(s) < d.minLength {
        return false
    }

    entropy := d.shannonEntropy(s)
    return entropy > d.threshold
}

func (d *EntropyDetector) shannonEntropy(s string) float64 {
    if len(s) == 0 {
        return 0
    }

    // Calculate character frequency
    freq := make(map[rune]int)
    for _, char := range s {
        freq[char]++
    }

    // Calculate Shannon entropy
    var entropy float64
    length := float64(len(s))
    for _, count := range freq {
        p := float64(count) / length
        entropy -= p * math.Log2(p)
    }

    return entropy
}

// ScanLine checks a line for high-entropy strings
func (d *EntropyDetector) ScanLine(line string) []string {
    words := strings.Fields(line)
    var suspects []string

    for _, word := range words {
        // Remove quotes and common delimiters
        word = strings.Trim(word, `"',:;()[]{}`)

        if d.IsHighEntropy(word) {
            suspects = append(suspects, word)
        }
    }

    return suspects
}
```

## 7. Integration Points

### 7.1 Orchestrator Updates

The orchestrator needs to integrate with the store:

```go
// internal/usecase/review/orchestrator.go

func (o *Orchestrator) ReviewBranch(ctx context.Context, req BranchRequest) (Result, error) {
    // ... existing logic ...

    // After reviews complete, save to store if available
    if o.deps.Store != nil {
        run := store.Run{
            RunID:      generateRunID(),
            Timestamp:  time.Now(),
            Scope:      fmt.Sprintf("%s..%s", req.BaseRef, req.TargetRef),
            ConfigHash: calculateConfigHash(req),
            BaseRef:    req.BaseRef,
            TargetRef:  req.TargetRef,
            Repository: req.Repository,
        }

        if err := o.deps.Store.CreateRun(ctx, run); err != nil {
            return Result{}, err
        }

        // Save reviews and findings
        for _, review := range reviews {
            if err := o.saveReviewToStore(ctx, run.RunID, review); err != nil {
                return Result{}, err
            }
        }
    }

    // ... rest of logic ...
}
```

### 7.2 CLI Command for TUI

```go
// internal/adapter/cli/tui.go

func newTUICommand(deps Dependencies) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "tui",
        Short: "Launch interactive TUI for reviewing findings",
        RunE: func(cmd *cobra.Command, args []string) error {
            store, err := sqlite.NewStore(deps.DBPath)
            if err != nil {
                return err
            }
            defer store.Close()

            model := tui.NewModel(store)
            program := tea.NewProgram(model, tea.WithAltScreen())

            _, err = program.Run()
            return err
        },
    }

    return cmd
}
```

## 8. Testing Strategy

### 8.1 Unit Tests

- **Store**: Test each CRUD operation with in-memory SQLite (`:memory:`)
- **Precision Priors**: Test Beta distribution calculations
- **Entropy Detector**: Test with known high/low entropy strings
- **Intelligent Merger**: Test scoring and ranking with mock priors

### 8.2 Integration Tests

- **End-to-end flow**: Run review → Save to DB → Load in TUI → Record feedback → Verify priors updated
- **TUI navigation**: Test key bindings and state transitions

## 9. Configuration Updates

Add new configuration options:

```yaml
# .bop.yaml
store:
  enabled: true
  path: ~/.config/bop/reviews.db

tui:
  enabled: true
  defaultView: reviewList

merge:
  strategy: intelligent  # "consensus" or "intelligent"
  weights:
    agreement: 0.4
    severity: 0.3
    precision: 0.3

redaction:
  enabled: true
  entropyThreshold: 4.5
  entropyMinLength: 20
```

## 10. Success Criteria

- [ ] SQLite database with all schema tables created
- [ ] Store interface fully implemented with 100% test coverage
- [ ] TUI launches and displays review history
- [ ] Users can accept/reject findings via TUI
- [ ] Precision priors update correctly based on feedback
- [ ] Intelligent merger uses priors to rank findings
- [ ] Entropy-based redaction catches high-entropy strings
- [ ] All integration tests passing
- [ ] Documentation updated with TUI usage examples

## 11. Dependencies

New Go modules required:

```
github.com/mattn/go-sqlite3 v1.14.18
github.com/charmbracelet/bubbletea v0.25.0
github.com/charmbracelet/bubbles v0.18.0
github.com/charmbracelet/lipgloss v0.9.1
```

## 12. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| SQLite cgo dependency complicates cross-compilation | Use build tags; document cgo requirement; consider pure-Go alternative like modernc.org/sqlite |
| TUI complexity delays delivery | Implement minimal viable TUI first; enhance iteratively |
| Beta distribution cold start problem | Use α=1, β=1 uniform prior; require minimum samples before weighting heavily |
| Entropy detection false positives | Make threshold configurable; add whitelist for known safe patterns |

## 13. Timeline

- **Week 1**: SQLite store implementation and tests
- **Week 2**: Basic TUI with review/finding navigation
- **Week 3**: Feedback capture, precision priors, intelligent merger, entropy detection
- **Week 4**: Integration testing, documentation, polish
