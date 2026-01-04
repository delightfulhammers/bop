package ollama_test

import (
	"context"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/llm"
	"github.com/delightfulhammers/bop/internal/adapter/llm/ollama"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubClient struct {
	requests []ollama.Request
	response llm.ProviderResponse
	err      error
}

func (s *stubClient) CreateReview(ctx context.Context, req ollama.Request) (llm.ProviderResponse, error) {
	s.requests = append(s.requests, req)
	return s.response, s.err
}

func TestProvider_Review(t *testing.T) {
	t.Run("forwards request to client correctly", func(t *testing.T) {
		client := &stubClient{
			response: llm.ProviderResponse{
				Model:   "llama2",
				Summary: "Test summary",
				Findings: []domain.Finding{
					{ID: "id1", File: "main.go", LineStart: 1, LineEnd: 5, Severity: "low", Category: "style"},
				},
				Usage: llm.UsageMetadata{TokensIn: 100, TokensOut: 50, Cost: 0.0}, // Ollama is free
			},
		}

		provider := ollama.NewProvider("llama2", client)

		reviewData, err := provider.Review(context.Background(), review.ProviderRequest{
			Prompt:  "review this code",
			Seed:    456,
			MaxSize: 2048,
		})

		require.NoError(t, err)
		require.Len(t, client.requests, 1)

		assert.Equal(t, uint64(456), client.requests[0].Seed)
		assert.Equal(t, "review this code", client.requests[0].Prompt)
		assert.Equal(t, "llama2", client.requests[0].Model)
		assert.Equal(t, 2048, client.requests[0].MaxTokens)

		assert.Equal(t, "ollama", reviewData.ProviderName)
		assert.Equal(t, "llama2", reviewData.ModelName)
		assert.Equal(t, "Test summary", reviewData.Summary)
		assert.Len(t, reviewData.Findings, 1)
	})

	t.Run("returns error when client is nil", func(t *testing.T) {
		provider := ollama.NewProvider("llama2", nil)

		_, err := provider.Review(context.Background(), review.ProviderRequest{
			Prompt: "test",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ollama client missing")
	})

	t.Run("propagates client errors", func(t *testing.T) {
		client := &stubClient{
			err: assert.AnError,
		}

		provider := ollama.NewProvider("llama2", client)

		_, err := provider.Review(context.Background(), review.ProviderRequest{
			Prompt: "test",
		})

		assert.Error(t, err)
	})
}
