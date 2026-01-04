# Phase 3.5: Local Mode

**Version:** 0.1
**Date:** 2025-12-31
**Author:** Brandon Young
**Status:** Design

---

## 1. Executive Summary

Phase 3.5 returns code-reviewer to its original vision: a local-first tool that can review code without requiring GitHub Actions integration. While Phases 1-3 focused on GitHub-centric workflows (PR comments, SARIF annotations, MCP triage), this phase decouples the review engine from GitHub I/O, enabling:

1. **On-demand PR review** — Review any GitHub PR from local machine without Actions setup
2. **Interactive triage** — TUI for selecting/editing findings before posting
3. **Session-based local state** — Persist findings across review runs, tied to repo+branch
4. **Coding assistant integration** — MCP tools for review invocation (not just triage)

### Motivation

The primary use case is reviewing PRs in GitHub Enterprise repositories where setting up the GitHub Action isn't feasible due to organizational friction. Secondary benefits include:

- Faster iteration during development (no push-wait-review cycle)
- Review of any public/accessible repo without owner cooperation
- Foundation for coding assistant-driven review workflows

---

## 2. Current State vs. Desired State

### Current Architecture

```
GitHub Actions → bop review → LLM providers → GitHub PR comments
                                           → SARIF annotations

bop-mcp ← GitHub API (read findings for triage)
```

**Limitations:**
- Review requires GitHub Actions setup in target repo
- Findings always posted (no selection/editing)
- CLI output (markdown/JSON) exists but lacks triage workflow
- MCP server only reads findings, cannot invoke reviews

### Desired Architecture

```
[Input Adapters]              [Review Engine]           [Output Adapters]

GitHub Actions ──┐                                    ┌── GitHub PR comments
CLI (local) ─────┼──→ Personas → LLM → Merge → ──────┼── Local files (sessions)
MCP tools ───────┤         Verify                     ├── In-memory (return to caller)
TUI/REPL ────────┘                                    └── Interactive (selective post)
```

---

## 3. Feature Specifications

### 3.1 Remote PR Review (`bop review pr`)

**Command:**
```bash
cr review pr owner/repo#123
cr review pr https://github.com/owner/repo/pull/123
cr review pr owner/repo#123 --reviewers security,maintainability
cr review pr owner/repo#123 --output ./findings
cr review pr owner/repo#123 --interactive  # launches TUI
```

**Behavior:**
1. Fetch PR metadata and diff from GitHub API
2. Run configured reviewers against the diff
3. Output findings to stdout, file, or TUI (based on flags)
4. Optionally post selected findings back to PR

**Authentication:**
- Uses `GITHUB_TOKEN` environment variable
- Works with github.com and GitHub Enterprise (`GH_HOST` or config)

**Differences from `bop review branch`:**
| Aspect | `review branch` | `review pr` |
|--------|-----------------|-------------|
| Input | Local git diff | GitHub PR diff |
| Context | Local files | PR metadata, existing comments |
| Output | Local + optional GitHub post | Same |
| Deduplication | Against existing PR comments | Same |

### 3.2 Interactive TUI

**Purpose:** Review findings, select which to post, edit wording before posting.

**Design principles:**
- Full terminal, responsive to resize
- Minimal, tasteful color (inspired by Claude Code, lazygit)
- Intuitive keyboard navigation
- No mouse required, but clicks work where sensible

**Layout:**
```
┌─ code-reviewer ─────────────────────────────────────────────────────────┐
│ PR: owner/repo#123 | 3 reviewers | 7 findings | 0 selected              │
├─────────────────────────────────────────────────────────────────────────┤
│ FINDINGS                                                                │
│                                                                         │
│ ● [1/7] HIGH security — SQL injection risk                              │
│   auth/handler.go:45-52                                                 │
│   ────────────────────────────────────────────────────────────────────  │
│   The query uses string interpolation which allows SQL injection.       │
│   Consider using parameterized queries instead.                         │
│                                                                         │
│   ○ [2/7] MEDIUM maintainability — Complex function (CC: 15)            │
│   ○ [3/7] MEDIUM maintainability — Missing error handling               │
│   ○ [4/7] LOW docs — Outdated comment references removed function       │
│   ...                                                                   │
│                                                                         │
├─────────────────────────────────────────────────────────────────────────┤
│ [space] toggle  [e]dit  [v]iew code  [d]iff  │  [p]ost selected  [q]uit │
└─────────────────────────────────────────────────────────────────────────┘
```

**Key interactions:**
| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate findings |
| `space` | Toggle selection for posting |
| `e` | Edit finding text (opens editor or inline) |
| `v` | View source code context |
| `d` | View diff hunk |
| `a` | Select all |
| `n` | Select none |
| `p` | Post selected findings to PR |
| `s` | Save to local session (don't post) |
| `q` | Quit |

**Technology:** Bubble Tea (Go) — Elm-architecture, composable, modern.

### 3.3 Session-Based Local State

**Purpose:** Persist findings and triage state across review runs for local workflows.

**Session identity:**
```
session_id = sha256(canonical_repo_path + "/" + branch_name)[:16]
```

**Storage structure:**
```
~/.cache/code-reviewer/sessions/
├── a1b2c3d4e5f6g7h8/              # session directory
│   ├── meta.json                   # repo path, branch, created, updated
│   ├── reviews/
│   │   ├── 2025-01-01T10-00-00Z.json  # review run 1
│   │   └── 2025-01-01T14-30-00Z.json  # review run 2
│   └── state.json                  # triage decisions per finding
└── ...
```

**meta.json:**
```json
{
  "repo_path": "/Users/dev/projects/my-app",
  "branch": "feature/auth",
  "created_at": "2025-01-01T10:00:00Z",
  "updated_at": "2025-01-01T14:30:00Z"
}
```

**state.json:**
```json
{
  "findings": {
    "abc123": {"status": "acknowledged", "updated_at": "..."},
    "def456": {"status": "fixed", "updated_at": "..."},
    "ghi789": {"status": "disputed", "reason": "False positive", "updated_at": "..."}
  }
}
```

**Session management commands:**
```bash
cr sessions list              # Show active sessions
cr sessions prune             # Delete orphaned sessions (branch deleted)
cr sessions prune --older 30d # Delete sessions older than 30 days
cr sessions clean             # Delete all sessions
```

**Deduplication:** When running subsequent reviews on the same session, findings are deduplicated against prior runs using the same fingerprint logic as GitHub comments.

### 3.4 MCP Review Tools

**Purpose:** Allow coding assistants to invoke reviews, not just triage existing findings.

**New tools:**

```typescript
// Invoke review on a GitHub PR
review_pr: {
  owner: string,
  repo: string,
  pr_number: number,
  reviewers?: string[],  // Optional: specific reviewers
} → {
  findings: Finding[],
  metadata: {
    reviewers_used: string[],
    tokens_used: number,
    duration_ms: number
  }
}

// Invoke review on local branch
review_branch: {
  base_ref: string,      // e.g., "main"
  reviewers?: string[],
} → {
  findings: Finding[],
  metadata: {...}
}

// Post selected findings to GitHub
post_findings: {
  owner: string,
  repo: string,
  pr_number: number,
  findings: Finding[],   // Subset of findings from review
} → {
  posted: number,
  comment_ids: number[]
}

// Edit a finding before posting
edit_finding: {
  finding: Finding,
  new_description?: string,
  new_suggestion?: string,
} → Finding
```

**Streaming (future):** Use MCP progress notifications to emit findings as each reviewer completes, rather than waiting for all reviewers.

**Workflow:**
1. Coding assistant calls `review_pr` or `review_branch`
2. Findings returned to assistant (not posted anywhere)
3. Assistant discusses findings with user, filters/edits as needed
4. Assistant calls `post_findings` with curated list
5. Only selected findings appear on the PR

---

## 4. Architecture Changes

### 4.1 Review Engine Decoupling

The review engine already produces `[]domain.Finding`. Changes needed:

| Component | Current | Change |
|-----------|---------|--------|
| Orchestrator | Returns `Result` with findings | No change |
| GitHub Poster | Always posts all findings | Make posting optional/separate |
| Output writers | Write to files | Add "return" output for MCP |

### 4.2 New Components

```
internal/
  adapter/
    tui/                    # NEW: Bubble Tea TUI
      app.go                # Main application
      model.go              # State model
      views/
        findings.go         # Findings list view
        detail.go           # Finding detail view
        editor.go           # Finding editor
    session/                # NEW: Local session storage
      store.go              # Session CRUD
      prune.go              # Orphan cleanup
  usecase/
    review/
      remote.go             # NEW: Fetch and review remote PR
```

### 4.3 CLI Changes

```go
// New commands
cr review pr <owner/repo#number>   // Review remote PR
cr review pr <url>                 // Review from URL
  --interactive                    // Launch TUI
  --output <dir>                   // Write to files
  --post                           // Post all findings (no TUI)
  --reviewers <list>               // Specific reviewers

cr sessions list                   // List sessions
cr sessions prune                  // Clean orphans
cr sessions clean                  // Delete all
```

---

## 5. Security Considerations

| Risk | Mitigation |
|------|------------|
| Token in environment | Standard `GITHUB_TOKEN` handling, never logged |
| Reviewing malicious PRs | Redaction applies to diff content |
| Session data exposure | Stored in user cache dir with standard permissions |
| Edited findings misattribution | Posted findings attributed to token owner, not tool |

---

## 6. Implementation Phases

### Phase 3.5a: Remote PR Review (Foundation)

**Scope:** `bop review pr owner/repo#123` with file output

**Deliverables:**
- [ ] GitHub adapter: fetch PR diff via API
- [ ] CLI command: `bop review pr`
- [ ] Output to stdout/files (no TUI yet)
- [ ] Works with github.com and GHE

**Effort:** Small-Medium

### Phase 3.5b: Session Storage

**Scope:** Local session persistence for branch-based reviews

**Deliverables:**
- [ ] Session store implementation
- [ ] Session ID derivation (repo+branch hash)
- [ ] `bop sessions` commands
- [ ] Deduplication against prior session runs

**Effort:** Medium

### Phase 3.5c: Interactive TUI

**Scope:** Full TUI for finding review/selection/editing

**Deliverables:**
- [ ] Bubble Tea application scaffold
- [ ] Findings list view with navigation
- [ ] Finding detail view with code context
- [ ] Selection toggling
- [ ] Finding editor
- [ ] Post selected findings

**Effort:** Medium-High

### Phase 3.5d: MCP Review Tools

**Scope:** Expose review invocation via MCP

**Deliverables:**
- [ ] `review_pr` tool
- [ ] `review_branch` tool
- [ ] `post_findings` tool
- [ ] `edit_finding` tool
- [ ] Update MCP server with new tools

**Effort:** Medium

### Phase 3.5e: Streaming & Polish

**Scope:** UX improvements

**Deliverables:**
- [ ] MCP progress notifications for streaming findings
- [ ] TUI refinements based on usage
- [ ] Session auto-prune on review

**Effort:** Small

---

## 7. Out of Scope

| Item | Reason |
|------|--------|
| GitLab/Bitbucket support | Platform expansion is Phase 4 (Aspirational) |
| Multi-PR batch review | Complexity, unclear value |
| Persistent finding database | Sessions are ephemeral by design |
| Review scheduling/automation | Not a local mode concern |

---

## 8. Success Criteria

1. Can review any accessible GitHub PR without Actions setup
2. Can select/edit findings before posting
3. Local reviews persist across runs until branch deleted
4. Coding assistant can invoke reviews via MCP
5. TUI is intuitive and responsive

---

## 9. Open Questions

1. **Editor for findings:** Inline in TUI, or shell out to `$EDITOR`?
2. **GHE discovery:** Auto-detect from git remote, or require explicit config?
3. **Session storage location:** `~/.cache` vs `~/.local/share` vs configurable?
4. **Review without posting:** Should `bop review pr` require `--post` to post, or `--dry-run` to skip?

---

## 10. Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 0.1 | 2025-12-31 | Brandon | Initial design |
