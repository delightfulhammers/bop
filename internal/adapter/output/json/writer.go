package json

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// Writer implements the review.JSONWriter interface.
type Writer struct {
	now func() string
}

// NewWriter creates a new JSON writer.
func NewWriter(now func() string) *Writer {
	return &Writer{now: now}
}

// ReviewerInfo represents a reviewer in the JSON output.
type ReviewerInfo struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}

// JSONOutput is the enhanced JSON structure with reviewer metadata.
type JSONOutput struct {
	// Reviewers lists unique reviewers who contributed findings
	Reviewers []ReviewerInfo `json:"reviewers,omitempty"`

	// Embed all Review fields
	ProviderName string           `json:"providerName"`
	ModelName    string           `json:"modelName"`
	Summary      string           `json:"summary"`
	Findings     []domain.Finding `json:"findings"`

	// Usage metadata
	TokensIn  int     `json:"tokensIn"`
	TokensOut int     `json:"tokensOut"`
	Cost      float64 `json:"cost"`

	// Agent verification fields
	DiscoveryFindings  []domain.CandidateFinding `json:"discoveryFindings,omitempty"`
	VerifiedFindings   []domain.VerifiedFinding  `json:"verifiedFindings,omitempty"`
	ReportableFindings []domain.VerifiedFinding  `json:"reportableFindings,omitempty"`

	// Size guard fields
	SizeLimitExceeded bool     `json:"sizeLimitExceeded,omitempty"`
	WasTruncated      bool     `json:"wasTruncated,omitempty"`
	TruncatedFiles    []string `json:"truncatedFiles,omitempty"`
	TruncationWarning string   `json:"truncationWarning,omitempty"`
}

// Write persists a review to disk as a JSON file.
func (w *Writer) Write(ctx context.Context, artifact domain.JSONArtifact) (string, error) {
	outputDir := filepath.Join(artifact.OutputDir, fmt.Sprintf("%s_%s", artifact.Repository, artifact.TargetRef), w.now())
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	filePath := filepath.Join(outputDir, fmt.Sprintf("review-%s.json", artifact.ProviderName))

	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create json file: %w", err)
	}
	defer func() { _ = file.Close() }()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	output := buildJSONOutput(artifact.Review)
	if err := encoder.Encode(output); err != nil {
		return "", fmt.Errorf("failed to encode review to json: %w", err)
	}

	return filePath, nil
}

// buildJSONOutput creates the enhanced JSON structure with reviewer metadata.
func buildJSONOutput(review domain.Review) JSONOutput {
	output := JSONOutput{
		ProviderName:       review.ProviderName,
		ModelName:          review.ModelName,
		Summary:            review.Summary,
		Findings:           review.Findings,
		TokensIn:           review.TokensIn,
		TokensOut:          review.TokensOut,
		Cost:               review.Cost,
		DiscoveryFindings:  review.DiscoveryFindings,
		VerifiedFindings:   review.VerifiedFindings,
		ReportableFindings: review.ReportableFindings,
		SizeLimitExceeded:  review.SizeLimitExceeded,
		WasTruncated:       review.WasTruncated,
		TruncatedFiles:     review.TruncatedFiles,
		TruncationWarning:  review.TruncationWarning,
	}

	// Extract unique reviewers from findings
	output.Reviewers = extractReviewers(review.Findings)

	return output
}

// extractReviewers collects unique reviewer info from findings.
func extractReviewers(findings []domain.Finding) []ReviewerInfo {
	seen := make(map[string]bool)
	var reviewers []ReviewerInfo

	for _, f := range findings {
		if f.ReviewerName == "" {
			continue
		}
		if !seen[f.ReviewerName] {
			seen[f.ReviewerName] = true
			reviewers = append(reviewers, ReviewerInfo{
				Name:   f.ReviewerName,
				Weight: f.ReviewerWeight,
			})
		}
	}

	return reviewers
}
