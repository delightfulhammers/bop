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

	if err := encoder.Encode(artifact.Review); err != nil {
		return "", fmt.Errorf("failed to encode review to json: %w", err)
	}

	return filePath, nil
}
