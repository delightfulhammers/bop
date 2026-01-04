package json_test

import (
	"context"
	stdjson "encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/output/json"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestWriter_Write(t *testing.T) {
	// Given
	tempDir := t.TempDir()
	now := func() string { return "20251020T120000Z" }
	writer := json.NewWriter(now)

	review := domain.Review{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		Summary:      "Test summary",
		Findings: []domain.Finding{
			{ID: "123", File: "main.go", LineStart: 1, LineEnd: 5, Description: "Test finding"},
		},
	}

	artifact := domain.JSONArtifact{
		OutputDir:    tempDir,
		Repository:   "test-repo",
		BaseRef:      "main",
		TargetRef:    "feature",
		Review:       review,
		ProviderName: "test-provider",
	}

	// When
	path, err := writer.Write(context.Background(), artifact)

	// Then
	assert.NoError(t, err)

	expectedPath := filepath.Join(tempDir, "test-repo_feature", "20251020T120000Z", "review-test-provider.json")
	assert.Equal(t, expectedPath, path)

	_, err = os.Stat(path)
	assert.NoError(t, err, "Expected file to be created")

	// Verify content
	content, err := os.ReadFile(path)
	assert.NoError(t, err)

	var writtenReview domain.Review
	err = stdjson.Unmarshal(content, &writtenReview)
	assert.NoError(t, err)
	assert.Equal(t, review, writtenReview)
}

func TestWriter_Write_IncludesCostField(t *testing.T) {
	// Given
	tempDir := t.TempDir()
	now := func() string { return "20251020T120000Z" }
	writer := json.NewWriter(now)

	review := domain.Review{
		ProviderName: "openai",
		ModelName:    "gpt-4o",
		Summary:      "Test summary",
		Cost:         0.0523,
		Findings:     []domain.Finding{},
	}

	artifact := domain.JSONArtifact{
		OutputDir:    tempDir,
		Repository:   "test-repo",
		BaseRef:      "main",
		TargetRef:    "feature",
		Review:       review,
		ProviderName: "openai",
	}

	// When
	path, err := writer.Write(context.Background(), artifact)

	// Then
	assert.NoError(t, err)

	// Verify cost field is in JSON
	content, err := os.ReadFile(path)
	assert.NoError(t, err)

	var writtenReview domain.Review
	err = stdjson.Unmarshal(content, &writtenReview)
	assert.NoError(t, err)
	assert.Equal(t, 0.0523, writtenReview.Cost)
	assert.Equal(t, review, writtenReview)
}

func TestWriter_Write_IncludesReviewersArray(t *testing.T) {
	// Given
	tempDir := t.TempDir()
	now := func() string { return "20251020T120000Z" }
	writer := json.NewWriter(now)

	review := domain.Review{
		ProviderName: "merged",
		ModelName:    "multi-reviewer",
		Summary:      "Multi-reviewer review",
		Cost:         0.10,
		Findings: []domain.Finding{
			{
				File:           "main.go",
				LineStart:      10,
				Description:    "Security issue",
				Severity:       "high",
				Category:       "security",
				ReviewerName:   "security",
				ReviewerWeight: 1.5,
			},
			{
				File:           "main.go",
				LineStart:      25,
				Description:    "Design issue",
				Severity:       "medium",
				Category:       "architecture",
				ReviewerName:   "architecture",
				ReviewerWeight: 1.0,
			},
			{
				File:           "main.go",
				LineStart:      50,
				Description:    "Another security issue",
				Severity:       "critical",
				ReviewerName:   "security",
				ReviewerWeight: 1.5,
			},
		},
	}

	artifact := domain.JSONArtifact{
		OutputDir:    tempDir,
		Repository:   "test-repo",
		BaseRef:      "main",
		TargetRef:    "feature",
		Review:       review,
		ProviderName: "merged",
	}

	// When
	path, err := writer.Write(context.Background(), artifact)

	// Then
	assert.NoError(t, err)

	// Verify reviewers array in JSON
	content, err := os.ReadFile(path)
	assert.NoError(t, err)

	var output json.JSONOutput
	err = stdjson.Unmarshal(content, &output)
	assert.NoError(t, err)

	// Should have 2 unique reviewers
	assert.Len(t, output.Reviewers, 2)

	// Verify security reviewer
	assert.Equal(t, "security", output.Reviewers[0].Name)
	assert.Equal(t, 1.5, output.Reviewers[0].Weight)

	// Verify architecture reviewer
	assert.Equal(t, "architecture", output.Reviewers[1].Name)
	assert.Equal(t, 1.0, output.Reviewers[1].Weight)

	// Verify findings have reviewer attribution
	assert.Equal(t, "security", output.Findings[0].ReviewerName)
	assert.Equal(t, 1.5, output.Findings[0].ReviewerWeight)
}
