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
	client   simple.Client
	config   review.ThemeExtractionConfig
	strategy review.ExtractionStrategy
}

// NewExtractor creates a new theme extractor with the given client and config.
func NewExtractor(client simple.Client, config review.ThemeExtractionConfig) *Extractor {
	// Default to comprehensive strategy if not specified
	strategy := config.Strategy
	if strategy == "" {
		strategy = review.StrategyComprehensive
	}

	return &Extractor{
		client:   client,
		config:   config,
		strategy: strategy,
	}
}

// ExtractThemes implements review.ThemeExtractor.
// It analyzes prior findings and returns themes, conclusions, and disputed patterns
// based on the configured extraction strategy.
func (e *Extractor) ExtractThemes(ctx context.Context, findings []domain.TriagedFinding) (review.ThemeExtractionResult, error) {
	result := review.ThemeExtractionResult{
		Strategy:     e.strategy,
		FindingCount: len(findings),
	}

	// Skip if not enough findings to form themes
	if len(findings) < e.config.MinFindingsForTheme {
		return result, nil
	}

	// Build the prompt based on strategy
	prompt := e.buildExtractionPrompt(findings)

	// Call the LLM
	response, err := e.client.Call(ctx, prompt, e.config.MaxTokens)
	if err != nil {
		log.Printf("warning: theme extraction LLM call failed: %v", err)
		return result, err
	}

	// Parse the response based on strategy
	parsed, err := e.parseExtractionResponse(response)
	if err != nil {
		log.Printf("warning: failed to parse theme extraction response: %v", err)
		return result, err
	}

	return parsed, nil
}

// buildExtractionPrompt creates the prompt for theme extraction based on strategy.
func (e *Extractor) buildExtractionPrompt(findings []domain.TriagedFinding) string {
	switch e.strategy {
	case review.StrategyAbstract:
		return buildAbstractPrompt(findings, e.config.MaxThemes)
	case review.StrategySpecific:
		return buildSpecificPrompt(findings, e.config.MaxThemes)
	case review.StrategyComprehensive:
		return buildComprehensivePrompt(findings, e.config.MaxThemes)
	default:
		// Default to comprehensive for unknown strategies
		return buildComprehensivePrompt(findings, e.config.MaxThemes)
	}
}

// buildAbstractPrompt creates the original abstract-themes-only prompt.
func buildAbstractPrompt(findings []domain.TriagedFinding, maxThemes int) string {
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

// buildSpecificPrompt creates a prompt that extracts themes with specific conclusions.
func buildSpecificPrompt(findings []domain.TriagedFinding, maxThemes int) string {
	var sb strings.Builder

	sb.WriteString(`You are analyzing code review findings to identify THEMES and extract SPECIFIC CONCLUSIONS.

A THEME is a high-level conceptual area (e.g., "response size limits", "api key security").
A CONCLUSION is a specific decision or observation from the review (e.g., "truncation check uses >= intentionally").

Your task: Identify themes AND the specific conclusions that have been reached about them.

`)

	sb.WriteString("## Findings to Analyze\n\n")
	for i, f := range findings {
		sb.WriteString(fmt.Sprintf("### Finding %d\n", i+1))
		sb.WriteString(fmt.Sprintf("- File: `%s`\n", f.File))
		sb.WriteString(fmt.Sprintf("- Category: %s\n", f.Category))
		sb.WriteString(fmt.Sprintf("- Status: %s\n", f.Status))
		sb.WriteString(fmt.Sprintf("- Description: %s\n", f.Description))
		if f.StatusReason != "" {
			sb.WriteString(fmt.Sprintf("- Rationale: %s\n", f.StatusReason))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf(`## Response Format

Respond with a JSON object containing themes and conclusions:

`+"```json\n"+`{
  "themes": ["response size limits", "api key security"],
  "conclusions": [
    {
      "theme": "response size limits",
      "conclusion": "Truncation check uses >= intentionally as conservative bound",
      "anti_pattern": "Do not suggest changing >= to >"
    }
  ]
}
`+"```\n"+`

Guidelines:
- Extract up to %d themes (2-5 words each, lowercase)
- For each theme with a clear decision, add a conclusion
- anti_pattern describes what NOT to suggest based on this conclusion
- Focus on conclusions that would prevent repeat findings
`, maxThemes))

	return sb.String()
}

// buildComprehensivePrompt creates the full prompt including disputed pattern extraction.
func buildComprehensivePrompt(findings []domain.TriagedFinding, maxThemes int) string {
	var sb strings.Builder

	sb.WriteString(`You are analyzing code review findings to prevent REPEAT FINDINGS in future reviews.

Your task is to extract THREE types of context:
1. THEMES: High-level conceptual areas (e.g., "response size limits")
2. CONCLUSIONS: Specific decisions that have been made (e.g., "truncation uses >= intentionally")
3. DISPUTED PATTERNS: Findings that were disputed with rationales explaining WHY

This context will be injected into future review prompts to prevent the same concerns from being raised again.

`)

	// Separate findings by status to highlight disputed ones
	var disputed, acknowledged, other []domain.TriagedFinding
	for _, f := range findings {
		switch strings.ToLower(string(f.Status)) {
		case "disputed":
			disputed = append(disputed, f)
		case "acknowledged":
			acknowledged = append(acknowledged, f)
		default:
			other = append(other, f)
		}
	}

	sb.WriteString("## Findings to Analyze\n\n")

	// Show disputed findings first with emphasis
	if len(disputed) > 0 {
		sb.WriteString("### DISPUTED Findings (IMPORTANT - these should NOT be re-raised)\n\n")
		for i, f := range disputed {
			sb.WriteString(fmt.Sprintf("%d. **%s** in `%s`\n", i+1, f.Category, f.File))
			sb.WriteString(fmt.Sprintf("   - Description: %s\n", f.Description))
			if f.StatusReason != "" {
				sb.WriteString(fmt.Sprintf("   - DISPUTE RATIONALE: %s\n", f.StatusReason))
			}
			sb.WriteString("\n")
		}
	}

	// Show acknowledged findings
	if len(acknowledged) > 0 {
		sb.WriteString("### Acknowledged Findings\n\n")
		for i, f := range acknowledged {
			sb.WriteString(fmt.Sprintf("%d. **%s** in `%s`\n", i+1, f.Category, f.File))
			sb.WriteString(fmt.Sprintf("   - Description: %s\n", f.Description))
			if f.StatusReason != "" {
				sb.WriteString(fmt.Sprintf("   - Note: %s\n", f.StatusReason))
			}
			sb.WriteString("\n")
		}
	}

	// Show other findings
	if len(other) > 0 {
		sb.WriteString("### Other Findings\n\n")
		for i, f := range other {
			sb.WriteString(fmt.Sprintf("%d. **%s** in `%s`\n", i+1, f.Category, f.File))
			sb.WriteString(fmt.Sprintf("   - Description: %s\n", f.Description))
			sb.WriteString("\n")
		}
	}

	sb.WriteString(fmt.Sprintf(`## Response Format

Respond with a JSON object:

`+"```json\n"+`{
  "themes": ["response size limits", "api key security"],
  "conclusions": [
    {
      "theme": "response size limits",
      "conclusion": "Truncation check uses >= intentionally as conservative bound",
      "anti_pattern": "Do not suggest changing >= to >"
    }
  ],
  "disputed_patterns": [
    {
      "pattern": "off-by-one in truncation check",
      "rationale": "LimitReader caps at n bytes, so >= is intentional conservative bound"
    }
  ]
}
`+"```\n"+`

Guidelines:
- Extract up to %d themes (2-5 words each, lowercase)
- For disputed findings, create disputed_patterns entries using the DISPUTE RATIONALE
- disputed_patterns are CRITICAL - they prevent the exact same concern from being raised
- conclusions capture decisions that should not be second-guessed
- Be specific: vague patterns like "code quality" won't prevent repeats
`, maxThemes))

	return sb.String()
}

// parseExtractionResponse parses the LLM response based on strategy.
func (e *Extractor) parseExtractionResponse(response string) (review.ThemeExtractionResult, error) {
	result := review.ThemeExtractionResult{
		Strategy:     e.strategy,
		FindingCount: 0, // Will be set by caller
	}

	// Extract JSON from response (may be wrapped in markdown code blocks)
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return result, fmt.Errorf("no JSON found in response")
	}

	switch e.strategy {
	case review.StrategyAbstract:
		return parseAbstractResponse(jsonStr, e.config.MaxThemes, e.strategy)
	case review.StrategySpecific:
		return parseSpecificResponse(jsonStr, e.config.MaxThemes, e.strategy)
	case review.StrategyComprehensive:
		return parseComprehensiveResponse(jsonStr, e.config.MaxThemes, e.strategy)
	default:
		return parseComprehensiveResponse(jsonStr, e.config.MaxThemes, e.strategy)
	}
}

// abstractResponse is the expected JSON structure for abstract strategy.
type abstractResponse struct {
	Themes []string `json:"themes"`
}

// specificResponse is the expected JSON structure for specific strategy.
type specificResponse struct {
	Themes      []string             `json:"themes"`
	Conclusions []conclusionResponse `json:"conclusions"`
}

type conclusionResponse struct {
	Theme       string `json:"theme"`
	Conclusion  string `json:"conclusion"`
	AntiPattern string `json:"anti_pattern"`
}

// comprehensiveResponse is the expected JSON structure for comprehensive strategy.
type comprehensiveResponse struct {
	Themes           []string             `json:"themes"`
	Conclusions      []conclusionResponse `json:"conclusions"`
	DisputedPatterns []patternResponse    `json:"disputed_patterns"`
}

type patternResponse struct {
	Pattern   string `json:"pattern"`
	Rationale string `json:"rationale"`
}

// Length limits for validation (defense against prompt injection)
const (
	maxThemeLength      = 100
	maxConclusionLength = 300
	maxRationaleLength  = 500
)

// parseAbstractResponse parses themes-only response.
func parseAbstractResponse(jsonStr string, maxThemes int, strategy review.ExtractionStrategy) (review.ThemeExtractionResult, error) {
	result := review.ThemeExtractionResult{Strategy: strategy}

	var resp abstractResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return result, fmt.Errorf("failed to parse JSON: %w", err)
	}

	result.Themes = sanitizeThemes(resp.Themes, maxThemes)
	return result, nil
}

// parseSpecificResponse parses themes + conclusions response.
func parseSpecificResponse(jsonStr string, maxThemes int, strategy review.ExtractionStrategy) (review.ThemeExtractionResult, error) {
	result := review.ThemeExtractionResult{Strategy: strategy}

	var resp specificResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return result, fmt.Errorf("failed to parse JSON: %w", err)
	}

	result.Themes = sanitizeThemes(resp.Themes, maxThemes)
	result.Conclusions = sanitizeConclusions(resp.Conclusions)
	return result, nil
}

// parseComprehensiveResponse parses full response with disputed patterns.
func parseComprehensiveResponse(jsonStr string, maxThemes int, strategy review.ExtractionStrategy) (review.ThemeExtractionResult, error) {
	result := review.ThemeExtractionResult{Strategy: strategy}

	var resp comprehensiveResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return result, fmt.Errorf("failed to parse JSON: %w", err)
	}

	result.Themes = sanitizeThemes(resp.Themes, maxThemes)
	result.Conclusions = sanitizeConclusions(resp.Conclusions)
	result.DisputedPatterns = sanitizePatterns(resp.DisputedPatterns)
	return result, nil
}

// sanitizeThemes validates and limits themes.
func sanitizeThemes(themes []string, maxThemes int) []string {
	var result []string
	for _, theme := range themes {
		theme = strings.TrimSpace(theme)
		if theme == "" {
			continue
		}
		// UTF-8 safe truncation
		runes := []rune(theme)
		if len(runes) > maxThemeLength {
			theme = string(runes[:maxThemeLength])
		}
		// Normalize to lowercase
		theme = strings.ToLower(theme)
		result = append(result, theme)
		if len(result) >= maxThemes {
			break
		}
	}
	return result
}

// sanitizeConclusions validates and limits conclusions.
func sanitizeConclusions(conclusions []conclusionResponse) []review.ThemeConclusion {
	var result []review.ThemeConclusion
	for _, c := range conclusions {
		tc := review.ThemeConclusion{
			Theme:       truncateRunes(strings.TrimSpace(c.Theme), maxThemeLength),
			Conclusion:  truncateRunes(strings.TrimSpace(c.Conclusion), maxConclusionLength),
			AntiPattern: truncateRunes(strings.TrimSpace(c.AntiPattern), maxConclusionLength),
		}
		// Skip empty conclusions
		if tc.Conclusion == "" {
			continue
		}
		result = append(result, tc)
	}
	return result
}

// sanitizePatterns validates and limits disputed patterns.
func sanitizePatterns(patterns []patternResponse) []review.DisputedPattern {
	var result []review.DisputedPattern
	for _, p := range patterns {
		dp := review.DisputedPattern{
			Pattern:   truncateRunes(strings.TrimSpace(p.Pattern), maxConclusionLength),
			Rationale: truncateRunes(strings.TrimSpace(p.Rationale), maxRationaleLength),
		}
		// Skip empty patterns
		if dp.Pattern == "" {
			continue
		}
		result = append(result, dp)
	}
	return result
}

// truncateRunes truncates a string to maxLen runes (UTF-8 safe).
func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return s
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
