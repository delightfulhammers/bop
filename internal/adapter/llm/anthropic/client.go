package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bkyoung/code-reviewer/internal/adapter/llm"
	llmhttp "github.com/bkyoung/code-reviewer/internal/adapter/llm/http"
	"github.com/bkyoung/code-reviewer/internal/config"
	"github.com/bkyoung/code-reviewer/internal/domain"
)

const (
	defaultBaseURL          = "https://api.anthropic.com"
	defaultTimeout          = 60 * time.Second
	defaultAnthropicVersion = "2023-06-01"
)

// HTTPClient is an HTTP client for the Anthropic API.
type HTTPClient struct {
	apiKey    string
	model     string
	baseURL   string
	timeout   time.Duration
	retryConf llmhttp.RetryConfig
	client    *http.Client

	// Observability components
	logger  llmhttp.Logger
	metrics llmhttp.Metrics
	pricing llmhttp.Pricing
}

// NewHTTPClient creates a new Anthropic HTTP client.
func NewHTTPClient(apiKey, model string, providerCfg config.ProviderConfig, httpCfg config.HTTPConfig) *HTTPClient {
	timeout := llmhttp.ParseTimeout(providerCfg.Timeout, httpCfg.Timeout, defaultTimeout)
	retryConf := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	return &HTTPClient{
		apiKey:    apiKey,
		model:     model,
		baseURL:   defaultBaseURL,
		timeout:   timeout,
		retryConf: retryConf,
		client:    &http.Client{Timeout: timeout},
	}
}

// SetBaseURL sets a custom base URL (for testing).
func (c *HTTPClient) SetBaseURL(url string) {
	c.baseURL = url
}

// SetTimeout sets the HTTP timeout.
func (c *HTTPClient) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
	c.client.Timeout = timeout
}

// SetLogger sets the logger for this client.
func (c *HTTPClient) SetLogger(logger llmhttp.Logger) {
	c.logger = logger
}

// SetMetrics sets the metrics tracker for this client.
func (c *HTTPClient) SetMetrics(metrics llmhttp.Metrics) {
	c.metrics = metrics
}

// SetPricing sets the pricing calculator for this client.
func (c *HTTPClient) SetPricing(pricing llmhttp.Pricing) {
	c.pricing = pricing
}

// CallOptions contains options for the API call.
type CallOptions struct {
	Temperature float64
	MaxTokens   int
	System      string
}

// APIResponse represents the parsed response from the API.
type APIResponse struct {
	Text       string
	TokensIn   int
	TokensOut  int
	Model      string
	StopReason string
	Cost       float64 // Cost in USD
}

// Call makes a request to the Anthropic Messages API.
func (c *HTTPClient) Call(ctx context.Context, prompt string, options CallOptions) (*APIResponse, error) {
	startTime := time.Now()

	// Log request (if logger configured)
	if c.logger != nil {
		c.logger.LogRequest(ctx, llmhttp.RequestLog{
			Provider:    "anthropic",
			Model:       c.model,
			Timestamp:   startTime,
			PromptChars: len(prompt),
			APIKey:      c.apiKey,
		})
	}

	// Record request metric
	if c.metrics != nil {
		c.metrics.RecordRequest("anthropic", c.model)
	}

	// Build request
	reqBody := MessagesRequest{
		Model: c.model,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		MaxTokens: options.MaxTokens,
	}

	// Add optional system message
	if options.System != "" {
		reqBody.System = options.System
	} else {
		reqBody.System = "You are a code review assistant. Analyze the code and provide feedback in JSON format."
	}

	// Add temperature if provided
	if options.Temperature > 0 {
		reqBody.Temperature = options.Temperature
	}

	// Marshal request
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := c.baseURL + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers (Anthropic uses x-api-key instead of Authorization)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", defaultAnthropicVersion)

	// Execute request with retry logic (using configured retry settings)
	var resp *http.Response

	err = llmhttp.RetryWithBackoff(ctx, func(ctx context.Context) error {
		// Recreate request for each retry with fresh context
		retryReq, reqErr := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if reqErr != nil {
			return &llmhttp.Error{
				Type:      llmhttp.ErrTypeUnknown,
				Message:   reqErr.Error(),
				Retryable: false,
				Provider:  "anthropic",
			}
		}

		retryReq.Header.Set("Content-Type", "application/json")
		retryReq.Header.Set("x-api-key", c.apiKey)
		retryReq.Header.Set("anthropic-version", defaultAnthropicVersion)

		var callErr error
		resp, callErr = c.client.Do(retryReq)
		if callErr != nil {
			// Use helper that correctly marks timeouts as retryable
			return llmhttp.NewTimeoutError("anthropic", callErr.Error())
		}

		// Check for error status codes
		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return c.handleErrorResponse(resp.StatusCode, bodyBytes)
		}

		return nil
	}, c.retryConf)

	duration := time.Since(startTime)

	if err != nil {
		// Log error
		if c.logger != nil {
			var httpErr *llmhttp.Error
			if errors.As(err, &httpErr) {
				c.logger.LogError(ctx, llmhttp.ErrorLog{
					Provider:   "anthropic",
					Model:      c.model,
					Timestamp:  time.Now(),
					Duration:   duration,
					Error:      err,
					ErrorType:  httpErr.Type,
					StatusCode: httpErr.StatusCode,
					Retryable:  httpErr.Retryable,
				})
			}
		}
		// Record error metric
		if c.metrics != nil {
			var httpErr *llmhttp.Error
			if errors.As(err, &httpErr) {
				c.metrics.RecordError("anthropic", c.model, httpErr.Type)
			}
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Parse response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var messagesResp MessagesResponse
	if err := json.Unmarshal(bodyBytes, &messagesResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract text from content blocks
	if len(messagesResp.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	var textParts []string
	for _, block := range messagesResp.Content {
		if block.Type == "text" {
			textParts = append(textParts, block.Text)
		}
	}

	response := &APIResponse{
		Text:       strings.Join(textParts, ""),
		TokensIn:   messagesResp.Usage.InputTokens,
		TokensOut:  messagesResp.Usage.OutputTokens,
		Model:      messagesResp.Model,
		StopReason: messagesResp.StopReason,
	}

	// Calculate cost
	var cost float64
	if c.pricing != nil {
		cost = c.pricing.GetCost("anthropic", c.model, response.TokensIn, response.TokensOut)
		response.Cost = cost
	}

	// Log response
	if c.logger != nil {
		c.logger.LogResponse(ctx, llmhttp.ResponseLog{
			Provider:     "anthropic",
			Model:        c.model,
			Timestamp:    time.Now(),
			Duration:     duration,
			TokensIn:     response.TokensIn,
			TokensOut:    response.TokensOut,
			Cost:         cost,
			StatusCode:   200,
			FinishReason: response.StopReason,
		})
	}

	// Record metrics
	if c.metrics != nil {
		c.metrics.RecordDuration("anthropic", c.model, duration)
		c.metrics.RecordTokens("anthropic", c.model, response.TokensIn, response.TokensOut)
		c.metrics.RecordCost("anthropic", c.model, cost)
	}

	return response, nil
}

// handleErrorResponse maps HTTP status codes to typed errors.
func (c *HTTPClient) handleErrorResponse(statusCode int, body []byte) error {
	// Try to parse Anthropic error format
	var errResp ErrorResponse
	defaultMessage := fmt.Sprintf("HTTP %d", statusCode)
	message := defaultMessage

	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		message = errResp.Error.Message
	}

	// Map status codes to error types
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeAuthentication,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "anthropic",
		}
	case http.StatusTooManyRequests:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeRateLimit,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  true,
			Provider:   "anthropic",
		}
	case http.StatusBadRequest:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeInvalidRequest,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "anthropic",
		}
	case 529: // Anthropic-specific: overloaded
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeServiceUnavailable,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  true,
			Provider:   "anthropic",
		}
	case http.StatusServiceUnavailable, http.StatusInternalServerError:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeServiceUnavailable,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  true,
			Provider:   "anthropic",
		}
	default:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeUnknown,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "anthropic",
		}
	}
}

// CreateReview implements the Client interface for the Provider.
func (c *HTTPClient) CreateReview(ctx context.Context, req Request) (llm.ProviderResponse, error) {
	apiResp, err := c.Call(ctx, req.Prompt, CallOptions{
		MaxTokens: req.MaxTokens,
		System:    "",
	})
	if err != nil {
		return llm.ProviderResponse{}, fmt.Errorf("anthropic: %w", err)
	}

	// Build usage metadata from API response
	usage := llm.UsageMetadata{
		TokensIn:  apiResp.TokensIn,
		TokensOut: apiResp.TokensOut,
		Cost:      apiResp.Cost,
	}

	// Parse the response text to extract JSON review
	review, err := parseReviewJSON(apiResp.Text)
	if err != nil {
		// If JSON parsing fails, return text as summary
		return llm.ProviderResponse{
			Model:    apiResp.Model,
			Summary:  apiResp.Text,
			Findings: []domain.Finding{},
			Usage:    usage,
		}, nil
	}

	review.Model = apiResp.Model
	review.Usage = usage
	return review, nil
}

// parseReviewJSON extracts and parses the JSON review from the response text.
// The LLM may return JSON wrapped in markdown code blocks.
func parseReviewJSON(text string) (llm.ProviderResponse, error) {
	// Use shared JSON parsing utility
	summary, findings, err := llmhttp.ParseReviewResponse(text)
	if err != nil {
		return llm.ProviderResponse{}, err
	}

	return llm.ProviderResponse{
		Summary:  summary,
		Findings: findings,
	}, nil
}
