package review_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/review"
)

// Test fixtures for reviewer personas.
var (
	securityReviewer = domain.Reviewer{
		Name:     "security",
		Provider: "anthropic",
		Model:    "claude-opus-4",
		Weight:   1.5,
		Persona:  "You are a security expert specializing in OWASP vulnerabilities, authentication, and authorization.",
		Focus:    []string{"security", "authentication", "authorization"},
		Ignore:   []string{"style", "documentation"},
		Enabled:  true,
	}

	maintainabilityReviewer = domain.Reviewer{
		Name:     "maintainability",
		Provider: "openai",
		Model:    "gpt-4o",
		Weight:   1.0,
		Persona:  "You are a software architect focused on code maintainability, SOLID principles, and clean code.",
		Focus:    []string{"maintainability", "complexity", "readability"},
		Ignore:   []string{},
		Enabled:  true,
	}

	minimalReviewer = domain.Reviewer{
		Name:     "minimal",
		Provider: "anthropic",
		Model:    "claude-sonnet-4",
		Weight:   1.0,
		Persona:  "",
		Focus:    []string{},
		Ignore:   []string{},
		Enabled:  true,
	}
)

func TestNewPersonaPromptBuilder(t *testing.T) {
	t.Parallel()

	base := review.NewEnhancedPromptBuilder()

	t.Run("creates builder with base builder", func(t *testing.T) {
		builder := review.NewPersonaPromptBuilder(base)
		assert.NotNil(t, builder)
	})

	t.Run("panics with nil base builder", func(t *testing.T) {
		assert.Panics(t, func() {
			review.NewPersonaPromptBuilder(nil)
		})
	})
}

func TestPersonaPromptBuilder_Build(t *testing.T) {
	t.Parallel()

	base := review.NewEnhancedPromptBuilder()
	builder := review.NewPersonaPromptBuilder(base)

	baseDiff := domain.Diff{
		FromCommitHash: "abc123",
		ToCommitHash:   "def456",
		Files: []domain.FileDiff{
			{
				Path:   "auth/handler.go",
				Status: "modified",
				Patch:  "@@ -10,6 +10,8 @@\n func Login(w http.ResponseWriter, r *http.Request) {\n+\tusername := r.FormValue(\"username\")\n+\tpassword := r.FormValue(\"password\")\n }",
			},
		},
	}

	baseContext := review.ProjectContext{
		Architecture: "Clean architecture with domain, usecase, adapter layers",
		README:       "# Test Project",
	}

	baseReq := review.BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature/auth",
	}

	t.Run("injects persona at prompt start", func(t *testing.T) {
		t.Parallel()

		result, err := builder.Build(baseContext, baseDiff, baseReq, securityReviewer)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Prompt)

		// Persona should appear before the standard prompt content
		personaIdx := strings.Index(result.Prompt, securityReviewer.Persona)
		codeChangesIdx := strings.Index(result.Prompt, "Code Changes to Review")

		assert.Greater(t, personaIdx, -1, "persona should be in prompt")
		assert.Greater(t, codeChangesIdx, -1, "code changes section should be in prompt")
		assert.Less(t, personaIdx, codeChangesIdx, "persona should come before code changes")
	})

	t.Run("adds focus instructions when reviewer has focus", func(t *testing.T) {
		t.Parallel()

		result, err := builder.Build(baseContext, baseDiff, baseReq, securityReviewer)

		require.NoError(t, err)

		// Should contain focus directive
		assert.Contains(t, result.Prompt, "FOCUS")
		assert.Contains(t, result.Prompt, "security")
		assert.Contains(t, result.Prompt, "authentication")
		assert.Contains(t, result.Prompt, "authorization")
	})

	t.Run("adds ignore instructions when reviewer has ignore", func(t *testing.T) {
		t.Parallel()

		result, err := builder.Build(baseContext, baseDiff, baseReq, securityReviewer)

		require.NoError(t, err)

		// Should contain ignore directive
		assert.Contains(t, result.Prompt, "IGNORE")
		assert.Contains(t, result.Prompt, "style")
		assert.Contains(t, result.Prompt, "documentation")
	})

	t.Run("skips persona injection for empty persona", func(t *testing.T) {
		t.Parallel()

		result, err := builder.Build(baseContext, baseDiff, baseReq, minimalReviewer)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Prompt)

		// Should still have base prompt content
		assert.Contains(t, result.Prompt, "Code Changes to Review")

		// Should not have empty persona section
		assert.NotContains(t, result.Prompt, "## Reviewer Persona\n\n\n")
	})

	t.Run("skips focus/ignore for reviewer without focus", func(t *testing.T) {
		t.Parallel()

		result, err := builder.Build(baseContext, baseDiff, baseReq, minimalReviewer)

		require.NoError(t, err)

		// Should not contain explicit focus section if no focus defined
		// (base prompt may have different content, so check for persona-specific section)
		assert.NotContains(t, result.Prompt, "## Category Focus")
	})

	t.Run("includes reviewer name in response", func(t *testing.T) {
		t.Parallel()

		result, err := builder.Build(baseContext, baseDiff, baseReq, securityReviewer)

		require.NoError(t, err)
		assert.Equal(t, "security", result.ReviewerName)
	})

	t.Run("uses correct provider from reviewer", func(t *testing.T) {
		t.Parallel()

		// Test with maintainability reviewer that uses OpenAI
		result, err := builder.Build(baseContext, baseDiff, baseReq, maintainabilityReviewer)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Prompt)
		assert.Equal(t, "maintainability", result.ReviewerName)
	})
}

func TestPersonaPromptBuilder_Build_PriorFindingsFiltering(t *testing.T) {
	t.Parallel()

	base := review.NewEnhancedPromptBuilder()
	builder := review.NewPersonaPromptBuilder(base)

	baseDiff := domain.Diff{
		FromCommitHash: "abc123",
		ToCommitHash:   "def456",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "@@ -1 +1 @@\n-old\n+new"},
		},
	}

	baseReq := review.BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature/test",
	}

	t.Run("includes all prior findings regardless of reviewer name", func(t *testing.T) {
		t.Parallel()

		ctx := review.ProjectContext{
			Architecture: "Test architecture",
			TriagedFindings: &domain.TriagedFindingContext{
				PRNumber: 123,
				Findings: []domain.TriagedFinding{
					{
						File:         "auth.go",
						Category:     "security",
						Description:  "SQL injection vulnerability",
						Status:       domain.StatusAcknowledged,
						StatusReason: "Fixed in commit abc",
						ReviewerName: "security",
					},
					{
						File:         "utils.go",
						Category:     "complexity",
						Description:  "Cyclomatic complexity too high",
						Status:       domain.StatusDisputed,
						StatusReason: "Acceptable for this use case",
						ReviewerName: "maintainability",
					},
					{
						File:         "db.go",
						Category:     "security",
						Description:  "Credential exposure risk",
						Status:       domain.StatusAcknowledged,
						StatusReason: "Using env vars now",
						ReviewerName: "security",
					},
				},
			},
		}

		// Security reviewer should see ALL findings (no filtering)
		// This prevents cross-persona duplicates
		result, err := builder.Build(ctx, baseDiff, baseReq, securityReviewer)
		require.NoError(t, err)

		// Should contain security findings
		assert.Contains(t, result.Prompt, "SQL injection vulnerability")
		assert.Contains(t, result.Prompt, "Credential exposure risk")

		// Should ALSO contain maintainability findings (no filtering)
		assert.Contains(t, result.Prompt, "Cyclomatic complexity too high")
	})

	t.Run("all personas see all prior findings to prevent cross-persona duplicates", func(t *testing.T) {
		t.Parallel()

		ctx := review.ProjectContext{
			Architecture: "Test architecture",
			TriagedFindings: &domain.TriagedFindingContext{
				PRNumber: 123,
				Findings: []domain.TriagedFinding{
					{
						File:         "auth.go",
						Category:     "security",
						Description:  "SQL injection",
						Status:       domain.StatusAcknowledged,
						StatusReason: "Fixed",
						ReviewerName: "security",
					},
				},
			},
		}

		// Maintainability reviewer should ALSO see security findings
		// This prevents cross-persona duplicates
		result, err := builder.Build(ctx, baseDiff, baseReq, maintainabilityReviewer)
		require.NoError(t, err)

		// Should contain security findings even though it's a different reviewer
		assert.Contains(t, result.Prompt, "SQL injection")
		// Should have prior findings section
		assert.Contains(t, result.Prompt, "Previously Addressed Concerns")
	})

	t.Run("handles nil triaged findings gracefully", func(t *testing.T) {
		t.Parallel()

		ctx := review.ProjectContext{
			Architecture:    "Test architecture",
			TriagedFindings: nil,
		}

		result, err := builder.Build(ctx, baseDiff, baseReq, securityReviewer)
		require.NoError(t, err)
		assert.NotEmpty(t, result.Prompt)
	})

	t.Run("handles empty triaged findings gracefully", func(t *testing.T) {
		t.Parallel()

		ctx := review.ProjectContext{
			Architecture: "Test architecture",
			TriagedFindings: &domain.TriagedFindingContext{
				PRNumber: 123,
				Findings: []domain.TriagedFinding{},
			},
		}

		result, err := builder.Build(ctx, baseDiff, baseReq, securityReviewer)
		require.NoError(t, err)
		assert.NotEmpty(t, result.Prompt)
	})
}

func TestPersonaPromptBuilder_BuildWithSizeGuards(t *testing.T) {
	t.Parallel()

	base := review.NewEnhancedPromptBuilder()
	builder := review.NewPersonaPromptBuilder(base)

	// Create a mock token estimator
	estimator := &mockTokenEstimator{tokensPerChar: 0.25}

	limits := review.SizeGuardLimits{
		WarnTokens: 50000,
		MaxTokens:  100000,
	}

	baseDiff := domain.Diff{
		FromCommitHash: "abc123",
		ToCommitHash:   "def456",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "@@ -1 +1 @@\n-old\n+new"},
		},
	}

	baseContext := review.ProjectContext{
		Architecture: "Test architecture",
	}

	baseReq := review.BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature/test",
	}

	t.Run("builds with size guards successfully", func(t *testing.T) {
		t.Parallel()

		result, truncResult, err := builder.BuildWithSizeGuards(
			baseContext, baseDiff, baseReq, securityReviewer, estimator, limits,
		)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Prompt)
		assert.Equal(t, "security", result.ReviewerName)
		assert.False(t, truncResult.WasTruncated)
	})

	t.Run("includes persona in size calculation", func(t *testing.T) {
		t.Parallel()

		result, truncResult, err := builder.BuildWithSizeGuards(
			baseContext, baseDiff, baseReq, securityReviewer, estimator, limits,
		)

		require.NoError(t, err)

		// Persona adds tokens, so final count should include persona overhead
		assert.Greater(t, truncResult.FinalTokens, 0)
		assert.Contains(t, result.Prompt, securityReviewer.Persona)
	})

	t.Run("returns error for nil estimator", func(t *testing.T) {
		t.Parallel()

		_, _, err := builder.BuildWithSizeGuards(
			baseContext, baseDiff, baseReq, securityReviewer, nil, limits,
		)

		assert.Error(t, err)
	})

	t.Run("persona overhead increases final token count", func(t *testing.T) {
		t.Parallel()

		// Build without persona
		minimalBuilder := review.NewPersonaPromptBuilder(review.NewEnhancedPromptBuilder())
		resultMinimal, truncMinimal, err := minimalBuilder.BuildWithSizeGuards(
			baseContext, baseDiff, baseReq, minimalReviewer, estimator, limits,
		)
		require.NoError(t, err)

		// Build with persona
		resultWithPersona, truncWithPersona, err := builder.BuildWithSizeGuards(
			baseContext, baseDiff, baseReq, securityReviewer, estimator, limits,
		)
		require.NoError(t, err)

		// Persona version should have more tokens
		assert.Greater(t, truncWithPersona.FinalTokens, truncMinimal.FinalTokens,
			"prompt with persona should have more tokens than minimal")
		assert.Greater(t, len(resultWithPersona.Prompt), len(resultMinimal.Prompt),
			"prompt with persona should be longer")
	})

	t.Run("reserves token budget for persona content", func(t *testing.T) {
		t.Parallel()

		// Use a tight token limit to verify budget reservation works
		tightLimits := review.SizeGuardLimits{
			WarnTokens: 1000,
			MaxTokens:  2000,
		}

		result, truncResult, err := builder.BuildWithSizeGuards(
			baseContext, baseDiff, baseReq, securityReviewer, estimator, tightLimits,
		)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Prompt)

		// The final token count should not exceed the original MaxTokens
		// This verifies that persona overhead was reserved from the budget
		assert.LessOrEqual(t, truncResult.FinalTokens, tightLimits.MaxTokens,
			"final token count should not exceed MaxTokens after persona injection")
	})
}

// mockTokenEstimator is a simple mock for testing.
type mockTokenEstimator struct {
	tokensPerChar float64
}

func (m *mockTokenEstimator) EstimateTokens(text string) int {
	return int(float64(len(text)) * m.tokensPerChar)
}
