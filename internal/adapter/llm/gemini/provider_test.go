package gemini_test

import (
	"context"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/llm"
	"github.com/delightfulhammers/bop/internal/adapter/llm/gemini"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubClient struct {
	requests []gemini.Request
	response llm.ProviderResponse
	err      error
}

func (s *stubClient) CreateReview(ctx context.Context, req gemini.Request) (llm.ProviderResponse, error) {
	s.requests = append(s.requests, req)
	return s.response, s.err
}

func TestProvider_Review(t *testing.T) {
	t.Run("forwards request to client correctly", func(t *testing.T) {
		client := &stubClient{
			response: llm.ProviderResponse{
				Model:   "gemini-pro",
				Summary: "Test summary",
				Findings: []domain.Finding{
					{ID: "id1", File: "main.go", LineStart: 1, LineEnd: 5, Severity: "medium", Category: "performance"},
				},
				Usage: llm.UsageMetadata{TokensIn: 100, TokensOut: 50, Cost: 0.005},
			},
		}

		provider := gemini.NewProvider("gemini-pro", client)

		reviewData, err := provider.Review(context.Background(), review.ProviderRequest{
			Prompt:  "review this code",
			Seed:    123,
			MaxSize: 8192,
		})

		require.NoError(t, err)
		require.Len(t, client.requests, 1)

		assert.Equal(t, uint64(123), client.requests[0].Seed)
		assert.Equal(t, "review this code", client.requests[0].Prompt)
		assert.Equal(t, "gemini-pro", client.requests[0].Model)
		assert.Equal(t, 8192, client.requests[0].MaxTokens)

		assert.Equal(t, "gemini", reviewData.ProviderName)
		assert.Equal(t, "gemini-pro", reviewData.ModelName)
		assert.Equal(t, "Test summary", reviewData.Summary)
		assert.Len(t, reviewData.Findings, 1)
	})

	t.Run("returns error when client is nil", func(t *testing.T) {
		provider := gemini.NewProvider("gemini-pro", nil)

		_, err := provider.Review(context.Background(), review.ProviderRequest{
			Prompt: "test",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "gemini client missing")
	})

	t.Run("propagates client errors", func(t *testing.T) {
		client := &stubClient{
			err: assert.AnError,
		}

		provider := gemini.NewProvider("gemini-pro", client)

		_, err := provider.Review(context.Background(), review.ProviderRequest{
			Prompt: "test",
		})

		assert.Error(t, err)
	})
}
