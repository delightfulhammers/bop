package llm

import "github.com/delightfulhammers/bop/internal/domain"

// UsageMetadata captures token usage and cost information from LLM API calls.
// This metadata flows alongside the content through the adapter layer.
type UsageMetadata struct {
	TokensIn  int     // Input tokens consumed
	TokensOut int     // Output tokens generated
	Cost      float64 // Cost in USD
}

// ProviderResponse is the standardized response from any LLM provider.
// All provider clients (openai, anthropic, gemini, ollama) return this type,
// eliminating duplication of Response structs across providers.
type ProviderResponse struct {
	Model    string
	Summary  string
	Findings []domain.Finding
	Usage    UsageMetadata
}
