package sarif_test

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/output/sarif"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriter_Write(t *testing.T) {
	now := func() string { return "2025-10-20T12-00-00" }

	t.Run("writes SARIF file successfully", func(t *testing.T) {
		tmpDir := t.TempDir()

		writer := sarif.NewWriter(now)
		artifact := review.SARIFArtifact{
			OutputDir:    tmpDir,
			Repository:   "test-repo",
			BaseRef:      "main",
			TargetRef:    "feature",
			Review:       createTestReview(),
			ProviderName: "openai",
		}

		path, err := writer.Write(context.Background(), artifact)
		require.NoError(t, err)

		expectedPath := filepath.Join(tmpDir, "test-repo_feature", "2025-10-20T12-00-00", "review-openai.sarif")
		assert.Equal(t, expectedPath, path)

		// Verify file exists
		_, err = os.Stat(path)
		require.NoError(t, err)

		// Verify it's valid JSON
		content, err := os.ReadFile(path)
		require.NoError(t, err)

		var sarifDoc map[string]interface{}
		err = json.Unmarshal(content, &sarifDoc)
		require.NoError(t, err)

		// Verify SARIF structure
		assert.Equal(t, "2.1.0", sarifDoc["version"])
		assert.NotNil(t, sarifDoc["runs"])
	})

	t.Run("creates output directory if it doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputDir := filepath.Join(tmpDir, "nested", "path")

		writer := sarif.NewWriter(now)
		artifact := review.SARIFArtifact{
			OutputDir:    outputDir,
			Repository:   "test-repo",
			BaseRef:      "main",
			TargetRef:    "feature",
			Review:       createTestReview(),
			ProviderName: "openai",
		}

		path, err := writer.Write(context.Background(), artifact)
		require.NoError(t, err)

		// Verify file exists
		_, err = os.Stat(path)
		require.NoError(t, err)
	})

	t.Run("converts findings to SARIF results", func(t *testing.T) {
		tmpDir := t.TempDir()

		finding := domain.NewFinding(domain.FindingInput{
			File:        "main.go",
			LineStart:   10,
			LineEnd:     15,
			Severity:    "high",
			Category:    "security",
			Description: "SQL injection vulnerability",
			Suggestion:  "Use parameterized queries",
			Evidence:    true,
		})

		testReview := domain.Review{
			ProviderName: "openai",
			ModelName:    "gpt-4",
			Summary:      "Test review",
			Findings:     []domain.Finding{finding},
		}

		writer := sarif.NewWriter(now)
		artifact := review.SARIFArtifact{
			OutputDir:    tmpDir,
			Repository:   "test-repo",
			BaseRef:      "main",
			TargetRef:    "feature",
			Review:       testReview,
			ProviderName: "openai",
		}

		path, err := writer.Write(context.Background(), artifact)
		require.NoError(t, err)

		content, err := os.ReadFile(path)
		require.NoError(t, err)

		var sarifDoc map[string]interface{}
		err = json.Unmarshal(content, &sarifDoc)
		require.NoError(t, err)

		// Verify results exist
		runs := sarifDoc["runs"].([]interface{})
		require.Len(t, runs, 1)

		run := runs[0].(map[string]interface{})
		results := run["results"].([]interface{})
		require.Len(t, results, 1)

		result := results[0].(map[string]interface{})
		assert.Equal(t, "SQL injection vulnerability", result["message"].(map[string]interface{})["text"])
	})
}

func createTestReview() domain.Review {
	finding := domain.NewFinding(domain.FindingInput{
		File:        "internal/test.go",
		LineStart:   1,
		LineEnd:     5,
		Severity:    "low",
		Category:    "style",
		Description: "Test finding",
		Suggestion:  "Fix it",
		Evidence:    true,
	})

	return domain.Review{
		ProviderName: "openai",
		ModelName:    "gpt-4",
		Summary:      "This is a test review.",
		Findings:     []domain.Finding{finding},
	}
}

func TestWriter_Write_IncludesCostInProperties(t *testing.T) {
	tmpDir := t.TempDir()
	now := func() string { return "2025-10-20T12-00-00" }

	testReview := domain.Review{
		ProviderName: "openai",
		ModelName:    "gpt-4o",
		Summary:      "Test review",
		TokensIn:     1000,
		TokensOut:    500,
		Cost:         0.0523,
		Findings:     []domain.Finding{},
	}

	writer := sarif.NewWriter(now)
	artifact := review.SARIFArtifact{
		OutputDir:    tmpDir,
		Repository:   "test-repo",
		BaseRef:      "main",
		TargetRef:    "feature",
		Review:       testReview,
		ProviderName: "openai",
	}

	path, err := writer.Write(context.Background(), artifact)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var sarifDoc map[string]interface{}
	err = json.Unmarshal(content, &sarifDoc)
	require.NoError(t, err)

	// Verify cost is in run properties
	runs := sarifDoc["runs"].([]interface{})
	require.Len(t, runs, 1)

	run := runs[0].(map[string]interface{})
	properties := run["properties"].(map[string]interface{})
	assert.Equal(t, 0.0523, properties["cost"])
	assert.Equal(t, "Test review", properties["summary"])
	assert.Equal(t, float64(1000), properties["tokensIn"])
	assert.Equal(t, float64(500), properties["tokensOut"])
}

func TestWriter_Write_HandlesInvalidCost(t *testing.T) {
	now := func() string { return "2025-10-20T12-00-00" }

	tests := []struct {
		name          string
		cost          float64
		shouldInclude bool
	}{
		{
			name:          "valid cost",
			cost:          1.23,
			shouldInclude: true,
		},
		{
			name:          "zero cost",
			cost:          0.0,
			shouldInclude: true,
		},
		{
			name:          "NaN cost",
			cost:          math.NaN(),
			shouldInclude: false,
		},
		{
			name:          "positive infinity",
			cost:          math.Inf(1),
			shouldInclude: false,
		},
		{
			name:          "negative infinity",
			cost:          math.Inf(-1),
			shouldInclude: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			testReview := domain.Review{
				ProviderName: "openai",
				ModelName:    "gpt-4o",
				Summary:      "Test review",
				Cost:         tt.cost,
				Findings:     []domain.Finding{},
			}

			writer := sarif.NewWriter(now)
			artifact := review.SARIFArtifact{
				OutputDir:    tmpDir,
				Repository:   "test-repo",
				BaseRef:      "main",
				TargetRef:    "feature",
				Review:       testReview,
				ProviderName: "openai",
			}

			path, err := writer.Write(context.Background(), artifact)
			require.NoError(t, err)

			content, err := os.ReadFile(path)
			require.NoError(t, err)

			var sarifDoc map[string]interface{}
			err = json.Unmarshal(content, &sarifDoc)
			require.NoError(t, err)

			// Verify cost handling in properties
			runs := sarifDoc["runs"].([]interface{})
			require.Len(t, runs, 1)

			run := runs[0].(map[string]interface{})
			properties := run["properties"].(map[string]interface{})

			if tt.shouldInclude {
				assert.Contains(t, properties, "cost", "valid cost should be included")
				assert.Equal(t, tt.cost, properties["cost"])
			} else {
				assert.NotContains(t, properties, "cost", "invalid cost should be excluded")
			}

			// Summary should always be included
			assert.Equal(t, "Test review", properties["summary"])
		})
	}
}

func TestWriter_Write_IncludesReviewerExtensions(t *testing.T) {
	tmpDir := t.TempDir()
	now := func() string { return "2025-10-20T12-00-00" }

	// Create findings with reviewer attribution
	testReview := domain.Review{
		ProviderName: "merged",
		ModelName:    "multi-reviewer",
		Summary:      "Multi-reviewer review",
		Findings: []domain.Finding{
			{
				File:           "main.go",
				LineStart:      10,
				Description:    "SQL injection vulnerability",
				Severity:       "critical",
				Category:       "security",
				ReviewerName:   "security",
				ReviewerWeight: 1.5,
			},
			{
				File:           "main.go",
				LineStart:      25,
				Description:    "Function too complex",
				Severity:       "medium",
				Category:       "maintainability",
				ReviewerName:   "architecture",
				ReviewerWeight: 1.0,
			},
			{
				File:           "main.go",
				LineStart:      50,
				Description:    "Auth bypass possible",
				Severity:       "high",
				ReviewerName:   "security",
				ReviewerWeight: 1.5,
			},
		},
	}

	writer := sarif.NewWriter(now)
	artifact := review.SARIFArtifact{
		OutputDir:    tmpDir,
		Repository:   "test-repo",
		BaseRef:      "main",
		TargetRef:    "feature",
		Review:       testReview,
		ProviderName: "merged",
	}

	path, err := writer.Write(context.Background(), artifact)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var sarifDoc map[string]interface{}
	err = json.Unmarshal(content, &sarifDoc)
	require.NoError(t, err)

	// Verify tool extensions for reviewers
	runs := sarifDoc["runs"].([]interface{})
	require.Len(t, runs, 1)

	run := runs[0].(map[string]interface{})
	tool := run["tool"].(map[string]interface{})

	// Should have extensions array
	extensions, ok := tool["extensions"].([]interface{})
	require.True(t, ok, "tool should have extensions array")
	require.Len(t, extensions, 2, "should have 2 unique reviewers")

	// Verify first extension (security)
	ext1 := extensions[0].(map[string]interface{})
	assert.Equal(t, "security", ext1["name"])
	ext1Props := ext1["properties"].(map[string]interface{})
	assert.Equal(t, 1.5, ext1Props["weight"])
	assert.Equal(t, "reviewer-persona", ext1Props["role"])

	// Verify second extension (architecture)
	ext2 := extensions[1].(map[string]interface{})
	assert.Equal(t, "architecture", ext2["name"])
	ext2Props := ext2["properties"].(map[string]interface{})
	assert.Equal(t, 1.0, ext2Props["weight"])

	// Verify findings have reviewer properties
	results := run["results"].([]interface{})
	require.Len(t, results, 3)

	result1 := results[0].(map[string]interface{})
	result1Props := result1["properties"].(map[string]interface{})
	assert.Equal(t, "security", result1Props["reviewerName"])
	assert.Equal(t, 1.5, result1Props["reviewerWeight"])
}
