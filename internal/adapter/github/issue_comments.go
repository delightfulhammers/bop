package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/delightfulhammers/bop/internal/usecase/triage"
)

const issueCommentsCacheTTL = 2 * time.Minute

// issueCommentsCacheKey identifies a cached set of issue comments.
type issueCommentsCacheKey struct {
	Owner    string
	Repo     string
	PRNumber int
}

// issueCommentsCacheEntry holds cached results with a timestamp for TTL.
type issueCommentsCacheEntry struct {
	comments  []IssueComment
	fetchedAt time.Time
}

// issueCommentsCache provides a concurrency-safe in-memory cache for ListIssueComments.
// Note: the lock is released before HTTP fetches on cache miss, so concurrent callers
// for the same key may both fetch. This is acceptable for a performance-only cache —
// use golang.org/x/sync/singleflight if single-flight semantics are needed.
type issueCommentsCache struct {
	mu      sync.Mutex
	entries map[issueCommentsCacheKey]issueCommentsCacheEntry
}

func newIssueCommentsCache() *issueCommentsCache {
	return &issueCommentsCache{
		entries: make(map[issueCommentsCacheKey]issueCommentsCacheEntry),
	}
}

// outOfDiffPattern matches CR_OOD:true markers in comment bodies.
var outOfDiffPattern = regexp.MustCompile(`CR_OOD:true`)

// IssueComment represents a GitHub issue comment (PR conversation comment).
// Unlike review comments, issue comments don't have file paths or line numbers.
type IssueComment struct {
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	User      User   `json:"user"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	HTMLURL   string `json:"html_url"`
}

// HasFingerprint returns true if this comment contains a CR_FP marker.
func (c IssueComment) HasFingerprint() bool {
	return fingerprintPattern.MatchString(c.Body)
}

// IsOutOfDiffFinding returns true if this comment is an out-of-diff finding
// (contains both CR_FP and CR_OOD:true markers).
func (c IssueComment) IsOutOfDiffFinding() bool {
	return c.HasFingerprint() && outOfDiffPattern.MatchString(c.Body)
}

// CreateIssueComment posts an issue comment on a PR (in the conversation thread).
// GitHub API: POST /repos/{owner}/{repo}/issues/{issue_number}/comments
// Note: PRs are treated as issues in the GitHub API, so we use the issues endpoint.
// Returns the ID of the created comment.
func (c *Client) CreateIssueComment(ctx context.Context, owner, repo string, prNumber int, body string) (int64, error) {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return 0, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return 0, err
	}
	if prNumber <= 0 {
		return 0, fmt.Errorf("invalid PR number: %d (must be positive)", prNumber)
	}
	if strings.TrimSpace(body) == "" {
		return 0, fmt.Errorf("empty body: comment body is required")
	}

	// Build request body
	reqBody := struct {
		Body string `json:"body"`
	}{Body: body}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("marshal request: %w", err)
	}

	// GitHub API: POST /repos/{owner}/{repo}/issues/{issue_number}/comments
	apiURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), prNumber)

	var resp *http.Response
	err = llmhttp.RetryWithBackoff(ctx, func(ctx context.Context) error {
		req, reqErr := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonData))
		if reqErr != nil {
			return &llmhttp.Error{
				Type:      llmhttp.ErrTypeUnknown,
				Message:   reqErr.Error(),
				Retryable: false,
				Provider:  providerName,
			}
		}

		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		var callErr error
		resp, callErr = c.httpClient.Do(req)
		if callErr != nil {
			return llmhttp.ClassifyNetworkError(providerName, callErr, ctx)
		}

		if resp.StatusCode >= 400 {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				return &llmhttp.Error{
					Type:       llmhttp.ErrTypeUnknown,
					Message:    fmt.Sprintf("HTTP %d (failed to read response: %v)", resp.StatusCode, readErr),
					StatusCode: resp.StatusCode,
					Retryable:  resp.StatusCode >= 500,
					Provider:   providerName,
				}
			}
			return MapHTTPError(resp.StatusCode, bodyBytes)
		}

		return nil
	}, c.retryConf)

	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result IssueComment
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	// Invalidate cache for this PR since a new comment was added
	cacheKey := issueCommentsCacheKey{Owner: owner, Repo: repo, PRNumber: prNumber}
	c.issueCommentsCache.mu.Lock()
	delete(c.issueCommentsCache.entries, cacheKey)
	c.issueCommentsCache.mu.Unlock()

	return result.ID, nil
}

// ListIssueComments retrieves issue comments on a PR with in-memory caching.
// GitHub API: GET /repos/{owner}/{repo}/issues/{issue_number}/comments
// Note: PRs are treated as issues in the GitHub API, so we use the issues endpoint.
// Returns comments in chronological order (oldest first).
//
// Accepts an optional ListIssueCommentsOptions to control pagination. If MaxPages
// is set to a positive value, pagination stops after that many pages (caller-controlled
// soft limit). MaxPages=0 means unlimited (up to the hard cap of maxPaginationPages).
// When MaxPages is specified, caching is bypassed since partial results should not
// be cached.
func (c *Client) ListIssueComments(ctx context.Context, owner, repo string, prNumber int, opts ...triage.ListIssueCommentsOptions) ([]IssueComment, error) {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return nil, err
	}
	if prNumber <= 0 {
		return nil, fmt.Errorf("invalid PR number: %d (must be positive)", prNumber)
	}

	// Merge options
	var options triage.ListIssueCommentsOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	// Only use cache for unlimited (full) fetches
	useCache := options.MaxPages == 0
	cacheKey := issueCommentsCacheKey{Owner: owner, Repo: repo, PRNumber: prNumber}

	if useCache {
		c.issueCommentsCache.mu.Lock()
		if entry, ok := c.issueCommentsCache.entries[cacheKey]; ok {
			if time.Since(entry.fetchedAt) < issueCommentsCacheTTL {
				result := make([]IssueComment, len(entry.comments))
				copy(result, entry.comments)
				c.issueCommentsCache.mu.Unlock()
				return result, nil
			}
		}
		c.issueCommentsCache.mu.Unlock()
	}

	var allComments []IssueComment
	visitedURLs := make(map[string]bool)
	pageCount := 0

	// Start with the first page
	nextURL := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=100",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), prNumber)

	for nextURL != "" {
		// Caller-controlled soft limit (checked before the hard cap)
		if options.MaxPages > 0 && pageCount >= options.MaxPages {
			break
		}
		// Hard cap safety limit
		if pageCount >= maxPaginationPages {
			return nil, fmt.Errorf("pagination limit exceeded (%d pages)", maxPaginationPages)
		}
		if visitedURLs[nextURL] {
			return nil, fmt.Errorf("pagination loop detected: URL already visited")
		}
		visitedURLs[nextURL] = true
		pageCount++

		pageComments, next, err := c.fetchIssueCommentsPage(ctx, nextURL)
		if err != nil {
			return nil, err
		}
		allComments = append(allComments, pageComments...)

		// Validate and resolve pagination URL
		if next != "" {
			resolved, err := c.ValidateAndResolvePaginationURL(next)
			if err != nil {
				return nil, fmt.Errorf("unsafe pagination URL in Link header: %w", err)
			}
			next = resolved
		}
		nextURL = next
	}

	// Only cache full (unlimited) fetches
	if useCache {
		cached := make([]IssueComment, len(allComments))
		copy(cached, allComments)
		c.issueCommentsCache.mu.Lock()
		c.issueCommentsCache.entries[cacheKey] = issueCommentsCacheEntry{
			comments:  cached,
			fetchedAt: time.Now(),
		}
		c.issueCommentsCache.mu.Unlock()
	}

	return allComments, nil
}

// ClearIssueCommentsCache removes all cached issue comment data.
func (c *Client) ClearIssueCommentsCache() {
	c.issueCommentsCache.mu.Lock()
	c.issueCommentsCache.entries = make(map[issueCommentsCacheKey]issueCommentsCacheEntry)
	c.issueCommentsCache.mu.Unlock()
}

// fetchIssueCommentsPage fetches a single page of issue comments.
func (c *Client) fetchIssueCommentsPage(ctx context.Context, pageURL string) ([]IssueComment, string, error) {
	var resp *http.Response
	err := llmhttp.RetryWithBackoff(ctx, func(ctx context.Context) error {
		req, reqErr := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
		if reqErr != nil {
			return &llmhttp.Error{
				Type:      llmhttp.ErrTypeUnknown,
				Message:   reqErr.Error(),
				Retryable: false,
				Provider:  providerName,
			}
		}

		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		var callErr error
		resp, callErr = c.httpClient.Do(req)
		if callErr != nil {
			return llmhttp.ClassifyNetworkError(providerName, callErr, ctx)
		}

		if resp.StatusCode >= 400 {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				return &llmhttp.Error{
					Type:       llmhttp.ErrTypeUnknown,
					Message:    fmt.Sprintf("HTTP %d (failed to read response: %v)", resp.StatusCode, readErr),
					StatusCode: resp.StatusCode,
					Retryable:  resp.StatusCode >= 500,
					Provider:   providerName,
				}
			}
			return MapHTTPError(resp.StatusCode, bodyBytes)
		}

		return nil
	}, c.retryConf)

	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var comments []IssueComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, "", fmt.Errorf("decode response: %w", err)
	}

	// Parse Link header for pagination
	nextURL := parseNextLink(resp.Header.Get("Link"))

	return comments, nextURL, nil
}

// GetIssueComment retrieves a single issue comment by ID.
// GitHub API: GET /repos/{owner}/{repo}/issues/comments/{comment_id}
// Returns ErrCommentNotFound if the comment doesn't exist.
func (c *Client) GetIssueComment(ctx context.Context, owner, repo string, commentID int64) (*IssueComment, error) {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return nil, err
	}
	if commentID <= 0 {
		return nil, fmt.Errorf("invalid comment ID: %d (must be positive)", commentID)
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), commentID)

	var resp *http.Response
	err := llmhttp.RetryWithBackoff(ctx, func(ctx context.Context) error {
		req, reqErr := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if reqErr != nil {
			return &llmhttp.Error{
				Type:      llmhttp.ErrTypeUnknown,
				Message:   reqErr.Error(),
				Retryable: false,
				Provider:  providerName,
			}
		}

		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		var callErr error
		resp, callErr = c.httpClient.Do(req)
		if callErr != nil {
			return llmhttp.ClassifyNetworkError(providerName, callErr, ctx)
		}

		if resp.StatusCode == 404 {
			_ = resp.Body.Close()
			return triage.ErrCommentNotFound
		}

		if resp.StatusCode >= 400 {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				return &llmhttp.Error{
					Type:       llmhttp.ErrTypeUnknown,
					Message:    fmt.Sprintf("HTTP %d (failed to read response: %v)", resp.StatusCode, readErr),
					StatusCode: resp.StatusCode,
					Retryable:  resp.StatusCode >= 500,
					Provider:   providerName,
				}
			}
			return MapHTTPError(resp.StatusCode, bodyBytes)
		}

		return nil
	}, c.retryConf)

	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var comment IssueComment
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &comment, nil
}
