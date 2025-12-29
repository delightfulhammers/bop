package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
)

// ReplyCommentRequest is the request body for replying to a PR comment.
// See: https://docs.github.com/en/rest/pulls/comments#create-a-reply-for-a-review-comment
type ReplyCommentRequest struct {
	Body      string `json:"body"`
	InReplyTo int64  `json:"in_reply_to"`
}

// CreateCommentRequest is the request body for creating a new PR review comment.
// See: https://docs.github.com/en/rest/pulls/comments#create-a-review-comment-for-a-pull-request
type CreateCommentRequest struct {
	Body     string `json:"body"`
	CommitID string `json:"commit_id"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side,omitempty"` // "LEFT" or "RIGHT", defaults to "RIGHT"
}

// CommentResponse is the response from creating or replying to a comment.
type CommentResponse struct {
	ID     int64  `json:"id"`
	NodeID string `json:"node_id"`
	Body   string `json:"body"`
	Path   string `json:"path"`
	Line   *int   `json:"line"`
	User   User   `json:"user"`
}

// RequestReviewersRequest is the request body for requesting reviewers.
// See: https://docs.github.com/en/rest/pulls/review-requests#request-reviewers-for-a-pull-request
type RequestReviewersRequest struct {
	Reviewers     []string `json:"reviewers,omitempty"`
	TeamReviewers []string `json:"team_reviewers,omitempty"`
}

// ReplyToComment posts a reply to an existing PR review comment.
// Returns the ID of the newly created reply comment.
func (c *Client) ReplyToComment(ctx context.Context, owner, repo string, prNumber int, replyTo int64, body string) (int64, error) {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return 0, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return 0, err
	}
	if body == "" {
		return 0, fmt.Errorf("body cannot be empty")
	}
	if replyTo <= 0 {
		return 0, fmt.Errorf("replyTo must be a positive comment ID")
	}

	reqBody := ReplyCommentRequest{
		Body:      body,
		InReplyTo: replyTo,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments",
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
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	var commentResp CommentResponse
	if err := json.NewDecoder(resp.Body).Decode(&commentResp); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	return commentResp.ID, nil
}

// CreateComment creates a new review comment at a specific file and line.
// The line parameter is the line number in the file (not the diff position).
// Returns the ID of the newly created comment.
func (c *Client) CreateComment(ctx context.Context, owner, repo string, prNumber int, commitSHA, path string, line int, body string) (int64, error) {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return 0, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return 0, err
	}
	if body == "" {
		return 0, fmt.Errorf("body cannot be empty")
	}
	if commitSHA == "" {
		return 0, fmt.Errorf("commit SHA cannot be empty")
	}
	if path == "" {
		return 0, fmt.Errorf("path cannot be empty")
	}
	if line <= 0 {
		return 0, fmt.Errorf("line must be positive")
	}

	reqBody := CreateCommentRequest{
		Body:     body,
		CommitID: commitSHA,
		Path:     path,
		Line:     line,
		Side:     "RIGHT", // Always comment on the new side of the diff
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments",
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
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	var commentResp CommentResponse
	if err := json.NewDecoder(resp.Body).Decode(&commentResp); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	return commentResp.ID, nil
}

// RequestReviewers requests review from specified users or teams.
// At least one reviewer (user or team) must be specified.
func (c *Client) RequestReviewers(ctx context.Context, owner, repo string, prNumber int, reviewers []string, teamReviewers []string) error {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return err
	}
	if len(reviewers) == 0 && len(teamReviewers) == 0 {
		return fmt.Errorf("at least one reviewer (user or team) must be specified")
	}

	reqBody := RequestReviewersRequest{
		Reviewers:     reviewers,
		TeamReviewers: teamReviewers,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/requested_reviewers",
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
		return err
	}
	_ = resp.Body.Close()

	return nil
}
