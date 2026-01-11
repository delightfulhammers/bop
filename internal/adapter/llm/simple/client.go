// Package simple provides a simplified LLM client interface for auxiliary tasks.
// Unlike the full Provider interface (which handles structured code review),
// this client is for simple prompt→text operations like theme extraction
// and semantic deduplication.
package simple

import (
	"context"
)

// Usage contains token consumption metrics from an LLM call.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Client defines a simple interface for LLM text completion.
// This is used by auxiliary services like theme extraction and semantic dedup
// that don't need the full structured review response format.
type Client interface {
	// Call sends a prompt to the LLM and returns the response text and token usage.
	// maxTokens limits the response size.
	Call(ctx context.Context, prompt string, maxTokens int) (string, Usage, error)
}
