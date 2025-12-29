package triage

import (
	"regexp"
	"strings"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// Regex patterns for extracting suggestions from comments/annotations.
var (
	// githubSuggestionPattern matches GitHub suggestion blocks.
	// Format: ```suggestion\ncode here\n```
	githubSuggestionPattern = regexp.MustCompile("(?s)```suggestion\\s*\\n(.+?)\\n```")

	// genericCodeBlockPattern matches generic fenced code blocks.
	// Format: ```lang\ncode here\n``` or ```\ncode here\n```
	genericCodeBlockPattern = regexp.MustCompile("(?s)```\\w*\\s*\\n(.+?)\\n```")
)

// DefaultSuggestionExtractor implements SuggestionExtractor using regex parsing.
type DefaultSuggestionExtractor struct{}

// NewSuggestionExtractor creates a new DefaultSuggestionExtractor.
func NewSuggestionExtractor() *DefaultSuggestionExtractor {
	return &DefaultSuggestionExtractor{}
}

// ExtractFromAnnotation extracts a suggestion from an annotation message.
// It looks for suggestion blocks in either the Message or RawDetails field.
func (e *DefaultSuggestionExtractor) ExtractFromAnnotation(annotation *domain.Annotation) (*domain.Suggestion, error) {
	if annotation == nil {
		return nil, ErrNoSuggestion
	}

	// Try RawDetails first (SARIF tools often put structured data here)
	if annotation.RawDetails != "" {
		if newCode := e.extractSuggestionCode(annotation.RawDetails); newCode != "" {
			return &domain.Suggestion{
				File:        annotation.Path,
				NewCode:     newCode,
				Explanation: extractExplanation(annotation.Message),
				Source:      "annotation",
			}, nil
		}
	}

	// Fall back to Message field
	if newCode := e.extractSuggestionCode(annotation.Message); newCode != "" {
		return &domain.Suggestion{
			File:        annotation.Path,
			NewCode:     newCode,
			Explanation: extractExplanation(annotation.Message),
			Source:      "annotation",
		}, nil
	}

	return nil, ErrNoSuggestion
}

// ExtractFromComment extracts a suggestion from a PR comment body.
// It looks for GitHub suggestion blocks (```suggestion ... ```) or generic code blocks.
func (e *DefaultSuggestionExtractor) ExtractFromComment(finding *domain.PRFinding) (*domain.Suggestion, error) {
	if finding == nil {
		return nil, ErrNoSuggestion
	}

	newCode := e.extractSuggestionCode(finding.Body)
	if newCode == "" {
		return nil, ErrNoSuggestion
	}

	return &domain.Suggestion{
		File:        finding.Path,
		NewCode:     newCode,
		Explanation: extractExplanation(finding.Body),
		Source:      "comment",
	}, nil
}

// extractSuggestionCode extracts the code from a suggestion block.
// Tries GitHub suggestion syntax first, then generic code blocks.
func (e *DefaultSuggestionExtractor) extractSuggestionCode(text string) string {
	// Try GitHub suggestion block format first
	if matches := githubSuggestionPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}

	// Try generic code block
	if matches := genericCodeBlockPattern.FindStringSubmatch(text); len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}

	return ""
}

// extractExplanation extracts the explanation text from a comment/message.
// Returns the text before the first code block, or the full text if no code block.
func extractExplanation(text string) string {
	// Find the first code block and take everything before it
	idx := strings.Index(text, "```")
	if idx > 0 {
		explanation := strings.TrimSpace(text[:idx])
		// Remove common prefixes like **Severity: high** etc
		lines := strings.Split(explanation, "\n")
		var cleanLines []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Skip metadata lines
			if strings.HasPrefix(line, "**Severity:") ||
				strings.HasPrefix(line, "**Category:") ||
				strings.HasPrefix(line, "CR_FP:") ||
				line == "" {
				continue
			}
			cleanLines = append(cleanLines, line)
		}
		if len(cleanLines) > 0 {
			return strings.Join(cleanLines, "\n")
		}
	}

	// If no code block, just clean metadata from the text
	if !strings.Contains(text, "```") {
		return cleanMetadata(text)
	}

	return ""
}

// cleanMetadata removes common metadata prefixes from text.
func cleanMetadata(text string) string {
	lines := strings.Split(text, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "**Severity:") ||
			strings.HasPrefix(line, "**Category:") ||
			strings.HasPrefix(line, "CR_FP:") {
			continue
		}
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}
	return strings.Join(cleanLines, "\n")
}
