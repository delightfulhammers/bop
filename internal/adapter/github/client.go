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
	"sort"
	"strings"
	"time"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
)

const (
	defaultBaseURL        = "https://api.github.com"
	defaultTimeout        = 30 * time.Second
	defaultMaxRetries     = 3
	defaultInitialBackoff = 2 * time.Second
	maxPaginationPages    = 100 // Prevent infinite pagination loops
)

// validatePathSegment validates that a path segment (owner, repo) doesn't contain
// characters that could cause path injection attacks.
func validatePathSegment(value, name string) error {
	if strings.Contains(value, "..") {
		return fmt.Errorf("invalid %s: must not contain '..'", name)
	}
	if strings.Contains(value, "/") {
		return fmt.Errorf("invalid %s: must not contain '/'", name)
	}
	if value == "" {
		return fmt.Errorf("invalid %s: must not be empty", name)
	}
	return nil
}

// Client is an HTTP client for the GitHub Pull Request Reviews API.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	retryConf  llmhttp.RetryConfig
}

// NewClient creates a new GitHub API client with the given token.
// The token should be a GitHub personal access token or GITHUB_TOKEN from Actions.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
		retryConf: llmhttp.RetryConfig{
			MaxRetries:     defaultMaxRetries,
			InitialBackoff: defaultInitialBackoff,
			MaxBackoff:     32 * time.Second,
			Multiplier:     2.0,
		},
	}
}

// SetBaseURL sets a custom base URL (for testing).
// All trailing slashes are trimmed to ensure consistent URL construction.
func (c *Client) SetBaseURL(baseURL string) {
	c.baseURL = strings.TrimRight(baseURL, "/")
}

// SetTimeout sets the HTTP timeout.
func (c *Client) SetTimeout(timeout time.Duration) {
	c.httpClient.Timeout = timeout
}

// SetMaxRetries sets the maximum number of retry attempts.
func (c *Client) SetMaxRetries(maxRetries int) {
	c.retryConf.MaxRetries = maxRetries
}

// SetInitialBackoff sets the initial backoff duration for retries.
func (c *Client) SetInitialBackoff(backoff time.Duration) {
	c.retryConf.InitialBackoff = backoff
}

// CreateReviewInput contains all data needed to create a PR review.
type CreateReviewInput struct {
	Owner      string
	Repo       string
	PullNumber int
	CommitSHA  string
	Event      ReviewEvent
	Summary    string
	Findings   []PositionedFinding

	// ReviewActions configures blocking behavior for accurate "Blocking: yes/no" indicators.
	// Optional: if empty, defaults to severity-only blocking (critical/high = blocking).
	ReviewActions ReviewActions
}

// CreateReview posts a pull request review with inline comments.
// Only findings with a valid DiffPosition (InDiff() == true) are posted as inline comments.
// Returns an error if the request fails after all retries.
func (c *Client) CreateReview(ctx context.Context, input CreateReviewInput) (*CreateReviewResponse, error) {
	// Validate path segments to prevent injection attacks
	if err := validatePathSegment(input.Owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(input.Repo, "repo"); err != nil {
		return nil, err
	}

	// Build the API request with proper blocking indicators
	comments := BuildReviewCommentsWithActions(input.Findings, input.ReviewActions)

	reqBody := CreateReviewRequest{
		CommitID: input.CommitSHA,
		Event:    input.Event,
		Body:     input.Summary,
		Comments: comments,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL with path-escaped segments to prevent path injection
	apiURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews",
		c.baseURL, url.PathEscape(input.Owner), url.PathEscape(input.Repo), input.PullNumber)

	// Execute with retry
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

		// Set headers
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		var callErr error
		resp, callErr = c.httpClient.Do(req)
		if callErr != nil {
			// Could be timeout or network error
			return &llmhttp.Error{
				Type:      llmhttp.ErrTypeTimeout,
				Message:   callErr.Error(),
				Retryable: true,
				Provider:  providerName,
			}
		}

		// Check for error status codes
		if resp.StatusCode >= 400 {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				// If we can't read the error body, return a generic error with the status code
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

	// Parse response
	var reviewResp CreateReviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&reviewResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &reviewResp, nil
}

// ListReviews fetches all reviews for a pull request.
// Returns reviews in chronological order (oldest first).
// Handles GitHub API pagination to ensure all reviews are returned.
func (c *Client) ListReviews(ctx context.Context, owner, repo string, pullNumber int) ([]ReviewSummary, error) {
	// Validate path segments to prevent injection attacks
	if err := validatePathSegment(owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return nil, err
	}

	var allReviews []ReviewSummary
	visitedURLs := make(map[string]bool) // Prevent infinite pagination loops
	pageCount := 0

	// Start with the first page, using max per_page to minimize API calls
	// Path-escape owner/repo to prevent path injection attacks
	nextURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews?per_page=100",
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

		pageReviews, next, err := c.fetchReviewsPage(ctx, nextURL)
		if err != nil {
			return nil, err
		}
		allReviews = append(allReviews, pageReviews...)

		// Validate and resolve pagination URL to prevent SSRF attacks
		// This also handles relative URLs by resolving against baseURL
		if next != "" {
			resolved, err := c.ValidateAndResolvePaginationURL(next)
			if err != nil {
				// Return error instead of silently truncating - fail secure
				return nil, fmt.Errorf("unsafe pagination URL in Link header: %w", err)
			}
			next = resolved
		}
		nextURL = next
	}

	// Sort reviews chronologically (oldest first) to guarantee ordering
	// Don't rely on GitHub API ordering which could change
	sortReviewsChronologically(allReviews)

	return allReviews, nil
}

// sortReviewsChronologically sorts reviews by SubmittedAt timestamp (oldest first).
// Falls back to sorting by ID if timestamps cannot be parsed.
func sortReviewsChronologically(reviews []ReviewSummary) {
	sort.Slice(reviews, func(i, j int) bool {
		// Parse timestamps - GitHub uses RFC3339 format
		ti, errI := time.Parse(time.RFC3339, reviews[i].SubmittedAt)
		tj, errJ := time.Parse(time.RFC3339, reviews[j].SubmittedAt)

		// If both parse successfully, compare times
		if errI == nil && errJ == nil {
			return ti.Before(tj)
		}

		// Fall back to ID comparison if timestamps can't be parsed
		return reviews[i].ID < reviews[j].ID
	})
}

// ValidateAndResolvePaginationURL validates and resolves a pagination URL.
// This prevents SSRF attacks while supporting relative URLs and enterprise setups.
// Returns the resolved absolute URL or an error if validation fails.
func (c *Client) ValidateAndResolvePaginationURL(rawURL string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Reject URLs with userinfo (e.g., http://user@evil.com/...) - potential SSRF obfuscation
	if parsed.User != nil {
		return "", fmt.Errorf("URL must not contain userinfo")
	}

	// Reject URLs with fragments - not expected in pagination URLs
	if parsed.Fragment != "" {
		return "", fmt.Errorf("URL must not contain fragment")
	}

	// Resolve relative URLs against baseURL
	if !parsed.IsAbs() {
		parsed = base.ResolveReference(parsed)
	}

	// Only allow http/https schemes (defense in depth against file:, gopher:, etc.)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme: %s (only http/https allowed)", parsed.Scheme)
	}

	// Prevent scheme downgrade attacks (https -> http)
	// Allow http in tests, but never downgrade from https
	if base.Scheme == "https" && parsed.Scheme == "http" {
		return "", fmt.Errorf("scheme downgrade not allowed: %s -> %s", base.Scheme, parsed.Scheme)
	}

	// Only trust the configured baseURL host
	// This works for both public GitHub (api.github.com) and Enterprise setups
	if parsed.Host != base.Host {
		return "", fmt.Errorf("untrusted host: %s (expected %s)", parsed.Host, base.Host)
	}

	// Validate path is a GitHub API repos endpoint
	// The host check above is the primary SSRF defense; this provides defense in depth
	// Accept both /repos/... and /api/v3/repos/... patterns for GHES compatibility
	if !strings.Contains(parsed.Path, "/repos/") {
		return "", fmt.Errorf("unexpected API path: %s (must be a /repos/ endpoint)", parsed.Path)
	}

	// Block known dangerous paths even on the same host (defense in depth)
	dangerousPaths := []string{"/admin", "/settings", "/stafftools", "/_private", "/setup"}
	pathLower := strings.ToLower(parsed.Path)
	for _, dangerous := range dangerousPaths {
		if strings.Contains(pathLower, dangerous) {
			return "", fmt.Errorf("blocked path pattern in URL: %s", parsed.Path)
		}
	}

	return parsed.String(), nil
}

// fetchReviewsPage fetches a single page of reviews and returns the next page URL if present.
func (c *Client) fetchReviewsPage(ctx context.Context, pageURL string) ([]ReviewSummary, string, error) {
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

	var reviews []ReviewSummary
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		return nil, "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Parse Link header for pagination
	nextURL := parseNextLink(resp.Header.Get("Link"))

	return reviews, nextURL, nil
}

// parseNextLink extracts the "next" URL from the GitHub Link header.
// Format: <url>; rel="next", <url>; rel="last"
func parseNextLink(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	// Match pattern: <URL>; rel="next"
	re := regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)
	matches := re.FindStringSubmatch(linkHeader)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// DismissReview dismisses a pull request review with the given message.
// Returns an error if the request fails after all retries.
func (c *Client) DismissReview(ctx context.Context, owner, repo string, pullNumber int, reviewID int64, message string) (*DismissReviewResponse, error) {
	// Validate path segments to prevent injection attacks
	if err := validatePathSegment(owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return nil, err
	}

	reqBody := DismissReviewRequest{Message: message}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Path-escape owner/repo to prevent path injection attacks
	apiURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews/%d/dismissals",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), pullNumber, reviewID)

	var resp *http.Response
	err = llmhttp.RetryWithBackoff(ctx, func(ctx context.Context) error {
		req, reqErr := http.NewRequestWithContext(ctx, "PUT", apiURL, bytes.NewReader(jsonData))
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
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var dismissResp DismissReviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&dismissResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &dismissResp, nil
}
