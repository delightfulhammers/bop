package simple

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/delightfulhammers/bop/internal/config"
)

const (
	openaiBaseURL     = "https://api.openai.com/v1/chat/completions"
	openaiTimeout     = 120 * time.Second
	openaiMaxResponse = 10 * 1024 * 1024 // 10MB
)

// OpenAIClient implements Client using the OpenAI API.
type OpenAIClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
	retryConf  llmhttp.RetryConfig
}

// NewOpenAIClient creates a new OpenAI client.
func NewOpenAIClient(apiKey, model string, providerCfg config.ProviderConfig, httpCfg config.HTTPConfig) *OpenAIClient {
	timeout := llmhttp.ParseTimeout(providerCfg.Timeout, httpCfg.Timeout, openaiTimeout)
	retryConf := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	return &OpenAIClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryConf: retryConf,
	}
}

// Call implements Client.
func (c *OpenAIClient) Call(ctx context.Context, prompt string, maxTokens int) (string, error) {
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

// isReasoningModel checks if the model is an OpenAI reasoning model.
func isReasoningModel(model string) bool {
	modelLower := strings.ToLower(model)
	reasoningPrefixes := []string{"o1", "o3", "o4"}
	for _, prefix := range reasoningPrefixes {
		if modelLower == prefix || strings.HasPrefix(modelLower, prefix+"-") {
			return true
		}
	}
	return false
}

// usesMaxCompletionTokens checks if the model requires max_completion_tokens.
func usesMaxCompletionTokens(model string) bool {
	if isReasoningModel(model) {
		return true
	}
	modelLower := strings.ToLower(model)
	newPrefixes := []string{"gpt-5", "gpt-6", "gpt-7", "gpt-8", "gpt-9"}
	for _, prefix := range newPrefixes {
		if strings.HasPrefix(modelLower, prefix) {
			return true
		}
	}
	return false
}

func (c *OpenAIClient) doRequest(ctx context.Context, prompt string, maxTokens int) (string, error) {
	reqBody := openaiRequest{
		Model: c.model,
		Messages: []openaiMessage{
			{Role: "user", Content: prompt},
		},
	}

	// Handle model-specific token limits
	if usesMaxCompletionTokens(c.model) {
		reqBody.MaxCompletionTokens = maxTokens
	} else {
		reqBody.MaxTokens = maxTokens
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openaiBaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", classifyHTTPError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	limitedReader := io.LimitReader(resp.Body, openaiMaxResponse)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check if response was truncated
	if int64(len(body)) >= openaiMaxResponse {
		return "", fmt.Errorf("response exceeded maximum size of %d bytes", openaiMaxResponse)
	}

	if resp.StatusCode != http.StatusOK {
		return "", mapOpenAIError(resp.StatusCode, body)
	}

	var apiResp openaiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return apiResp.Choices[0].Message.Content, nil
}

type openaiRequest struct {
	Model               string          `json:"model"`
	Messages            []openaiMessage `json:"messages"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}

func mapOpenAIError(statusCode int, body []byte) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return llmhttp.NewAuthenticationError("openai", string(body))
	case http.StatusTooManyRequests:
		return llmhttp.NewRateLimitError("openai", string(body))
	case http.StatusBadRequest:
		return llmhttp.NewInvalidRequestError("openai", string(body))
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return llmhttp.NewServiceUnavailableError("openai", string(body))
	default:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeUnknown,
			Message:    string(body),
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "openai",
		}
	}
}
