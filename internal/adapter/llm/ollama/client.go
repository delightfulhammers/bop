package ollama

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
	defaultTimeout = 120 * time.Second // Local models can be slower
)

// HTTPClient is an HTTP client for the Ollama API.
type HTTPClient struct {
	baseURL   string
	model     string
	timeout   time.Duration
	retryConf llmhttp.RetryConfig
	client    *http.Client

	// Observability components
	logger  llmhttp.Logger
	metrics llmhttp.Metrics
	pricing llmhttp.Pricing
}

// NewHTTPClient creates a new Ollama HTTP client.
func NewHTTPClient(baseURL, model string, providerCfg config.ProviderConfig, httpCfg config.HTTPConfig) *HTTPClient {
	timeout := llmhttp.ParseTimeout(providerCfg.Timeout, httpCfg.Timeout, defaultTimeout)
	retryConf := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	return &HTTPClient{
		baseURL:   baseURL,
		model:     model,
		timeout:   timeout,
		retryConf: retryConf,
		client:    &http.Client{Timeout: timeout},
	}
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
	Seed        *uint64
}

// APIResponse represents the parsed response from the API.
type APIResponse struct {
	Text      string
	TokensIn  int
	TokensOut int
	Model     string
	Cost      float64 // Cost in USD (always $0 for Ollama/local)
}

// Call makes a request to the Ollama Generate API.
func (c *HTTPClient) Call(ctx context.Context, prompt string, options CallOptions) (*APIResponse, error) {
	startTime := time.Now()

	// Log request (if logger configured)
	if c.logger != nil {
		c.logger.LogRequest(ctx, llmhttp.RequestLog{
			Provider:      "ollama",
			Model:         c.model,
			Timestamp:     startTime,
			PromptChars:   len(prompt),
			APIKey:        "",     // Ollama doesn't use API keys
			PromptContent: prompt, // For trace-level logging
		})
	}

	// Record request metric
	if c.metrics != nil {
		c.metrics.RecordRequest("ollama", c.model)
	}

	// Build request
	reqBody := GenerateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false, // We don't use streaming
	}

	// Add options
	opts := make(map[string]interface{})
	if options.Temperature > 0 {
		opts["temperature"] = options.Temperature
	}
	if options.Seed != nil {
		opts["seed"] = float64(*options.Seed)
	}
	if len(opts) > 0 {
		reqBody.Options = opts
	}

	// Marshal request
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := c.baseURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request with retry logic (using configured retry settings)
	var resp *http.Response

	err = llmhttp.RetryWithBackoff(ctx, func(ctx context.Context) error {
		// Recreate request for each retry
		retryReq, reqErr := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if reqErr != nil {
			return &llmhttp.Error{
				Type:      llmhttp.ErrTypeUnknown,
				Message:   reqErr.Error(),
				Retryable: false,
				Provider:  "ollama",
			}
		}

		retryReq.Header.Set("Content-Type", "application/json")

		var callErr error
		resp, callErr = c.client.Do(retryReq)
		if callErr != nil {
			// Check for connection refused (Ollama not running)
			if strings.Contains(callErr.Error(), "connection refused") {
				return &llmhttp.Error{
					Type:      llmhttp.ErrTypeServiceUnavailable,
					Message:   fmt.Sprintf("Ollama server not reachable. Is Ollama running? Try: ollama serve. Error: %s", callErr.Error()),
					Retryable: false,
					Provider:  "ollama",
				}
			}
			return &llmhttp.Error{
				Type:      llmhttp.ErrTypeTimeout,
				Message:   callErr.Error(),
				Retryable: false,
				Provider:  "ollama",
			}
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
					Provider:   "ollama",
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
				c.metrics.RecordError("ollama", c.model, httpErr.Type)
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

	var genResp GenerateResponse
	if err := json.Unmarshal(bodyBytes, &genResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Validate response
	if !genResp.Done {
		return nil, fmt.Errorf("incomplete response from Ollama (done=false)")
	}

	if genResp.Response == "" {
		return nil, fmt.Errorf("empty response from Ollama")
	}

	response := &APIResponse{
		Text:      genResp.Response,
		TokensIn:  genResp.PromptEvalCount,
		TokensOut: genResp.EvalCount,
		Model:     genResp.Model,
		Cost:      0.0, // Ollama is always free (local)
	}

	// Log response
	if c.logger != nil {
		c.logger.LogResponse(ctx, llmhttp.ResponseLog{
			Provider:        "ollama",
			Model:           c.model,
			Timestamp:       time.Now(),
			Duration:        duration,
			TokensIn:        response.TokensIn,
			TokensOut:       response.TokensOut,
			Cost:            0.0,
			StatusCode:      200,
			FinishReason:    "complete",
			ResponseContent: response.Text, // For trace-level logging
		})
	}

	// Record metrics
	if c.metrics != nil {
		c.metrics.RecordDuration("ollama", c.model, duration)
		c.metrics.RecordTokens("ollama", c.model, response.TokensIn, response.TokensOut)
		c.metrics.RecordCost("ollama", c.model, 0.0)
	}

	return response, nil
}

// handleErrorResponse maps HTTP status codes to typed errors.
func (c *HTTPClient) handleErrorResponse(statusCode int, body []byte) error {
	// Try to parse Ollama error format
	var errResp ErrorResponse
	defaultMessage := fmt.Sprintf("HTTP %d", statusCode)
	message := defaultMessage

	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
		message = errResp.Error
	}

	// Map status codes to error types
	switch statusCode {
	case http.StatusNotFound:
		// Model not found
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeModelNotFound,
			Message:    fmt.Sprintf("%s. Pull it with: ollama pull %s", message, c.model),
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "ollama",
		}
	case http.StatusBadRequest:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeInvalidRequest,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "ollama",
		}
	case http.StatusServiceUnavailable, http.StatusInternalServerError:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeServiceUnavailable,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  true,
			Provider:   "ollama",
		}
	default:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeUnknown,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "ollama",
		}
	}
}

// CreateReview implements the Client interface for the Provider.
func (c *HTTPClient) CreateReview(ctx context.Context, req Request) (llm.ProviderResponse, error) {
	var seed *uint64
	if req.Seed > 0 {
		seed = &req.Seed
	}

	apiResp, err := c.Call(ctx, req.Prompt, CallOptions{
		Seed: seed,
	})
	if err != nil {
		return llm.ProviderResponse{}, fmt.Errorf("ollama: %w", err)
	}

	// Build usage metadata from API response
	// Note: Ollama is local/free, so Cost will be 0
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
