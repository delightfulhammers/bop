package github

import (
	"fmt"
	"sort"
	"strings"

	"github.com/delightfulhammers/bop/internal/domain"
)

// SummaryAppendixOptions configures what sections to include in the appendix.
type SummaryAppendixOptions struct {
	// ExcludeOutOfDiff skips the "Findings Outside Diff" section.
	// Set to true when out-of-diff findings are posted as individual comments.
	ExcludeOutOfDiff bool
}

// BuildSummaryAppendix creates structured appendix sections for edge cases.
// Returns an empty string if there are no edge cases to report.
// The appendix includes sections for:
// - Findings outside diff (deleted lines, lines not in hunks) - unless excluded via opts
// - Binary files changed
// - Renamed files
func BuildSummaryAppendix(findings []PositionedFinding, d domain.Diff, opts ...SummaryAppendixOptions) string {
	var options SummaryAppendixOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	var sections []string

	// Section 1: Findings outside diff (unless excluded)
	if !options.ExcludeOutOfDiff {
		outOfDiff := FilterOutOfDiff(findings)
		if len(outOfDiff) > 0 {
			sections = append(sections, formatOutOfDiffSection(outOfDiff))
		}
	}

	// Section 2: Binary files changed
	binaryFiles := FilterBinaryFiles(d.Files)
	if len(binaryFiles) > 0 {
		sections = append(sections, formatBinaryFilesSection(binaryFiles))
	}

	// Section 3: Renamed files
	renamedFiles := FilterRenamedFiles(d.Files)
	if len(renamedFiles) > 0 {
		sections = append(sections, formatRenamedFilesSection(renamedFiles))
	}

	if len(sections) == 0 {
		return ""
	}

	return "\n\n---\n\n" + strings.Join(sections, "\n\n")
}

// AppendSections appends the summary appendix to the original summary.
// If the appendix is empty, returns the original summary unchanged.
func AppendSections(originalSummary, appendix string) string {
	if appendix == "" {
		return originalSummary
	}
	return originalSummary + appendix
}

// FilterOutOfDiff returns findings that are not in the diff (DiffPosition == nil).
func FilterOutOfDiff(findings []PositionedFinding) []PositionedFinding {
	var result []PositionedFinding
	for _, pf := range findings {
		if !pf.InDiff() {
			result = append(result, pf)
		}
	}
	return result
}

// FilterBinaryFiles returns files that are binary.
func FilterBinaryFiles(files []domain.FileDiff) []domain.FileDiff {
	var result []domain.FileDiff
	for _, f := range files {
		if f.IsBinary {
			result = append(result, f)
		}
	}
	return result
}

// FilterRenamedFiles returns files that were renamed.
func FilterRenamedFiles(files []domain.FileDiff) []domain.FileDiff {
	var result []domain.FileDiff
	for _, f := range files {
		if f.Status == domain.FileStatusRenamed {
			result = append(result, f)
		}
	}
	return result
}

// formatOutOfDiffSection formats the "Findings Outside Diff" section.
func formatOutOfDiffSection(findings []PositionedFinding) string {
	var sb strings.Builder

	sb.WriteString("## Findings Outside Diff\n\n")
	sb.WriteString("The following findings are on lines not included in this diff ")
	sb.WriteString("(e.g., deleted lines or unchanged context):\n\n")

	for _, pf := range findings {
		f := pf.Finding
		sb.WriteString(fmt.Sprintf("- **%s** in `%s` (line %d): %s\n",
			f.Severity, escapeMarkdownInlineCode(f.File), f.LineStart, f.Description))
	}

	return sb.String()
}

// formatBinaryFilesSection formats the "Binary Files Changed" section.
func formatBinaryFilesSection(files []domain.FileDiff) string {
	var sb strings.Builder

	sb.WriteString("## Binary Files Changed\n\n")
	sb.WriteString("The following binary files were changed and excluded from review:\n\n")

	for _, f := range files {
		sb.WriteString(fmt.Sprintf("- `%s` (%s)\n", escapeMarkdownInlineCode(f.Path), f.Status))
	}

	return sb.String()
}

// formatRenamedFilesSection formats the "Files Renamed" section.
func formatRenamedFilesSection(files []domain.FileDiff) string {
	var sb strings.Builder

	sb.WriteString("## Files Renamed\n\n")

	for _, f := range files {
		sb.WriteString(fmt.Sprintf("- `%s` → `%s`\n", escapeMarkdownInlineCode(f.OldPath), escapeMarkdownInlineCode(f.Path)))
	}

	return sb.String()
}

// =============================================================================
// Markdown Escaping Helpers
// =============================================================================

// escapeMarkdownInlineCode escapes characters that could break inline code formatting.
// Specifically handles backticks and newlines which would break `code` spans.
func escapeMarkdownInlineCode(s string) string {
	// Replace backticks with escaped version
	s = strings.ReplaceAll(s, "`", "\\`")
	// Replace newlines with space (newlines break inline code)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// escapeMarkdownTableCell escapes characters that could break table cell formatting.
// Specifically handles pipes and newlines which would break | cell | structure.
func escapeMarkdownTableCell(s string) string {
	// Replace pipes with escaped version
	s = strings.ReplaceAll(s, "|", "\\|")
	// Replace newlines with space (newlines break table rows)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// =============================================================================
// Truncation Warning Helpers
// =============================================================================

// FormatTruncationWarning creates a warning section for reviews that were truncated.
// Returns an empty string if the review was not truncated.
func FormatTruncationWarning(r domain.Review) string {
	if !r.WasTruncated && !r.SizeLimitExceeded {
		return ""
	}

	var sb strings.Builder

	if r.WasTruncated {
		sb.WriteString("## ⚠️ Incomplete Review\n\n")
		sb.WriteString("> **Warning:** This PR exceeded the token limit. ")
		sb.WriteString("Some files were excluded from review.\n\n")
		if r.TruncationWarning != "" {
			sb.WriteString(r.TruncationWarning)
			sb.WriteString("\n\n")
		}
		if len(r.TruncatedFiles) > 0 {
			sb.WriteString("**Files excluded from review:**\n")
			for _, f := range r.TruncatedFiles {
				sb.WriteString(fmt.Sprintf("- `%s`\n", escapeMarkdownInlineCode(f)))
			}
			sb.WriteString("\n")
		}
	} else if r.SizeLimitExceeded {
		sb.WriteString("> ⚠️ **Large PR Notice:** This PR is approaching the token limit. ")
		sb.WriteString("Consider splitting into smaller PRs for more thorough reviews.\n\n")
	}

	return sb.String()
}

// =============================================================================
// Programmatic Summary Builder
// =============================================================================

// BuildProgrammaticSummary generates a structured code review summary from findings.
// This replaces the LLM-generated summary with a consistent, programmatic format.
//
// The summary includes:
//   - Badge line with file count and severity counts (only in-diff findings)
//   - Files Requiring Attention section (files with severities that trigger REQUEST_CHANGES)
//   - Findings by Category table
//
// The actions parameter determines which severities appear in "Files Requiring Attention".
// Any severity configured to trigger REQUEST_CHANGES will be included.
// If actions is empty/default, critical and high severities are included.
//
// When findings exist but none trigger REQUEST_CHANGES, the summary shows
// "✅ Approved with suggestions" to indicate a non-blocking approval.
func BuildProgrammaticSummary(findings []PositionedFinding, d domain.Diff, actions ReviewActions) string {
	fileCount := len(d.Files)

	// Filter to only in-diff findings for counting
	inDiffFindings := filterInDiff(findings)

	// Count findings by severity
	counts := countBySeverity(inDiffFindings)
	totalFindings := counts["critical"] + counts["high"] + counts["medium"] + counts["low"]

	// Clean code case
	if totalFindings == 0 {
		return fmt.Sprintf("✅ **No issues found.** Reviewed %d files.", fileCount)
	}

	// Check if any finding would trigger REQUEST_CHANGES
	// Use the shared HasBlockingFindings function to ensure consistency
	// with DetermineReviewEventWithActions
	hasBlockingFindings := HasBlockingFindings(findings, actions)

	var sb strings.Builder

	// Show approval prefix only when the review will actually be APPROVE
	// (not when onNonBlocking is set to COMMENT)
	if !hasBlockingFindings {
		resolvedEvent := resolveNonBlockingEvent(actions)
		if resolvedEvent == EventApprove {
			sb.WriteString("✅ **Approved with suggestions.** ")
		}
	}

	attentionSeverities := getAttentionSeverities(actions)

	// Badge line
	sb.WriteString(formatBadgeLine(fileCount, counts))
	sb.WriteString("\n\n")

	// Files requiring attention (based on configured blocking severities)
	if section := formatFilesRequiringAttention(inDiffFindings, attentionSeverities); section != "" {
		sb.WriteString(section)
		sb.WriteString("\n")
	}

	// Category breakdown table
	categoryGroups := groupByCategory(inDiffFindings)
	if table := formatCategoryTable(categoryGroups); table != "" {
		sb.WriteString(table)
	}

	return strings.TrimRight(sb.String(), "\n")
}

// resolveNonBlockingEvent returns the event that will be used for non-blocking reviews.
// This mirrors the logic in DetermineReviewEventWithActions for the non-blocking case.
func resolveNonBlockingEvent(actions ReviewActions) ReviewEvent {
	if actions.OnNonBlocking != "" {
		if event, valid := NormalizeAction(actions.OnNonBlocking); valid {
			return event
		}
	}
	return EventApprove // default
}

// countBySeverity returns counts for each severity level.
func countBySeverity(findings []PositionedFinding) map[string]int {
	counts := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}
	for _, pf := range findings {
		severity := strings.ToLower(pf.Finding.Severity)
		if _, ok := counts[severity]; ok {
			counts[severity]++
		}
	}
	return counts
}

// formatBadgeLine creates the emoji badge summary line.
// Example: 📊 **Reviewed 12 files** | 🔴 2 critical | 🟠 5 high | 🟡 3 medium | 🟢 1 low
func formatBadgeLine(fileCount int, counts map[string]int) string {
	parts := []string{
		fmt.Sprintf("📊 **Reviewed %d files**", fileCount),
	}

	// Always show all severity levels for consistency
	parts = append(parts, fmt.Sprintf("🔴 %d critical", counts["critical"]))
	parts = append(parts, fmt.Sprintf("🟠 %d high", counts["high"]))
	parts = append(parts, fmt.Sprintf("🟡 %d medium", counts["medium"]))
	parts = append(parts, fmt.Sprintf("🟢 %d low", counts["low"]))

	return strings.Join(parts, " | ")
}

// getAttentionSeverities returns a map of severities that should appear in
// the "Files Requiring Attention" section. These are severities configured
// to trigger REQUEST_CHANGES.
func getAttentionSeverities(actions ReviewActions) map[string]bool {
	result := make(map[string]bool)

	checkAction := func(severity, action string, defaultBlocking bool) {
		if action == "" {
			// Use default behavior
			if defaultBlocking {
				result[severity] = true
			}
			return
		}
		if event, valid := NormalizeAction(action); valid && event == EventRequestChanges {
			result[severity] = true
		}
	}

	// Default: critical and high trigger request_changes
	checkAction("critical", actions.OnCritical, true)
	checkAction("high", actions.OnHigh, true)
	checkAction("medium", actions.OnMedium, false)
	checkAction("low", actions.OnLow, false)

	return result
}

// severityOrder defines the display order for severity levels (highest first).
var severityOrder = []string{"critical", "high", "medium", "low"}

// formatFilesRequiringAttention creates the "Files Requiring Attention" section.
// Only includes files with findings at attention-worthy severities.
func formatFilesRequiringAttention(findings []PositionedFinding, attentionSeverities map[string]bool) string {
	if len(attentionSeverities) == 0 {
		return ""
	}

	// Group findings by file, counting by severity (map-based approach)
	fileFindings := make(map[string]map[string]int)

	for _, pf := range findings {
		severity := strings.ToLower(pf.Finding.Severity)
		if !attentionSeverities[severity] {
			continue
		}

		if fileFindings[pf.Finding.File] == nil {
			fileFindings[pf.Finding.File] = make(map[string]int)
		}
		fileFindings[pf.Finding.File][severity]++
	}

	if len(fileFindings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### Files Requiring Attention\n\n")

	// Sort files for deterministic output
	var files []string
	for file := range fileFindings {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		counts := fileFindings[file]

		var badges []string
		for _, severity := range severityOrder {
			if count := counts[severity]; count > 0 {
				badges = append(badges, fmt.Sprintf("%d %s", count, severity))
			}
		}

		sb.WriteString(fmt.Sprintf("- `%s` (%s)\n", escapeMarkdownInlineCode(file), strings.Join(badges, ", ")))
	}

	return sb.String()
}

// groupByCategory groups findings by their category.
func groupByCategory(findings []PositionedFinding) map[string]int {
	groups := make(map[string]int)
	for _, pf := range findings {
		category := pf.Finding.Category
		if category == "" {
			category = "general"
		}
		groups[category]++
	}
	return groups
}

// formatCategoryTable creates the "Findings by Category" table.
func formatCategoryTable(categoryCounts map[string]int) string {
	if len(categoryCounts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### Findings by Category\n\n")
	sb.WriteString("| Category | Count |\n")
	sb.WriteString("|----------|-------|\n")

	// Sort categories for deterministic output
	var categories []string
	for cat := range categoryCounts {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", escapeMarkdownTableCell(cat), categoryCounts[cat]))
	}

	return sb.String()
}
