package dedup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
	"github.com/bkyoung/code-reviewer/internal/config"
)

// maxResponseSize limits the response body to prevent memory exhaustion.
const maxResponseSize = 10 * 1024 * 1024 // 10MB

const (
	anthropicBaseURL  = "https://api.anthropic.com/v1/messages"
	anthropicVersion  = "2023-06-01"
	defaultTimeout    = 120 * time.Second
	defaultMaxRetries = 3
)

// AnthropicClient implements the Client interface using the Anthropic API.
type AnthropicClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
	retryConf  llmhttp.RetryConfig
}

// NewAnthropicClient creates a new Anthropic client for semantic deduplication.
func NewAnthropicClient(apiKey, model string, providerCfg config.ProviderConfig, httpCfg config.HTTPConfig) *AnthropicClient {
	timeout := llmhttp.ParseTimeout(providerCfg.Timeout, httpCfg.Timeout, defaultTimeout)
	retryConf := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	return &AnthropicClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: anthropicBaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryConf: retryConf,
	}
}

// Compare sends a comparison prompt to Anthropic and returns the response text.
func (c *AnthropicClient) Compare(ctx context.Context, prompt string, maxTokens int) (string, error) {
	var result string

	operation := func(ctx context.Context) error {
		resp, err := c.doRequest(ctx, prompt, maxTokens)
		if err != nil {
			return err
		}
		result = resp
		return nil
	}

	err := llmhttp.RetryWithBackoff(ctx, operation, c.retryConf)
	if err != nil {
		return "", err
	}

	return result, nil
}

// doRequest makes a single HTTP request to the Anthropic API.
func (c *AnthropicClient) doRequest(ctx context.Context, prompt string, maxTokens int) (string, error) {
	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: maxTokens,
		Messages: []anthropicMessage{
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", classifyHTTPError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check if response was truncated
	if int64(len(body)) == maxResponseSize {
		return "", fmt.Errorf("response exceeded maximum size of %d bytes", maxResponseSize)
	}

	if resp.StatusCode != http.StatusOK {
		return "", c.mapError(resp.StatusCode, body)
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract text from content blocks
	var text string
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return text, nil
}

// classifyHTTPError categorizes HTTP client errors appropriately.
// Distinguishes between timeouts, context cancellation, and network errors.
func classifyHTTPError(err error) error {
	// Check for context cancellation first
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("request canceled: %w", err)
	}

	// Check for context deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return llmhttp.NewTimeoutError("anthropic", "request timed out")
	}

	// Check for network timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return llmhttp.NewTimeoutError("anthropic", "network timeout")
	}

	// Other network errors are retryable service unavailable
	return llmhttp.NewServiceUnavailableError("anthropic", err.Error())
}

// mapError converts HTTP status codes to typed errors.
func (c *AnthropicClient) mapError(statusCode int, body []byte) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return llmhttp.NewAuthenticationError("anthropic", string(body))
	case http.StatusTooManyRequests:
		return llmhttp.NewRateLimitError("anthropic", string(body))
	case http.StatusBadRequest:
		return llmhttp.NewInvalidRequestError("anthropic", string(body))
	case 529: // Anthropic-specific: overloaded
		return llmhttp.NewServiceUnavailableError("anthropic", string(body))
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return llmhttp.NewServiceUnavailableError("anthropic", string(body))
	default:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeUnknown,
			Message:    string(body),
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "anthropic",
		}
	}
}

// Anthropic API types

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
	Usage   anthropicUsage     `json:"usage,omitempty"`
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
