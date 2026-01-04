package json

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/delightfulhammers/bop/internal/domain"
)

// VerificationReport captures the full verification results for transparency.
type VerificationReport struct {
	GeneratedAt   string                    `json:"generated_at"`
	TotalFindings int                       `json:"total_findings"`
	Verified      int                       `json:"verified_count"`
	Filtered      int                       `json:"filtered_count"`
	Reportable    int                       `json:"reportable_count"`
	Findings      []VerificationFindingInfo `json:"findings"`
}

// VerificationFindingInfo captures the verification result for a single finding.
type VerificationFindingInfo struct {
	// Original finding info
	File        string `json:"file"`
	LineStart   int    `json:"line_start"`
	LineEnd     int    `json:"line_end,omitempty"`
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Description string `json:"description"`

	// Verification results
	Verified        bool   `json:"verified"`
	Classification  string `json:"classification,omitempty"`
	Confidence      int    `json:"confidence"`
	Evidence        string `json:"evidence"`
	BlocksOperation bool   `json:"blocks_operation"`

	// Filtering decision
	Reportable     bool   `json:"reportable"`
	FilteredReason string `json:"filtered_reason,omitempty"`
	Threshold      int    `json:"threshold,omitempty"`
}

// WriteVerificationReport writes a detailed verification report to JSON.
func WriteVerificationReport(
	outputDir string,
	repository string,
	targetRef string,
	verified []domain.VerifiedFinding,
	reportable []domain.VerifiedFinding,
	getThreshold func(severity string) int,
) (string, error) {
	// Create output directory if needed
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("creating output directory: %w", err)
	}

	// Build set of reportable finding fingerprints for quick lookup
	reportableSet := make(map[string]bool)
	for _, f := range reportable {
		key := fmt.Sprintf("%s:%d:%s", f.Finding.File, f.Finding.LineStart, f.Finding.Description)
		reportableSet[key] = true
	}

	// Build finding info list
	findings := make([]VerificationFindingInfo, 0, len(verified))
	for _, v := range verified {
		key := fmt.Sprintf("%s:%d:%s", v.Finding.File, v.Finding.LineStart, v.Finding.Description)
		isReportable := reportableSet[key]
		threshold := getThreshold(v.Finding.Severity)

		info := VerificationFindingInfo{
			File:            v.Finding.File,
			LineStart:       v.Finding.LineStart,
			LineEnd:         v.Finding.LineEnd,
			Severity:        v.Finding.Severity,
			Category:        v.Finding.Category,
			Description:     v.Finding.Description,
			Verified:        v.Verified,
			Classification:  string(v.Classification),
			Confidence:      v.Confidence,
			Evidence:        v.Evidence,
			BlocksOperation: v.BlocksOperation,
			Reportable:      isReportable,
			Threshold:       threshold,
		}

		// Determine filter reason
		if !isReportable {
			if !v.Verified {
				info.FilteredReason = "not_verified"
			} else if v.Confidence < threshold {
				info.FilteredReason = fmt.Sprintf("confidence_below_threshold (%d < %d)", v.Confidence, threshold)
			} else {
				info.FilteredReason = "unknown"
			}
		}

		findings = append(findings, info)
	}

	report := VerificationReport{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		TotalFindings: len(verified),
		Verified:      countVerified(verified),
		Filtered:      len(verified) - len(reportable),
		Reportable:    len(reportable),
		Findings:      findings,
	}

	// Write to file
	filename := fmt.Sprintf("%s_%s_verification_report.json",
		sanitizeFilename(repository),
		sanitizeFilename(targetRef),
	)
	path := filepath.Join(outputDir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling report: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("writing report: %w", err)
	}

	return path, nil
}

func countVerified(findings []domain.VerifiedFinding) int {
	count := 0
	for _, f := range findings {
		if f.Verified {
			count++
		}
	}
	return count
}

func sanitizeFilename(s string) string {
	// Replace problematic characters
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else if c == '/' || c == '\\' || c == ' ' {
			result = append(result, '_')
		}
	}
	return string(result)
}
