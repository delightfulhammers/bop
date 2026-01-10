package simple

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

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/delightfulhammers/bop/internal/config"
)

const (
	anthropicBaseURL = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	defaultTimeout   = 120 * time.Second
	maxResponseSize  = 10 * 1024 * 1024 // 10MB
)

// AnthropicClient implements Client using the Anthropic API.
type AnthropicClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
	retryConf  llmhttp.RetryConfig
}

// NewAnthropicClient creates a new Anthropic client.
func NewAnthropicClient(apiKey, model string, providerCfg config.ProviderConfig, httpCfg config.HTTPConfig) *AnthropicClient {
	timeout := llmhttp.ParseTimeout(providerCfg.Timeout, httpCfg.Timeout, defaultTimeout)
	retryConf := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	return &AnthropicClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryConf: retryConf,
	}
}

// Call implements Client.
func (c *AnthropicClient) Call(ctx context.Context, prompt string, maxTokens int) (string, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicBaseURL, bytes.NewReader(bodyBytes))
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

	limitedReader := io.LimitReader(resp.Body, maxResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", mapAnthropicError(resp.StatusCode, body)
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	var text string
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return text, nil
}

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
}

type anthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// classifyHTTPError categorizes HTTP client errors appropriately.
func classifyHTTPError(err error) error {
	// Check for context cancellation first
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("request canceled: %w", err)
	}

	// Check for context deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return llmhttp.NewTimeoutError("llm", "request timed out")
	}

	// Check for network timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return llmhttp.NewTimeoutError("llm", "network timeout")
	}

	// Other network errors are retryable service unavailable
	return llmhttp.NewServiceUnavailableError("llm", err.Error())
}

func mapAnthropicError(statusCode int, body []byte) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return llmhttp.NewAuthenticationError("anthropic", string(body))
	case http.StatusTooManyRequests:
		return llmhttp.NewRateLimitError("anthropic", string(body))
	case http.StatusBadRequest:
		return llmhttp.NewInvalidRequestError("anthropic", string(body))
	case 529:
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
