package sarif

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/review"
)

// Writer implements the review.SARIFWriter interface.
type Writer struct {
	now func() string
}

// NewWriter creates a new SARIF writer.
func NewWriter(now func() string) *Writer {
	return &Writer{now: now}
}

// Write persists a review to disk as a SARIF file.
func (w *Writer) Write(ctx context.Context, artifact review.SARIFArtifact) (string, error) {
	outputDir := filepath.Join(artifact.OutputDir, fmt.Sprintf("%s_%s", artifact.Repository, artifact.TargetRef), w.now())
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	filePath := filepath.Join(outputDir, fmt.Sprintf("review-%s.sarif", artifact.ProviderName))

	sarifDoc := w.convertToSARIF(artifact)

	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create sarif file: %w", err)
	}
	defer func() { _ = file.Close() }()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(sarifDoc); err != nil {
		return "", fmt.Errorf("failed to encode review to sarif: %w", err)
	}

	return filePath, nil
}

// convertToSARIF converts a domain.Review to SARIF format.
func (w *Writer) convertToSARIF(artifact review.SARIFArtifact) map[string]interface{} {
	results := make([]map[string]interface{}, 0, len(artifact.Review.Findings))

	for _, finding := range artifact.Review.Findings {
		// SARIF requires non-empty message text
		messageText := finding.Description
		if messageText == "" {
			messageText = "No description provided"
		}

		// Use a valid ruleId (category or fallback)
		ruleID := finding.Category
		if ruleID == "" {
			ruleID = "code-review"
		}

		result := map[string]interface{}{
			"ruleId": ruleID,
			"level":  convertSeverity(finding.Severity),
			"message": map[string]interface{}{
				"text": messageText,
			},
		}

		// Build location only if we have meaningful file info
		// Omit locations entirely for file-level or project-level findings
		if finding.File != "" {
			physicalLocation := map[string]interface{}{
				"artifactLocation": map[string]interface{}{
					"uri": finding.File,
				},
			}

			// Only include region if we have meaningful line info
			// (don't fabricate line 1 for findings without specific locations)
			if finding.LineStart >= 1 {
				startLine := finding.LineStart
				endLine := finding.LineEnd
				if endLine < startLine {
					endLine = startLine
				}
				physicalLocation["region"] = map[string]interface{}{
					"startLine": startLine,
					"endLine":   endLine,
				}
			}

			result["locations"] = []map[string]interface{}{
				{"physicalLocation": physicalLocation},
			}
		}

		// Add suggestion as a property (fixes requires artifactChanges which we don't have)
		if finding.Suggestion != "" {
			result["properties"] = map[string]interface{}{
				"suggestion": finding.Suggestion,
			}
		}

		results = append(results, result)
	}

	return map[string]interface{}{
		"version": "2.1.0",
		"$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		"runs": []map[string]interface{}{
			{
				"tool": map[string]interface{}{
					"driver": map[string]interface{}{
						"name":            artifact.ProviderName,
						"informationUri":  "https://github.com/bkyoung/code-reviewer",
						"version":         "1.0.0",
						"semanticVersion": "1.0.0",
						"rules": []map[string]interface{}{
							{
								"id":               "code-review",
								"name":             "CodeReview",
								"shortDescription": map[string]interface{}{"text": "AI-powered code review findings"},
								"fullDescription":  map[string]interface{}{"text": "Findings from multi-LLM code review analysis"},
							},
						},
					},
				},
				"results":    results,
				"properties": buildProperties(artifact.Review),
			},
		},
	}
}

// buildProperties creates the properties map for SARIF run, validating cost.
func buildProperties(review domain.Review) map[string]interface{} {
	properties := map[string]interface{}{
		"summary":   review.Summary,
		"model":     review.ModelName,
		"tokensIn":  review.TokensIn,
		"tokensOut": review.TokensOut,
	}

	// Only include cost if it's a valid number (not NaN or Inf)
	// JSON encoding will fail on NaN and Inf values
	if !math.IsNaN(review.Cost) && !math.IsInf(review.Cost, 0) {
		properties["cost"] = review.Cost
	}

	return properties
}

// convertSeverity maps our severity levels to SARIF levels.
func convertSeverity(severity string) string {
	switch severity {
	case "critical", "high":
		return "error"
	case "medium":
		return "warning"
	case "low":
		return "note"
	default:
		return "warning"
	}
}
