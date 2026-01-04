package http

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/delightfulhammers/bop/internal/domain"
)

var (
	// Compile regex once and reuse (thread-safe)
	// Updated to handle nested code blocks: match from ```json (or ```) at start
	// to the LAST ``` in the text (greedy match), not the first
	jsonBlockRegex = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*)```")
)

// ExtractJSONFromMarkdown extracts JSON from markdown code blocks.
//
// Supports both ```json and ``` code blocks. Uses greedy matching to extract
// content from the first opening backticks to the LAST closing backticks.
//
// This greedy approach is necessary to handle nested code blocks within JSON
// content. For example, when LLM suggestions contain example code like:
//
//	"suggestion": "Use this code:\n\n```go\nfunc main() {}\n```"
//
// The greedy regex correctly extracts the entire JSON block by matching to the
// outermost closing backticks, not the inner ones from the code example.
//
// Assumption: LLMs are instructed to return a single JSON code block. If multiple
// separate code blocks are present, the greedy match will include all content
// between the first and last backticks, which may result in invalid JSON.
// This trade-off is acceptable for the typical LLM response patterns we observe.
//
// Returns extracted JSON or original text if no code block found.
func ExtractJSONFromMarkdown(text string) string {
	matches := jsonBlockRegex.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	// No code block found, return original text (might be raw JSON)
	return strings.TrimSpace(text)
}

// flexibleFinding is an intermediate struct that accepts both camelCase and snake_case.
// LLMs sometimes ignore schema instructions, so we handle both formats.
type flexibleFinding struct {
	File        string `json:"file"`
	LineStart   int    `json:"lineStart"`
	LineEnd     int    `json:"lineEnd"`
	LineStartSC int    `json:"line_start"` // snake_case fallback
	LineEndSC   int    `json:"line_end"`   // snake_case fallback
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
	Evidence    bool   `json:"evidence"`
}

// toFinding converts a flexibleFinding to a domain.Finding, preferring camelCase.
// Uses domain.NewFinding to generate deterministic IDs for proper deduplication.
// This is intentional: using domain.Finding{} resulted in empty IDs causing all
// findings to collapse during deduplication. The deterministic ID is computed
// from content, making the change backward-compatible for merge logic.
func (f flexibleFinding) toFinding() domain.Finding {
	lineStart := f.LineStart
	if lineStart == 0 && f.LineStartSC != 0 {
		lineStart = f.LineStartSC
	}
	lineEnd := f.LineEnd
	if lineEnd == 0 && f.LineEndSC != 0 {
		lineEnd = f.LineEndSC
	}
	return domain.NewFinding(domain.FindingInput{
		File:        f.File,
		LineStart:   lineStart,
		LineEnd:     lineEnd,
		Severity:    f.Severity,
		Category:    f.Category,
		Description: f.Description,
		Suggestion:  f.Suggestion,
		Evidence:    f.Evidence,
	})
}

// flexibleResponse handles both string summaries and object summaries.
type flexibleResponse struct {
	Summary       string            `json:"summary"`
	SummaryObject json.RawMessage   `json:"-"` // For detecting object summaries
	Findings      []flexibleFinding `json:"findings"`
}

// UnmarshalJSON handles the case where summary might be an object instead of string.
func (r *flexibleResponse) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal with summary as string
	type Alias flexibleResponse
	var alias Alias
	if err := json.Unmarshal(data, &alias); err == nil && alias.Summary != "" {
		*r = flexibleResponse(alias)
		return nil
	}

	// Summary might be an object; parse findings and synthesize summary
	var raw struct {
		Summary  json.RawMessage   `json:"summary"`
		Findings []flexibleFinding `json:"findings"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Findings = raw.Findings

	// Try to extract summary as string
	var summaryStr string
	if err := json.Unmarshal(raw.Summary, &summaryStr); err == nil {
		r.Summary = summaryStr
		return nil
	}

	// Summary is an object; generate a summary from findings count
	r.Summary = fmt.Sprintf("Code review completed with %d finding(s).", len(r.Findings))
	return nil
}

// ParseReviewResponse parses JSON into a structured review response.
// Handles both markdown-wrapped and raw JSON responses.
// Accepts both camelCase and snake_case field names for robustness.
func ParseReviewResponse(text string) (summary string, findings []domain.Finding, err error) {
	// Extract JSON from markdown if present
	jsonText := ExtractJSONFromMarkdown(text)

	// Parse into flexible intermediate structure
	var result flexibleResponse
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		return "", nil, fmt.Errorf("failed to parse JSON review: %w", err)
	}

	// Convert flexible findings to domain findings
	domainFindings := make([]domain.Finding, len(result.Findings))
	for i, f := range result.Findings {
		domainFindings[i] = f.toFinding()
	}

	return result.Summary, domainFindings, nil
}
