// Package dedup provides LLM-based semantic deduplication of findings.
package dedup

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/dedup"
)

// Client defines the interface for making LLM calls for semantic comparison.
// This allows different LLM providers to be used interchangeably.
type Client interface {
	// Compare sends a prompt to the LLM and returns the response text.
	Compare(ctx context.Context, prompt string, maxTokens int) (string, error)
}

// UsageProvider is an optional interface for clients that track token usage.
// If the client implements this interface, usage can be retrieved for cost accounting.
type UsageProvider interface {
	// TotalUsage returns the accumulated token usage.
	TotalUsage() Usage
	// ResetUsage clears the accumulated usage.
	ResetUsage()
}

// Usage contains token consumption metrics.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Comparer implements semantic comparison using an LLM.
type Comparer struct {
	client    Client
	maxTokens int
}

// NewComparer creates a new semantic comparer with the given client.
func NewComparer(client Client, maxTokens int) *Comparer {
	return &Comparer{
		client:    client,
		maxTokens: maxTokens,
	}
}

// TotalUsage returns the accumulated token usage if the client tracks it.
// Returns zero usage if the client doesn't implement UsageProvider.
// This implements dedup.UsageProvider for cost accounting.
func (c *Comparer) TotalUsage() dedup.Usage {
	if up, ok := c.client.(UsageProvider); ok {
		u := up.TotalUsage()
		return dedup.Usage{
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
		}
	}
	return dedup.Usage{}
}

// ResetUsage clears the accumulated token usage if the client tracks it.
// This implements dedup.UsageProvider for cost accounting.
func (c *Comparer) ResetUsage() {
	if up, ok := c.client.(UsageProvider); ok {
		up.ResetUsage()
	}
}

// Compare implements dedup.SemanticComparer.
// It batches all candidates into a single LLM call and parses the structured response.
func (c *Comparer) Compare(ctx context.Context, candidates []dedup.CandidatePair) (*dedup.ComparisonResult, error) {
	if len(candidates) == 0 {
		return &dedup.ComparisonResult{}, nil
	}

	// Build the prompt
	prompt := buildComparisonPrompt(candidates)

	// Call the LLM
	response, err := c.client.Compare(ctx, prompt, c.maxTokens)
	if err != nil {
		log.Printf("warning: semantic dedup LLM call failed: %v (treating all as unique)", err)
		// Fail open: return all new findings as unique
		return failOpen(candidates), nil
	}

	// Parse the response
	result, err := parseComparisonResponse(response, candidates)
	if err != nil {
		log.Printf("warning: failed to parse semantic dedup response: %v (treating all as unique)", err)
		return failOpen(candidates), nil
	}

	return result, nil
}

// buildComparisonPrompt creates the prompt for semantic comparison.
func buildComparisonPrompt(candidates []dedup.CandidatePair) string {
	var sb strings.Builder

	sb.WriteString(`You are analyzing code review findings to identify semantic duplicates.

Two findings are DUPLICATES if they describe the SAME underlying issue, even if worded differently.
Two findings are NOT duplicates if they describe different issues, even if they're on the same code.

For each candidate pair, determine if the NEW finding is a semantic duplicate of the EXISTING finding.

`)

	sb.WriteString("## Candidate Pairs\n\n")

	for i, cp := range candidates {
		sb.WriteString(fmt.Sprintf("### Pair %d\n\n", i))

		sb.WriteString("**EXISTING finding:**\n")
		sb.WriteString(fmt.Sprintf("- File: `%s`\n", cp.Existing.File))
		sb.WriteString(fmt.Sprintf("- Lines: %d-%d\n", cp.Existing.LineStart, cp.Existing.LineEnd))
		sb.WriteString(fmt.Sprintf("- Severity: %s\n", cp.Existing.Severity))
		sb.WriteString(fmt.Sprintf("- Category: %s\n", cp.Existing.Category))
		sb.WriteString(fmt.Sprintf("- Description: %s\n\n", cp.Existing.Description))

		sb.WriteString("**NEW finding:**\n")
		sb.WriteString(fmt.Sprintf("- File: `%s`\n", cp.New.File))
		sb.WriteString(fmt.Sprintf("- Lines: %d-%d\n", cp.New.LineStart, cp.New.LineEnd))
		sb.WriteString(fmt.Sprintf("- Severity: %s\n", cp.New.Severity))
		sb.WriteString(fmt.Sprintf("- Category: %s\n", cp.New.Category))
		sb.WriteString(fmt.Sprintf("- Description: %s\n\n", cp.New.Description))
	}

	sb.WriteString(`## Response Format

Respond with a JSON object containing your analysis:

` + "```json\n" + `{
  "comparisons": [
    {
      "pair_index": 0,
      "is_duplicate": true,
      "reason": "Both describe the same error handling gap in the comment fetching logic"
    },
    {
      "pair_index": 1,
      "is_duplicate": false,
      "reason": "First is about performance, second is about correctness - different issues"
    }
  ]
}
` + "```\n" + `
Include one entry per pair in the same order as the input. Be concise in reasons (1 sentence).
`)

	return sb.String()
}

// comparisonResponse is the expected JSON structure from the LLM.
type comparisonResponse struct {
	Comparisons []comparison `json:"comparisons"`
}

type comparison struct {
	PairIndex   int    `json:"pair_index"`
	IsDuplicate bool   `json:"is_duplicate"`
	Reason      string `json:"reason"`
}

// parseComparisonResponse extracts duplicate matches from the LLM response.
func parseComparisonResponse(response string, candidates []dedup.CandidatePair) (*dedup.ComparisonResult, error) {
	// Extract JSON from response (may be wrapped in markdown code blocks)
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var resp comparisonResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	result := &dedup.ComparisonResult{}

	// Track which new findings are duplicates
	duplicateIndices := make(map[int]bool)

	for _, comp := range resp.Comparisons {
		if comp.PairIndex < 0 || comp.PairIndex >= len(candidates) {
			continue // Invalid index, skip
		}

		cp := candidates[comp.PairIndex]

		if comp.IsDuplicate {
			result.Duplicates = append(result.Duplicates, dedup.DuplicateMatch{
				NewFinding:          cp.New,
				ExistingFingerprint: cp.Existing.Fingerprint,
				Reason:              comp.Reason,
			})
			duplicateIndices[comp.PairIndex] = true
		}
	}

	// Collect unique findings (those not marked as duplicates)
	seen := make(map[string]bool)
	for i, cp := range candidates {
		if duplicateIndices[i] {
			continue
		}
		// Key by file+description to avoid duplicates in the unique list
		key := cp.New.File + "|" + cp.New.Description
		if !seen[key] {
			result.Unique = append(result.Unique, cp.New)
			seen[key] = true
		}
	}

	return result, nil
}

// extractJSON attempts to extract JSON from a response that may contain markdown.
func extractJSON(response string) string {
	// Try to find JSON in code blocks first
	start := strings.Index(response, "```json")
	if start != -1 {
		start += 7 // Skip "```json"
		end := strings.Index(response[start:], "```")
		if end != -1 {
			return strings.TrimSpace(response[start : start+end])
		}
	}

	// Try plain code blocks
	start = strings.Index(response, "```")
	if start != -1 {
		start += 3
		end := strings.Index(response[start:], "```")
		if end != -1 {
			return strings.TrimSpace(response[start : start+end])
		}
	}

	// Try to find raw JSON (starts with {)
	start = strings.Index(response, "{")
	if start != -1 {
		// Find matching closing brace
		depth := 0
		for i := start; i < len(response); i++ {
			switch response[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return response[start : i+1]
				}
			}
		}
	}

	return ""
}

// failOpen returns a result where all new findings are treated as unique.
// This is used when the LLM call fails to avoid silently dropping valid feedback.
func failOpen(candidates []dedup.CandidatePair) *dedup.ComparisonResult {
	seen := make(map[string]bool)
	var unique []domain.Finding

	for _, cp := range candidates {
		key := cp.New.File + "|" + cp.New.Description
		if !seen[key] {
			unique = append(unique, cp.New)
			seen[key] = true
		}
	}

	return &dedup.ComparisonResult{
		Unique: unique,
	}
}
