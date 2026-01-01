package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
)

// PRFile represents a file in a pull request from the GitHub API.
// See: https://docs.github.com/en/rest/pulls/pulls#list-pull-requests-files
type PRFile struct {
	Filename         string `json:"filename"`
	Status           string `json:"status"` // "added", "removed", "modified", "renamed", "copied", "changed", "unchanged"
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch,omitempty"`
	PreviousFilename string `json:"previous_filename,omitempty"`
	BlobURL          string `json:"blob_url"`
	RawURL           string `json:"raw_url"`
	ContentsURL      string `json:"contents_url"`
}

// GetPRDiff fetches the diff for a pull request.
// Returns domain.Diff compatible with existing review flow.
// Handles pagination for PRs with many files.
func (c *Client) GetPRDiff(ctx context.Context, owner, repo string, prNumber int) (domain.Diff, error) {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return domain.Diff{}, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return domain.Diff{}, err
	}
	if prNumber <= 0 {
		return domain.Diff{}, fmt.Errorf("invalid PR number: %d (must be positive)", prNumber)
	}

	var allFiles []PRFile
	visitedURLs := make(map[string]bool)
	pageCount := 0

	// Start with first page
	nextURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/files?per_page=100",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), prNumber)

	for nextURL != "" {
		// Pagination loop protection
		if pageCount >= maxPaginationPages {
			return domain.Diff{}, fmt.Errorf("pagination limit exceeded (%d pages)", maxPaginationPages)
		}
		if visitedURLs[nextURL] {
			return domain.Diff{}, fmt.Errorf("pagination loop detected: URL already visited")
		}
		visitedURLs[nextURL] = true
		pageCount++

		pageFiles, next, err := c.fetchPRFilesPage(ctx, nextURL)
		if err != nil {
			return domain.Diff{}, err
		}
		allFiles = append(allFiles, pageFiles...)

		// Validate and resolve pagination URL
		if next != "" {
			resolved, err := c.ValidateAndResolvePaginationURL(next)
			if err != nil {
				return domain.Diff{}, fmt.Errorf("unsafe pagination URL in Link header: %w", err)
			}
			next = resolved
		}
		nextURL = next
	}

	// Convert to domain.Diff
	return prFilesToDiff(allFiles), nil
}

// fetchPRFilesPage fetches a single page of PR files and returns the next page URL if present.
func (c *Client) fetchPRFilesPage(ctx context.Context, pageURL string) ([]PRFile, string, error) {
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

		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return triage.ErrPRNotFound
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

	var files []PRFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Parse Link header for pagination
	nextURL := parseNextLink(resp.Header.Get("Link"))

	return files, nextURL, nil
}

// prFilesToDiff converts GitHub PR files to domain.Diff.
func prFilesToDiff(files []PRFile) domain.Diff {
	fileDiffs := make([]domain.FileDiff, 0, len(files))

	for _, f := range files {
		fileDiffs = append(fileDiffs, domain.FileDiff{
			Path:     f.Filename,
			OldPath:  f.PreviousFilename,
			Status:   mapPRFileStatus(f.Status),
			Patch:    f.Patch,
			IsBinary: isPRFileBinary(f),
		})
	}

	return domain.Diff{
		Files: fileDiffs,
	}
}

// mapPRFileStatus maps GitHub API file status to domain file status.
func mapPRFileStatus(status string) string {
	switch status {
	case "added":
		return domain.FileStatusAdded
	case "removed":
		return domain.FileStatusDeleted
	case "renamed", "copied":
		return domain.FileStatusRenamed
	default:
		// "modified", "changed", "unchanged" all map to modified
		return domain.FileStatusModified
	}
}

// isPRFileBinary detects if a PR file is binary.
// GitHub doesn't include a patch for binary files, but they may still have additions/deletions counts.
// A file is considered binary if it has changes but no patch.
func isPRFileBinary(f PRFile) bool {
	// If there's a patch, it's not binary
	if f.Patch != "" {
		return false
	}
	// If there are changes but no patch, it's binary
	return f.Additions > 0 || f.Deletions > 0
}
