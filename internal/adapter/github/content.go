package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
)

// ContentFile represents a file from the GitHub Contents API.
// See: https://docs.github.com/en/rest/repos/contents#get-repository-content
type ContentFile struct {
	Type     string `json:"type"`     // "file", "dir", "symlink", "submodule"
	Encoding string `json:"encoding"` // "base64" for files
	Content  string `json:"content"`  // Base64-encoded content
	Size     int    `json:"size"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
}

// GetFileContent fetches a file's content from a repository.
// The ref parameter specifies the branch, tag, or commit SHA.
// Returns the decoded file content as a string.
// Returns an error if the path is a directory or the file doesn't exist.
func (c *Client) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return "", err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("invalid path: must not be empty")
	}
	// Validate path for traversal sequences
	if strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("invalid path: contains traversal sequence or absolute path")
	}

	// Build URL - path needs to be escaped but preserve slashes for nested paths
	escapedPath := url.PathEscape(path)
	// PathEscape converts / to %2F, but GitHub expects literal slashes in the path
	escapedPath = strings.ReplaceAll(escapedPath, "%2F", "/")

	apiURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), escapedPath)

	if ref != "" {
		apiURL += "?ref=" + url.QueryEscape(ref)
	}

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

		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return fmt.Errorf("file not found: %s", path)
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
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var content ContentFile
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Verify it's a file, not a directory
	if content.Type != "file" {
		return "", fmt.Errorf("path is not a file: %s (type: %s)", path, content.Type)
	}

	// Validate content size before decoding to prevent memory exhaustion
	const maxContentSize = 10 * 1024 * 1024 // 10MB
	if content.Size > maxContentSize {
		return "", fmt.Errorf("file too large: %d bytes (max: %d)", content.Size, maxContentSize)
	}

	// Decode base64 content
	// GitHub may include newlines in the base64 string for readability
	cleanContent := strings.ReplaceAll(content.Content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(cleanContent)
	if err != nil {
		return "", fmt.Errorf("failed to decode content: %w", err)
	}

	return string(decoded), nil
}
