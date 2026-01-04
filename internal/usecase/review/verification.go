package review

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/delightfulhammers/bop/internal/domain"
)

// convertFindingsToCandidates converts merged findings to candidate findings for verification.
// Each finding becomes a candidate with source information from the merged review.
func convertFindingsToCandidates(findings []domain.Finding, providerName string) []domain.CandidateFinding {
	candidates := make([]domain.CandidateFinding, 0, len(findings))

	for _, f := range findings {
		// Parse sources from the merged review's provider name if available
		// Format is typically "merged (openai, anthropic)" or just "openai"
		sources := parseSources(providerName)
		if len(sources) == 0 {
			sources = []string{providerName}
		}

		candidates = append(candidates, domain.CandidateFinding{
			Finding:        f,
			Sources:        sources,
			AgreementScore: 1.0, // Merged findings have implicit agreement
		})
	}

	return candidates
}

// parseSources extracts provider names from a merged provider string.
// Input: "merged (openai, anthropic, gemini)" -> ["openai", "anthropic", "gemini"]
// Input: "openai" -> ["openai"]
func parseSources(providerName string) []string {
	if !strings.HasPrefix(providerName, "merged") {
		return []string{providerName}
	}

	// Extract content between parentheses
	start := strings.Index(providerName, "(")
	end := strings.LastIndex(providerName, ")")

	if start == -1 || end == -1 || end <= start {
		return []string{providerName}
	}

	content := providerName[start+1 : end]
	parts := strings.Split(content, ",")

	sources := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			sources = append(sources, trimmed)
		}
	}

	return sources
}

// buildVerifiedFindings combines candidates with their verification results.
func buildVerifiedFindings(candidates []domain.CandidateFinding, results []domain.VerificationResult) []domain.VerifiedFinding {
	if len(candidates) != len(results) {
		// This shouldn't happen, but handle gracefully
		return nil
	}

	verified := make([]domain.VerifiedFinding, 0, len(candidates))

	for i, candidate := range candidates {
		result := results[i]

		verified = append(verified, domain.VerifiedFinding{
			Finding:         candidate.Finding,
			Verified:        result.Verified,
			Classification:  result.Classification,
			Confidence:      result.Confidence,
			Evidence:        result.Evidence,
			BlocksOperation: result.BlocksOperation,
			VerificationLog: result.Actions,
		})
	}

	return verified
}

// filterByConfidence filters verified findings based on confidence thresholds.
// Findings below the threshold for their severity level are excluded.
// Unverified findings (Verified=false) are always excluded.
func filterByConfidence(findings []domain.VerifiedFinding, settings VerificationSettings) []domain.VerifiedFinding {
	reportable := make([]domain.VerifiedFinding, 0, len(findings))

	for _, f := range findings {
		// Skip unverified findings
		if !f.Verified {
			continue
		}

		// Get threshold for this severity
		threshold := getThresholdForSeverity(f.Finding.Severity, settings)

		// Include if confidence meets threshold
		if f.Confidence >= threshold {
			reportable = append(reportable, f)
		}
	}

	return reportable
}

// getThresholdForSeverity returns the confidence threshold for a given severity level.
func getThresholdForSeverity(severity string, settings VerificationSettings) int {
	switch strings.ToLower(severity) {
	case "critical":
		if settings.ConfidenceCritical > 0 {
			return settings.ConfidenceCritical
		}
	case "high":
		if settings.ConfidenceHigh > 0 {
			return settings.ConfidenceHigh
		}
	case "medium":
		if settings.ConfidenceMedium > 0 {
			return settings.ConfidenceMedium
		}
	case "low":
		if settings.ConfidenceLow > 0 {
			return settings.ConfidenceLow
		}
	}

	// Use default if set
	if settings.ConfidenceDefault > 0 {
		return settings.ConfidenceDefault
	}

	// Fallback defaults based on severity
	switch strings.ToLower(severity) {
	case "critical":
		return 50 // Lower bar for critical issues
	case "high":
		return 60
	case "medium":
		return 70
	case "low":
		return 80 // Higher bar for low severity
	default:
		return 70
	}
}

// verifyFindings runs the verification stage on merged findings.
// Returns nil slices if verification is skipped.
func (o *Orchestrator) verifyFindings(
	ctx context.Context,
	findings []domain.Finding,
	providerName string,
	settings VerificationSettings,
) ([]domain.CandidateFinding, []domain.VerifiedFinding, []domain.VerifiedFinding, error) {
	// Convert findings to candidates
	candidates := convertFindingsToCandidates(findings, providerName)

	if len(candidates) == 0 {
		return candidates, nil, nil, nil
	}

	// Verify candidates
	results, err := o.deps.Verifier.VerifyBatch(ctx, candidates)
	if err != nil {
		return candidates, nil, nil, err
	}

	// Build verified findings
	verified := buildVerifiedFindings(candidates, results)

	// Filter by confidence thresholds
	reportable := filterByConfidence(verified, settings)

	return candidates, verified, reportable, nil
}

// convertVerifiedToFindings converts verified findings back to regular findings
// for backward compatibility with existing GitHub poster and markdown writer.
func convertVerifiedToFindings(verified []domain.VerifiedFinding) []domain.Finding {
	findings := make([]domain.Finding, 0, len(verified))
	for _, v := range verified {
		findings = append(findings, v.Finding)
	}
	return findings
}

// logVerificationDetails logs detailed verification results for each finding.
// This provides visibility into what was filtered and why.
func logVerificationDetails(ctx context.Context, verified []domain.VerifiedFinding, reportable []domain.VerifiedFinding, settings VerificationSettings, logger Logger) {
	// Build set of reportable findings for quick lookup
	reportableSet := make(map[string]bool)
	for _, f := range reportable {
		key := fmt.Sprintf("%s:%d", f.Finding.File, f.Finding.LineStart)
		reportableSet[key] = true
	}

	// Log header
	log.Println("=== VERIFICATION REPORT ===")
	log.Printf("Total findings: %d | Reportable: %d | Filtered: %d",
		len(verified), len(reportable), len(verified)-len(reportable))
	log.Println("")

	// Log each finding's verification result
	for i, v := range verified {
		key := fmt.Sprintf("%s:%d", v.Finding.File, v.Finding.LineStart)
		isReportable := reportableSet[key]
		threshold := getThresholdForSeverity(v.Finding.Severity, settings)

		// Determine filter reason
		filterReason := ""
		if !isReportable {
			if !v.Verified {
				filterReason = "NOT_VERIFIED"
			} else if v.Confidence < threshold {
				filterReason = fmt.Sprintf("CONFIDENCE_BELOW_THRESHOLD(%d<%d)", v.Confidence, threshold)
			}
		}

		// Status indicator
		status := "✓ PASS"
		if !isReportable {
			status = "✗ FILTERED"
		}

		// Log the finding
		log.Printf("[%d] %s | %s:%d | %s | confidence=%d threshold=%d",
			i+1, status, v.Finding.File, v.Finding.LineStart, v.Finding.Severity, v.Confidence, threshold)
		log.Printf("    Description: %.80s...", truncateString(v.Finding.Description, 80))
		log.Printf("    Verified: %t | Classification: %s", v.Verified, v.Classification)
		log.Printf("    Evidence: %.100s", truncateString(v.Evidence, 100))
		if filterReason != "" {
			log.Printf("    Filter Reason: %s", filterReason)
		}
		log.Println("")
	}

	log.Println("=== END VERIFICATION REPORT ===")
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
