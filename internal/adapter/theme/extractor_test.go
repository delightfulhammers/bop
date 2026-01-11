package theme

import (
	"context"
	"errors"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/llm/simple"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockClient is a test double for the simple.Client interface.
type mockClient struct {
	response string
	usage    simple.Usage
	err      error
	called   bool
	prompt   string
}

func (m *mockClient) Call(ctx context.Context, prompt string, maxTokens int) (string, simple.Usage, error) {
	m.called = true
	m.prompt = prompt
	return m.response, m.usage, m.err
}

func TestExtractor_ExtractThemes_Success(t *testing.T) {
	client := &mockClient{
		response: `{
			"themes": [
				"input validation",
				"error handling",
				"sql injection prevention"
			]
		}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategyAbstract,
		MaxThemes:           10,
		MinFindingsForTheme: 2,
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "api/handler.go", Category: "security", Description: "Missing input validation for payload size"},
		{File: "api/auth.go", Category: "security", Description: "Validate JWT token claims before processing"},
		{File: "db/repo.go", Category: "bug", Description: "Use parameterized queries to prevent SQL injection"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.True(t, client.called)
	assert.Equal(t, []string{"input validation", "error handling", "sql injection prevention"}, result.Themes)
	assert.Equal(t, review.StrategyAbstract, result.Strategy)
}

func TestExtractor_ExtractThemes_TooFewFindings(t *testing.T) {
	client := &mockClient{}

	config := review.ThemeExtractionConfig{
		MaxThemes:           10,
		MinFindingsForTheme: 3, // Requires at least 3 findings
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "api/handler.go", Category: "security", Description: "Missing validation"},
		{File: "api/auth.go", Category: "security", Description: "JWT issues"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.False(t, client.called, "LLM should not be called when too few findings")
	assert.True(t, result.IsEmpty())
	assert.Equal(t, 2, result.FindingCount)
}

func TestExtractor_ExtractThemes_LLMError(t *testing.T) {
	client := &mockClient{
		err: errors.New("API rate limit exceeded"),
	}

	config := review.DefaultThemeExtractionConfig()
	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.Error(t, err)
	assert.True(t, result.IsEmpty())
	assert.Contains(t, err.Error(), "rate limit")
}

func TestExtractor_ExtractThemes_InvalidJSON(t *testing.T) {
	client := &mockClient{
		response: "This is not valid JSON",
	}

	config := review.DefaultThemeExtractionConfig()
	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.Error(t, err)
	assert.True(t, result.IsEmpty())
}

func TestExtractor_ExtractThemes_MarkdownCodeBlock(t *testing.T) {
	client := &mockClient{
		response: "Here are the themes:\n```json\n{\"themes\": [\"validation\", \"logging\"]}\n```\n",
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategyAbstract,
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}
	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Equal(t, []string{"validation", "logging"}, result.Themes)
}

func TestExtractor_ExtractThemes_EmptyThemes(t *testing.T) {
	client := &mockClient{
		response: `{"themes": []}`,
	}

	config := review.DefaultThemeExtractionConfig()
	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.True(t, result.IsEmpty())
}

func TestExtractor_ExtractThemes_MaxThemesLimit(t *testing.T) {
	client := &mockClient{
		response: `{"themes": ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"]}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategyAbstract,
		MaxThemes:           5, // Only allow 5 themes
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Len(t, result.Themes, 5)
	assert.Equal(t, []string{"a", "b", "c", "d", "e"}, result.Themes)
}

func TestExtractor_ExtractThemes_NormalizesToLowercase(t *testing.T) {
	client := &mockClient{
		response: `{"themes": ["Input Validation", "ERROR HANDLING", "Sql Injection"]}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategyAbstract,
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}
	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Equal(t, []string{"input validation", "error handling", "sql injection"}, result.Themes)
}

func TestExtractor_ExtractThemes_PromptContainsFindingDetails(t *testing.T) {
	client := &mockClient{
		response: `{"themes": ["validation"]}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategyAbstract,
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}
	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "api/handler.go", Category: "security", Description: "Missing input validation"},
		{File: "db/repo.go", Category: "bug", Description: "SQL injection vulnerability"},
		{File: "service/auth.go", Category: "security", Description: "JWT validation issue"},
	}

	_, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)

	// Verify the prompt contains finding details
	assert.Contains(t, client.prompt, "api/handler.go")
	assert.Contains(t, client.prompt, "Missing input validation")
	assert.Contains(t, client.prompt, "security")
	assert.Contains(t, client.prompt, "db/repo.go")
	assert.Contains(t, client.prompt, "SQL injection vulnerability")
}

// New tests for comprehensive strategy

func TestExtractor_ComprehensiveStrategy_ParsesConclusions(t *testing.T) {
	client := &mockClient{
		response: `{
			"themes": ["response size limits"],
			"conclusions": [
				{
					"theme": "response size limits",
					"conclusion": "Truncation uses >= intentionally as conservative bound",
					"anti_pattern": "Do not suggest changing >= to >"
				}
			],
			"disputed_patterns": []
		}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategyComprehensive,
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Equal(t, review.StrategyComprehensive, result.Strategy)
	assert.Equal(t, []string{"response size limits"}, result.Themes)
	require.Len(t, result.Conclusions, 1)
	assert.Equal(t, "response size limits", result.Conclusions[0].Theme)
	assert.Equal(t, "Truncation uses >= intentionally as conservative bound", result.Conclusions[0].Conclusion)
	assert.Equal(t, "Do not suggest changing >= to >", result.Conclusions[0].AntiPattern)
}

func TestExtractor_ComprehensiveStrategy_ParsesDisputedPatterns(t *testing.T) {
	client := &mockClient{
		response: `{
			"themes": ["error handling"],
			"conclusions": [],
			"disputed_patterns": [
				{
					"pattern": "off-by-one in truncation check",
					"rationale": "LimitReader caps at n bytes, so >= is intentional"
				}
			]
		}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategyComprehensive,
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1", Status: domain.StatusDisputed, StatusReason: "Intentional"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	require.Len(t, result.DisputedPatterns, 1)
	assert.Equal(t, "off-by-one in truncation check", result.DisputedPatterns[0].Pattern)
	assert.Equal(t, "LimitReader caps at n bytes, so >= is intentional", result.DisputedPatterns[0].Rationale)
}

func TestExtractor_ComprehensiveStrategy_ParsesDisputePrinciples(t *testing.T) {
	client := &mockClient{
		response: `{
			"themes": ["trust boundaries"],
			"conclusions": [],
			"disputed_patterns": [],
			"dispute_principles": [
				{
					"principle": "Internal data paths are trusted",
					"applies_to": ["database data", "config files", "LLM responses"],
					"do_not_flag": ["prompt injection", "input validation"],
					"rationale": "These data sources are not user-controlled"
				}
			]
		}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategyComprehensive,
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{
			File:         "a.go",
			Description:  "Prompt injection from database data",
			Status:       domain.StatusDisputed,
			StatusReason: "Data comes from our DB, not user input",
		},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	require.Len(t, result.DisputePrinciples, 1)
	assert.Equal(t, "Internal data paths are trusted", result.DisputePrinciples[0].Principle)
	assert.Equal(t, []string{"database data", "config files", "llm responses"}, result.DisputePrinciples[0].AppliesTo)
	assert.Equal(t, []string{"prompt injection", "input validation"}, result.DisputePrinciples[0].DoNotFlag)
	assert.Equal(t, "These data sources are not user-controlled", result.DisputePrinciples[0].Rationale)
}

func TestExtractor_ComprehensiveStrategy_PrincipleLimitsEnforced(t *testing.T) {
	// Response with more principles than the limit
	client := &mockClient{
		response: `{
			"themes": [],
			"conclusions": [],
			"disputed_patterns": [],
			"dispute_principles": [
				{"principle": "P1", "applies_to": ["a"], "do_not_flag": ["x"], "rationale": "R1"},
				{"principle": "P2", "applies_to": ["b"], "do_not_flag": ["y"], "rationale": "R2"},
				{"principle": "P3", "applies_to": ["c"], "do_not_flag": ["z"], "rationale": "R3"},
				{"principle": "P4", "applies_to": ["d"], "do_not_flag": ["w"], "rationale": "R4"}
			]
		}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:             review.StrategyComprehensive,
		MaxThemes:            10,
		MinFindingsForTheme:  3,
		MaxTokens:            4096,
		MaxDisputePrinciples: 2, // Only allow 2
		MaxPrincipleItems:    5,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Len(t, result.DisputePrinciples, 2, "should be limited to MaxDisputePrinciples")
	assert.Equal(t, "P1", result.DisputePrinciples[0].Principle)
	assert.Equal(t, "P2", result.DisputePrinciples[1].Principle)
}

func TestExtractor_ComprehensiveStrategy_PrincipleItemsLimited(t *testing.T) {
	client := &mockClient{
		response: `{
			"themes": [],
			"conclusions": [],
			"disputed_patterns": [],
			"dispute_principles": [
				{
					"principle": "Test",
					"applies_to": ["a", "b", "c", "d", "e", "f", "g"],
					"do_not_flag": ["x", "y", "z"],
					"rationale": "Reason"
				}
			]
		}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:             review.StrategyComprehensive,
		MaxThemes:            10,
		MinFindingsForTheme:  3,
		MaxTokens:            4096,
		MaxDisputePrinciples: 5,
		MaxPrincipleItems:    3, // Only allow 3 items per array
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	require.Len(t, result.DisputePrinciples, 1)
	// Verify correct count
	assert.Len(t, result.DisputePrinciples[0].AppliesTo, 3, "should be limited to MaxPrincipleItems")
	assert.Len(t, result.DisputePrinciples[0].DoNotFlag, 3, "should be limited to MaxPrincipleItems")
	// Verify first N items are preserved in order (lowercased by sanitizeStringSlice)
	assert.Equal(t, []string{"a", "b", "c"}, result.DisputePrinciples[0].AppliesTo, "should keep first 3 items")
	assert.Equal(t, []string{"x", "y", "z"}, result.DisputePrinciples[0].DoNotFlag, "should keep all 3 items")
}

func TestExtractor_ComprehensiveStrategy_SkipsEmptyOrIncomplete(t *testing.T) {
	client := &mockClient{
		response: `{
			"themes": [],
			"conclusions": [],
			"disputed_patterns": [],
			"dispute_principles": [
				{"principle": "", "applies_to": ["a"], "do_not_flag": ["x"], "rationale": "R1"},
				{"principle": "Valid", "applies_to": ["a"], "do_not_flag": ["x"], "rationale": ""},
				{"principle": "NoArrays", "applies_to": [], "do_not_flag": [], "rationale": "R3"},
				{"principle": "Good", "applies_to": ["a"], "do_not_flag": [], "rationale": "R4"}
			]
		}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:             review.StrategyComprehensive,
		MaxThemes:            10,
		MinFindingsForTheme:  3,
		MaxTokens:            4096,
		MaxDisputePrinciples: 10,
		MaxPrincipleItems:    10,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	// Only "Good" should pass: has principle, rationale, and at least one array
	require.Len(t, result.DisputePrinciples, 1)
	assert.Equal(t, "Good", result.DisputePrinciples[0].Principle)
}

func TestExtractor_ComprehensiveStrategy_DeduplicatesItems(t *testing.T) {
	client := &mockClient{
		response: `{
			"themes": [],
			"conclusions": [],
			"disputed_patterns": [],
			"dispute_principles": [
				{
					"principle": "Test",
					"applies_to": ["Database Data", "database data", "DATABASE DATA", "config"],
					"do_not_flag": ["SQL", "sql", "SQL"],
					"rationale": "Reason"
				}
			]
		}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:             review.StrategyComprehensive,
		MaxThemes:            10,
		MinFindingsForTheme:  3,
		MaxTokens:            4096,
		MaxDisputePrinciples: 5,
		MaxPrincipleItems:    10,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	require.Len(t, result.DisputePrinciples, 1)
	// After lowercase normalization and dedup: "database data", "config"
	assert.Len(t, result.DisputePrinciples[0].AppliesTo, 2, "duplicates should be removed")
	assert.Equal(t, []string{"database data", "config"}, result.DisputePrinciples[0].AppliesTo)
	// After lowercase normalization and dedup: "sql"
	assert.Len(t, result.DisputePrinciples[0].DoNotFlag, 1, "duplicates should be removed")
	assert.Equal(t, []string{"sql"}, result.DisputePrinciples[0].DoNotFlag)
}

func TestExtractor_ComprehensiveStrategy_PromptIncludesDisputedFindings(t *testing.T) {
	client := &mockClient{
		response: `{"themes": [], "conclusions": [], "disputed_patterns": []}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategyComprehensive,
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{
			File:         "a.go",
			Description:  "Off-by-one error",
			Status:       domain.StatusDisputed,
			StatusReason: "This is intentional conservative bound",
		},
		{File: "b.go", Description: "Issue 2", Status: domain.StatusAcknowledged},
		{File: "c.go", Description: "Issue 3"},
	}

	_, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)

	// Verify the prompt contains dispute information
	assert.Contains(t, client.prompt, "DISPUTED")
	assert.Contains(t, client.prompt, "Off-by-one error")
	assert.Contains(t, client.prompt, "This is intentional conservative bound")
	assert.Contains(t, client.prompt, "Acknowledged")
}

func TestExtractor_SpecificStrategy_ParsesConclusionsOnly(t *testing.T) {
	client := &mockClient{
		response: `{
			"themes": ["api security"],
			"conclusions": [
				{
					"theme": "api security",
					"conclusion": "API key via header is secure",
					"anti_pattern": "Do not suggest query param"
				}
			]
		}`,
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategySpecific,
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Equal(t, review.StrategySpecific, result.Strategy)
	assert.Equal(t, []string{"api security"}, result.Themes)
	require.Len(t, result.Conclusions, 1)
	// Specific strategy doesn't parse disputed_patterns
	assert.Empty(t, result.DisputedPatterns)
}

func TestExtractor_DefaultsToComprehensiveStrategy(t *testing.T) {
	client := &mockClient{
		response: `{"themes": ["test"], "conclusions": [], "disputed_patterns": []}`,
	}

	// No strategy specified - should default to comprehensive
	config := review.ThemeExtractionConfig{
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Equal(t, review.StrategyComprehensive, result.Strategy)
}

func TestExtractor_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		result   review.ThemeExtractionResult
		expected bool
	}{
		{
			name:     "all empty",
			result:   review.ThemeExtractionResult{},
			expected: true,
		},
		{
			name: "has themes",
			result: review.ThemeExtractionResult{
				Themes: []string{"test"},
			},
			expected: false,
		},
		{
			name: "has conclusions",
			result: review.ThemeExtractionResult{
				Conclusions: []review.ThemeConclusion{{Conclusion: "test"}},
			},
			expected: false,
		},
		{
			name: "has disputed patterns",
			result: review.ThemeExtractionResult{
				DisputedPatterns: []review.DisputedPattern{{Pattern: "test"}},
			},
			expected: false,
		},
		{
			name: "has dispute principles",
			result: review.ThemeExtractionResult{
				DisputePrinciples: []review.DisputePrinciple{{Principle: "test", AppliesTo: []string{"data"}}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.IsEmpty())
		})
	}
}

func TestExtractor_ExtractThemes_CapturesUsage(t *testing.T) {
	client := &mockClient{
		response: `{"themes": ["validation"]}`,
		usage:    simple.Usage{InputTokens: 500, OutputTokens: 100},
	}

	config := review.ThemeExtractionConfig{
		Strategy:            review.StrategyAbstract,
		MaxThemes:           10,
		MinFindingsForTheme: 3,
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Equal(t, 500, result.TokensIn)
	assert.Equal(t, 100, result.TokensOut)
}

func TestExtractor_ExtractThemes_TooFewFindings_NoUsage(t *testing.T) {
	client := &mockClient{
		usage: simple.Usage{InputTokens: 500, OutputTokens: 100},
	}

	config := review.ThemeExtractionConfig{
		MaxThemes:           10,
		MinFindingsForTheme: 5, // Require 5, only provide 2
		MaxTokens:           4096,
	}

	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
	}

	result, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.False(t, client.called, "LLM should not be called when too few findings")
	// No LLM call means no usage
	assert.Equal(t, 0, result.TokensIn)
	assert.Equal(t, 0, result.TokensOut)
}
