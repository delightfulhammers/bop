// Package dedup provides LLM-based semantic deduplication of findings.
package dedup

import (
	"context"
	"sync"

	"github.com/delightfulhammers/bop/internal/adapter/llm/simple"
)

// SimpleClientAdapter adapts a simple.Client to the dedup.Client interface.
// This allows the simple LLM clients (Anthropic, OpenAI, Gemini) to be used
// for semantic deduplication, enabling multi-provider support.
//
// The adapter accumulates token usage across all Compare calls, which can be
// retrieved via TotalUsage() for cost accounting.
type SimpleClientAdapter struct {
	client simple.Client

	mu         sync.Mutex
	totalUsage simple.Usage
}

// NewSimpleClientAdapter creates an adapter that wraps a simple.Client
// to satisfy the dedup.Client interface.
func NewSimpleClientAdapter(client simple.Client) *SimpleClientAdapter {
	return &SimpleClientAdapter{client: client}
}

// Compare implements dedup.Client by delegating to simple.Client.Call.
// Token usage is accumulated and can be retrieved via TotalUsage().
func (a *SimpleClientAdapter) Compare(ctx context.Context, prompt string, maxTokens int) (string, error) {
	text, usage, err := a.client.Call(ctx, prompt, maxTokens)
	if err != nil {
		return "", err
	}

	// Accumulate usage for cost accounting
	a.mu.Lock()
	a.totalUsage.InputTokens += usage.InputTokens
	a.totalUsage.OutputTokens += usage.OutputTokens
	a.mu.Unlock()

	return text, nil
}

// TotalUsage returns the accumulated token usage across all Compare calls.
// This is used for cost accounting after deduplication completes.
// Implements the UsageProvider interface for the Comparer.
func (a *SimpleClientAdapter) TotalUsage() Usage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Usage{
		InputTokens:  a.totalUsage.InputTokens,
		OutputTokens: a.totalUsage.OutputTokens,
	}
}

// ResetUsage clears the accumulated token usage.
// Useful when the adapter is reused across multiple reviews.
// Implements the UsageProvider interface for the Comparer.
func (a *SimpleClientAdapter) ResetUsage() {
	a.mu.Lock()
	a.totalUsage = simple.Usage{}
	a.mu.Unlock()
}
