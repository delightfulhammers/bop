package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
)

const providerName = "github"

// MapHTTPError maps GitHub API HTTP status codes to typed llmhttp.Error.
// This allows reuse of existing retry logic and error handling infrastructure.
func MapHTTPError(statusCode int, body []byte) *llmhttp.Error {
	message := parseErrorMessage(statusCode, body)

	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeAuthentication,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   providerName,
		}

	case http.StatusTooManyRequests:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeRateLimit,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  true,
			Provider:   providerName,
		}

	case http.StatusNotFound:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeInvalidRequest,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   providerName,
		}

	case http.StatusUnprocessableEntity:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeInvalidRequest,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   providerName,
		}

	case http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeServiceUnavailable,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  true,
			Provider:   providerName,
		}

	default:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeUnknown,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   providerName,
		}
	}
}

// parseErrorMessage extracts a user-friendly error message from GitHub's response.
func parseErrorMessage(statusCode int, body []byte) string {
	var errResp GitHubErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		// Include body preview for debugging non-JSON responses
		bodyPreview := string(body)
		if len(bodyPreview) > 100 {
			bodyPreview = bodyPreview[:100] + "..."
		}
		if bodyPreview == "" {
			return fmt.Sprintf("HTTP %d", statusCode)
		}
		return fmt.Sprintf("HTTP %d: %s", statusCode, bodyPreview)
	}

	if errResp.Message == "" {
		return fmt.Sprintf("HTTP %d", statusCode)
	}

	// If there are validation errors, append them
	if len(errResp.Errors) > 0 {
		var details []string
		for _, e := range errResp.Errors {
			if e.Message != "" {
				details = append(details, e.Message)
			} else if e.Field != "" {
				details = append(details, fmt.Sprintf("%s: %s", e.Field, e.Code))
			}
		}
		if len(details) > 0 {
			return fmt.Sprintf("%s: %s", errResp.Message, strings.Join(details, "; "))
		}
	}

	return errResp.Message
}
