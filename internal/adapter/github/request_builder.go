package github

import (
	"fmt"
	"strings"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// fingerprintMarkerStart is the HTML comment prefix for embedding fingerprints.
// This marker is invisible in rendered markdown but can be extracted for reply matching.
const fingerprintMarkerStart = "<!-- CR_FP:"

// reviewerMarkerPrefix is the prefix for embedding reviewer name in the same HTML comment.
const reviewerMarkerPrefix = " CR_REVIEWER:"

// fingerprintMarkerEnd closes the HTML comment containing the fingerprint and reviewer.
const fingerprintMarkerEnd = " -->"

// legacyFingerprintMarker is the old format for backward compatibility when parsing.
const legacyFingerprintMarker = "<!-- CR_FINGERPRINT:"

// ReviewActions configures the GitHub review action for each finding severity level.
// This mirrors config.ReviewActions but lives in the adapter layer to avoid coupling.
type ReviewActions struct {
	OnCritical    string // Action for critical severity findings
	OnHigh        string // Action for high severity findings
	OnMedium      string // Action for medium severity findings
	OnLow         string // Action for low severity findings
	OnClean       string // Action when no findings in diff
	OnNonBlocking string // Action when findings exist but none trigger REQUEST_CHANGES

	// AlwaysBlockCategories lists finding categories that always trigger REQUEST_CHANGES
	// regardless of severity. This provides an additive override for specific categories
	// like "security" that should always block, even if severity-based config wouldn't.
	AlwaysBlockCategories []string
}

// NormalizeAction converts a string action to ReviewEvent.
// It handles case-insensitive input and common variations.
// Returns (event, true) if valid, (EventComment, false) if invalid.
func NormalizeAction(action string) (ReviewEvent, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(action))
	// Handle hyphenated variant
	normalized = strings.ReplaceAll(normalized, "-", "_")

	switch normalized {
	case "APPROVE":
		return EventApprove, true
	case "REQUEST_CHANGES":
		return EventRequestChanges, true
	case "COMMENT":
		return EventComment, true
	default:
		return EventComment, false
	}
}

// defaultBlockingSeverities defines which severities trigger REQUEST_CHANGES by default.
// Critical and high block by default; medium and low do not.
var defaultBlockingSeverities = map[string]bool{
	"critical": true,
	"high":     true,
	"medium":   false,
	"low":      false,
}

// BuildReviewComments converts positioned findings to GitHub review comments.
// Only findings with a valid DiffPosition (InDiff() == true) are included.
// Each comment includes an embedded fingerprint for linking replies to findings.
// This function is pure and does not modify the input.
func BuildReviewComments(findings []PositionedFinding) []ReviewComment {
	var comments []ReviewComment

	for _, pf := range findings {
		if !pf.InDiff() {
			continue
		}

		// Compute fingerprint and embed in comment body
		fingerprint := domain.FingerprintFromFinding(pf.Finding)
		body := FormatFindingCommentWithFingerprint(pf.Finding, fingerprint)

		comments = append(comments, ReviewComment{
			Path:     pf.Finding.File,
			Position: *pf.DiffPosition,
			Body:     body,
		})
	}

	return comments
}

// FormatFindingComment formats a domain.Finding as a GitHub-flavored Markdown comment.
func FormatFindingComment(f domain.Finding) string {
	var sb strings.Builder

	// Header with severity and category
	sb.WriteString(fmt.Sprintf("**Severity:** %s", f.Severity))
	if f.Category != "" {
		sb.WriteString(fmt.Sprintf(" | **Category:** %s", f.Category))
	}
	sb.WriteString("\n\n")

	// Line reference
	if f.LineStart == f.LineEnd || f.LineEnd == 0 {
		sb.WriteString(fmt.Sprintf("📍 Line %d\n\n", f.LineStart))
	} else {
		sb.WriteString(fmt.Sprintf("📍 Lines %d-%d\n\n", f.LineStart, f.LineEnd))
	}

	// Description
	sb.WriteString(f.Description)
	sb.WriteString("\n")

	// Suggestion if present
	if f.Suggestion != "" {
		sb.WriteString("\n**Suggestion:** ")
		sb.WriteString(f.Suggestion)
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatFindingCommentWithFingerprint formats a finding as a GitHub comment with embedded fingerprint.
// The fingerprint and optional reviewer name are stored in an HTML comment that is invisible
// when rendered but can be extracted to link replies back to the original finding.
//
// Format: <!-- CR_FP:abc123 CR_REVIEWER:security -->
// If no reviewer is set, only the fingerprint is included: <!-- CR_FP:abc123 -->
func FormatFindingCommentWithFingerprint(f domain.Finding, fingerprint domain.FindingFingerprint) string {
	var sb strings.Builder

	// Start with the standard format
	sb.WriteString(FormatFindingComment(f))

	// Add fingerprint (and optional reviewer) in hidden HTML comment
	sb.WriteString("\n")
	sb.WriteString(fingerprintMarkerStart)
	sb.WriteString(string(fingerprint))

	// Include reviewer name if present (Phase 3.2)
	if f.ReviewerName != "" {
		sb.WriteString(reviewerMarkerPrefix)
		sb.WriteString(f.ReviewerName)
	}

	sb.WriteString(fingerprintMarkerEnd)

	return sb.String()
}

// fingerprintLength is the expected length of a fingerprint (32 hex chars from 16 bytes of SHA256).
const fingerprintLength = 32

// ExtractFingerprintFromComment extracts the finding fingerprint from a GitHub comment body.
// Returns the fingerprint and true if found and valid, or empty and false if not present or invalid.
// This is used to link reply comments back to their original findings.
//
// Supports both new format (<!-- CR_FP:abc123 CR_REVIEWER:security -->)
// and legacy format (<!-- CR_FINGERPRINT:abc123 -->) for backward compatibility.
//
// The function validates that the extracted fingerprint matches the expected format
// (32-character hexadecimal string) to prevent injection of arbitrary strings.
func ExtractFingerprintFromComment(body string) (domain.FindingFingerprint, bool) {
	// Try new format first
	startIdx := strings.Index(body, fingerprintMarkerStart)
	markerLen := len(fingerprintMarkerStart)

	// Fall back to legacy format if new format not found
	if startIdx == -1 {
		startIdx = strings.Index(body, legacyFingerprintMarker)
		markerLen = len(legacyFingerprintMarker)
	}

	if startIdx == -1 {
		return "", false
	}

	// Find the content start (after the marker)
	contentStart := startIdx + markerLen

	// Find the end marker
	remaining := body[contentStart:]
	endIdx := strings.Index(remaining, fingerprintMarkerEnd)
	if endIdx == -1 {
		return "", false
	}

	// Extract content between markers (may include reviewer tag)
	content := remaining[:endIdx]

	// If there's a reviewer tag, fingerprint is before it
	if reviewerIdx := strings.Index(content, reviewerMarkerPrefix); reviewerIdx != -1 {
		content = content[:reviewerIdx]
	}

	// Extract, trim, and normalize case (fingerprints are stored lowercase internally,
	// but might be edited to uppercase accidentally in GitHub comments)
	fp := strings.ToLower(strings.TrimSpace(content))
	if fp == "" {
		return "", false
	}

	// Validate fingerprint format: must be exactly 32 hexadecimal characters
	if !isValidFingerprint(fp) {
		return "", false
	}

	return domain.FindingFingerprint(fp), true
}

// ExtractReviewerFromComment extracts the reviewer name from a GitHub comment body.
// Returns the reviewer name and true if found, or empty and false if not present.
// This is used for Phase 3.2 triage context to filter findings by reviewer.
func ExtractReviewerFromComment(body string) (string, bool) {
	// Find the fingerprint marker first (reviewer is always after fingerprint)
	startIdx := strings.Index(body, fingerprintMarkerStart)
	if startIdx == -1 {
		return "", false
	}

	contentStart := startIdx + len(fingerprintMarkerStart)
	remaining := body[contentStart:]

	// Find the reviewer marker
	reviewerIdx := strings.Index(remaining, reviewerMarkerPrefix)
	if reviewerIdx == -1 {
		return "", false
	}

	// Find where the reviewer name ends (at the end marker)
	reviewerStart := reviewerIdx + len(reviewerMarkerPrefix)
	afterReviewer := remaining[reviewerStart:]
	endIdx := strings.Index(afterReviewer, fingerprintMarkerEnd)
	if endIdx == -1 {
		return "", false
	}

	reviewer := strings.TrimSpace(afterReviewer[:endIdx])
	if reviewer == "" {
		return "", false
	}

	return reviewer, true
}

// isValidFingerprint checks if a string is a valid fingerprint format.
// A valid fingerprint is exactly 32 lowercase hexadecimal characters.
func isValidFingerprint(fp string) bool {
	if len(fp) != fingerprintLength {
		return false
	}
	for _, c := range fp {
		// Check if character is not a valid hex digit (0-9 or a-f)
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// DetermineReviewEvent determines the appropriate ReviewEvent based on finding severities.
// This function uses the default review actions (legacy behavior).
// Returns:
//   - EventApprove if no findings (in diff)
//   - EventRequestChanges if any high or critical severity findings (in diff)
//   - EventComment otherwise
func DetermineReviewEvent(findings []PositionedFinding) ReviewEvent {
	// Use empty ReviewActions to trigger default/fallback behavior
	return DetermineReviewEventWithActions(findings, ReviewActions{})
}

// DetermineReviewEventWithActions determines the appropriate ReviewEvent based on
// finding severities and the provided action configuration.
//
// Returns:
//   - OnClean action if no findings in diff (default: APPROVE)
//   - OnNonBlocking action if findings exist but none trigger REQUEST_CHANGES (default: APPROVE)
//   - REQUEST_CHANGES if any in-diff finding triggers it based on severity config
//     (default: critical/high block, medium/low don't; configurable via OnCritical, etc.)
func DetermineReviewEventWithActions(findings []PositionedFinding, actions ReviewActions) ReviewEvent {
	inDiffFindings := filterInDiff(findings)

	// No findings = clean code
	if len(inDiffFindings) == 0 {
		if actions.OnClean != "" {
			if event, valid := NormalizeAction(actions.OnClean); valid {
				return event
			}
		}
		return EventApprove // default for clean
	}

	// Check if any in-diff finding would trigger REQUEST_CHANGES
	if HasBlockingFindings(inDiffFindings, actions) {
		return EventRequestChanges
	}

	// No blocking findings - use OnNonBlocking action
	if actions.OnNonBlocking != "" {
		if event, valid := NormalizeAction(actions.OnNonBlocking); valid {
			return event
		}
	}
	// Default for non-blocking: APPROVE (findings exist but are informational)
	return EventApprove
}

// wouldTriggerRequestChanges checks if the given action would result in REQUEST_CHANGES.
// If action is empty, uses defaultBlocking. If action is invalid (typo/unknown),
// falls back to defaultBlocking to prevent accidental approval from config errors.
func wouldTriggerRequestChanges(action string, defaultBlocking bool) bool {
	if action == "" {
		return defaultBlocking
	}
	event, valid := NormalizeAction(action)
	if !valid {
		// Invalid action string (typo) - fall back to default behavior
		// to prevent accidental approval from config errors
		return defaultBlocking
	}
	return event == EventRequestChanges
}

// HasBlockingFindings checks if any in-diff finding would trigger REQUEST_CHANGES
// based on the provided action configuration.
// This is exported so that summary_builder can use the same logic to determine
// whether to show "Approved with suggestions" prefix.
//
// A finding blocks if EITHER:
// - Its severity triggers REQUEST_CHANGES (via OnCritical/OnHigh/etc. or defaults)
// - Its category is in the AlwaysBlockCategories list (additive override)
func HasBlockingFindings(findings []PositionedFinding, actions ReviewActions) bool {
	inDiffFindings := filterInDiff(findings)
	if len(inDiffFindings) == 0 {
		return false
	}

	// Build action map from configuration
	actionMap := map[string]string{
		"critical": actions.OnCritical,
		"high":     actions.OnHigh,
		"medium":   actions.OnMedium,
		"low":      actions.OnLow,
	}

	// Build set of always-block categories (case-insensitive, trimmed)
	blockCategories := make(map[string]bool, len(actions.AlwaysBlockCategories))
	for _, cat := range actions.AlwaysBlockCategories {
		normalized := strings.ToLower(strings.TrimSpace(cat))
		if normalized != "" {
			blockCategories[normalized] = true
		}
	}

	// Check each finding against blocking configuration
	for _, pf := range inDiffFindings {
		// Check category-based blocking first (additive override)
		// Normalize category: trim whitespace and lowercase for matching
		normalizedCategory := strings.ToLower(strings.TrimSpace(pf.Finding.Category))
		if normalizedCategory != "" && blockCategories[normalizedCategory] {
			return true
		}

		// Check severity-based blocking
		severity := strings.ToLower(pf.Finding.Severity)
		defaultBlocking, known := defaultBlockingSeverities[severity]
		if !known {
			// Unknown severities don't block by default
			continue
		}
		if wouldTriggerRequestChanges(actionMap[severity], defaultBlocking) {
			return true
		}
	}
	return false
}

// CountInDiffFindings returns the count of findings that are in the diff.
func CountInDiffFindings(findings []PositionedFinding) int {
	count := 0
	for _, pf := range findings {
		if pf.InDiff() {
			count++
		}
	}
	return count
}

// filterInDiff returns only findings that are in the diff.
func filterInDiff(findings []PositionedFinding) []PositionedFinding {
	var result []PositionedFinding
	for _, pf := range findings {
		if pf.InDiff() {
			result = append(result, pf)
		}
	}
	return result
}

// ExtractedCommentDetails holds parsed information from a bot comment body.
// This is used for semantic deduplication where we need to compare finding content.
type ExtractedCommentDetails struct {
	Fingerprint domain.FindingFingerprint
	Severity    string
	Category    string
	Description string
	LineStart   int
	LineEnd     int
}

// ExtractCommentDetails parses a bot comment body to extract finding details.
// Returns nil if the comment doesn't appear to be a structured finding comment.
func ExtractCommentDetails(body string) *ExtractedCommentDetails {
	// Must have a fingerprint to be a valid finding comment
	fp, ok := ExtractFingerprintFromComment(body)
	if !ok {
		return nil
	}

	details := &ExtractedCommentDetails{
		Fingerprint: fp,
	}

	// Extract severity: **Severity:** value
	if idx := strings.Index(body, "**Severity:** "); idx != -1 {
		start := idx + len("**Severity:** ")
		end := start
		for end < len(body) && body[end] != '\n' && body[end] != '|' {
			end++
		}
		details.Severity = strings.TrimSpace(body[start:end])
	}

	// Extract category: | **Category:** value
	if idx := strings.Index(body, "**Category:** "); idx != -1 {
		start := idx + len("**Category:** ")
		end := start
		for end < len(body) && body[end] != '\n' {
			end++
		}
		details.Category = strings.TrimSpace(body[start:end])
	}

	// Extract line reference: 📍 Line X or 📍 Lines X-Y
	if idx := strings.Index(body, "📍 Line"); idx != -1 {
		start := idx + len("📍 Line")
		// Skip optional 's' for "Lines"
		if start < len(body) && body[start] == 's' {
			start++
		}
		// Skip whitespace
		for start < len(body) && (body[start] == ' ' || body[start] == '\t') {
			start++
		}
		// Parse first number
		numStart := start
		for start < len(body) && body[start] >= '0' && body[start] <= '9' {
			start++
		}
		if numStart < start {
			// Ignore parse error - LineStart stays 0 if unparseable
			_, _ = fmt.Sscanf(body[numStart:start], "%d", &details.LineStart)
			details.LineEnd = details.LineStart // Default to same line
		}
		// Check for range: -Y
		if start < len(body) && body[start] == '-' {
			start++ // skip '-'
			numStart = start
			for start < len(body) && body[start] >= '0' && body[start] <= '9' {
				start++
			}
			if numStart < start {
				// Ignore parse error - LineEnd stays at LineStart if unparseable
				_, _ = fmt.Sscanf(body[numStart:start], "%d", &details.LineEnd)
			}
		}
	}

	// Extract description: text between line reference and **Suggestion:** (or fingerprint if no suggestion)
	descStart := -1
	if idx := strings.Index(body, "📍 Line"); idx != -1 {
		// Find end of line reference line
		lineEnd := strings.Index(body[idx:], "\n")
		if lineEnd != -1 {
			descStart = idx + lineEnd + 1
			// Skip blank line after line reference
			for descStart < len(body) && body[descStart] == '\n' {
				descStart++
			}
		}
	}

	if descStart != -1 && descStart < len(body) {
		// Find end of description (before **Suggestion:** or fingerprint marker)
		descEnd := len(body)
		if idx := strings.Index(body[descStart:], "\n**Suggestion:**"); idx != -1 {
			descEnd = descStart + idx
		} else if idx := strings.Index(body[descStart:], fingerprintMarkerStart); idx != -1 {
			// New format: <!-- CR_FP:
			descEnd = descStart + idx
		} else if idx := strings.Index(body[descStart:], legacyFingerprintMarker); idx != -1 {
			// Legacy format: <!-- CR_FINGERPRINT:
			descEnd = descStart + idx
		}
		details.Description = strings.TrimSpace(body[descStart:descEnd])
	}

	return details
}
