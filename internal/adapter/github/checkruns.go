package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
)

// ListCheckRuns returns check runs for a commit, optionally filtered by name.
// If checkName is nil, all check runs are returned.
// Note: Returns up to 100 check runs in GitHub API order (not sorted).
// TODO: Implement pagination for repos with >100 check runs per commit.
func (c *Client) ListCheckRuns(ctx context.Context, owner, repo, ref string, checkName *string) ([]domain.CheckRunSummary, error) {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return nil, err
	}
	if ref == "" {
		return nil, fmt.Errorf("invalid ref: must not be empty")
	}

	// Build URL
	apiURL := fmt.Sprintf("%s/repos/%s/%s/commits/%s/check-runs",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(ref))

	// Add query params
	params := url.Values{}
	params.Set("per_page", "100")
	if checkName != nil {
		params.Set("check_name", *checkName)
	}
	apiURL = apiURL + "?" + params.Encode()

	// Execute request with retry
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

	// Parse response
	var apiResp checkRunsAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse check runs response: %w", err)
	}

	// Convert to domain types
	result := make([]domain.CheckRunSummary, len(apiResp.CheckRuns))
	for i, cr := range apiResp.CheckRuns {
		result[i] = checkRunToDomain(cr)
	}

	return result, nil
}

// GetAnnotations retrieves annotations for a specific check run.
// Note: Returns up to 100 annotations in GitHub API order.
// TODO: Implement pagination for check runs with >100 annotations.
func (c *Client) GetAnnotations(ctx context.Context, owner, repo string, checkRunID int64) ([]domain.Annotation, error) {
	// Validate inputs
	if err := validatePathSegment(owner, "owner"); err != nil {
		return nil, err
	}
	if err := validatePathSegment(repo, "repo"); err != nil {
		return nil, err
	}

	// Build URL
	apiURL := fmt.Sprintf("%s/repos/%s/%s/check-runs/%d/annotations?per_page=100",
		c.baseURL, url.PathEscape(owner), url.PathEscape(repo), checkRunID)

	// Execute request with retry
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

	// Parse response
	var apiAnns []annotationAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiAnns); err != nil {
		return nil, fmt.Errorf("failed to parse annotations response: %w", err)
	}

	// Convert to domain types with indices
	result := make([]domain.Annotation, len(apiAnns))
	for i, ann := range apiAnns {
		result[i] = annotationToDomain(ann, checkRunID, i)
	}

	return result, nil
}

// GetAnnotation retrieves a single annotation by check run ID and index.
// Returns ErrAnnotationNotFound if the index is out of range.
//
// Note: This fetches all annotations and returns the one at the specified index.
// The GitHub API doesn't support fetching a single annotation directly.
// For check runs with many annotations, consider using GetAnnotations with
// client-side caching if repeated single-annotation lookups are needed.
func (c *Client) GetAnnotation(ctx context.Context, owner, repo string, checkRunID int64, index int) (*domain.Annotation, error) {
	// Validate index early
	if index < 0 {
		return nil, triage.ErrAnnotationNotFound
	}

	// Fetch all annotations - GitHub API doesn't support single annotation retrieval
	annotations, err := c.GetAnnotations(ctx, owner, repo, checkRunID)
	if err != nil {
		return nil, err
	}

	// Check bounds
	if index >= len(annotations) {
		return nil, triage.ErrAnnotationNotFound
	}

	return &annotations[index], nil
}

// API response types

type checkRunsAPIResponse struct {
	TotalCount int                   `json:"total_count"`
	CheckRuns  []checkRunAPIResponse `json:"check_runs"`
}

type checkRunAPIResponse struct {
	ID          int64                     `json:"id"`
	Name        string                    `json:"name"`
	Status      string                    `json:"status"`
	Conclusion  *string                   `json:"conclusion"`
	HeadSHA     string                    `json:"head_sha"`
	StartedAt   *time.Time                `json:"started_at"`
	CompletedAt *time.Time                `json:"completed_at"`
	Output      checkRunOutputAPIResponse `json:"output"`
}

type checkRunOutputAPIResponse struct {
	AnnotationsCount int `json:"annotations_count"`
}

type annotationAPIResponse struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	AnnotationLevel string `json:"annotation_level"`
	Message         string `json:"message"`
	Title           string `json:"title"`
	RawDetails      string `json:"raw_details"`
}

// Conversion functions

func checkRunToDomain(cr checkRunAPIResponse) domain.CheckRunSummary {
	summary := domain.CheckRunSummary{
		ID:              cr.ID,
		Name:            cr.Name,
		Status:          cr.Status,
		HeadSHA:         cr.HeadSHA,
		AnnotationCount: cr.Output.AnnotationsCount,
	}

	if cr.Conclusion != nil {
		summary.Conclusion = *cr.Conclusion
	}
	if cr.StartedAt != nil {
		summary.StartedAt = *cr.StartedAt
	}
	if cr.CompletedAt != nil {
		summary.CompletedAt = *cr.CompletedAt
	}

	return summary
}

func annotationToDomain(ann annotationAPIResponse, checkRunID int64, index int) domain.Annotation {
	return domain.Annotation{
		CheckRunID: checkRunID,
		Index:      index,
		Path:       ann.Path,
		StartLine:  ann.StartLine,
		EndLine:    ann.EndLine,
		Level:      annotationLevelFromAPI(ann.AnnotationLevel),
		Message:    ann.Message,
		Title:      ann.Title,
		RawDetails: ann.RawDetails,
	}
}

func annotationLevelFromAPI(level string) domain.AnnotationLevel {
	switch level {
	case "notice":
		return domain.AnnotationLevelNotice
	case "warning":
		return domain.AnnotationLevelWarning
	case "failure":
		return domain.AnnotationLevelFailure
	default:
		return domain.AnnotationLevelNotice
	}
}
