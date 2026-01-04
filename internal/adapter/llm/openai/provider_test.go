package openai_test

import (
	"context"
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/llm"
	"github.com/delightfulhammers/bop/internal/adapter/llm/openai"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

type stubClient struct {
	requests []openai.Request
	response llm.ProviderResponse
	err      error
}

func (s *stubClient) CreateReview(ctx context.Context, req openai.Request) (llm.ProviderResponse, error) {
	s.requests = append(s.requests, req)
	return s.response, s.err
}

func TestProviderReview(t *testing.T) {
	client := &stubClient{
		response: llm.ProviderResponse{
			Summary: "summary",
			Findings: []domain.Finding{
				{ID: "id", File: "main.go", LineStart: 1, LineEnd: 1, Severity: "low", Category: "style"},
			},
			Usage: llm.UsageMetadata{TokensIn: 100, TokensOut: 50, Cost: 0.01},
		},
	}

	provider := openai.NewProvider("gpt-4o", client)

	reviewData, err := provider.Review(context.Background(), review.ProviderRequest{
		Prompt:  "prompt",
		Seed:    42,
		MaxSize: 4096,
	})
	if err != nil {
		t.Fatalf("provider returned error: %v", err)
	}

	if len(client.requests) != 1 {
		t.Fatalf("expected single API call, got %d", len(client.requests))
	}

	if client.requests[0].Seed != 42 {
		t.Fatalf("expected seed to be forwarded, got %d", client.requests[0].Seed)
	}

	if reviewData.ProviderName != "openai" {
		t.Fatalf("expected provider name openai, got %s", reviewData.ProviderName)
	}
}

func TestStaticClientProducesDeterministicSummary(t *testing.T) {
	client := openai.NewStaticClient()
	response, err := client.CreateReview(context.Background(), openai.Request{
		Model:  "any",
		Prompt: "diff content",
		Seed:   1,
	})
	if err != nil {
		t.Fatalf("static client returned error: %v", err)
	}

	if !strings.Contains(response.Summary, "diff content") {
		t.Fatalf("expected summary to echo prompt content, got %s", response.Summary)
	}
}
