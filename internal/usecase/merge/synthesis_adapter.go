package merge

import (
	"context"

	"github.com/delightfulhammers/bop/internal/domain"
)

// ProviderRequest is a minimal struct matching review.ProviderRequest.
// Duplicated here to avoid circular dependency.
type ProviderRequest struct {
	Prompt  string
	Seed    uint64
	MaxSize int
}

// ReviewProvider defines the interface for code review providers.
// This matches review.Provider but is defined here to avoid circular dependencies.
type ReviewProvider interface {
	Review(ctx context.Context, req ProviderRequest) (domain.Review, error)
}

// SynthesisAdapter adapts a review.Provider to the SynthesisProvider interface.
type SynthesisAdapter struct {
	provider ReviewProvider
}

// NewSynthesisAdapter creates a new synthesis adapter.
func NewSynthesisAdapter(provider ReviewProvider) *SynthesisAdapter {
	return &SynthesisAdapter{provider: provider}
}

// Review implements SynthesisProvider by calling the underlying provider.
func (a *SynthesisAdapter) Review(ctx context.Context, prompt string, seed uint64) (string, error) {
	// Create a request with the synthesis prompt
	req := ProviderRequest{
		Prompt:  prompt,
		Seed:    seed,
		MaxSize: 2000, // Synthesis summaries should be relatively short
	}

	review, err := a.provider.Review(ctx, req)
	if err != nil {
		return "", err
	}

	// Return the summary from the review
	return review.Summary, nil
}
