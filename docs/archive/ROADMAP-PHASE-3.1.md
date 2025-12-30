# Phase 3.1 Implementation Roadmap

## Triage MCP Server (`code-reviewer-mcp`)

**Target Version:** v0.5.0
**Estimated Effort:** ~50 hours
**Status:** ✅ COMPLETE (All milestones delivered)

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
| MCP Server Binary | P0 | 8h | ✅ Done (M1) |
| Triage Use Case Layer | P0 | 10h | ✅ Done (M2) |
| Read Adapter Layer | P0 | 10h | ✅ Done (M2.5) |
| GitHub Write Extensions | P0 | 8h | ✅ Done (M3) |
| 12 MCP Tool Handlers | P0 | 12h | ✅ Done |
| Triage Skill Document | P1 | 2h | ✅ Done (M5) |
| Integration Tests | P0 | 4h | ✅ Done (M4) |

---

## Milestone Overview

```
M1: Foundation        M2: Core Tools       M2.5: Adapters       M3: Write Tools      M4-M5: Test/Polish
├── Domain types      ├── 7 read handlers  ├── PRReader         ├── Write handlers   ├── Unit tests
├── Service skeleton  ├── Use case logic   ├── CommentReader    ├── Thread ops       ├── Integration
├── MCP server setup  ├── Tool registration├── Wire to server   ├── Review mgmt      ├── Skill doc
│                     │                    │                    │                    │
│  ✅ DONE            │  ✅ DONE           │  ✅ DONE           │  ✅ DONE           │  ✅ DONE
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### Dependency Chain

```
M1 (Foundation) ✅
    └── M2 (Core Tool Handlers) ✅
            └── M2.5 (Adapter Layer) ✅
                    └── M3 (Write Tools) ✅
                            └── M4 (Testing) ✅
                                    └── M5 (Polish) ✅  ← PHASE 3.1 COMPLETE
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

## Milestone 2: Core Tools (10 hours) - COMPLETE

**Goal:** Implement the 7 read-only information tools.

**Status:** All MCP handlers, use case logic, and adapters are fully implemented.

### Tools Implemented

| Tool | Source | Handler | Use Case | Adapter |
|------|--------|---------|----------|---------|
| `list_annotations` | SARIF | ✅ | ✅ | ✅ |
| `get_annotation` | SARIF | ✅ | ✅ | ✅ |
| `list_findings` | PR Comments | ✅ | ✅ | ✅ |
| `get_finding` | PR Comments | ✅ | ✅ | ✅ |
| `get_suggestion` | Both | ✅ | ✅ | ✅ |
| `get_code_context` | Git | ✅ | ✅ | ✅ |
| `get_diff_context` | Git | ✅ | ✅ | ✅ |

### Acceptance Criteria

- [x] All 7 tools registered and callable via MCP
- [x] Tools return proper JSON responses
- [x] Error handling returns structured MCP errors
- [x] Handlers implemented with proper validation
- [x] Tools functional end-to-end

---

## Milestone 2.5: Adapter Layer (10 hours) - COMPLETE

**Issue:** #130 (closed)
**Goal:** Implement GitHub adapter methods required by M2 read tools.

**Why This Existed:** M2 implemented handlers and use case logic, but the underlying adapter implementations (GitHub API calls) were implicitly assumed but not explicitly scoped. This milestone completed the adapter layer.

### Tasks

| Task | Effort | Status |
|------|--------|--------|
| 2.5.1 PRReader - GetPRMetadata | 1.5h | ✅ |
| 2.5.2 CommentReader - ListPRComments | 2h | ✅ |
| 2.5.3 CommentReader - GetPRComment | 1h | ✅ |
| 2.5.4 CommentReader - GetPRCommentByFingerprint | 1.5h | ✅ |
| 2.5.5 CommentReader - GetThreadHistory | 1h | ✅ |
| 2.5.6 Wire adapters into MCP server | 1h | ✅ |
| 2.5.7 Unit tests for adapters | 2h | ✅ |

### Deliverables

- [x] `internal/adapter/github/pr_metadata.go` - PRReader implementation
- [x] `internal/adapter/github/pr_comments.go` - CommentReader implementation
- [x] Update `internal/adapter/mcp/server.go` - Wire adapters to PRService
- [x] Unit tests for new adapter methods

### Acceptance Criteria

- [x] `list_annotations` returns real SARIF data for a PR
- [x] `list_findings` returns real PR comment findings
- [x] `get_finding` works with both fingerprint and comment ID
- [x] `get_code_context` returns file content from PR head
- [x] `get_diff_context` returns diff hunks
- [x] All tests pass

---

## Milestone 3: GitHub Integration (9.5 hours) - COMPLETE

**Issue:** #120 (closed)
**Depends On:** #130 (M2.5) ✅
**Goal:** Implement GitHub adapter write extensions and 5 write tool handlers.

### Tasks

| Task | Effort | Status |
|------|--------|--------|
| 3.1 `comment_writer.go` - Thread write operations | 2h | ✅ |
| 3.2 `review_manager.go` - Review listing/dismissal | 2h | ✅ |
| 3.3 `get_thread` handler | 1h | ✅ |
| 3.4 `reply_to_finding` handler | 1h | ✅ |
| 3.5 `post_comment` handler | 1.5h | ✅ |
| 3.6 `mark_resolved` handler | 1h | ✅ |
| 3.7 `request_rereview` handler | 1h | ✅ |

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

- [x] All 5 GitHub tools implemented and callable
- [x] Thread resolution works correctly
- [x] Reply creates proper threaded comments
- [x] `post_comment` creates new comment at file/line
- [x] Stale review dismissal works
- [x] Rate limiting handled gracefully

---

## Milestone 4: Testing (4 hours) - COMPLETE

**Issue:** #121 (closed)
**PR:** #139
**Goal:** Comprehensive test coverage for all components.

### Tasks

| Task | Effort | Status |
|------|--------|--------|
| 4.1 Unit tests for service layer | 1.5h | ✅ (`pr_service_test.go`, `suggestion_extractor_test.go`) |
| 4.2 Unit tests for MCP handlers | 1h | ✅ (`internal/adapter/mcp/handlers_test.go`) |
| 4.3 Integration tests (GitHub mock) | 1h | ✅ |
| 4.4 E2E workflow test | 0.5h | ✅ |

### Test Coverage

| Package | Tests Exist | Coverage |
|---------|-------------|----------|
| `internal/usecase/triage/` | ✅ | >80% |
| `internal/adapter/mcp/` | ✅ | >80% |
| `internal/adapter/github/` | ✅ | >80% |

### Acceptance Criteria

- [x] >80% code coverage on service layer
- [x] All MCP tool handlers have unit tests
- [x] Integration test passes with mock GitHub
- [x] E2E workflow test demonstrates full flow
- [x] `go test -race ./...` passes

---

## Milestone 5: Polish & Documentation (4 hours) - COMPLETE

**Issue:** #122 (closed)
**PR:** #140
**Goal:** Skill document, user documentation, and edge case handling.

### Tasks

| Task | Effort | Status |
|------|--------|--------|
| 5.1 Create triage-pr-review skill | 1.5h | ✅ |
| 5.2 Update README with MCP docs | 1h | ✅ |
| 5.3 Edge case handling | 1h | ✅ |
| 5.4 Error messages review | 0.5h | ✅ |

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

- [x] All 12 MCP tools working via Claude Code
- [x] Can triage findings end-to-end (list → fix → commit → update)
- [x] Works with both SARIF annotations and PR comments
- [x] Works with real GitHub PRs
- [x] Handles edge cases gracefully

### Non-Functional

- [x] Response time <2s for most operations
- [x] Clear error messages for all failure modes
- [x] >80% test coverage
- [x] Documentation complete

---

## Revision History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-28 | Initial roadmap |
| 1.1 | 2025-12-29 | Added M2.5 milestone (#130) for adapter layer. Updated tool count to 12 (7 read + 5 write). Clarified M2 status (handlers done, adapters pending). Added dependency chain diagram. |
| 1.2 | 2025-12-29 | Status sync: M1-M3 marked complete. M2.5 adapter layer done. M3 write tools done. M4 in progress (service tests exist, MCP handler tests missing). Updated all checklists and task tables. |
| 1.3 | 2025-12-30 | **Phase 3.1 Complete!** M4 (PR #139) and M5 (PR #140) merged. All milestones delivered. Updated all status indicators, acceptance criteria, and success criteria to reflect completion. |
