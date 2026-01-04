package github_test

import (
	"encoding/json"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/github"
	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapHTTPError_Authentication(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "401 Unauthorized",
			statusCode: 401,
			body:       `{"message": "Bad credentials"}`,
		},
		{
			name:       "403 Forbidden",
			statusCode: 403,
			body:       `{"message": "Must have admin rights"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := github.MapHTTPError(tt.statusCode, []byte(tt.body))

			require.NotNil(t, err)
			assert.Equal(t, llmhttp.ErrTypeAuthentication, err.Type)
			assert.Equal(t, "github", err.Provider)
			assert.Equal(t, tt.statusCode, err.StatusCode)
			assert.False(t, err.Retryable)
		})
	}
}

func TestMapHTTPError_RateLimit(t *testing.T) {
	body := `{"message": "API rate limit exceeded"}`
	err := github.MapHTTPError(429, []byte(body))

	require.NotNil(t, err)
	assert.Equal(t, llmhttp.ErrTypeRateLimit, err.Type)
	assert.Equal(t, "github", err.Provider)
	assert.Equal(t, 429, err.StatusCode)
	assert.True(t, err.Retryable)
	assert.Contains(t, err.Message, "rate limit")
}

func TestMapHTTPError_InvalidRequest(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "404 Not Found",
			statusCode: 404,
			body:       `{"message": "Not Found"}`,
		},
		{
			name:       "422 Validation Failed",
			statusCode: 422,
			body:       `{"message": "Validation Failed", "errors": [{"field": "position", "code": "invalid"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := github.MapHTTPError(tt.statusCode, []byte(tt.body))

			require.NotNil(t, err)
			assert.Equal(t, llmhttp.ErrTypeInvalidRequest, err.Type)
			assert.Equal(t, "github", err.Provider)
			assert.Equal(t, tt.statusCode, err.StatusCode)
			assert.False(t, err.Retryable)
		})
	}
}

func TestMapHTTPError_ServiceUnavailable(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "500 Internal Server Error", statusCode: 500},
		{name: "502 Bad Gateway", statusCode: 502},
		{name: "503 Service Unavailable", statusCode: 503},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := github.MapHTTPError(tt.statusCode, []byte(`{"message": "Server error"}`))

			require.NotNil(t, err)
			assert.Equal(t, llmhttp.ErrTypeServiceUnavailable, err.Type)
			assert.Equal(t, "github", err.Provider)
			assert.Equal(t, tt.statusCode, err.StatusCode)
			assert.True(t, err.Retryable)
		})
	}
}

func TestMapHTTPError_UnknownError(t *testing.T) {
	err := github.MapHTTPError(418, []byte(`{"message": "I'm a teapot"}`))

	require.NotNil(t, err)
	assert.Equal(t, llmhttp.ErrTypeUnknown, err.Type)
	assert.Equal(t, "github", err.Provider)
	assert.Equal(t, 418, err.StatusCode)
	assert.False(t, err.Retryable)
}

func TestMapHTTPError_ParsesErrorMessage(t *testing.T) {
	body := `{"message": "Specific error message from GitHub"}`
	err := github.MapHTTPError(400, []byte(body))

	require.NotNil(t, err)
	assert.Contains(t, err.Message, "Specific error message from GitHub")
}

func TestMapHTTPError_HandlesInvalidJSON(t *testing.T) {
	err := github.MapHTTPError(500, []byte(`not json`))

	require.NotNil(t, err)
	assert.Equal(t, llmhttp.ErrTypeServiceUnavailable, err.Type)
	// Should still have a reasonable message
	assert.NotEmpty(t, err.Message)
}

func TestMapHTTPError_ParsesValidationErrors(t *testing.T) {
	body, _ := json.Marshal(github.GitHubErrorResponse{
		Message: "Validation Failed",
		Errors: []struct {
			Resource string `json:"resource"`
			Field    string `json:"field"`
			Code     string `json:"code"`
			Message  string `json:"message"`
		}{
			{Resource: "PullRequestReviewComment", Field: "position", Code: "invalid", Message: "position is invalid"},
		},
	})

	err := github.MapHTTPError(422, body)

	require.NotNil(t, err)
	assert.Contains(t, err.Message, "position")
}
