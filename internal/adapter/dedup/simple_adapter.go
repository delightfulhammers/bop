// Package dedup provides LLM-based semantic deduplication of findings.
package dedup

import (
	"context"

	"github.com/delightfulhammers/bop/internal/adapter/llm/simple"
)

// SimpleClientAdapter adapts a simple.Client to the dedup.Client interface.
// This allows the simple LLM clients (Anthropic, OpenAI, Gemini) to be used
// for semantic deduplication, enabling multi-provider support.
type SimpleClientAdapter struct {
	client simple.Client
}

// NewSimpleClientAdapter creates an adapter that wraps a simple.Client
// to satisfy the dedup.Client interface.
func NewSimpleClientAdapter(client simple.Client) *SimpleClientAdapter {
	return &SimpleClientAdapter{client: client}
}

// Compare implements dedup.Client by delegating to simple.Client.Call.
// The method signatures are identical, only the names differ.
func (a *SimpleClientAdapter) Compare(ctx context.Context, prompt string, maxTokens int) (string, error) {
	return a.client.Call(ctx, prompt, maxTokens)
}
