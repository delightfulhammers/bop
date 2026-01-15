// Package feedback provides an HTTP client for the platform feedback-service.
package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/google/uuid"

	"github.com/delightfulhammers/bop/internal/auth"
	"github.com/delightfulhammers/platform/contracts/feedback"
)

// ProductID is the product identifier for bop feedback.
const ProductID = "bop"

// Client is an HTTP client for the feedback-service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	tokenStore *auth.TokenStore
	version    string
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithVersion sets the client version for context.
func WithVersion(version string) ClientOption {
	return func(c *Client) {
		c.version = version
	}
}

// NewClient creates a new feedback client.
func NewClient(baseURL string, tokenStore *auth.TokenStore, opts ...ClientOption) *Client {
	c := &Client{
		baseURL:    baseURL,
		tokenStore: tokenStore,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SubmitRequest contains the data needed to submit feedback.
type SubmitRequest struct {
	Category    feedback.Category
	Title       string
	Description string
	ClientType  feedback.ClientType
}

// SubmitResponse is the response from submitting feedback.
type SubmitResponse struct {
	ID      uuid.UUID       `json:"id"`
	Status  feedback.Status `json:"status"`
	Message string          `json:"message"`
}

// Submit sends feedback to the feedback-service.
func (c *Client) Submit(ctx context.Context, req SubmitRequest) (*SubmitResponse, error) {
	stored, err := c.tokenStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load auth: %w", err)
	}

	if stored.IsExpired() {
		return nil, fmt.Errorf("authentication expired - run 'bop auth login' to re-authenticate")
	}

	// Build the request body
	body := feedback.SubmitFeedbackRequest{
		ProductID:     ProductID,
		Category:      req.Category,
		Title:         req.Title,
		Description:   req.Description,
		ClientType:    req.ClientType,
		ClientVersion: c.version,
		Context: &feedback.Context{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/feedback", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+stored.AccessToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		var errResp errorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return nil, fmt.Errorf("submit feedback failed: %s", errResp.Message)
		}
		return nil, fmt.Errorf("submit feedback failed: HTTP %d", resp.StatusCode)
	}

	var result SubmitResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result, nil
}

// ListResponse is the response from listing feedback.
type ListResponse struct {
	Feedback []*feedback.Feedback `json:"feedback"`
	Total    int                  `json:"total"`
	HasMore  bool                 `json:"has_more"`
}

// List retrieves the user's own feedback.
func (c *Client) List(ctx context.Context) (*ListResponse, error) {
	stored, err := c.tokenStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load auth: %w", err)
	}

	if stored.IsExpired() {
		return nil, fmt.Errorf("authentication expired - run 'bop auth login' to re-authenticate")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/feedback", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+stored.AccessToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return nil, fmt.Errorf("list feedback failed: %s", errResp.Message)
		}
		return nil, fmt.Errorf("list feedback failed: HTTP %d", resp.StatusCode)
	}

	var result ListResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result, nil
}

// Get retrieves a specific feedback item by ID.
func (c *Client) Get(ctx context.Context, id uuid.UUID) (*feedback.Feedback, error) {
	stored, err := c.tokenStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load auth: %w", err)
	}

	if stored.IsExpired() {
		return nil, fmt.Errorf("authentication expired - run 'bop auth login' to re-authenticate")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/feedback/"+id.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+stored.AccessToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("feedback not found")
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return nil, fmt.Errorf("get feedback failed: %s", errResp.Message)
		}
		return nil, fmt.Errorf("get feedback failed: HTTP %d", resp.StatusCode)
	}

	var result feedback.Feedback
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &result, nil
}

// errorResponse represents an API error response.
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
