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

	return nil
}

// mapGraphQLErrors converts GraphQL errors to appropriate error types.
func mapGraphQLErrors(errors []GraphQLError) error {
	if len(errors) == 0 {
		return nil
	}

	// Check for specific error types
	for _, e := range errors {
		lowerMsg := strings.ToLower(e.Message)
		lowerType := strings.ToLower(e.Type)

		// Not found errors
		if lowerType == "not_found" || strings.Contains(lowerMsg, "could not resolve to a node") {
			return fmt.Errorf("thread not found: %s", e.Message)
		}

		// Permission errors
		if lowerType == "forbidden" || strings.Contains(lowerMsg, "not accessible") {
			return fmt.Errorf("permission denied: %s", e.Message)
		}
	}

	// Return first error message for generic errors
	return fmt.Errorf("GraphQL error: %s", errors[0].Message)
}
