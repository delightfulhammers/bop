package openai

import (
	"context"
	"fmt"

	"github.com/delightfulhammers/bop/internal/adapter/llm"
	"github.com/delightfulhammers/bop/internal/domain"
)

// StaticClient provides an offline-friendly OpenAI client implementation.
type StaticClient struct{}

// NewStaticClient constructs a stubbed OpenAI client.
func NewStaticClient() *StaticClient {
	return &StaticClient{}
}

// CreateReview returns a deterministic placeholder review.
func (s *StaticClient) CreateReview(ctx context.Context, req Request) (llm.ProviderResponse, error) {
	summary := fmt.Sprintf("Static review for model %s with seed %d over prompt: %.40s", req.Model, req.Seed, req.Prompt)
	return llm.ProviderResponse{
		Model:    req.Model,
		Summary:  summary,
		Findings: []domain.Finding{},
		Usage:    llm.UsageMetadata{}, // Static client has no usage (offline)
	}, nil
}
