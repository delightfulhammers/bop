package theme

import (
	"context"
	"errors"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockClient is a test double for the simple.Client interface.
type mockClient struct {
	response string
	err      error
	called   bool
	prompt   string
}

func (m *mockClient) Call(ctx context.Context, prompt string, maxTokens int) (string, error) {
	m.called = true
	m.prompt = prompt
	return m.response, m.err
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

	themes, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.True(t, client.called)
	assert.Equal(t, []string{"input validation", "error handling", "sql injection prevention"}, themes)
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

	themes, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.False(t, client.called, "LLM should not be called when too few findings")
	assert.Nil(t, themes)
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

	themes, err := extractor.ExtractThemes(context.Background(), findings)

	require.Error(t, err)
	assert.Nil(t, themes)
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

	themes, err := extractor.ExtractThemes(context.Background(), findings)

	require.Error(t, err)
	assert.Nil(t, themes)
}

func TestExtractor_ExtractThemes_MarkdownCodeBlock(t *testing.T) {
	client := &mockClient{
		response: "Here are the themes:\n```json\n{\"themes\": [\"validation\", \"logging\"]}\n```\n",
	}

	config := review.DefaultThemeExtractionConfig()
	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	themes, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Equal(t, []string{"validation", "logging"}, themes)
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

	themes, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Empty(t, themes)
}

func TestExtractor_ExtractThemes_MaxThemesLimit(t *testing.T) {
	client := &mockClient{
		response: `{"themes": ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"]}`,
	}

	config := review.ThemeExtractionConfig{
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

	themes, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Len(t, themes, 5)
	assert.Equal(t, []string{"a", "b", "c", "d", "e"}, themes)
}

func TestExtractor_ExtractThemes_NormalizesToLowercase(t *testing.T) {
	client := &mockClient{
		response: `{"themes": ["Input Validation", "ERROR HANDLING", "Sql Injection"]}`,
	}

	config := review.DefaultThemeExtractionConfig()
	extractor := NewExtractor(client, config)

	findings := []domain.TriagedFinding{
		{File: "a.go", Description: "Issue 1"},
		{File: "b.go", Description: "Issue 2"},
		{File: "c.go", Description: "Issue 3"},
	}

	themes, err := extractor.ExtractThemes(context.Background(), findings)

	require.NoError(t, err)
	assert.Equal(t, []string{"input validation", "error handling", "sql injection"}, themes)
}

func TestExtractor_ExtractThemes_PromptContainsFindingDetails(t *testing.T) {
	client := &mockClient{
		response: `{"themes": ["validation"]}`,
	}

	config := review.DefaultThemeExtractionConfig()
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
