package github

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
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

	// noncePrefix is the HTML comment prefix used for idempotent retry detection.
	// Format: <!-- bop-rid:HEXSTRING -->
	// This is invisible to users but allows us to detect if a review was already
	// created when retrying after a transient error (e.g., HTTP 500).
	noncePrefix = "<!-- bop-rid:"
	nonceSuffix = " -->"
	nonceLength = 16 // 16 bytes = 32 hex chars, sufficient uniqueness
)

// validRepoPrefixes defines the allowed path prefixes for pagination URLs.
// Package-level to avoid allocation on each validation call.
var validRepoPrefixes = []string{"/repos/", "/repositories/", "/api/v3/repos/", "/api/v3/repositories/"}

// dangerousPaths defines path patterns that should be blocked even on trusted hosts.
// Package-level to avoid allocation on each validation call.
var dangerousPaths = []string{"/admin", "/settings", "/stafftools", "/_private", "/setup"}

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

// generateNonce creates a cryptographically random nonce for idempotent retry detection.
// The nonce is used to uniquely identify a review creation attempt so we can detect
// if the review was already created when retrying after a transient error.
func generateNonce() (string, error) {
	b := make([]byte, nonceLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// embedNonce prepends an invisible HTML comment containing the nonce to the review body.
// The format is: <!-- bop-rid:HEXSTRING --> followed by the original body.
// This is invisible when rendered but can be detected via API responses.
func embedNonce(body, nonce string) string {
	return noncePrefix + nonce + nonceSuffix + "\n" + body
}

// extractNonce extracts the nonce from a review body, if present.
// Returns empty string if no nonce is found.
func extractNonce(body string) string {
	start := strings.Index(body, noncePrefix)
	if start == -1 {
		return ""
	}
	start += len(noncePrefix)
	end := strings.Index(body[start:], nonceSuffix)
	if end == -1 {
		return ""
	}
	return body[start : start+end]
}

// containsNonce checks if a review body contains the specified nonce.
func containsNonce(body, nonce string) bool {
	return extractNonce(body) == nonce
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

// SetBaseURL sets a custom base URL (e.g., for GitHub Enterprise Server).
// All trailing slashes are trimmed to ensure consistent URL construction.
func (c *Client) SetBaseURL(baseURL string) {
	c.baseURL = strings.TrimRight(baseURL, "/")
}

// ValidateAPIURL validates that a GitHub API URL is well-formed.
// Returns an error if the URL is malformed or missing scheme/host.
// Logs a warning if HTTP (non-TLS) is used, since the Authorization header
// containing the GitHub token will be sent in plaintext.
func ValidateAPIURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("malformed URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("must be an absolute URL with scheme and host, got %q", rawURL)
	}
	if u.Scheme == "http" {
		log.Printf("[WARN] GITHUB_API_URL uses http:// — GitHub token will be sent over an unencrypted connection. Use https:// unless you are certain this is intentional (e.g., behind a corporate VPN).")
	}
	return nil
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
//
// This function uses idempotent retry logic: a unique nonce is embedded in the review body
// as an invisible HTML comment. On transient errors (5xx, timeout), before retrying,
// we check if a review with the nonce already exists. This prevents duplicate reviews
// when the server processes the request but fails to return the response.
func (c *Client) CreateReview(ctx context.Context, input CreateReviewInput) (*CreateReviewResponse, error) {
	// Validate path segments to prevent injection attacks
	if err := validatePathSegment(input.Owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(input.Repo, "repo"); err != nil {
		return nil, err
	}

	// Generate a unique nonce for idempotent retry detection
	nonce, err := generateNonce()
	if err != nil {
		return nil, fmt.Errorf("create review: %w", err)
	}

	// Embed nonce in the review body (invisible HTML comment)
	bodyWithNonce := embedNonce(input.Summary, nonce)

	// Build the API request with proper blocking indicators
	comments := BuildReviewCommentsWithActions(input.Findings, input.ReviewActions)

	reqBody := CreateReviewRequest{
		CommitID: input.CommitSHA,
		Event:    input.Event,
		Body:     bodyWithNonce,
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

	// Execute with idempotent retry
	var resp *http.Response
	var attempt int
	err = llmhttp.RetryWithBackoff(ctx, func(ctx context.Context) error {
		attempt++

		// On retry (attempt > 1), check if review was already created
		// This handles the case where the server created the review but returned an error
		if attempt > 1 {
			existingReview, found := c.findReviewByNonce(ctx, input.Owner, input.Repo, input.PullNumber, nonce)
			if found {
				// Review already exists - construct response from existing review
				resp = nil // Signal to use existingReview instead
				return &reviewAlreadyExistsError{review: existingReview}
			}
			// Review not found, proceed with retry
		}

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
			// Could be timeout or network error - the request may have been processed
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

	// Check if we found an existing review during retry
	var existsErr *reviewAlreadyExistsError
	if errors.As(err, &existsErr) {
		// Return the existing review - this is not an error, just idempotent success
		return existsErr.review, nil
	}

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

// reviewAlreadyExistsError is a sentinel error used to signal that a review
// was found during idempotent retry. This is not a failure - it means the
// previous attempt succeeded and we should return that review.
type reviewAlreadyExistsError struct {
	review *CreateReviewResponse
}

func (e *reviewAlreadyExistsError) Error() string {
	return fmt.Sprintf("review already exists with ID %d", e.review.ID)
}

// findReviewByNonce searches recent reviews for one containing the specified nonce.
// Returns the review and true if found, nil and false otherwise.
// This is used for idempotent retry detection - if a review with our nonce exists,
// we know a previous attempt succeeded despite returning an error.
//
// Performance: Only fetches the first page (100 reviews) since idempotent retry
// detection is looking for a review created seconds ago, not historical reviews.
func (c *Client) findReviewByNonce(ctx context.Context, owner, repo string, pullNumber int, nonce string) (*CreateReviewResponse, bool) {
	// Only fetch the first page - the review we're looking for was just created
	reviews, err := c.listReviewsFirstPage(ctx, owner, repo, pullNumber)
	if err != nil {
		// If we can't list reviews, proceed with retry (fail open)
		return nil, false
	}

	// Search from newest to oldest (more likely to find recent review quickly)
	for i := len(reviews) - 1; i >= 0; i-- {
		if containsNonce(reviews[i].Body, nonce) {
			// Found our review! Convert ReviewSummary to CreateReviewResponse
			return &CreateReviewResponse{
				ID:          reviews[i].ID,
				NodeID:      reviews[i].NodeID,
				User:        reviews[i].User,
				Body:        reviews[i].Body,
				State:       reviews[i].State,
				HTMLURL:     "", // Not available in ReviewSummary, but not critical
				SubmittedAt: reviews[i].SubmittedAt,
			}, true
		}
	}

	return nil, false
}

// listReviewsFirstPage fetches only the first page of reviews (up to 100).
// This is more efficient than ListReviews for idempotent retry detection
// where we're looking for a very recently created review.
func (c *Client) listReviewsFirstPage(ctx context.Context, owner, repo string, pullNumber int) ([]ReviewSummary, error) {
	// Validate path segments to prevent injection attacks
	if err := validatePathSegment(owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return nil, err
	}

	// Fetch only the first page with max per_page
	pageURL := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews?per_page=100",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), pullNumber)

	reviews, _, err := c.fetchReviewsPage(ctx, pageURL)
	if err != nil {
		return nil, err
	}

	return reviews, nil
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

	// Validate path is a GitHub API repository endpoint
	// The host check above is the primary SSRF defense; this provides defense in depth
	// Accept these patterns:
	//   - /repos/{owner}/{repo}/... - the standard named format
	//   - /api/v3/repos/... - GitHub Enterprise Server format
	//   - /repositories/{id}/... - numeric ID format (used in some pagination links)
	//
	// Use path.Clean to normalize and prevent traversal attacks (e.g., /admin/repos/../secrets)
	// Use HasPrefix for structural validation instead of Contains
	cleanPath := path.Clean(parsed.Path)
	hasValidPrefix := false
	for _, prefix := range validRepoPrefixes {
		if strings.HasPrefix(cleanPath, prefix) {
			hasValidPrefix = true
			break
		}
	}
	if !hasValidPrefix {
		return "", fmt.Errorf("unexpected API path: %s (must start with /repos/ or /repositories/)", cleanPath)
	}

	// Block known dangerous paths even on the same host (defense in depth)
	// Use the cleaned path to prevent bypass via traversal
	// Check for complete path segments to avoid false positives (e.g., /repos/org/admin-tools is valid)
	pathLower := strings.ToLower(cleanPath)
	for _, dangerous := range dangerousPaths {
		// Check if dangerous pattern appears as a complete path segment
		// Must be followed by "/" or end of string to be a real match
		pattern := dangerous + "/"
		if strings.Contains(pathLower, pattern) || strings.HasSuffix(pathLower, dangerous) {
			return "", fmt.Errorf("blocked path pattern in URL: %s", cleanPath)
		}
	}

	// Use the cleaned/normalized path in the returned URL
	parsed.Path = cleanPath
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
