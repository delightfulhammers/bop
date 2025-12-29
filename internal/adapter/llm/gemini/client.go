package gemini

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
	defaultBaseURL = "https://generativelanguage.googleapis.com"
	defaultTimeout = 60 * time.Second

	// systemInstruction is the system instruction for Gemini to return structured JSON.
	// Gemini requires explicit formatting instructions unlike OpenAI/Anthropic.
	systemInstruction = `You are a code review assistant. Analyze the code and provide feedback in JSON format.

Return your response as JSON wrapped in a markdown code block like this:
` + "```json\n" + `{
  "summary": "Brief overview of the code changes",
  "findings": [
    {
      "severity": "error|warning|info",
      "category": "bug|style|performance|security|maintainability",
      "message": "Description of the issue",
      "file": "path/to/file.ext",
      "line": 42,
      "suggestion": "How to fix it (optional)"
    }
  ]
}
` + "```"
)

// HTTPClient is an HTTP client for the Google Gemini API.
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

// NewHTTPClient creates a new Gemini HTTP client.
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
	Temperature       float64
	MaxTokens         int
	SystemInstruction *string // Optional override for system instruction (nil=use default, empty=""=disable, non-empty=override)
}

// APIResponse represents the parsed response from the API.
type APIResponse struct {
	Text         string
	TokensIn     int
	TokensOut    int
	FinishReason string
	Cost         float64 // Cost in USD
}

// Call makes a request to the Gemini generateContent API.
func (c *HTTPClient) Call(ctx context.Context, prompt string, options CallOptions) (*APIResponse, error) {
	startTime := time.Now()

	// Log request (if logger configured)
	if c.logger != nil {
		c.logger.LogRequest(ctx, llmhttp.RequestLog{
			Provider:    "gemini",
			Model:       c.model,
			Timestamp:   startTime,
			PromptChars: len(prompt),
			APIKey:      c.apiKey,
		})
	}

	// Record request metric
	if c.metrics != nil {
		c.metrics.RecordRequest("gemini", c.model)
	}

	// Determine system instruction: nil=use default, empty=""=disable, non-empty=override
	var sysInstrContent *Content
	if options.SystemInstruction == nil {
		// Use default system instruction
		sysInstrContent = &Content{
			Parts: []Part{{Text: systemInstruction}},
		}
	} else if *options.SystemInstruction != "" {
		// Use provided override
		sysInstrContent = &Content{
			Parts: []Part{{Text: *options.SystemInstruction}},
		}
	}
	// If options.SystemInstruction is empty string pointer, sysInstrContent stays nil (disabled)

	// Build request
	reqBody := GenerateContentRequest{
		SystemInstruction: sysInstrContent,
		Contents: []Content{
			{
				Parts: []Part{
					{Text: prompt},
				},
			},
		},
	}

	// Add generation config if options provided
	if options.Temperature > 0 || options.MaxTokens > 0 {
		reqBody.GenerationConfig = &GenerationConfig{}
		if options.Temperature > 0 {
			reqBody.GenerationConfig.Temperature = options.Temperature
		}
		if options.MaxTokens > 0 {
			reqBody.GenerationConfig.MaxOutputTokens = options.MaxTokens
		}
		reqBody.GenerationConfig.CandidateCount = 1
	}

	// Add default safety settings (block only high severity)
	reqBody.SafetySettings = []SafetySetting{
		{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "BLOCK_ONLY_HIGH"},
		{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_ONLY_HIGH"},
		{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_ONLY_HIGH"},
		{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "BLOCK_ONLY_HIGH"},
	}

	// Marshal request
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create URL with API key
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)

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
				Provider:  "gemini",
			}
		}

		retryReq.Header.Set("Content-Type", "application/json")

		var callErr error
		resp, callErr = c.client.Do(retryReq)
		if callErr != nil {
			// Use helper that correctly marks timeouts as retryable
			return llmhttp.NewTimeoutError("gemini", callErr.Error())
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
					Provider:   "gemini",
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
				c.metrics.RecordError("gemini", c.model, httpErr.Type)
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

	var genResp GenerateContentResponse
	if err := json.Unmarshal(bodyBytes, &genResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Validate response
	if len(genResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	candidate := genResp.Candidates[0]

	// Check for content filtering
	if candidate.FinishReason == "SAFETY" {
		return nil, &llmhttp.Error{
			Type:      llmhttp.ErrTypeContentFiltered,
			Message:   "Content blocked by safety filters",
			Retryable: false,
			Provider:  "gemini",
		}
	}

	// Extract text from parts
	var textParts []string
	for _, part := range candidate.Content.Parts {
		textParts = append(textParts, part.Text)
	}

	responseText := strings.Join(textParts, "")

	// Log if we got an empty response for debugging
	if c.logger != nil && responseText == "" {
		c.logger.LogWarning(ctx, "Gemini returned empty response", map[string]interface{}{
			"finishReason":        candidate.FinishReason,
			"numParts":            len(candidate.Content.Parts),
			"numCandidates":       len(genResp.Candidates),
			"tokensOut":           genResp.UsageMetadata.CandidatesTokenCount,
			"responsePreview":     llmhttp.SafeLogResponse(string(bodyBytes)),
			"responseLengthBytes": len(bodyBytes),
		})
	}

	response := &APIResponse{
		Text:         responseText,
		TokensIn:     genResp.UsageMetadata.PromptTokenCount,
		TokensOut:    genResp.UsageMetadata.CandidatesTokenCount,
		FinishReason: candidate.FinishReason,
	}

	// Calculate cost
	var cost float64
	if c.pricing != nil {
		cost = c.pricing.GetCost("gemini", c.model, response.TokensIn, response.TokensOut)
		response.Cost = cost
	}

	// Log response
	if c.logger != nil {
		c.logger.LogResponse(ctx, llmhttp.ResponseLog{
			Provider:     "gemini",
			Model:        c.model,
			Timestamp:    time.Now(),
			Duration:     duration,
			TokensIn:     response.TokensIn,
			TokensOut:    response.TokensOut,
			Cost:         cost,
			StatusCode:   200,
			FinishReason: response.FinishReason,
		})
	}

	// Record metrics
	if c.metrics != nil {
		c.metrics.RecordDuration("gemini", c.model, duration)
		c.metrics.RecordTokens("gemini", c.model, response.TokensIn, response.TokensOut)
		c.metrics.RecordCost("gemini", c.model, cost)
	}

	return response, nil
}

// handleErrorResponse maps HTTP status codes to typed errors.
func (c *HTTPClient) handleErrorResponse(statusCode int, body []byte) error {
	// Try to parse Gemini error format
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
			Provider:   "gemini",
		}
	case http.StatusTooManyRequests:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeRateLimit,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  true,
			Provider:   "gemini",
		}
	case http.StatusBadRequest:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeInvalidRequest,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "gemini",
		}
	case http.StatusServiceUnavailable, http.StatusInternalServerError:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeServiceUnavailable,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  true,
			Provider:   "gemini",
		}
	default:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeUnknown,
			Message:    message,
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "gemini",
		}
	}
}

// CreateReview implements the Client interface for the Provider.
func (c *HTTPClient) CreateReview(ctx context.Context, req Request) (llm.ProviderResponse, error) {
	apiResp, err := c.Call(ctx, req.Prompt, CallOptions{
		MaxTokens: req.MaxTokens,
	})
	if err != nil {
		return llm.ProviderResponse{}, fmt.Errorf("gemini: %w", err)
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
		// Log the parsing failure to help with debugging
		if c.logger != nil {
			c.logger.LogWarning(ctx, "Gemini JSON parsing failed, returning raw text as summary", map[string]interface{}{
				"error":           err.Error(),
				"responseLength":  len(apiResp.Text),
				"responsePreview": llmhttp.SafeLogResponse(apiResp.Text),
			})
		}
		// If JSON parsing fails, return text as summary
		return llm.ProviderResponse{
			Model:    c.model,
			Summary:  apiResp.Text,
			Findings: []domain.Finding{},
			Usage:    usage,
		}, nil
	}

	review.Model = c.model
	review.Usage = usage
	return review, nil
}

// parseReviewJSON extracts and parses the JSON review from the response text.
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
