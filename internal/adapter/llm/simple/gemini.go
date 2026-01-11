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
func (c *GeminiClient) Call(ctx context.Context, prompt string, maxTokens int) (string, Usage, error) {
	var result string
	var usage Usage

	operation := func(ctx context.Context) error {
		resp, u, err := c.doRequest(ctx, prompt, maxTokens)
		if err != nil {
			return err
		}
		result = resp
		usage = u
		return nil
	}

	err := llmhttp.RetryWithBackoff(ctx, operation, c.retryConf)
	if err != nil {
		return "", Usage{}, err
	}

	return result, usage, nil
}

func (c *GeminiClient) doRequest(ctx context.Context, prompt string, maxTokens int) (string, Usage, error) {
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
		return "", Usage{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/v1beta/models/%s:generateContent", geminiBaseURL, c.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", Usage{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey) // Use header instead of query param to prevent log leakage

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", Usage{}, classifyHTTPError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	limitedReader := io.LimitReader(resp.Body, geminiMaxResponse)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", Usage{}, fmt.Errorf("failed to read response: %w", err)
	}

	// Check if response was truncated
	if int64(len(body)) >= geminiMaxResponse {
		return "", Usage{}, fmt.Errorf("response exceeded maximum size of %d bytes", geminiMaxResponse)
	}

	if resp.StatusCode != http.StatusOK {
		return "", Usage{}, mapGeminiError(resp.StatusCode, body)
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", Usage{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(apiResp.Candidates) == 0 || len(apiResp.Candidates[0].Content.Parts) == 0 {
		return "", Usage{}, fmt.Errorf("no content in response")
	}

	// Concatenate all parts (Gemini can return multiple parts for complex responses)
	var sb strings.Builder
	for _, part := range apiResp.Candidates[0].Content.Parts {
		sb.WriteString(part.Text)
	}

	usage := Usage{
		InputTokens:  apiResp.UsageMetadata.PromptTokenCount,
		OutputTokens: apiResp.UsageMetadata.CandidatesTokenCount,
	}

	return sb.String(), usage, nil
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
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
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
