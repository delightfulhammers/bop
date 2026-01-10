// Package review provides code review orchestration and related utilities.
package review

import (
	"context"

	"github.com/delightfulhammers/bop/internal/domain"
)

// ThemeExtractor extracts high-level themes from prior findings.
// Themes are abstract concepts like "input validation", "error handling",
// "SQL injection concerns" that may manifest as multiple specific findings.
//
// By extracting and presenting themes (rather than just individual findings),
// we help the LLM recognize when it's exploring the same conceptual area
// from different angles, reducing thematic duplication in reviews.
type ThemeExtractor interface {
	// ExtractThemes analyzes prior findings and returns a list of themes.
	// Each theme is a short descriptive phrase (e.g., "payload size validation").
	//
	// If extraction fails, returns nil and an error. Callers should handle
	// this gracefully by proceeding without theme context.
	ExtractThemes(ctx context.Context, findings []domain.TriagedFinding) ([]string, error)
}

// ThemeExtractionResult contains the extracted themes and metadata.
type ThemeExtractionResult struct {
	// Themes is the list of extracted theme phrases.
	Themes []string

	// FindingCount is the number of findings that were analyzed.
	FindingCount int
}

// ThemeExtractionConfig holds configuration for theme extraction.
type ThemeExtractionConfig struct {
	// MaxThemes limits the number of themes to extract.
	// Default: 10 (enough to cover major areas without overwhelming the prompt)
	MaxThemes int

	// MinFindingsForTheme is the minimum number of findings required
	// before theme extraction is attempted.
	// Default: 3 (single findings don't form themes)
	MinFindingsForTheme int

	// MaxTokens is the maximum output tokens for the LLM response.
	// Default: 4096 (themes are short phrases, this is plenty)
	MaxTokens int
}

// DefaultThemeExtractionConfig returns sensible defaults for theme extraction.
func DefaultThemeExtractionConfig() ThemeExtractionConfig {
	return ThemeExtractionConfig{
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}
}
