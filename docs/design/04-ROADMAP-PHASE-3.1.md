# Phase 3.1 Implementation Roadmap

## Triage MCP Server (`code-reviewer-mcp`)

**Target Version:** v0.5.0
**Estimated Effort:** ~50 hours
**Status:** In Progress (M1, M2 handlers complete)

---

## Executive Summary

Phase 3.1 delivers an MCP server that enables AI assistants (Claude Code) to triage code review findings. The server provides **information and orchestration** while Claude Code handles file modifications using its native tools.

### Two Sources of Findings

| Source | API | Behavior |
|--------|-----|----------|
| **SARIF Annotations** | Check Runs API | Static analysis from latest check run, reset each push |
| **PR Comments** | Pull Request API | Reviewer feedback accumulated across commits |

Both must be queryable for complete triage.

### Key Deliverables

| Deliverable | Priority | Effort | Status |
|-------------|----------|--------|--------|
| MCP Server Binary | P0 | 8h | Done (M1) |
| Triage Use Case Layer | P0 | 10h | Done (M2) |
| Read Adapter Layer | P0 | 10h | **Pending (M2.5)** |
| GitHub Write Extensions | P0 | 8h | Pending (M3) |
| 12 MCP Tool Handlers | P0 | 12h | Handlers done, adapters pending |
| Triage Skill Document | P1 | 2h | Pending (M5) |
| Integration Tests | P0 | 4h | Pending (M4) |

---

## Milestone Overview

```
M1: Foundation        M2: Core Tools       M2.5: Adapters       M3: Write Tools      M4-M5: Test/Polish
├── Domain types      ├── 7 read handlers  ├── PRReader         ├── Write handlers   ├── Unit tests
├── Service skeleton  ├── Use case logic   ├── CommentReader    ├── Thread ops       ├── Integration
├── MCP server setup  ├── Tool registration├── Wire to server   ├── Review mgmt      ├── Skill doc
│                     │                    │                    │                    │
│  DONE               │  DONE (handlers)   │  PENDING #130      │  PENDING #120      │  PENDING
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### Dependency Chain

```
M1 (Foundation)
    └── M2 (Core Tool Handlers) - DONE
            └── M2.5 (Adapter Layer) - #130
                    └── M3 (Write Tools) - #120
                            └── M4 (Testing)
                                    └── M5 (Polish)
```

---

## Milestone 1: Foundation (8 hours) - COMPLETE

**Goal:** Establish project structure, domain types, and MCP server skeleton.

### Deliverables - All Complete

- [x] `cmd/code-reviewer-mcp/main.go` - Entry point, config loading
- [x] `internal/domain/triage.go` - TriageStatus, TriageFinding types
- [x] `internal/domain/annotation.go` - Annotation, CheckRunSummary types
- [x] `internal/domain/findings.go` - PRFinding, PRMetadata types
- [x] `internal/usecase/triage/ports.go` - Port interface definitions
- [x] `internal/usecase/triage/pr_service.go` - PR-based triage service
- [x] `internal/adapter/mcp/server.go` - MCP server initialization

---

## Milestone 2: Core Tools (10 hours) - HANDLERS COMPLETE

**Goal:** Implement the 7 read-only information tools.

**Status:** MCP handlers and use case logic are implemented. Tools return "not implemented" until M2.5 provides the adapter layer.

### Tools Implemented

| Tool | Source | Handler | Use Case | Adapter |
|------|--------|---------|----------|---------|
| `list_annotations` | SARIF | Done | Done | **Needs PRReader** |
| `get_annotation` | SARIF | Done | Done | Done (checkruns.go) |
| `list_findings` | PR Comments | Done | Done | **Needs CommentReader** |
| `get_finding` | PR Comments | Done | Done | **Needs CommentReader** |
| `get_suggestion` | Both | Done | Stub | **Needs SuggestionExtractor** |
| `get_code_context` | Git | Done | Done | **Needs PRReader** |
| `get_diff_context` | Git | Done | Done | **Needs PRReader** |

### Acceptance Criteria

- [x] All 7 tools registered and callable via MCP
- [x] Tools return proper JSON responses
- [x] Error handling returns structured MCP errors
- [x] Handlers implemented with proper validation
- [ ] **Blocked:** Tools functional end-to-end (requires M2.5)

---

## Milestone 2.5: Adapter Layer (10 hours) - PENDING

**Issue:** #130
**Goal:** Implement GitHub adapter methods required by M2 read tools.

**Why This Exists:** M2 implemented handlers and use case logic, but the underlying adapter implementations (GitHub API calls) were implicitly assumed but not explicitly scoped. This milestone completes the adapter layer.

### Tasks

| Task | Effort | Enables |
|------|--------|---------|
| 2.5.1 PRReader - GetPRMetadata | 1.5h | list_annotations, get_code_context, get_diff_context |
| 2.5.2 CommentReader - ListPRComments | 2h | list_findings |
| 2.5.3 CommentReader - GetPRComment | 1h | get_finding |
| 2.5.4 CommentReader - GetPRCommentByFingerprint | 1.5h | get_finding |
| 2.5.5 CommentReader - GetThreadHistory | 1h | get_finding (thread context) |
| 2.5.6 Wire adapters into MCP server | 1h | All M2 tools |
| 2.5.7 Integration tests | 2h | Verification |

### Port Interfaces (Already Defined in ports.go)

```go
type PRReader interface {
    GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error)
}

type CommentReader interface {
    ListPRComments(ctx context.Context, owner, repo string, prNumber int, filterByFingerprint bool) ([]domain.PRFinding, error)
    GetPRComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*domain.PRFinding, error)
    GetPRCommentByFingerprint(ctx context.Context, owner, repo string, prNumber int, fingerprint string) (*domain.PRFinding, error)
    GetThreadHistory(ctx context.Context, owner, repo string, commentID int64) ([]domain.ThreadComment, error)
}
```

### Deliverables

- [ ] `internal/adapter/github/pr_metadata.go` - PRReader implementation
- [ ] `internal/adapter/github/pr_comments.go` - CommentReader implementation
- [ ] Update `internal/adapter/mcp/server.go` - Wire adapters to PRService
- [ ] Unit tests for new adapter methods
- [ ] Integration test: end-to-end M2 tool calls

### Acceptance Criteria

- [ ] `list_annotations` returns real SARIF data for a PR
- [ ] `list_findings` returns real PR comment findings
- [ ] `get_finding` works with both fingerprint and comment ID
- [ ] `get_code_context` returns file content from PR head
- [ ] `get_diff_context` returns diff hunks
- [ ] All tests pass

---

## Milestone 3: GitHub Integration (9.5 hours) - PENDING

**Issue:** #120
**Depends On:** #130 (M2.5)
**Goal:** Implement GitHub adapter write extensions and 5 write tool handlers.

### Tasks

| Task | Effort | Dependencies |
|------|--------|--------------|
| 3.1 `comments.go` - Thread write operations | 2h | M2.5 |
| 3.2 `reviews.go` - Review listing/dismissal | 2h | M2.5 |
| 3.3 `get_thread` handler | 1h | 3.1 |
| 3.4 `reply_to_finding` handler | 1h | 3.1 |
| 3.5 `post_comment` handler | 1.5h | 3.1 |
| 3.6 `mark_resolved` handler | 1h | 3.1 |
| 3.7 `request_rereview` handler | 1h | 3.2 |

### Tool Specifications

| Tool | Purpose |
|------|---------|
| `get_thread` | Get full comment thread for a review comment |
| `reply_to_finding` | Reply to PR comment thread with optional status |
| `post_comment` | Create new comment at file/line (for SARIF responses) |
| `mark_resolved` | Mark a review thread as resolved |
| `request_rereview` | Dismiss stale reviews and request fresh review |

### Note: Responding to SARIF Annotations

Check run annotations (SARIF findings) **cannot be replied to directly** via GitHub API. To respond to a SARIF finding, use `post_comment` to create a new PR review comment at the same file/line location.

### Acceptance Criteria

- [ ] All 5 GitHub tools implemented and callable
- [ ] Thread resolution works correctly
- [ ] Reply creates proper threaded comments
- [ ] `post_comment` creates new comment at file/line
- [ ] Stale review dismissal works
- [ ] Rate limiting handled gracefully

---

## Milestone 4: Testing (4 hours) - PENDING

**Goal:** Comprehensive test coverage for all components.

### Tasks

| Task | Effort | Dependencies |
|------|--------|--------------|
| 4.1 Unit tests for service layer | 1.5h | M2.5, M3 |
| 4.2 Unit tests for MCP handlers | 1h | M2.5, M3 |
| 4.3 Integration tests (GitHub mock) | 1h | M3 |
| 4.4 E2E workflow test | 0.5h | M3 |

### Acceptance Criteria

- [ ] >80% code coverage on service layer
- [ ] All MCP tool handlers have unit tests
- [ ] Integration test passes with mock GitHub
- [ ] E2E workflow test demonstrates full flow
- [ ] `go test -race ./...` passes

---

## Milestone 5: Polish & Documentation (4 hours) - PENDING

**Goal:** Skill document, user documentation, and edge case handling.

### Tasks

| Task | Effort | Dependencies |
|------|--------|--------------|
| 5.1 Create triage-pr-review skill | 1.5h | M4 |
| 5.2 Update README with MCP docs | 1h | M4 |
| 5.3 Edge case handling | 1h | M4 |
| 5.4 Error messages review | 0.5h | M4 |

### Skill Document Outline

Location: `skills/triage-pr-review/SKILL.md`

```markdown
# Triage PR Code Review Skill

## Available Tools

### Read Tools (Information)
- `list_annotations` - SARIF findings from check runs
- `get_annotation` - Single annotation details
- `list_findings` - PR comment findings
- `get_finding` - Single finding details
- `get_suggestion` - Structured code suggestion
- `get_code_context` - Current file content
- `get_diff_context` - Diff hunk at location

### Write Tools (GitHub Actions)
- `get_thread` - Full comment thread
- `reply_to_finding` - Reply with status
- `post_comment` - New comment (for SARIF responses)
- `mark_resolved` - Close resolved threads
- `request_rereview` - Request fresh review

## Workflow
1. Check BOTH sources: `list_annotations` + `list_findings`
2. For each finding: evaluate, decide action
3. Apply fixes using native `str_replace`
4. Update status: `reply_to_finding` or `post_comment`
5. After changes: `request_rereview`
```

---

## Tool Summary

### Read Tools (7) - M2

| Tool | Source | Purpose |
|------|--------|---------|
| `list_annotations` | SARIF | Annotations for HEAD commit |
| `get_annotation` | SARIF | Single annotation details |
| `list_findings` | PR Comments | PR comment findings (filterable) |
| `get_finding` | PR Comments | Single finding with thread |
| `get_suggestion` | Both | Structured data for str_replace |
| `get_code_context` | Git | Current code at location |
| `get_diff_context` | Git | Diff hunk at location |

### Write Tools (5) - M3

| Tool | Purpose |
|------|---------|
| `get_thread` | Full comment thread history |
| `reply_to_finding` | Reply to PR comment with status |
| `post_comment` | New comment at file/line |
| `mark_resolved` | Mark thread resolved |
| `request_rereview` | Dismiss stale reviews |

**Total: 12 MCP tools**

---

## Success Criteria

### Functional

- [ ] All 12 MCP tools working via Claude Code
- [ ] Can triage findings end-to-end (list → fix → commit → update)
- [ ] Works with both SARIF annotations and PR comments
- [ ] Works with real GitHub PRs
- [ ] Handles edge cases gracefully

### Non-Functional

- [ ] Response time <2s for most operations
- [ ] Clear error messages for all failure modes
- [ ] >80% test coverage
- [ ] Documentation complete

---

## Revision History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-28 | Initial roadmap |
| 1.1 | 2025-12-29 | Added M2.5 milestone (#130) for adapter layer. Updated tool count to 12 (7 read + 5 write). Clarified M2 status (handlers done, adapters pending). Added dependency chain diagram. |
