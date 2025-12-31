package markdown_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bkyoung/code-reviewer/internal/adapter/output/markdown"
	"github.com/bkyoung/code-reviewer/internal/domain"
)

func TestWriterProducesDeterministicMarkdown(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	writer := markdown.NewWriter(func() string {
		return "2025-01-01T00-00-00Z"
	})

	reviewData := domain.Review{
		ProviderName: "stub-openai",
		ModelName:    "gpt-4o",
		Summary:      "Summary text",
		Findings: []domain.Finding{
			{
				ID:          "id",
				File:        "main.go",
				LineStart:   10,
				LineEnd:     12,
				Severity:    "medium",
				Category:    "bug",
				Description: "Bug description",
				Suggestion:  "Fix it",
				Evidence:    true,
			},
		},
	}

	path, err := writer.Write(ctx, domain.MarkdownArtifact{
		OutputDir:    dir,
		Repository:   "repo",
		BaseRef:      "master",
		TargetRef:    "feature",
		Diff:         domain.Diff{},
		Review:       reviewData,
		ProviderName: "stub-openai",
	})
	if err != nil {
		t.Fatalf("writer returned error: %v", err)
	}

	if filepath.Base(path) != "repo_feature_stub-openai_2025-01-01T00-00-00Z.md" {
		t.Fatalf("unexpected filename: %s", filepath.Base(path))
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if !strings.Contains(string(content), "Summary text") {
		t.Fatalf("markdown missing summary: %s", string(content))
	}
}

func TestWriterIncludesCostInformation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	writer := markdown.NewWriter(func() string {
		return "2025-01-01T00-00-00Z"
	})

	reviewData := domain.Review{
		ProviderName: "openai",
		ModelName:    "gpt-4o",
		Summary:      "Review summary",
		TokensIn:     1000,
		TokensOut:    500,
		Cost:         0.0523, // $0.0523
		Findings:     []domain.Finding{},
	}

	path, err := writer.Write(ctx, domain.MarkdownArtifact{
		OutputDir:    dir,
		Repository:   "test-repo",
		BaseRef:      "main",
		TargetRef:    "feature",
		Diff:         domain.Diff{},
		Review:       reviewData,
		ProviderName: "openai",
	})
	if err != nil {
		t.Fatalf("writer returned error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)

	// Verify token usage is included
	if !strings.Contains(contentStr, "Tokens: 1000 in / 500 out") {
		t.Errorf("markdown missing token information: %s", contentStr)
	}

	// Verify cost is included with correct formatting
	if !strings.Contains(contentStr, "Cost: $0.0523") {
		t.Errorf("markdown missing cost information: %s", contentStr)
	}

	// Test zero cost case
	reviewData.Cost = 0.0
	path2, err := writer.Write(ctx, domain.MarkdownArtifact{
		OutputDir:    dir,
		Repository:   "test-repo",
		BaseRef:      "main",
		TargetRef:    "feature",
		Diff:         domain.Diff{},
		Review:       reviewData,
		ProviderName: "openai",
	})
	if err != nil {
		t.Fatalf("writer returned error: %v", err)
	}

	content2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Zero cost should show as $0.00
	if !strings.Contains(string(content2), "Cost: $0.00") {
		t.Errorf("markdown missing zero cost: %s", string(content2))
	}
}

func TestWriterIncludesVerificationMetadata(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	writer := markdown.NewWriter(func() string {
		return "2025-01-01T00-00-00Z"
	})

	// Create a review with verification data
	reviewData := domain.Review{
		ProviderName: "merged",
		ModelName:    "multi-provider",
		Summary:      "Review with verification",
		Cost:         0.15,
		Findings: []domain.Finding{
			{
				File:        "main.go",
				LineStart:   10,
				Description: "Null pointer dereference",
				Severity:    "high",
				Category:    "bug",
			},
		},
		DiscoveryFindings: []domain.CandidateFinding{
			{
				Finding: domain.Finding{
					File:        "main.go",
					LineStart:   10,
					Description: "Null pointer dereference",
					Severity:    "high",
				},
				Sources:        []string{"openai", "anthropic"},
				AgreementScore: 1.0,
			},
			{
				Finding: domain.Finding{
					File:        "utils.go",
					LineStart:   25,
					Description: "Unused variable",
					Severity:    "low",
				},
				Sources:        []string{"openai"},
				AgreementScore: 0.5,
			},
		},
		VerifiedFindings: []domain.VerifiedFinding{
			{
				Finding: domain.Finding{
					File:        "main.go",
					LineStart:   10,
					Description: "Null pointer dereference",
					Severity:    "high",
					Category:    "bug",
				},
				Verified:        true,
				Classification:  domain.ClassBlockingBug,
				Confidence:      90,
				Evidence:        "Confirmed null check missing at line 10",
				BlocksOperation: true,
			},
			{
				Finding: domain.Finding{
					File:        "utils.go",
					LineStart:   25,
					Description: "Unused variable",
					Severity:    "low",
				},
				Verified:       false,
				Classification: domain.ClassStyle,
				Confidence:     30,
				Evidence:       "Variable is used in conditional branch",
			},
		},
		ReportableFindings: []domain.VerifiedFinding{
			{
				Finding: domain.Finding{
					File:        "main.go",
					LineStart:   10,
					LineEnd:     10,
					Description: "Null pointer dereference",
					Severity:    "high",
					Category:    "bug",
					Suggestion:  "Add nil check",
				},
				Verified:        true,
				Classification:  domain.ClassBlockingBug,
				Confidence:      90,
				Evidence:        "Confirmed null check missing at line 10",
				BlocksOperation: true,
			},
		},
	}

	path, err := writer.Write(ctx, domain.MarkdownArtifact{
		OutputDir:    dir,
		Repository:   "test-repo",
		BaseRef:      "main",
		TargetRef:    "feature",
		Diff:         domain.Diff{},
		Review:       reviewData,
		ProviderName: "merged",
	})
	if err != nil {
		t.Fatalf("writer returned error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)

	// Verify verification summary section
	if !strings.Contains(contentStr, "## Verification Summary") {
		t.Error("markdown missing verification summary section")
	}
	if !strings.Contains(contentStr, "Discovery findings: 2") {
		t.Error("markdown missing discovery findings count")
	}
	if !strings.Contains(contentStr, "Verified findings: 2") {
		t.Error("markdown missing verified findings count")
	}
	if !strings.Contains(contentStr, "Reportable findings: 1") {
		t.Error("markdown missing reportable findings count")
	}

	// Verify finding includes verification metadata
	if !strings.Contains(contentStr, "Classification: blocking_bug") {
		t.Error("markdown missing classification")
	}
	if !strings.Contains(contentStr, "Confidence: 90%") {
		t.Error("markdown missing confidence")
	}
	if !strings.Contains(contentStr, "Blocks Operation: Yes") {
		t.Error("markdown missing blocks operation indicator")
	}
	if !strings.Contains(contentStr, "Verification Evidence: Confirmed null check missing") {
		t.Error("markdown missing verification evidence")
	}
}

func TestWriterFallsBackToLegacyFindings(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	writer := markdown.NewWriter(func() string {
		return "2025-01-01T00-00-00Z"
	})

	// Create a review without verification data (legacy mode)
	reviewData := domain.Review{
		ProviderName: "openai",
		ModelName:    "gpt-4o",
		Summary:      "Legacy review",
		Cost:         0.05,
		Findings: []domain.Finding{
			{
				File:        "main.go",
				LineStart:   10,
				LineEnd:     12,
				Description: "Some issue",
				Severity:    "medium",
				Category:    "bug",
				Suggestion:  "Fix it",
				Evidence:    true,
			},
		},
		// No DiscoveryFindings, VerifiedFindings, or ReportableFindings
	}

	path, err := writer.Write(ctx, domain.MarkdownArtifact{
		OutputDir:    dir,
		Repository:   "test-repo",
		BaseRef:      "main",
		TargetRef:    "feature",
		Diff:         domain.Diff{},
		Review:       reviewData,
		ProviderName: "openai",
	})
	if err != nil {
		t.Fatalf("writer returned error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)

	// Should NOT have verification summary
	if strings.Contains(contentStr, "## Verification Summary") {
		t.Error("legacy review should not have verification summary")
	}

	// Should have legacy format (Evidence: Provided)
	if !strings.Contains(contentStr, "Evidence: Provided") {
		t.Error("legacy review should use legacy evidence format")
	}

	// Should NOT have verification-specific fields
	if strings.Contains(contentStr, "Classification:") {
		t.Error("legacy review should not have classification")
	}
	if strings.Contains(contentStr, "Confidence:") {
		t.Error("legacy review should not have confidence")
	}
}

func TestWriterIncludesReviewerMetadata(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	writer := markdown.NewWriter(func() string {
		return "2025-01-01T00-00-00Z"
	})

	// Create a review with multiple reviewer personas
	reviewData := domain.Review{
		ProviderName: "merged",
		ModelName:    "multi-reviewer",
		Summary:      "Multi-reviewer review",
		Cost:         0.10,
		Findings: []domain.Finding{
			{
				File:           "main.go",
				LineStart:      10,
				LineEnd:        12,
				Description:    "SQL injection vulnerability",
				Severity:       "critical",
				Category:       "security",
				Suggestion:     "Use parameterized queries",
				ReviewerName:   "security",
				ReviewerWeight: 1.5,
			},
			{
				File:           "main.go",
				LineStart:      25,
				LineEnd:        30,
				Description:    "Function too complex",
				Severity:       "medium",
				Category:       "maintainability",
				Suggestion:     "Extract helper functions",
				ReviewerName:   "architecture",
				ReviewerWeight: 1.0,
			},
			{
				File:           "main.go",
				LineStart:      50,
				Description:    "Authentication bypass possible",
				Severity:       "high",
				Category:       "security",
				ReviewerName:   "security",
				ReviewerWeight: 1.5,
			},
		},
	}

	path, err := writer.Write(ctx, domain.MarkdownArtifact{
		OutputDir:    dir,
		Repository:   "test-repo",
		BaseRef:      "main",
		TargetRef:    "feature",
		Diff:         domain.Diff{},
		Review:       reviewData,
		ProviderName: "merged",
	})
	if err != nil {
		t.Fatalf("writer returned error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)

	// Verify reviewer summary section
	if !strings.Contains(contentStr, "### Reviewers") {
		t.Error("markdown missing reviewers section")
	}

	// Verify security reviewer stats
	if !strings.Contains(contentStr, "**security**") {
		t.Error("markdown missing security reviewer")
	}
	if !strings.Contains(contentStr, "(weight: 1.5)") {
		t.Error("markdown missing security reviewer weight")
	}
	if !strings.Contains(contentStr, "2 findings") {
		t.Error("markdown missing security reviewer finding count")
	}

	// Verify architecture reviewer stats
	if !strings.Contains(contentStr, "**architecture**") {
		t.Error("markdown missing architecture reviewer")
	}
	if !strings.Contains(contentStr, "(weight: 1.0)") {
		t.Error("markdown missing architecture reviewer weight")
	}
	if !strings.Contains(contentStr, "1 findings") {
		t.Error("markdown missing architecture reviewer finding count")
	}

	// Verify findings are grouped by reviewer (multi-reviewer format uses sections)
	if !strings.Contains(contentStr, "### security") {
		t.Error("markdown missing security reviewer section")
	}
	if !strings.Contains(contentStr, "### architecture") {
		t.Error("markdown missing architecture reviewer section")
	}
}
