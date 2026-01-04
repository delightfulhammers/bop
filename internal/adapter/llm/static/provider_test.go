package static

import (
	"context"
	"testing"

	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/stretchr/testify/assert"
)

func TestProvider_Review(t *testing.T) {
	// Given
	ctx := context.Background()
	provider := NewProvider("static-model")
	req := review.ProviderRequest{
		Prompt:  "test prompt",
		Seed:    12345,
		MaxSize: 1024,
	}

	// When
	review, err := provider.Review(ctx, req)

	// Then
	assert.NoError(t, err)
	assert.Equal(t, providerName, review.ProviderName)
	assert.Equal(t, "static-model", review.ModelName)
	assert.Equal(t, "This is a static review from a mock provider.", review.Summary)
	assert.Len(t, review.Findings, 1)

	finding := review.Findings[0]
	assert.Equal(t, "internal/adapter/llm/static/provider.go", finding.File)
	assert.Equal(t, 1, finding.LineStart)
	assert.Equal(t, 5, finding.LineEnd)
	assert.Equal(t, "low", finding.Severity)
	assert.Equal(t, "style", finding.Category)
	assert.Equal(t, "This is a static finding.", finding.Description)
	assert.Equal(t, "No suggestion.", finding.Suggestion)
	assert.True(t, finding.Evidence)
}
