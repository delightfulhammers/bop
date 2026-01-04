# Agent-Based Code Review Architecture

**Status:** Design Draft
**Author:** Brandon / Claude
**Date:** 2025-12-24
**Related:** Epic #53, Issues #61, #82, #90, #91

---

## Executive Summary

The current architecture treats LLM outputs as trusted, deterministic facts. This leads to:
- Persistent false positives (e.g., "missing import" when import exists)
- Style opinions mixed with blocking bugs
- No confidence scoring or verification
- Brittle user experience

This document proposes an **agent-based architecture** where:
1. Multiple LLMs generate **candidate findings** (discovery)
2. An agent **verifies** each candidate against the actual codebase
3. Only **high-confidence, verified findings** are reported
4. The review matches human patterns: specific, actionable, verdict-based

---

## Problem Statement

### Current Flow (Broken)

```
Diff → [LLMs see only diff] → Findings → Naive Merge → Post to GitHub
                                   ↑
                                   └── No verification
                                       No confidence
                                       Style mixed with bugs
```

### The `strings.ToLower` Example

During PR #89 review:
1. LLM saw `strings.ToLower()` in the diff
2. Import statement was at line 8 (not in diff)
3. LLM flagged "missing import" as HIGH severity
4. Tool trusted it and posted it
5. False positive persisted across 3+ review cycles

**Root cause:** The LLM made a claim it couldn't verify. The tool had no mechanism to check.

### Human Review Pattern (What We Want)

At a professional engineering org:
1. PR opened → reviewers requested
2. Reviewer examines code **in full context**
3. Reviewer creates review:
   - Inline comments on specific issues
   - Summary verdict: `APPROVED` or `REQUEST_CHANGES`
4. Change requests are **specific and blocking**:
   - Bugs that preclude operation ✓
   - Security vulnerabilities ✓
   - Critical performance issues ✓
   - Style/opinions ✗ NEVER

---

## Proposed Architecture

### High-Level Flow

```
┌─────────────────────────────────────────────────────────────────┐
│  STAGE 1: DISCOVERY                                             │
│                                                                 │
│  Diff + Context → [LLM 1] ─┐                                    │
│                  → [LLM 2] ─┼─→ Raw Findings Pool               │
│                  → [LLM N] ─┘                                    │
│                                                                 │
│  Goal: Cast wide net, gather all potential issues               │
│  Quality: Low precision, high recall                            │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  STAGE 2: COLLATION                                             │
│                                                                 │
│  Raw Findings Pool                                              │
│       │                                                         │
│       ├─→ Deduplicate (same issue from multiple LLMs)           │
│       ├─→ Group (similar issues across files)                   │
│       └─→ Normalize (consistent severity/category)              │
│               │                                                 │
│               ▼                                                 │
│       Candidate Findings                                        │
│                                                                 │
│  Goal: Reduce noise, identify distinct issues                   │
│  Output: ~10-50 candidates (down from ~100+ raw)                │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  STAGE 3: VERIFICATION  ← The Key Innovation                   │
│                                                                 │
│  For each Candidate Finding:                                    │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Agent Loop                                                 │ │
│  │                                                            │ │
│  │ 1. UNDERSTAND the claim                                    │ │
│  │    "LLM says import is missing"                            │ │
│  │                                                            │ │
│  │ 2. INVESTIGATE                                             │ │
│  │    - Read full file (not just diff)                        │ │
│  │    - Check imports section                                 │ │
│  │    - Run `go build` if needed                              │ │
│  │    - Trace references                                      │ │
│  │                                                            │ │
│  │ 3. VERIFY                                                  │ │
│  │    - Is the claim factually correct?                       │ │
│  │    - Can we prove it with evidence?                        │ │
│  │                                                            │ │
│  │ 4. CLASSIFY                                                │ │
│  │    - blocking_bug: Will code fail/crash?                   │ │
│  │    - security: Vulnerability?                              │ │
│  │    - performance: Unbounded resource use?                  │ │
│  │    - style: Opinion/preference?                            │ │
│  │                                                            │ │
│  │ 5. SCORE CONFIDENCE                                        │ │
│  │    - 0-100% based on verification evidence                 │ │
│  │    - "I read the file and import exists" → 0% (discard)    │ │
│  │    - "go build fails with this error" → 95%                │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  Output: Verified Findings with confidence + classification     │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  STAGE 4: FILTERING                                             │
│                                                                 │
│  Keep only findings that:                                       │
│    ✓ Confidence ≥ 75%                                           │
│    ✓ Classification ∈ {blocking_bug, security, performance}     │
│    ✓ Actually blocks operation OR security risk                 │
│                                                                 │
│  Discard:                                                       │
│    ✗ Style opinions (any confidence)                            │
│    ✗ Low confidence (< 75%)                                     │
│    ✗ "Nice to have" suggestions                                 │
│    ✗ Unverifiable claims                                        │
│                                                                 │
│  Output: Reportable Findings (typically 0-5 per PR)             │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  STAGE 5: REPORTING                                             │
│                                                                 │
│  GitHub PR Review:                                              │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Inline Comments (one per verified finding)                 │ │
│  │                                                            │ │
│  │   File: db.go, Line: 45                                    │ │
│  │   ──────────────────────────────────────                   │ │
│  │   🔴 SQL Injection Vulnerability                           │ │
│  │                                                            │ │
│  │   User input passed directly to SQL query.                 │ │
│  │                                                            │ │
│  │   Evidence:                                                │ │
│  │   ```go                                                    │ │
│  │   query := "SELECT * FROM users WHERE id = " + userID      │ │
│  │   ```                                                      │ │
│  │                                                            │ │
│  │   Confidence: 92% (verified via static analysis)           │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Review Verdict                                             │ │
│  │                                                            │ │
│  │   If any blocking finding → REQUEST_CHANGES                │ │
│  │   Else → APPROVE                                           │ │
│  │                                                            │ │
│  │   Summary: "1 security issue requires attention"           │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## Stage Details

### Stage 1: Discovery

**Purpose:** Generate a broad pool of potential issues without concern for precision.

**Inputs:**
- Diff (unified format)
- File context (surrounding code, not full files)
- Review instructions/guidelines

**Process:**
- Run N providers in parallel (current behavior)
- Each provider returns findings with their analysis
- No filtering at this stage

**Outputs:**
- Raw findings pool (all findings from all providers)
- Provider metadata (cost, model, timing)

**Key insight:** This stage remains similar to current, but we explicitly treat outputs as *candidates*, not *facts*.

---

### Stage 2: Collation

**Purpose:** Reduce noise and identify distinct issues.

**Inputs:**
- Raw findings pool

**Process:**
1. **Deduplicate:** Same issue flagged by multiple LLMs → one candidate
2. **Group:** Similar issues (same root cause) → one candidate
3. **Normalize:** Consistent severity/category taxonomy
4. **Enrich:** Add agreement score (how many LLMs flagged this?)

**Outputs:**
- Candidate findings (deduplicated, normalized)
- Agreement metadata per candidate

**Key insight:** This is similar to current merge logic, but output is explicitly "candidates for verification" not "findings to report."

---

### Stage 3: Verification (The Key Innovation)

**Purpose:** Verify each candidate against the actual codebase.

**Inputs:**
- Candidate findings
- Full codebase access (not just diff)
- Build/test capabilities

**Process (per candidate):**

```
┌─────────────────────────────────────────────────────────────┐
│ Verification Agent                                          │
│                                                             │
│ Tools available:                                            │
│   - Read: Read any file in the repo                         │
│   - Grep: Search for patterns                               │
│   - Glob: Find files by pattern                             │
│   - Bash: Run go build, go vet, go test                     │
│   - AST: Parse and analyze code structure (future)          │
│                                                             │
│ Agent prompt:                                               │
│   "The code review found this issue:                        │
│    [candidate description]                                  │
│                                                             │
│    Your job is to VERIFY whether this is accurate.          │
│    1. Read the relevant files                               │
│    2. Check if the claim is factually correct               │
│    3. Classify: blocking_bug, security, performance, style  │
│    4. Score your confidence 0-100%                          │
│    5. Provide evidence for your conclusion"                 │
│                                                             │
│ Agent actions:                                              │
│   - Read full file containing the issue                     │
│   - Check imports, function definitions, types              │
│   - Optionally run go build/vet to confirm                  │
│   - Search for related code patterns                        │
│                                                             │
│ Agent output:                                               │
│   - verified: true/false                                    │
│   - classification: blocking_bug|security|performance|style │
│   - confidence: 0-100                                       │
│   - evidence: explanation + code snippets                   │
│   - blocks_operation: true/false                            │
└─────────────────────────────────────────────────────────────┘
```

**Verification Examples:**

| Candidate | Agent Investigation | Result |
|-----------|---------------------|--------|
| "Missing import for strings" | Read file → import at line 8 | ❌ Discard (confidence: 0%) |
| "SQL injection in db.go:45" | Read file → confirmed concatenation | ✅ Keep (confidence: 92%, blocking) |
| "Consider using sync.Pool" | Read file → valid optimization | ⚠️ Style (confidence: 80%, not blocking) |
| "Nil pointer dereference" | Run go vet → confirmed | ✅ Keep (confidence: 95%, blocking) |

**Key insight:** The agent has access to the FULL codebase and can run tools. It's not limited to the diff.

---

### Stage 4: Filtering

**Purpose:** Select only high-confidence, actionable findings.

**Inputs:**
- Verified findings with confidence/classification

**Filter rules:**
```
KEEP if:
  confidence >= 75%
  AND classification IN (blocking_bug, security, performance)
  AND blocks_operation == true

MAYBE_KEEP if:
  confidence >= 75%
  AND classification == performance
  AND blocks_operation == false
  → Include as suggestion, not change request

DISCARD if:
  confidence < 75%
  OR classification == style
  OR blocks_operation == false (for bugs)
```

**Outputs:**
- Reportable findings (high-confidence, blocking)
- Suggestions (high-confidence, non-blocking) - optional

---

### Stage 5: Reporting

**Purpose:** Post review matching human patterns.

**Inputs:**
- Reportable findings
- PR metadata

**Process:**
1. Create inline comments for each finding
2. Determine verdict:
   - Any blocking finding → `REQUEST_CHANGES`
   - Only suggestions → `COMMENT` (or `APPROVE` with suggestions)
   - No findings → `APPROVE`
3. Post single review with all comments + verdict

**GitHub Review Structure:**
```
Review Event: REQUEST_CHANGES (or APPROVE)

Inline Comments:
  - file.go:45 → SQL injection (confidence: 92%)
  - file.go:123 → Nil dereference (confidence: 95%)

Body:
  "2 issues require attention before merge:
   - SQL injection vulnerability in db.go
   - Nil pointer dereference in handler.go

   Confidence: High (verified via static analysis and code inspection)"
```

---

## Domain Model Changes

### New Types

```go
// CandidateFinding represents an unverified finding from discovery
type CandidateFinding struct {
    Finding       Finding
    Sources       []string      // Which LLMs reported this
    AgreementScore float64      // 0-1, how many LLMs agreed
}

// VerifiedFinding represents a finding after agent verification
type VerifiedFinding struct {
    Finding         Finding
    Verified        bool          // Did verification confirm the issue?
    Classification  Classification // blocking_bug, security, performance, style
    Confidence      int           // 0-100
    Evidence        string        // Agent's explanation
    BlocksOperation bool          // Will code fail without fix?
    VerificationLog []string      // Agent's investigation steps
}

// Classification of findings
type Classification string
const (
    ClassBlockingBug  Classification = "blocking_bug"
    ClassSecurity     Classification = "security"
    ClassPerformance  Classification = "performance"
    ClassStyle        Classification = "style"
)

// VerificationResult from the agent
type VerificationResult struct {
    Verified        bool
    Classification  Classification
    Confidence      int
    Evidence        string
    BlocksOperation bool
    Actions         []VerificationAction // What the agent did
}

type VerificationAction struct {
    Tool    string // "read", "grep", "bash", etc.
    Input   string // File path, command, etc.
    Output  string // Result summary
}
```

### Modified Types

```go
// Review gains verification metadata
type Review struct {
    // ... existing fields ...

    // New fields for agent architecture
    DiscoveryFindings  []CandidateFinding  // Raw from LLMs
    VerifiedFindings   []VerifiedFinding   // After verification
    ReportableFindings []VerifiedFinding   // After filtering
}
```

---

## Interface Design

### Verifier Interface

```go
// Verifier verifies candidate findings against the codebase
type Verifier interface {
    // Verify checks a single candidate and returns verification result
    Verify(ctx context.Context, candidate CandidateFinding, repo Repository) (VerificationResult, error)

    // VerifyBatch verifies multiple candidates (may parallelize)
    VerifyBatch(ctx context.Context, candidates []CandidateFinding, repo Repository) ([]VerificationResult, error)
}

// Repository provides access to the codebase for verification
type Repository interface {
    ReadFile(path string) ([]byte, error)
    Glob(pattern string) ([]string, error)
    Grep(pattern string, paths ...string) ([]GrepMatch, error)
    RunCommand(cmd string, args ...string) (stdout, stderr string, err error)
}
```

### Agent-Based Verifier

```go
// AgentVerifier uses an LLM agent to verify findings
type AgentVerifier struct {
    agent  Agent       // Claude, GPT-4, etc.
    repo   Repository  // Codebase access
    tools  []Tool      // Read, Grep, Bash, etc.
}

func (v *AgentVerifier) Verify(ctx context.Context, candidate CandidateFinding, repo Repository) (VerificationResult, error) {
    prompt := buildVerificationPrompt(candidate)

    result, err := v.agent.Run(ctx, prompt, v.tools)
    if err != nil {
        return VerificationResult{}, err
    }

    return parseVerificationResult(result)
}
```

---

## Implementation Plan

### Phase 1: Domain Foundation
**Goal:** Establish the type system for the agent architecture.

**Tasks:**
- [ ] Add `CandidateFinding` type (finding + sources + agreement score)
- [ ] Add `VerifiedFinding` type (finding + classification + confidence + evidence)
- [ ] Add `Classification` enum (blocking_bug, security, performance, style)
- [ ] Add `VerificationResult` type (verified, classification, confidence, evidence, actions)
- [ ] Add configuration types for verification settings
- [ ] Update `Review` type to include discovery/verified/reportable findings

**Files to create/modify:**
- `internal/domain/verification.go` (new)
- `internal/domain/types.go` (modify)
- `internal/config/types.go` (modify)

### Phase 2: Verifier Interface & Repository
**Goal:** Define the contracts and codebase access layer.

**Tasks:**
- [ ] Define `Verifier` interface (Verify, VerifyBatch)
- [ ] Define `Repository` interface (ReadFile, Glob, Grep, RunCommand)
- [ ] Implement `LocalRepository` for filesystem access
- [ ] Implement `GitRepository` for git-aware access (respects .gitignore)
- [ ] Add cost tracking to verification

**Files to create:**
- `internal/usecase/verify/verifier.go` (interface)
- `internal/adapter/repository/local.go`
- `internal/adapter/repository/git.go`

### Phase 3: Agent Verifier Implementation
**Goal:** Build the LLM-powered verification agent.

**Tasks:**
- [ ] Implement `AgentVerifier` struct
- [ ] Define verification tools (Read, Grep, Glob, Bash)
- [ ] Build verification prompt template
- [ ] Implement confidence scoring logic
- [ ] Implement classification logic
- [ ] Add cost ceiling enforcement
- [ ] Add parallel verification with concurrency limit

**Files to create:**
- `internal/adapter/verify/agent.go`
- `internal/adapter/verify/tools.go`
- `internal/adapter/verify/prompts.go`

### Phase 4: Pipeline Integration
**Goal:** Wire the verification stage into the review flow.

**Tasks:**
- [ ] Modify `Orchestrator` to separate discovery from verification
- [ ] Add collation stage (current merge logic, reframed)
- [ ] Insert verification stage after collation
- [ ] Add filtering stage after verification
- [ ] Update `GitHubPoster` to use verified findings
- [ ] Update review output to include verification metadata

**Files to modify:**
- `internal/usecase/review/orchestrator.go`
- `internal/usecase/github/poster.go`
- `internal/adapter/output/markdown/writer.go`

### Phase 5: Configuration & CLI
**Goal:** Expose verification settings to users.

**Tasks:**
- [ ] Add verification config section to `.bop.yml`
- [ ] Add CLI flags for verification (--verify, --verification-depth, etc.)
- [ ] Add --no-verify flag to skip verification (fast mode)
- [ ] Update help/documentation

**Files to modify:**
- `internal/config/config.go`
- `cmd/bop/main.go`
- `docs/` (documentation)

### Phase 6: Testing & Refinement
**Goal:** Ensure reliability and tune thresholds.

**Tasks:**
- [ ] Unit tests for domain types
- [ ] Unit tests for verifier interface
- [ ] Integration tests for agent verifier
- [ ] End-to-end tests for full pipeline
- [ ] Benchmark cost/latency
- [ ] Tune confidence thresholds based on real-world results

**Success criteria:**
- False positive rate < 10%
- Cost per review < $0.50 (configurable)
- All reported findings are actionable

---

## Cost & Latency Analysis

### Current (No Verification)
- 3 LLM calls (one per provider): ~$0.02-0.05
- Latency: ~10-30 seconds

### With Verification
- 3 LLM calls for discovery: ~$0.02-0.05
- N verification calls (one per candidate): ~$0.01-0.03 each
- Total for 20 candidates: ~$0.20-0.60 additional
- Latency: ~30-60 seconds additional

**Mitigation strategies:**
1. **Parallel verification:** Verify candidates concurrently
2. **Batch verification:** Verify multiple similar candidates in one call
3. **Tiered verification:** Quick check first, deep dive only if needed
4. **Caching:** Cache verification results for similar patterns

---

## Success Metrics

| Metric | Current | Target |
|--------|---------|--------|
| False positive rate | ~30-50% | <10% |
| Actionable findings per PR | Varies widely | Consistent 0-5 |
| Style opinions reported | Many | Zero |
| User trust | Low | High |
| Time to review | Fast but noisy | Slightly slower, much cleaner |

---

## Design Decisions (Confirmed)

### 1. Verification Depth
**Decision:** Start with **Medium**, make configurable.

```yaml
verification:
  depth: medium  # quick | medium | deep
```

| Level | Actions | Use Case |
|-------|---------|----------|
| quick | Read the target file only | Fast, low cost |
| medium | Read file + grep related code + check imports | Default, balanced |
| deep | Run go build/vet/test | High confidence required |

### 2. Classification Taxonomy
**Decision:** Keep 4 categories. "Nice to have" performance will filter out via confidence.

```go
const (
    ClassBlockingBug  = "blocking_bug"   // Code will fail/crash
    ClassSecurity     = "security"       // Vulnerability
    ClassPerformance  = "performance"    // Resource issues
    ClassStyle        = "style"          // Always discarded
)
```

### 3. Confidence Threshold
**Decision:** Default 75%, configurable per-severity.

```yaml
verification:
  confidence:
    default: 75
    critical: 60    # Lower threshold for critical (more cautious)
    high: 70
    medium: 75
    low: 85         # Higher threshold for low (avoid noise)
```

### 4. Reporting Behavior
**Decision:** Non-blocking performance → comments, not change requests.

| Classification | Blocks Operation | Review Action |
|----------------|------------------|---------------|
| blocking_bug | true | REQUEST_CHANGES |
| security | true | REQUEST_CHANGES |
| security | false | COMMENT |
| performance | true | REQUEST_CHANGES |
| performance | false | COMMENT (suggestion) |
| style | * | DISCARD (never report) |

### 5. Cost Ceiling
**Decision:** Default $0.50 per PR, configurable.

```yaml
verification:
  costCeiling: 0.50  # USD, stop verification if exceeded
```

When ceiling is reached:
- Report findings verified so far
- Log warning about incomplete verification
- Consider remaining candidates as "unverified" (lower confidence)

---

## Open Questions (Remaining)

1. **Verification model:** Same model as discovery or different?
   - Same: Simpler, but might repeat mistakes
   - Different: More robust, but more complex
   - **Leaning:** Use a capable model (Claude/GPT-4) regardless of discovery providers

2. **Partial verification:** What if verification times out?
   - Report as "unverified" with lower confidence?
   - Skip entirely?
   - **Leaning:** Report with confidence penalty (e.g., -20%)

3. **Verification caching:** Cache results for similar patterns?
   - Could reduce cost significantly
   - Risk of stale cache
   - **Leaning:** Start without, add if cost is problematic

---

## Appendix: Current vs Proposed Comparison

| Aspect | Current | Proposed |
|--------|---------|----------|
| LLM trust | Full trust | Verify before trust |
| Finding quality | Mixed (bugs + style + false positives) | High (verified bugs only) |
| Confidence | Implicit (severity as proxy) | Explicit (0-100 score) |
| Classification | Category (vague) | blocking_bug/security/performance/style |
| Codebase access | Diff only | Full repo |
| Verification | None | Agent with tools |
| Output | Many findings | Few high-quality findings |
| User experience | Noisy, unreliable | Clean, actionable |
