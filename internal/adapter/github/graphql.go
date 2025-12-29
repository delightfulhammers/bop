package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
)

// GraphQL mutation for resolving a review thread.
const resolveThreadMutation = `
mutation ResolveReviewThread($threadId: ID!) {
	resolveReviewThread(input: {threadId: $threadId}) {
		thread {
			id
			isResolved
		}
	}
}`

// GraphQL mutation for unresolving a review thread.
const unresolveThreadMutation = `
mutation UnresolveReviewThread($threadId: ID!) {
	unresolveReviewThread(input: {threadId: $threadId}) {
		thread {
			id
			isResolved
		}
	}
}`

// GraphQL query to find thread ID for a comment by searching PR threads.
// This is needed because the comment's parent review ID is not the same as thread ID.
// Limitations: Fetches up to 100 threads with up to 100 comments each.
// PRs with more threads/comments may not find all comments.
const findThreadForCommentQuery = `
query FindThreadForComment($owner: String!, $repo: String!, $prNumber: Int!) {
	repository(owner: $owner, name: $repo) {
		pullRequest(number: $prNumber) {
			reviewThreads(first: 100) {
				nodes {
					id
					isResolved
					comments(first: 100) {
						nodes {
							databaseId
						}
					}
				}
			}
		}
	}
}`

// GraphQLRequest represents a GraphQL request payload.
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a GraphQL response from GitHub.
type GraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors,omitempty"`
}

// GraphQLError represents an error returned by the GraphQL API.
type GraphQLError struct {
	Message string   `json:"message"`
	Type    string   `json:"type"`
	Path    []string `json:"path,omitempty"`
}

// ResolveThread marks a review thread as resolved using the GraphQL API.
// The threadID should be the node_id of the review thread (e.g., "PRRT_kwDO...").
func (c *Client) ResolveThread(ctx context.Context, owner, repo, threadID string) error {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return err
	}
	if threadID == "" {
		return fmt.Errorf("thread ID cannot be empty")
	}

	return c.executeThreadMutation(ctx, resolveThreadMutation, threadID)
}

// UnresolveThread marks a review thread as unresolved using the GraphQL API.
// The threadID should be the node_id of the review thread (e.g., "PRRT_kwDO...").
func (c *Client) UnresolveThread(ctx context.Context, owner, repo, threadID string) error {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return err
	}
	if threadID == "" {
		return fmt.Errorf("thread ID cannot be empty")
	}

	return c.executeThreadMutation(ctx, unresolveThreadMutation, threadID)
}

// executeThreadMutation executes a GraphQL mutation for thread resolution.
func (c *Client) executeThreadMutation(ctx context.Context, mutation, threadID string) error {
	reqBody := GraphQLRequest{
		Query: mutation,
		Variables: map[string]interface{}{
			"threadId": threadID,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// GraphQL endpoint is at /graphql
	apiURL := c.baseURL + "/graphql"

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
		req.Header.Set("Content-Type", "application/json")
		// GraphQL API doesn't use versioned Accept header like REST
		req.Header.Set("Accept", "application/json")

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

		// HTTP-level errors (non-200 status codes)
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
	defer func() { _ = resp.Body.Close() }()

	// Parse GraphQL response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var gqlResp GraphQLResponse
	if err := json.Unmarshal(bodyBytes, &gqlResp); err != nil {
		return fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	// Check for GraphQL errors
	if len(gqlResp.Errors) > 0 {
		return mapGraphQLErrors(gqlResp.Errors)
	}

	// Validate response data is not empty/null
	// GraphQL can return HTTP 200 with empty data without errors in some edge cases
	// json.RawMessage for {"data":null} contains the literal bytes "null"
	dataStr := strings.TrimSpace(string(gqlResp.Data))
	if len(dataStr) == 0 || dataStr == "null" {
		return fmt.Errorf("GraphQL mutation returned empty data without errors")
	}

	return nil
}

// mapGraphQLErrors converts GraphQL errors to appropriate error types.
// Wraps with triage sentinel errors where appropriate for errors.Is() compatibility.
func mapGraphQLErrors(errors []GraphQLError) error {
	if len(errors) == 0 {
		return nil
	}

	// Check for specific error types first
	for _, e := range errors {
		lowerMsg := strings.ToLower(e.Message)
		lowerType := strings.ToLower(e.Type)

		// Not found errors - wrap with sentinel for errors.Is() compatibility
		if lowerType == "not_found" || strings.Contains(lowerMsg, "could not resolve to a node") {
			return fmt.Errorf("thread not found (%s): %w", e.Message, triage.ErrThreadNotFound)
		}

		// Permission errors - wrap with sentinel for errors.Is() compatibility
		if lowerType == "forbidden" || strings.Contains(lowerMsg, "not accessible") {
			return fmt.Errorf("permission denied (%s): %w", e.Message, triage.ErrPermissionDenied)
		}
	}

	// For generic errors, aggregate all messages to preserve context
	if len(errors) == 1 {
		return fmt.Errorf("GraphQL error: %s", errors[0].Message)
	}

	messages := make([]string, len(errors))
	for i, e := range errors {
		messages[i] = e.Message
	}
	return fmt.Errorf("GraphQL errors: %s", strings.Join(messages, "; "))
}

// ThreadInfo contains information about a review thread.
type ThreadInfo struct {
	ID         string // GraphQL node ID (PRRT_...)
	IsResolved bool
}

// FindThreadForComment finds the review thread ID for a given comment ID.
// The commentID is the REST API comment ID (database ID).
// Returns the thread's GraphQL node ID which can be used with ResolveThread/UnresolveThread.
func (c *Client) FindThreadForComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*ThreadInfo, error) {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return nil, err
	}
	if prNumber <= 0 {
		return nil, fmt.Errorf("prNumber must be positive")
	}
	if commentID <= 0 {
		return nil, fmt.Errorf("commentID must be positive")
	}

	reqBody := GraphQLRequest{
		Query: findThreadForCommentQuery,
		Variables: map[string]interface{}{
			"owner":    owner,
			"repo":     repo,
			"prNumber": prNumber,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := c.baseURL + "/graphql"

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
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

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
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the GraphQL response
	var gqlResp struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						Nodes []struct {
							ID         string `json:"id"`
							IsResolved bool   `json:"isResolved"`
							Comments   struct {
								Nodes []struct {
									DatabaseID int64 `json:"databaseId"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
		Errors []GraphQLError `json:"errors,omitempty"`
	}

	if err := json.Unmarshal(bodyBytes, &gqlResp); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return nil, mapGraphQLErrors(gqlResp.Errors)
	}

	// Search for the thread containing this comment
	for _, thread := range gqlResp.Data.Repository.PullRequest.ReviewThreads.Nodes {
		for _, comment := range thread.Comments.Nodes {
			if comment.DatabaseID == commentID {
				return &ThreadInfo{
					ID:         thread.ID,
					IsResolved: thread.IsResolved,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("thread not found for comment %d: %w", commentID, triage.ErrThreadNotFound)
}
