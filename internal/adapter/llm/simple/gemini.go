package simple

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/delightfulhammers/bop/internal/config"
)

const (
	geminiBaseURL     = "https://generativelanguage.googleapis.com"
	geminiTimeout     = 120 * time.Second
	geminiMaxResponse = 10 * 1024 * 1024 // 10MB
)

// GeminiClient implements Client using the Google Gemini API.
type GeminiClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
	retryConf  llmhttp.RetryConfig
}

// NewGeminiClient creates a new Gemini client.
func NewGeminiClient(apiKey, model string, providerCfg config.ProviderConfig, httpCfg config.HTTPConfig) *GeminiClient {
	timeout := llmhttp.ParseTimeout(providerCfg.Timeout, httpCfg.Timeout, geminiTimeout)
	retryConf := llmhttp.BuildRetryConfig(providerCfg, httpCfg)

	return &GeminiClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryConf: retryConf,
	}
}

// Call implements Client.
func (c *GeminiClient) Call(ctx context.Context, prompt string, maxTokens int) (string, error) {
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

func (c *GeminiClient) doRequest(ctx context.Context, prompt string, maxTokens int) (string, error) {
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{{Text: prompt}},
			},
		},
		GenerationConfig: &geminiGenConfig{
			MaxOutputTokens: maxTokens,
			CandidateCount:  1,
		},
		SafetySettings: []geminiSafetySetting{
			{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "BLOCK_ONLY_HIGH"},
			{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_ONLY_HIGH"},
			{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_ONLY_HIGH"},
			{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "BLOCK_ONLY_HIGH"},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", geminiBaseURL, c.model, url.QueryEscape(c.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", classifyHTTPError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	limitedReader := io.LimitReader(resp.Body, geminiMaxResponse)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check if response was truncated
	if int64(len(body)) >= geminiMaxResponse {
		return "", fmt.Errorf("response exceeded maximum size of %d bytes", geminiMaxResponse)
	}

	if resp.StatusCode != http.StatusOK {
		return "", mapGeminiError(resp.StatusCode, body)
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return apiResp.Candidates[0].Content.Parts[0].Text, nil
}

type geminiRequest struct {
	Contents         []geminiContent       `json:"contents"`
	GenerationConfig *geminiGenConfig      `json:"generationConfig,omitempty"`
	SafetySettings   []geminiSafetySetting `json:"safetySettings,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
	CandidateCount  int `json:"candidateCount,omitempty"`
}

type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

func mapGeminiError(statusCode int, body []byte) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return llmhttp.NewAuthenticationError("gemini", string(body))
	case http.StatusTooManyRequests:
		return llmhttp.NewRateLimitError("gemini", string(body))
	case http.StatusBadRequest:
		return llmhttp.NewInvalidRequestError("gemini", string(body))
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return llmhttp.NewServiceUnavailableError("gemini", string(body))
	default:
		return &llmhttp.Error{
			Type:       llmhttp.ErrTypeUnknown,
			Message:    string(body),
			StatusCode: statusCode,
			Retryable:  false,
			Provider:   "gemini",
		}
	}
}
