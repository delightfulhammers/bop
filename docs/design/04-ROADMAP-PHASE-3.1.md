# Phase 3.1 Implementation Roadmap

## Triage MCP Server (`code-reviewer-mcp`)

**Target Version:** v0.5.0  
**Estimated Effort:** ~40 hours  
**Status:** Planning Complete

---

## Executive Summary

Phase 3.1 delivers an MCP server that enables AI assistants (Claude Code) to triage code review findings. The server provides **information and orchestration** while Claude Code handles file modifications using its native tools.

### Key Deliverables

| Deliverable | Priority | Effort |
|-------------|----------|--------|
| MCP Server Binary | P0 | 8h |
| Triage Use Case Layer | P0 | 10h |
| GitHub Adapter Extensions | P0 | 8h |
| 9 MCP Tool Handlers | P0 | 10h |
| Triage Skill Document | P1 | 2h |
| Integration Tests | P0 | 4h |

---

## Milestone Overview

```
Week 1                          Week 2                          Week 3
├─ M1: Foundation ──────────────├─ M3: GitHub Integration ──────├─ M5: Polish ─────────┤
│  Domain types                 │  Comment threading            │  Skill document      │
│  Service skeleton             │  Review management            │  Documentation       │
│  MCP server setup             │  Thread resolution            │  Edge cases          │
│                               │                               │                      │
├─ M2: Core Tools ──────────────├─ M4: Testing ─────────────────├─ Release ────────────┤
│  list_findings                │  Unit tests                   │  v0.5.0              │
│  get_finding                  │  Integration tests            │                      │
│  get_suggestion               │  E2E workflow test            │                      │
│  get_code_context             │                               │                      │
│  get_diff_context             │                               │                      │
```

---

## Milestone 1: Foundation (8 hours)

**Goal:** Establish project structure, domain types, and MCP server skeleton.

### Tasks

| Task | Effort | Dependencies |
|------|--------|--------------|
| 1.1 Create `cmd/code-reviewer-mcp/main.go` | 1h | None |
| 1.2 Add mcp-go dependency | 0.5h | None |
| 1.3 Define domain types (`internal/domain/triage.go`) | 2h | None |
| 1.4 Create service interface (`internal/usecase/triage/interfaces.go`) | 1.5h | 1.3 |
| 1.5 Implement service skeleton (`internal/usecase/triage/service.go`) | 2h | 1.4 |
| 1.6 MCP server setup (`internal/adapter/mcp/server.go`) | 1h | 1.2 |

### Deliverables

- [ ] `cmd/code-reviewer-mcp/main.go` — Entry point, config loading, server startup
- [ ] `internal/domain/triage.go` — TriageStatus, TriageFinding, SuggestionBlock types
- [ ] `internal/usecase/triage/interfaces.go` — GitHubClient, GitEngine port definitions
- [ ] `internal/usecase/triage/service.go` — Service struct with dependency injection
- [ ] `internal/adapter/mcp/server.go` — MCP server initialization

### Acceptance Criteria

- [ ] `go build ./cmd/code-reviewer-mcp` succeeds
- [ ] Server starts and accepts MCP connections (stdio transport)
- [ ] `mcp-go` integrated and compiling
- [ ] Domain types have JSON tags for serialization

### Domain Types

```go
// internal/domain/triage.go

package domain

// TriageStatus represents the disposition of a finding.
type TriageStatus string

const (
    TriageStatusPending     TriageStatus = "pending"
    TriageStatusAcknowledged TriageStatus = "acknowledged"
    TriageStatusDisputed    TriageStatus = "disputed"
    TriageStatusFixed       TriageStatus = "fixed"
    TriageStatusWontFix     TriageStatus = "wont_fix"
)

// TriageFinding extends Finding with triage metadata.
type TriageFinding struct {
    Finding                     // Embedded base finding
    CommentID     int64         `json:"commentId"`
    ThreadID      int64         `json:"threadId"`
    Status        TriageStatus  `json:"status"`
    ReplyCount    int           `json:"replyCount"`
    IsResolved    bool          `json:"isResolved"`
    LastUpdated   time.Time     `json:"lastUpdated"`
}

// SuggestionBlock contains data for Claude Code to apply a fix.
type SuggestionBlock struct {
    FindingID     string `json:"findingId"`
    File          string `json:"file"`
    LineStart     int    `json:"lineStart"`
    LineEnd       int    `json:"lineEnd"`
    OriginalCode  string `json:"originalCode"`
    SuggestedCode string `json:"suggestedCode"`
    ContextBefore string `json:"contextBefore,omitempty"`
    ContextAfter  string `json:"contextAfter,omitempty"`
}

// CommentThread represents a GitHub review comment thread.
type CommentThread struct {
    ThreadID    int64           `json:"threadId"`
    CommentID   int64           `json:"commentId"`   // Root comment
    File        string          `json:"file"`
    LineStart   int             `json:"lineStart"`
    LineEnd     int             `json:"lineEnd"`
    IsResolved  bool            `json:"isResolved"`
    IsOutdated  bool            `json:"isOutdated"`
    Comments    []ThreadComment `json:"comments"`
}

// ThreadComment represents a single comment in a thread.
type ThreadComment struct {
    ID        int64     `json:"id"`
    Author    string    `json:"author"`
    Body      string    `json:"body"`
    CreatedAt time.Time `json:"createdAt"`
    IsBot     bool      `json:"isBot"`
}
```

---

## Milestone 2: Core Tools (10 hours)

**Goal:** Implement the 5 read-only information tools.

### Tasks

| Task | Effort | Dependencies |
|------|--------|--------------|
| 2.1 Tool registration framework | 1h | M1 |
| 2.2 `list_findings` handler | 2h | 2.1 |
| 2.3 `get_finding` handler | 1.5h | 2.1 |
| 2.4 `get_suggestion` handler | 2h | 2.1 |
| 2.5 `get_code_context` handler | 1.5h | 2.1 |
| 2.6 `get_diff_context` handler | 2h | 2.1 |

### Tool Specifications

#### `list_findings`

```yaml
name: list_findings
description: List code review findings for a PR with optional filters
parameters:
  pr_number:
    type: integer
    required: true
    description: Pull request number
  status:
    type: string
    enum: [pending, acknowledged, disputed, fixed, wont_fix]
    description: Filter by triage status
  severity:
    type: string
    enum: [critical, high, medium, low]
    description: Filter by severity
  category:
    type: string
    description: Filter by category (security, bug, etc.)
returns:
  findings: array of TriageFinding
  total: integer
```

#### `get_finding`

```yaml
name: get_finding
description: Get detailed information about a specific finding
parameters:
  finding_id:
    type: string
    required: true
    description: Finding ID (fingerprint or comment ID)
returns:
  finding: TriageFinding with full details
  thread: Associated comment thread
```

#### `get_suggestion`

```yaml
name: get_suggestion
description: Get structured suggestion data for applying a fix
parameters:
  finding_id:
    type: string
    required: true
    description: Finding ID
returns:
  suggestion: SuggestionBlock with exact code for str_replace
```

#### `get_code_context`

```yaml
name: get_code_context
description: Get current file content around specified lines
parameters:
  file:
    type: string
    required: true
    description: File path relative to repo root
  line_start:
    type: integer
    required: true
  line_end:
    type: integer
    required: true
  context_lines:
    type: integer
    default: 5
    description: Lines of context before/after
returns:
  content: string (the code)
  line_start: actual start line (with context)
  line_end: actual end line (with context)
```

#### `get_diff_context`

```yaml
name: get_diff_context
description: Get the diff hunk containing specified lines
parameters:
  pr_number:
    type: integer
    required: true
  file:
    type: string
    required: true
  line_start:
    type: integer
    required: true
  line_end:
    type: integer
    required: true
returns:
  hunk: string (unified diff format)
  base_branch: string
```

### Acceptance Criteria

- [ ] All 5 tools registered and callable via MCP
- [ ] Tools return proper JSON responses
- [ ] Error handling returns structured MCP errors
- [ ] Unit tests for each handler (mock dependencies)

---

## Milestone 3: GitHub Integration (8 hours)

**Goal:** Implement GitHub adapter extensions and remaining 4 tools.

### Tasks

| Task | Effort | Dependencies |
|------|--------|--------------|
| 3.1 `comments.go` - Thread operations | 2h | M1 |
| 3.2 `reviews.go` - Review listing/dismissal | 2h | M1 |
| 3.3 `get_thread` handler | 1h | 3.1 |
| 3.4 `reply_to_finding` handler | 1h | 3.1 |
| 3.5 `mark_resolved` handler | 1h | 3.1 |
| 3.6 `request_rereview` handler | 1h | 3.2 |

### Tool Specifications

#### `get_thread`

```yaml
name: get_thread
description: Get full comment thread for a review comment
parameters:
  comment_id:
    type: integer
    required: true
    description: Root comment ID of the thread
returns:
  thread: CommentThread with all replies
```

#### `reply_to_finding`

```yaml
name: reply_to_finding
description: Reply to a finding thread with optional status update
parameters:
  pr_number:
    type: integer
    required: true
  comment_id:
    type: integer
    required: true
    description: Comment ID to reply to
  body:
    type: string
    required: true
    description: Reply content (markdown)
  status:
    type: string
    enum: [acknowledged, disputed, fixed, wont_fix]
    description: Optional status declaration
returns:
  comment_id: ID of created reply
  url: Link to the comment
```

#### `mark_resolved`

```yaml
name: mark_resolved
description: Mark a review thread as resolved
parameters:
  pr_number:
    type: integer
    required: true
  comment_id:
    type: integer
    required: true
    description: Root comment ID of thread to resolve
returns:
  success: boolean
```

#### `request_rereview`

```yaml
name: request_rereview
description: Dismiss stale reviews and request fresh review
parameters:
  pr_number:
    type: integer
    required: true
returns:
  dismissed_count: number of reviews dismissed
  message: Status message
```

### GitHub API Extensions

```go
// internal/adapter/github/comments.go

// GetReviewComments returns all review comments for a PR.
func (c *Client) GetReviewComments(ctx context.Context, owner, repo string, prNumber int) ([]ReviewComment, error)

// GetCommentThread returns a comment and all its replies.
func (c *Client) GetCommentThread(ctx context.Context, owner, repo string, commentID int64) (*CommentThread, error)

// CreateReplyComment creates a reply to a review comment.
func (c *Client) CreateReplyComment(ctx context.Context, owner, repo string, prNumber int, commentID int64, body string) (*ReviewComment, error)

// ResolveThread marks a review thread as resolved.
func (c *Client) ResolveThread(ctx context.Context, owner, repo string, threadID int64) error
```

```go
// internal/adapter/github/reviews.go

// ListReviews returns all reviews for a PR (extends existing).
// Already exists - may need to add filtering by author.

// DismissReview dismisses a review (extends existing).
// Already exists.

// DismissStaleReviews dismisses all reviews from a specific user.
func (c *Client) DismissStaleReviews(ctx context.Context, owner, repo string, prNumber int, botUsername, message string) (int, error)
```

### Acceptance Criteria

- [ ] All 4 GitHub tools implemented and callable
- [ ] Thread resolution works correctly
- [ ] Reply creates proper threaded comments
- [ ] Stale review dismissal works
- [ ] Rate limiting handled gracefully

---

## Milestone 4: Testing (4 hours)

**Goal:** Comprehensive test coverage for all components.

### Tasks

| Task | Effort | Dependencies |
|------|--------|--------------|
| 4.1 Unit tests for service layer | 1.5h | M2, M3 |
| 4.2 Unit tests for MCP handlers | 1h | M2, M3 |
| 4.3 Integration tests (GitHub mock) | 1h | M3 |
| 4.4 E2E workflow test | 0.5h | M3 |

### Test Strategy

#### Unit Tests

```go
// internal/usecase/triage/service_test.go

func TestService_ListFindings(t *testing.T) {
    // Mock GitHub client returns sample comments
    // Verify filtering works correctly
    // Verify status detection from comment body
}

func TestService_GetSuggestion(t *testing.T) {
    // Mock file content
    // Verify exact code extraction
    // Verify suggestion parsing (```suggestion blocks)
    // Test edge cases: first/last lines, multi-line
}

func TestService_ParseSuggestionCode(t *testing.T) {
    // Test GitHub suggestion syntax
    // Test generic code blocks
    // Test plain text suggestions
}
```

#### Integration Tests

```go
// internal/adapter/mcp/handlers/integration_test.go

func TestMCPTools_Integration(t *testing.T) {
    // Start MCP server with mocked dependencies
    // Call each tool via MCP protocol
    // Verify responses match expected format
}
```

#### E2E Test

```go
// cmd/code-reviewer-mcp/e2e_test.go

func TestE2E_TriageWorkflow(t *testing.T) {
    // 1. list_findings → returns sample findings
    // 2. get_finding → returns detailed finding
    // 3. get_suggestion → returns apply-ready data
    // 4. reply_to_finding → creates comment
    // 5. mark_resolved → resolves thread
}
```

### Acceptance Criteria

- [ ] >80% code coverage on service layer
- [ ] All MCP tool handlers have unit tests
- [ ] Integration test passes with mock GitHub
- [ ] E2E workflow test demonstrates full flow
- [ ] `go test -race ./...` passes

---

## Milestone 5: Polish & Documentation (4 hours)

**Goal:** Skill document, user documentation, and edge case handling.

### Tasks

| Task | Effort | Dependencies |
|------|--------|--------------|
| 5.1 Create triage-pr-review skill | 1.5h | M4 |
| 5.2 Update README with MCP docs | 1h | M4 |
| 5.3 Edge case handling | 1h | M4 |
| 5.4 Error messages review | 0.5h | M4 |

### Skill Document

Location: `/mnt/skills/user/triage-pr-review/SKILL.md`

```markdown
# Triage PR Code Review Skill

## Purpose
Efficiently triage AI-generated code review findings using the code-reviewer MCP server.

## Available Tools
- `list_findings` - List findings with filters
- `get_finding` - Get finding details
- `get_suggestion` - Get apply-ready suggestion
- `get_code_context` - Read current code
- `get_diff_context` - See the diff
- `get_thread` - Read discussion thread
- `reply_to_finding` - Reply with status
- `mark_resolved` - Close resolved threads
- `request_rereview` - Request fresh review

## Workflow

### 1. Review Findings
```
list_findings(pr_number=123, status="pending")
```

### 2. Evaluate Each Finding
For each finding:
1. `get_finding(finding_id)` - Understand the issue
2. `get_code_context(file, line_start, line_end)` - See current code
3. Decide: fix, dispute, or acknowledge

### 3. Apply Fixes (Use Native Tools)
```
# Get the suggestion
get_suggestion(finding_id) → returns OriginalCode, SuggestedCode

# Apply using native str_replace
str_replace(
  path=suggestion.file,
  old_str=suggestion.original_code,
  new_str=suggestion.suggested_code
)
```

### 4. Handle Multiple Fixes in Same File
Apply bottom-up (highest line first) to avoid drift:
```
findings_in_file = sorted(findings, key=line_start, reverse=True)
for finding in findings_in_file:
    suggestion = get_suggestion(finding.id)
    str_replace(path, suggestion.original_code, suggestion.suggested_code)
```

### 5. Commit and Update Status
```bash
git add -A
git commit -m "fix: address code review findings"
git push
```

Then update GitHub:
```
reply_to_finding(pr_number, comment_id, "Fixed in <commit>", status="fixed")
mark_resolved(pr_number, comment_id)
```

### 6. Request Re-review
```
request_rereview(pr_number)
```

## Best Practices
- Process one file at a time for complex changes
- Always verify original code matches before applying
- Use `get_thread` to understand prior discussion
- Dispute with evidence, not just disagreement
```

### Edge Cases to Handle

| Edge Case | Handling |
|-----------|----------|
| Finding on deleted file | Return error with clear message |
| Line numbers out of bounds | Return error, suggest re-review |
| No suggestion in finding | Return error in `get_suggestion` |
| Thread already resolved | No-op for `mark_resolved` |
| Rate limit hit | Retry with backoff, surface to user |
| Large PR (>100 findings) | Pagination in `list_findings` |

### Acceptance Criteria

- [ ] Skill document is clear and actionable
- [ ] README documents MCP server setup
- [ ] Edge cases return helpful error messages
- [ ] Error messages guide user to resolution

---

## Implementation Schedule

### Week 1 (Days 1-5)

| Day | Focus | Hours | Milestone |
|-----|-------|-------|-----------|
| 1 | Project setup, domain types | 4h | M1 |
| 2 | Service skeleton, MCP server | 4h | M1 |
| 3 | list_findings, get_finding | 4h | M2 |
| 4 | get_suggestion, get_code_context | 4h | M2 |
| 5 | get_diff_context, tool registration | 3h | M2 |

### Week 2 (Days 6-10)

| Day | Focus | Hours | Milestone |
|-----|-------|-------|-----------|
| 6 | GitHub comments adapter | 4h | M3 |
| 7 | GitHub reviews adapter | 4h | M3 |
| 8 | Remaining 4 tool handlers | 4h | M3 |
| 9 | Unit tests | 3h | M4 |
| 10 | Integration tests, E2E | 2h | M4 |

### Week 3 (Days 11-12)

| Day | Focus | Hours | Milestone |
|-----|-------|-------|-----------|
| 11 | Skill document, README | 2.5h | M5 |
| 12 | Edge cases, polish, release | 1.5h | M5 |

**Total: ~40 hours**

---

## Risk Mitigation

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| mcp-go SDK issues | High | Medium | Evaluate SDK early; fallback to raw JSON-RPC |
| GitHub API rate limits | Medium | Medium | Implement caching, conditional requests |
| Line number drift | Medium | Low | Exact text matching handles this |
| Large PR performance | Low | Medium | Pagination, streaming responses |
| Thread resolution API changes | Low | Low | Abstract behind interface |

---

## Success Criteria

### Functional

- [ ] All 9 MCP tools working via Claude Code
- [ ] Can triage findings end-to-end (list → fix → commit → update)
- [ ] Works with real GitHub PRs
- [ ] Handles edge cases gracefully

### Non-Functional

- [ ] Response time <2s for most operations
- [ ] Clear error messages for all failure modes
- [ ] >80% test coverage
- [ ] Documentation complete

### User Experience

- [ ] Skill document enables productive triage
- [ ] Workflow feels natural in Claude Code
- [ ] Status updates visible in GitHub UI
- [ ] Fixed findings get marked resolved

---

## Post-Release Enhancements (Phase 3.2+)

After v0.5.0, consider:

1. **Batch Operations** — `list_findings` → `batch_reply` for bulk status updates
2. **Confidence Calibration** — Track accept/reject rates per provider/category
3. **Learning Loop** — Feed triage decisions back to improve reviews
4. **IDE Integration** — VS Code extension using same MCP server
5. **Metrics Dashboard** — Triage velocity, false positive rates

---

## Appendix: Configuration

### MCP Server Config

```yaml
# ~/.config/cr/mcp.yaml
server:
  transport: stdio  # or http for remote
  
github:
  # Uses GITHUB_TOKEN from environment
  owner: "bkyoung"
  repo: "code-reviewer"

git:
  repo_dir: "."  # Current directory

logging:
  level: info
  format: json
```

### Claude Desktop Config

```json
{
  "mcpServers": {
    "code-reviewer": {
      "command": "code-reviewer-mcp",
      "args": ["--config", "~/.config/cr/mcp.yaml"],
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      }
    }
  }
}
```