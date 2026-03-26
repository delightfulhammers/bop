# TDD-001: Issue Comments Cache and Pagination

## Overview

`ListIssueComments` fetches all issue comments on a PR via the GitHub REST API. On PRs with extensive discussion histories (100+ comments across multiple pages), this is slow and memory-intensive. It impacts two hot paths:

- **MCP triage tools** (`list_findings`) — fetches all comments to present findings to the user.
- **PostReview deduplication** — fetches comments to avoid re-posting out-of-diff findings.

This TDD covers the in-memory cache layer and the caller-controlled pagination limit added to address this.

## Components

### `issueCommentsCache` (`internal/adapter/github/issue_comments.go`)

A concurrency-safe, TTL-based in-memory cache for `ListIssueComments` results.

**Key design decisions:**

1. **TTL of 2 minutes.** Chosen to cover a typical triage session where multiple MCP tool calls hit the same PR in quick succession. Short enough that stale data is unlikely to cause confusion.

2. **Cache key: `(owner, repo, prNumber)`.** One entry per PR. No per-user scoping since the GitHub API returns the same comments regardless of authenticated user.

3. **Stored as pointer in `Client`.** The cache contains a `sync.Mutex`. Storing by value would make `Client` unsafe to copy — a well-known Go footgun. The pointer ensures the mutex is shared, not duplicated.

4. **No single-flight coalescing.** Concurrent callers for the same key may both get a cache miss and both fetch. This is acceptable for a performance-only cache. The comment on the struct documents this explicitly. If single-flight semantics are needed in the future, use `golang.org/x/sync/singleflight`.

5. **Defensive copy on read and write.** The cached slice is copied when stored and when returned, preventing callers from mutating cached data.

6. **Partial fetches bypass the cache.** When `MaxPages > 0`, the result is incomplete and must not be cached. Only unlimited (full) fetches populate and read from the cache.

### `ListIssueCommentsOptions` (`internal/usecase/triage/ports.go`)

A pagination options struct accepted as a variadic parameter by `ListIssueComments`.

**Key design decisions:**

1. **Lives in the usecase port layer, not the adapter.** It represents a caller-controlled data budget — the usecase tells the adapter how much data it needs. Moving it to the adapter would create a circular dependency since the adapter already imports the triage package.

2. **`MaxPages` field.** 0 means unlimited (existing behavior). Positive values stop pagination after N pages. Checked before the existing `maxPaginationPages=100` hard safety cap.

3. **Cross-usecase import.** The `github` usecase package imports `triage.ListIssueCommentsOptions` for interface consistency. This is documented as intentional in the interface comment.

## Interfaces

```go
// In internal/usecase/triage/ports.go
type ListIssueCommentsOptions struct {
    MaxPages int // 0 = unlimited
}

type IssueCommentReader interface {
    ListIssueComments(ctx context.Context, owner, repo string, prNumber int,
        opts ...ListIssueCommentsOptions) ([]IssueComment, error)
    // ...
}

// In internal/adapter/github/issue_comments.go
func (c *Client) ClearIssueCommentsCache()
```

## Data Model

```
issueCommentsCacheKey {
    Owner    string
    Repo     string
    PRNumber int
}

issueCommentsCacheEntry {
    comments  []IssueComment   // defensive copy of fetched results
    fetchedAt time.Time         // for TTL expiration check
}

issueCommentsCache {
    mu      sync.Mutex
    entries map[issueCommentsCacheKey]issueCommentsCacheEntry
}
```

## Cache Invalidation

- **On post:** `CreateIssueComment` deletes the cache entry for the affected PR after a successful post.
- **On TTL expiry:** Entries older than 2 minutes are treated as stale on the next read.
- **Explicit clear:** `ClearIssueCommentsCache()` removes all entries (used in tests).

## Caller Behavior

| Caller | MaxPages | Cache | Rationale |
|--------|----------|-------|-----------|
| `ListAllFindings` (triage) | 0 (unlimited) | Yes | User explicitly wants all findings; cache avoids repeat fetches in a triage session |
| `PostReview` (dedup) | 0 (unlimited) | Yes | Dedup needs complete data; cache already bounds the cost |

## Open Questions

None — all design decisions are resolved and documented.
