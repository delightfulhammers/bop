// Package review provides code review orchestration and related utilities.
package review

import (
	"context"

	"github.com/delightfulhammers/bop/internal/domain"
)

// ExtractionStrategy defines the approach for theme extraction.
// Different strategies provide varying levels of specificity.
type ExtractionStrategy string

const (
	// StrategyAbstract extracts only high-level themes (current behavior).
	// Example: "response size limits", "api key security"
	StrategyAbstract ExtractionStrategy = "abstract"

	// StrategySpecific extracts themes with specific conclusions and anti-patterns.
	// Example: "response size limits" + "truncation uses >= intentionally"
	StrategySpecific ExtractionStrategy = "specific"

	// StrategyComprehensive extracts themes, conclusions, AND disputed patterns.
	// This is the most effective at preventing repeat findings.
	// Default strategy.
	StrategyComprehensive ExtractionStrategy = "comprehensive"
)

// ThemeExtractor extracts high-level themes from prior findings.
// Themes are abstract concepts like "input validation", "error handling",
// "SQL injection concerns" that may manifest as multiple specific findings.
//
// By extracting and presenting themes (rather than just individual findings),
// we help the LLM recognize when it's exploring the same conceptual area
// from different angles, reducing thematic duplication in reviews.
type ThemeExtractor interface {
	// ExtractThemes analyzes prior findings and returns extraction results.
	// The result includes themes, conclusions, and disputed patterns depending
	// on the configured strategy.
	//
	// If extraction fails, returns an empty result and an error. Callers should
	// handle this gracefully by proceeding without theme context.
	ExtractThemes(ctx context.Context, findings []domain.TriagedFinding) (ThemeExtractionResult, error)
}

// ThemeExtractionResult contains extracted themes and related context.
// The populated fields depend on the extraction strategy used.
type ThemeExtractionResult struct {
	// Themes is the list of high-level theme phrases.
	// Example: ["response size limits", "api key security"]
	Themes []string

	// Conclusions contains specific decisions derived from findings.
	// Only populated with "specific" or "comprehensive" strategies.
	Conclusions []ThemeConclusion

	// DisputedPatterns contains patterns from disputed findings.
	// Only populated with "comprehensive" strategy.
	// These are derived from dispute rationales to prevent re-raising.
	DisputedPatterns []DisputedPattern

	// Strategy indicates which extraction strategy was used.
	Strategy ExtractionStrategy

	// FindingCount is the number of findings that were analyzed.
	FindingCount int
}

// ThemeConclusion represents a specific decision made during review.
// These are more concrete than themes and help prevent contradictory findings.
type ThemeConclusion struct {
	// Theme is the high-level theme this conclusion relates to.
	Theme string

	// Conclusion is the specific decision or observation.
	// Example: "Truncation uses >= intentionally as conservative bound"
	Conclusion string

	// AntiPattern describes what NOT to suggest based on this conclusion.
	// Example: "Do not suggest changing >= to >"
	AntiPattern string
}

// DisputedPattern represents a pattern that was disputed in prior reviews.
// The rationale explains WHY it was disputed, helping prevent re-raising.
type DisputedPattern struct {
	// Pattern is a short description of the disputed finding pattern.
	// Example: "off-by-one in truncation check"
	Pattern string

	// Rationale explains why this pattern was disputed.
	// Example: "LimitReader caps at n bytes, so >= is intentional conservative bound"
	Rationale string
}

// IsEmpty returns true if no themes or patterns were extracted.
func (r ThemeExtractionResult) IsEmpty() bool {
	return len(r.Themes) == 0 && len(r.Conclusions) == 0 && len(r.DisputedPatterns) == 0
}

// ThemeExtractionConfig holds configuration for theme extraction.
type ThemeExtractionConfig struct {
	// Strategy determines the extraction approach.
	// Default: "comprehensive" (most effective at preventing repeats)
	Strategy ExtractionStrategy

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
		Strategy:            StrategyComprehensive,
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}
}
