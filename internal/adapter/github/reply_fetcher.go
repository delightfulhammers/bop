package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
)

// ListPullRequestComments fetches all review comments on a pull request.
// This includes both parent comments and replies, which can be grouped using GroupCommentsByParent.
// The results are returned in chronological order (oldest first).
func (c *Client) ListPullRequestComments(ctx context.Context, owner, repo string, pullNumber int) ([]PullRequestComment, error) {
	// Validate path segments to prevent injection attacks
	if err := validatePathSegment(owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return nil, err
	}

	var allComments []PullRequestComment
	visitedURLs := make(map[string]bool) // Prevent infinite pagination loops
	pageCount := 0

	// Start with the first page
	nextURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments?per_page=100",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), pullNumber)

	for nextURL != "" {
		// Pagination loop protection
		if pageCount >= maxPaginationPages {
			return nil, fmt.Errorf("pagination limit exceeded (%d pages)", maxPaginationPages)
		}
		if visitedURLs[nextURL] {
			return nil, fmt.Errorf("pagination loop detected: URL already visited")
		}
		visitedURLs[nextURL] = true
		pageCount++

		pageComments, next, err := c.fetchCommentsPage(ctx, nextURL)
		if err != nil {
			return nil, err
		}
		allComments = append(allComments, pageComments...)

		// Validate and resolve pagination URL to prevent SSRF attacks
		if next != "" {
			resolved, err := c.ValidateAndResolvePaginationURL(next)
			if err != nil {
				return nil, fmt.Errorf("unsafe pagination URL in Link header: %w", err)
			}
			next = resolved
		}
		nextURL = next
	}

	// Sort comments chronologically by creation time
	sortCommentsChronologically(allComments)

	return allComments, nil
}

// fetchCommentsPage fetches a single page of comments and returns the next page URL if present.
func (c *Client) fetchCommentsPage(ctx context.Context, pageURL string) ([]PullRequestComment, string, error) {
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
			return &llmhttp.Error{
				Type:      llmhttp.ErrTypeTimeout,
				Message:   callErr.Error(),
				Retryable: true,
				Provider:  providerName,
			}
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

	var comments []PullRequestComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Parse Link header for pagination
	nextURL := parseNextLink(resp.Header.Get("Link"))

	return comments, nextURL, nil
}

// sortCommentsChronologically sorts comments by CreatedAt timestamp (oldest first).
func sortCommentsChronologically(comments []PullRequestComment) {
	sort.Slice(comments, func(i, j int) bool {
		// CreatedAt is in RFC3339 format, which sorts lexicographically
		return comments[i].CreatedAt < comments[j].CreatedAt
	})
}

// GroupCommentsByParent organizes comments into parent-reply groups.
// Only comments from the specified bot username are treated as parents.
// Replies from the bot itself are excluded (bot doesn't update status by replying to itself).
//
// This function is used to build the data structure needed for status detection:
// for each bot comment (with embedded fingerprint), find all human replies that
// might contain status update keywords.
func GroupCommentsByParent(comments []PullRequestComment, botUsername string) []CommentWithReplies {
	// Build a map of potential parent comment IDs (comments by the bot)
	parentMap := make(map[int64]*CommentWithReplies)
	for i := range comments {
		c := &comments[i]
		// Only bot's top-level comments (InReplyToID == 0) can be parents
		// Use case-insensitive comparison (GitHub usernames are case-insensitive)
		if strings.EqualFold(c.User.Login, botUsername) && c.InReplyToID == 0 {
			parentMap[c.ID] = &CommentWithReplies{
				Parent:  *c,
				Replies: []PullRequestComment{},
			}
		}
	}

	// Assign replies to their parents
	for i := range comments {
		c := &comments[i]
		// Skip top-level comments and bot's own replies
		if c.InReplyToID == 0 || strings.EqualFold(c.User.Login, botUsername) {
			continue
		}

		// Check if the reply's parent is one of our tracked parents
		if group, exists := parentMap[c.InReplyToID]; exists {
			group.Replies = append(group.Replies, *c)
		}
	}

	// Convert map to slice, sorted by parent ID for deterministic ordering
	result := make([]CommentWithReplies, 0, len(parentMap))
	for _, group := range parentMap {
		// Sort replies chronologically within each group
		sort.Slice(group.Replies, func(i, j int) bool {
			return group.Replies[i].CreatedAt < group.Replies[j].CreatedAt
		})
		result = append(result, *group)
	}

	// Sort groups by parent ID for deterministic output
	sort.Slice(result, func(i, j int) bool {
		return result[i].Parent.ID < result[j].Parent.ID
	})

	return result
}
