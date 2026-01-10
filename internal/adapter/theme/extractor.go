// Package theme provides LLM-based theme extraction from code review findings.
package theme

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/delightfulhammers/bop/internal/adapter/llm/simple"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// Extractor implements theme extraction using an LLM.
type Extractor struct {
	client simple.Client
	config review.ThemeExtractionConfig
}

// NewExtractor creates a new theme extractor with the given client and config.
func NewExtractor(client simple.Client, config review.ThemeExtractionConfig) *Extractor {
	return &Extractor{
		client: client,
		config: config,
	}
}

// ExtractThemes implements review.ThemeExtractor.
// It analyzes prior findings and returns a list of high-level themes.
func (e *Extractor) ExtractThemes(ctx context.Context, findings []domain.TriagedFinding) ([]string, error) {
	// Skip if not enough findings to form themes
	if len(findings) < e.config.MinFindingsForTheme {
		return nil, nil
	}

	// Build the prompt
	prompt := buildExtractionPrompt(findings, e.config.MaxThemes)

	// Call the LLM
	response, err := e.client.Call(ctx, prompt, e.config.MaxTokens)
	if err != nil {
		log.Printf("warning: theme extraction LLM call failed: %v", err)
		return nil, err
	}

	// Parse the response
	themes, err := parseExtractionResponse(response, e.config.MaxThemes)
	if err != nil {
		log.Printf("warning: failed to parse theme extraction response: %v", err)
		return nil, err
	}

	return themes, nil
}

// buildExtractionPrompt creates the prompt for theme extraction.
func buildExtractionPrompt(findings []domain.TriagedFinding, maxThemes int) string {
	var sb strings.Builder

	sb.WriteString(`You are analyzing code review findings to identify recurring THEMES.

A THEME is a high-level conceptual area that multiple findings explore from different angles.
Examples of themes:
- "input validation" (covers payload size checks, type validation, boundary validation)
- "error handling" (covers missing try/catch, error propagation, error logging)
- "SQL injection prevention" (covers parameterized queries, input sanitization)
- "null/nil safety" (covers null checks, optional handling, defensive programming)

Your task: Identify the main THEMES across these findings. Each theme should be a short phrase (2-5 words) that captures the conceptual area.

`)

	sb.WriteString("## Findings to Analyze\n\n")

	for i, f := range findings {
		sb.WriteString(fmt.Sprintf("### Finding %d\n", i+1))
		sb.WriteString(fmt.Sprintf("- File: `%s`\n", f.File))
		sb.WriteString(fmt.Sprintf("- Category: %s\n", f.Category))
		sb.WriteString(fmt.Sprintf("- Description: %s\n\n", f.Description))
	}

	sb.WriteString(fmt.Sprintf(`## Response Format

Extract up to %d themes. Respond with a JSON object:

`+"```json\n"+`{
  "themes": [
    "input validation",
    "error handling patterns",
    "null safety checks"
  ]
}
`+"```\n"+`

Guidelines:
- Each theme should be 2-5 words, in lowercase
- Only include themes that appear in 2+ findings
- Focus on conceptual areas, not specific implementation details
- If no clear themes emerge, return an empty array
`, maxThemes))

	return sb.String()
}

// extractionResponse is the expected JSON structure from the LLM.
type extractionResponse struct {
	Themes []string `json:"themes"`
}

// parseExtractionResponse extracts themes from the LLM response.
func parseExtractionResponse(response string, maxThemes int) ([]string, error) {
	// Extract JSON from response (may be wrapped in markdown code blocks)
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var resp extractionResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Validate and limit themes
	const maxThemeLength = 100 // Defense against overly long themes (prompt injection mitigation)
	var themes []string
	for _, theme := range resp.Themes {
		theme = strings.TrimSpace(theme)
		if theme == "" {
			continue
		}
		// Limit theme length to prevent processing of excessively long strings
		if len(theme) > maxThemeLength {
			theme = theme[:maxThemeLength]
		}
		// Normalize to lowercase
		theme = strings.ToLower(theme)
		themes = append(themes, theme)
		if len(themes) >= maxThemes {
			break
		}
	}

	return themes, nil
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
