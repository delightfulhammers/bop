package github

import (
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
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/triage"
)

// Regex patterns for parsing comment metadata.
var (
	// fingerprintPattern matches CR_FP:xxxx markers in comment bodies.
	fingerprintPattern = regexp.MustCompile(`CR_FP:([a-fA-F0-9]+)`)

	// severityPattern matches severity markers like **Severity: high** or [high].
	severityPattern = regexp.MustCompile(`(?i)(?:\*\*Severity:\s*(\w+)\*\*|\[(\w+)\]\s*(?:severity|$))`)

	// categoryPattern matches category markers like **Category: security**.
	categoryPattern = regexp.MustCompile(`(?i)\*\*Category:\s*(\w+)\*\*`)
)

// trustedBots is the allowlist of bot usernames whose comments are included
// in findings even without fingerprints. This prevents arbitrary bots from
// injecting findings into the triage workflow.
var trustedBots = map[string]bool{
	"github-actions[bot]":           true, // GitHub Actions (bop)
	"github-advanced-security[bot]": true, // GitHub Advanced Security (CodeQL, etc.)
	"dependabot[bot]":               true, // Dependabot security updates
	"renovate[bot]":                 true, // Renovate dependency updates
}

// ListPRComments retrieves all review comments on a PR.
// If filterByFingerprint is true, comments are included if they have CR_FP markers
// OR if they are from trusted bot users (in the trustedBots allowlist). This ensures
// automated review feedback from known sources is captured while preventing arbitrary
// bots from injecting findings into the triage workflow.
func (c *Client) ListPRComments(ctx context.Context, owner, repo string, prNumber int, filterByFingerprint bool) ([]domain.PRFinding, error) {
	// Fetch all raw comments using existing method
	rawComments, err := c.ListPullRequestComments(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}

	// Build reply metadata map (replies per parent with last reply info)
	replyMetadataMap := buildReplyMetadataMap(rawComments)

	// Convert to domain type with filtering
	var findings []domain.PRFinding
	for _, comment := range rawComments {
		// Skip replies - we only want top-level comments as findings
		if comment.InReplyToID != 0 {
			continue
		}

		// Parse fingerprint from body
		fingerprint := parseFingerprint(comment.Body)

		// Apply fingerprint filter: include if has fingerprint OR is from a trusted bot
		// This ensures automated review feedback from known sources is captured
		// while preventing arbitrary bots from injecting findings
		// Note: GitHub usernames are case-insensitive, so normalize to lowercase for lookup
		isTrustedBot := comment.User.Type == "Bot" && trustedBots[strings.ToLower(comment.User.Login)]
		if filterByFingerprint && fingerprint == "" && !isTrustedBot {
			continue
		}

		finding := commentToFinding(comment, fingerprint, replyMetadataMap[comment.ID])
		findings = append(findings, finding)
	}

	return findings, nil
}

// GetPRComment retrieves a single comment by ID.
// Returns ErrCommentNotFound if the comment doesn't exist or doesn't belong to the PR.
// The prNumber is required to validate the comment belongs to the expected PR.
//
// Performance note: This method fetches the comment efficiently via direct API call,
// but then calls countReplies() which fetches all PR comments to count replies.
// For PRs with many comments, this can be slow. The reply count is non-critical
// metadata, so failures are handled gracefully (defaulting to 0).
func (c *Client) GetPRComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*domain.PRFinding, error) {
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
	if commentID <= 0 {
		return nil, fmt.Errorf("invalid comment ID: %d (must be positive)", commentID)
	}

	// Fetch the specific comment
	// GitHub API: GET /repos/{owner}/{repo}/pulls/comments/{comment_id}
	apiURL := fmt.Sprintf("%s/repos/%s/%s/pulls/comments/%d",
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
			// Classify network errors properly (timeout vs DNS/TLS/connection)
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

	var comment commentAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&comment); err != nil {
		return nil, fmt.Errorf("failed to parse comment response: %w", err)
	}

	// Verify the comment belongs to the expected PR by checking the pull_request_url
	// The pull_request_url looks like: https://api.github.com/repos/{owner}/{repo}/pulls/{prNumber}
	expectedPRSuffix := fmt.Sprintf("/pulls/%d", prNumber)
	if !strings.HasSuffix(comment.PullRequestURL, expectedPRSuffix) {
		return nil, triage.ErrCommentNotFound
	}

	// Get reply metadata for this comment
	meta, err := c.getReplyMetadata(ctx, owner, repo, prNumber, commentID)
	if err != nil {
		// Non-fatal: just use empty metadata if we can't get it
		meta = replyMetadata{}
	}

	fingerprint := parseFingerprint(comment.Body)
	finding := apiCommentToFinding(comment, fingerprint, meta)
	return &finding, nil
}

// GetPRCommentByFingerprint retrieves a comment by its CR_FP fingerprint.
// Returns ErrCommentNotFound if no matching comment exists.
//
// This searches both review comments (in-diff findings) and issue comments
// (out-of-diff findings) to find the matching fingerprint.
//
// Performance note: Since GitHub doesn't index comments by our fingerprints,
// this fetches all findings with fingerprints and searches client-side.
// This is O(n) where n is the number of comments with fingerprints.
func (c *Client) GetPRCommentByFingerprint(ctx context.Context, owner, repo string, prNumber int, fingerprint string) (*domain.PRFinding, error) {
	// Normalize fingerprint (remove prefix if present)
	fingerprint = strings.TrimPrefix(fingerprint, domain.FindingIDPrefix)
	if fingerprint == "" {
		return nil, fmt.Errorf("empty fingerprint")
	}

	// Fetch all findings with fingerprints (both review comments and issue comments)
	findings, err := c.ListAllFindings(ctx, owner, repo, prNumber, true)
	if err != nil {
		return nil, err
	}

	// Search for matching fingerprint
	for i := range findings {
		if findings[i].Fingerprint == fingerprint {
			return &findings[i], nil
		}
	}

	return nil, triage.ErrCommentNotFound
}

// GetThreadHistory retrieves the reply chain for a comment thread.
func (c *Client) GetThreadHistory(ctx context.Context, owner, repo string, commentID int64) ([]domain.ThreadComment, error) {
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

	// We need to know the PR number to fetch all comments
	// First, get the comment to find its PR
	apiURL := fmt.Sprintf("%s/repos/%s/%s/pulls/comments/%d",
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
			// Classify network errors properly (timeout vs DNS/TLS/connection)
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

	var rootComment commentAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&rootComment); err != nil {
		return nil, fmt.Errorf("failed to parse comment response: %w", err)
	}

	// Extract PR number from pull_request_url
	prNumber := extractPRNumber(rootComment.PullRequestURL)
	if prNumber == 0 {
		return nil, fmt.Errorf("could not determine PR number from comment")
	}

	// Fetch all comments for the PR
	allComments, err := c.ListPullRequestComments(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("list PR comments: %w", err)
	}

	// Build thread: root comment + all replies
	var thread []domain.ThreadComment

	// Add root comment
	rootCreatedAt, _ := time.Parse(time.RFC3339, rootComment.CreatedAt)
	thread = append(thread, domain.ThreadComment{
		Author:    rootComment.User.Login,
		Body:      rootComment.Body,
		CreatedAt: rootCreatedAt,
		IsReply:   false,
	})

	// Add replies (comments that have InReplyToID == commentID)
	for _, comment := range allComments {
		if comment.InReplyToID == commentID {
			createdAt, _ := time.Parse(time.RFC3339, comment.CreatedAt)
			thread = append(thread, domain.ThreadComment{
				Author:    comment.User.Login,
				Body:      comment.Body,
				CreatedAt: createdAt,
				IsReply:   true,
			})
		}
	}

	// Sort by creation time
	sort.Slice(thread, func(i, j int) bool {
		return thread[i].CreatedAt.Before(thread[j].CreatedAt)
	})

	return thread, nil
}

// commentAPIResponse is the extended response type for single comment endpoint.
type commentAPIResponse struct {
	ID               int64  `json:"id"`
	Body             string `json:"body"`
	Path             string `json:"path"`
	Line             *int   `json:"line"`
	User             User   `json:"user"`
	CreatedAt        string `json:"created_at"`
	HTMLURL          string `json:"html_url"`
	InReplyToID      int64  `json:"in_reply_to_id,omitempty"`
	PullRequestURL   string `json:"pull_request_url"`
	SubjectType      string `json:"subject_type,omitempty"` // "line" or "file"
	StartLine        *int   `json:"start_line,omitempty"`   // For multi-line comments
	OriginalLine     *int   `json:"original_line,omitempty"`
	Position         *int   `json:"position,omitempty"`
	OriginalPosition *int   `json:"original_position,omitempty"`
}

// Helper functions

// replyMetadata captures reply statistics for a parent comment.
type replyMetadata struct {
	count       int
	lastReplyAt time.Time
	lastReplyBy string
}

// buildReplyMetadataMap aggregates reply metadata for each parent comment.
// Returns a map from parent comment ID to its reply metadata.
func buildReplyMetadataMap(comments []PullRequestComment) map[int64]replyMetadata {
	metadata := make(map[int64]replyMetadata)
	for _, c := range comments {
		if c.InReplyToID != 0 {
			createdAt, err := time.Parse(time.RFC3339, c.CreatedAt)
			existing := metadata[c.InReplyToID]
			existing.count++
			// Track the most recent reply (skip if timestamp unparseable)
			if err == nil && createdAt.After(existing.lastReplyAt) {
				existing.lastReplyAt = createdAt
				existing.lastReplyBy = c.User.Login
			}
			metadata[c.InReplyToID] = existing
		}
	}
	return metadata
}

// commentToFinding converts a PullRequestComment to a domain.PRFinding.
func commentToFinding(comment PullRequestComment, fingerprint string, meta replyMetadata) domain.PRFinding {
	line := 0
	if comment.Line != nil {
		line = *comment.Line
	}

	createdAt, _ := time.Parse(time.RFC3339, comment.CreatedAt)

	// Extract reviewer name from CR_REVIEWER marker (Phase 3.2)
	reviewer, _ := ExtractReviewerFromComment(comment.Body)

	finding := domain.PRFinding{
		CommentID:   comment.ID,
		Fingerprint: fingerprint,
		Path:        comment.Path,
		Line:        line,
		Body:        comment.Body,
		Author:      comment.User.Login,
		CreatedAt:   createdAt,
		IsResolved:  false, // GitHub doesn't include this in list endpoint
		ReplyCount:  meta.count,
		HasReply:    meta.count > 0,
		Severity:    parseSeverity(comment.Body),
		Category:    parseCategory(comment.Body),
		Reviewer:    reviewer,
	}

	// Set last reply info if there are replies
	if meta.count > 0 {
		finding.LastReplyAt = &meta.lastReplyAt
		finding.LastReplyBy = meta.lastReplyBy
	}

	return finding
}

// apiCommentToFinding converts a commentAPIResponse to a domain.PRFinding.
func apiCommentToFinding(comment commentAPIResponse, fingerprint string, meta replyMetadata) domain.PRFinding {
	line := 0
	if comment.Line != nil {
		line = *comment.Line
	}

	createdAt, _ := time.Parse(time.RFC3339, comment.CreatedAt)

	// Extract reviewer name from CR_REVIEWER marker (Phase 3.2)
	reviewer, _ := ExtractReviewerFromComment(comment.Body)

	finding := domain.PRFinding{
		CommentID:   comment.ID,
		Fingerprint: fingerprint,
		Path:        comment.Path,
		Line:        line,
		Body:        comment.Body,
		Author:      comment.User.Login,
		CreatedAt:   createdAt,
		IsResolved:  false,
		ReplyCount:  meta.count,
		HasReply:    meta.count > 0,
		Severity:    parseSeverity(comment.Body),
		Category:    parseCategory(comment.Body),
		Reviewer:    reviewer,
	}

	// Set last reply info if there are replies
	if meta.count > 0 {
		finding.LastReplyAt = &meta.lastReplyAt
		finding.LastReplyBy = meta.lastReplyBy
	}

	return finding
}

// parseFingerprint extracts the CR_FP marker from a comment body.
func parseFingerprint(body string) string {
	matches := fingerprintPattern.FindStringSubmatch(body)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// parseSeverity extracts the severity from a comment body.
func parseSeverity(body string) string {
	matches := severityPattern.FindStringSubmatch(body)
	if len(matches) >= 2 {
		// First capture group (from **Severity: X**)
		if matches[1] != "" {
			return strings.ToLower(matches[1])
		}
		// Second capture group (from [X] format)
		if len(matches) >= 3 && matches[2] != "" {
			return strings.ToLower(matches[2])
		}
	}
	return ""
}

// parseCategory extracts the category from a comment body.
func parseCategory(body string) string {
	matches := categoryPattern.FindStringSubmatch(body)
	if len(matches) >= 2 {
		return strings.ToLower(matches[1])
	}
	return ""
}

// extractPRNumber extracts the PR number from a pull_request_url.
// URL format: https://api.github.com/repos/{owner}/{repo}/pulls/{number}
func extractPRNumber(prURL string) int {
	parts := strings.Split(prURL, "/pulls/")
	if len(parts) != 2 {
		return 0
	}
	var num int
	_, _ = fmt.Sscanf(parts[1], "%d", &num)
	return num
}

// getReplyMetadata retrieves reply metadata for a specific comment.
//
// Performance limitation: This fetches all PR comments and filters client-side.
// GitHub's API doesn't provide a direct way to get reply info for a single comment.
// For PRs with hundreds of comments, consider caching the comment list or
// making reply metadata opt-in for callers who don't need it.
func (c *Client) getReplyMetadata(ctx context.Context, owner, repo string, prNumber int, parentID int64) (replyMetadata, error) {
	comments, err := c.ListPullRequestComments(ctx, owner, repo, prNumber)
	if err != nil {
		return replyMetadata{}, err
	}

	// Build full metadata map and extract the one we need
	metadataMap := buildReplyMetadataMap(comments)
	return metadataMap[parentID], nil
}

// ListAllFindings retrieves both review comments AND issue comments that contain
// CR_FP markers. This unified view includes in-diff findings (review comments)
// and out-of-diff findings (issue comments with CR_OOD markers).
//
// The returned findings have the IsOutOfDiff field set appropriately:
// - Review comments: IsOutOfDiff = false
// - Issue comments with CR_OOD:true: IsOutOfDiff = true
//
// Reply tracking for out-of-diff findings uses CR_REPLY_TO markers since GitHub
// issue comments don't have native threading.
func (c *Client) ListAllFindings(ctx context.Context, owner, repo string, prNumber int, filterByFingerprint bool) ([]domain.PRFinding, error) {
	// 1. Fetch review comments (in-diff findings)
	reviewFindings, err := c.ListPRComments(ctx, owner, repo, prNumber, filterByFingerprint)
	if err != nil {
		return nil, fmt.Errorf("list review comments: %w", err)
	}

	// 2. Fetch issue comments
	issueComments, err := c.ListIssueComments(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("list issue comments: %w", err)
	}

	// 3. Build reply metadata for issue comments (CR_REPLY_TO markers)
	issueReplyMap := buildIssueCommentReplyMap(issueComments)

	// 4. Convert issue comments with CR_FP markers to findings
	for _, ic := range issueComments {
		// Parse fingerprint
		fingerprint := parseFingerprint(ic.Body)

		// Apply fingerprint filter
		isTrustedBot := ic.User.Type == "Bot" && trustedBots[strings.ToLower(ic.User.Login)]
		if filterByFingerprint && fingerprint == "" && !isTrustedBot {
			continue
		}

		// Skip replies (they have CR_REPLY_TO markers)
		if _, isReply := ExtractReplyToMarker(ic.Body); isReply {
			continue
		}

		// Convert to domain finding
		finding := issueCommentToFinding(ic, fingerprint, issueReplyMap[fingerprint])
		reviewFindings = append(reviewFindings, finding)
	}

	return reviewFindings, nil
}

// buildIssueCommentReplyMap builds a map from fingerprint to reply metadata.
// Issue comments use CR_REPLY_TO markers to link replies to parent findings.
func buildIssueCommentReplyMap(comments []IssueComment) map[string]replyMetadata {
	metadata := make(map[string]replyMetadata)

	for _, c := range comments {
		parentFP, isReply := ExtractReplyToMarker(c.Body)
		if !isReply || parentFP == "" {
			continue
		}

		createdAt, err := time.Parse(time.RFC3339, c.CreatedAt)
		existing := metadata[parentFP]
		existing.count++

		// Track the most recent reply
		if err == nil && createdAt.After(existing.lastReplyAt) {
			existing.lastReplyAt = createdAt
			existing.lastReplyBy = c.User.Login
		}
		metadata[parentFP] = existing
	}

	return metadata
}

// issueCommentToFinding converts an IssueComment to a domain.PRFinding.
// This is used for out-of-diff findings posted as issue comments.
func issueCommentToFinding(comment IssueComment, fingerprint string, meta replyMetadata) domain.PRFinding {
	createdAt, _ := time.Parse(time.RFC3339, comment.CreatedAt)

	// Extract file and line from CR_FILE/CR_LINE markers
	file, line, _ := ExtractFileLineFromComment(comment.Body)

	// Extract reviewer name
	reviewer, _ := ExtractReviewerFromComment(comment.Body)

	// Determine if this is an out-of-diff finding
	isOutOfDiff := ExtractOutOfDiffMarker(comment.Body)

	finding := domain.PRFinding{
		CommentID:   comment.ID,
		Fingerprint: fingerprint,
		Path:        file,
		Line:        line,
		Body:        comment.Body,
		Author:      comment.User.Login,
		CreatedAt:   createdAt,
		IsResolved:  false, // Issue comments don't have resolution status
		ReplyCount:  meta.count,
		HasReply:    meta.count > 0,
		Severity:    parseSeverity(comment.Body),
		Category:    parseCategory(comment.Body),
		Reviewer:    reviewer,
		IsOutOfDiff: isOutOfDiff,
	}

	// Set last reply info if there are replies
	if meta.count > 0 {
		finding.LastReplyAt = &meta.lastReplyAt
		finding.LastReplyBy = meta.lastReplyBy
	}

	return finding
}
