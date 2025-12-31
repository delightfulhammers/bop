package markdown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

type clock func() string

// Writer renders provider reviews into Markdown files.
type Writer struct {
	now clock
}

// NewWriter constructs a Markdown writer with a timestamp supplier.
func NewWriter(now clock) *Writer {
	return &Writer{now: now}
}

// Write persists a Markdown artifact to disk.
func (w *Writer) Write(ctx context.Context, artifact domain.MarkdownArtifact) (string, error) {
	if err := os.MkdirAll(artifact.OutputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	filename := fmt.Sprintf("%s_%s_%s_%s.md",
		sanitise(artifact.Repository),
		sanitise(artifact.TargetRef),
		sanitise(artifact.ProviderName),
		w.now(),
	)
	path := filepath.Join(artifact.OutputDir, filename)

	content := buildContent(artifact)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write markdown: %w", err)
	}

	return path, nil
}

func buildContent(artifact domain.MarkdownArtifact) string {
	var builder strings.Builder
	caser := cases.Title(language.English)
	builder.WriteString("# Code Review Report\n\n")
	builder.WriteString(fmt.Sprintf("- Provider: %s (%s)\n", artifact.Review.ProviderName, artifact.Review.ModelName))
	builder.WriteString(fmt.Sprintf("- Base: %s\n", artifact.BaseRef))
	builder.WriteString(fmt.Sprintf("- Target: %s\n", artifact.TargetRef))
	builder.WriteString(fmt.Sprintf("- Tokens: %d in / %d out\n", artifact.Review.TokensIn, artifact.Review.TokensOut))
	builder.WriteString(fmt.Sprintf("- Cost: $%.4f\n", artifact.Review.Cost))

	// Show reviewer summary if findings have reviewer attribution
	reviewerStats := collectReviewerStats(artifact.Review.Findings)
	if len(reviewerStats) > 0 {
		builder.WriteString("\n### Reviewers\n")
		for _, stat := range reviewerStats {
			builder.WriteString(fmt.Sprintf("- **%s** (weight: %.1f): %d findings\n",
				stat.Name, stat.Weight, stat.Count))
		}
	}
	builder.WriteString("\n")

	// Include truncation warning if PR was too large
	if artifact.Review.WasTruncated {
		builder.WriteString("## ⚠️ Size Limit Warning\n\n")
		builder.WriteString("> **This review may be incomplete.** ")
		builder.WriteString("The PR exceeded the token limit and some files were excluded from review.\n\n")
		if artifact.Review.TruncationWarning != "" {
			builder.WriteString(artifact.Review.TruncationWarning)
			builder.WriteString("\n\n")
		}
		if len(artifact.Review.TruncatedFiles) > 0 {
			builder.WriteString("**Files excluded from review:**\n")
			for _, f := range artifact.Review.TruncatedFiles {
				builder.WriteString(fmt.Sprintf("- `%s`\n", escapeMarkdownInlineCode(f)))
			}
			builder.WriteString("\n")
		}
	} else if artifact.Review.SizeLimitExceeded {
		builder.WriteString("## ⚠️ Large PR Notice\n\n")
		builder.WriteString("> This PR is approaching the token limit. Consider splitting into smaller PRs for more thorough reviews.\n\n")
	}

	// Include verification summary if verification was performed
	if len(artifact.Review.DiscoveryFindings) > 0 {
		builder.WriteString("## Verification Summary\n\n")
		builder.WriteString(fmt.Sprintf("- Discovery findings: %d\n", len(artifact.Review.DiscoveryFindings)))
		builder.WriteString(fmt.Sprintf("- Verified findings: %d\n", len(artifact.Review.VerifiedFindings)))
		builder.WriteString(fmt.Sprintf("- Reportable findings: %d\n\n", len(artifact.Review.ReportableFindings)))
	}

	builder.WriteString("## Summary\n\n")
	builder.WriteString(artifact.Review.Summary)
	builder.WriteString("\n\n")

	if len(artifact.Review.Findings) == 0 {
		builder.WriteString("No findings reported.\n")
		return builder.String()
	}

	builder.WriteString("## Findings\n\n")

	// If we have verified findings, use those for richer output
	if len(artifact.Review.ReportableFindings) > 0 {
		for _, vf := range artifact.Review.ReportableFindings {
			builder.WriteString(fmt.Sprintf("### %s (%s)\n", vf.Finding.Description, caser.String(vf.Finding.Severity)))
			builder.WriteString(fmt.Sprintf("- File: %s:%d-%d\n", vf.Finding.File, vf.Finding.LineStart, vf.Finding.LineEnd))
			builder.WriteString(fmt.Sprintf("- Category: %s\n", vf.Finding.Category))
			builder.WriteString(fmt.Sprintf("- Suggestion: %s\n", vf.Finding.Suggestion))
			builder.WriteString(fmt.Sprintf("- Classification: %s\n", vf.Classification))
			builder.WriteString(fmt.Sprintf("- Confidence: %d%%\n", vf.Confidence))
			if vf.BlocksOperation {
				builder.WriteString("- Blocks Operation: Yes\n")
			}
			if vf.Evidence != "" {
				builder.WriteString(fmt.Sprintf("- Verification Evidence: %s\n", vf.Evidence))
			}
			builder.WriteString("\n")
		}
	} else {
		// Group findings by reviewer if they have reviewer attribution
		groupedFindings := groupFindingsByReviewer(artifact.Review.Findings)

		if len(groupedFindings) > 1 {
			// Multiple reviewers: show grouped sections
			for _, group := range groupedFindings {
				if group.ReviewerName != "" {
					builder.WriteString(fmt.Sprintf("### %s (weight: %.1f)\n\n", group.ReviewerName, group.ReviewerWeight))
				}
				for _, finding := range group.Findings {
					writeFinding(&builder, finding, caser)
				}
			}
		} else {
			// Single or no reviewer: flat list
			for _, finding := range artifact.Review.Findings {
				writeFinding(&builder, finding, caser)
			}
		}
	}

	return builder.String()
}

// reviewerStat holds statistics for a single reviewer.
type reviewerStat struct {
	Name   string
	Weight float64
	Count  int
}

// collectReviewerStats gathers finding counts per reviewer.
func collectReviewerStats(findings []domain.Finding) []reviewerStat {
	stats := make(map[string]*reviewerStat)
	order := make([]string, 0)

	for _, f := range findings {
		if f.ReviewerName == "" {
			continue
		}
		if _, exists := stats[f.ReviewerName]; !exists {
			stats[f.ReviewerName] = &reviewerStat{
				Name:   f.ReviewerName,
				Weight: f.ReviewerWeight,
			}
			order = append(order, f.ReviewerName)
		}
		stats[f.ReviewerName].Count++
	}

	result := make([]reviewerStat, 0, len(order))
	for _, name := range order {
		result = append(result, *stats[name])
	}
	return result
}

// findingGroup holds findings from a single reviewer.
type findingGroup struct {
	ReviewerName   string
	ReviewerWeight float64
	Findings       []domain.Finding
}

// groupFindingsByReviewer organizes findings by their reviewer attribution.
func groupFindingsByReviewer(findings []domain.Finding) []findingGroup {
	groups := make(map[string]*findingGroup)
	order := make([]string, 0)

	for _, f := range findings {
		key := f.ReviewerName
		if key == "" {
			key = "_unattributed_"
		}
		if _, exists := groups[key]; !exists {
			groups[key] = &findingGroup{
				ReviewerName:   f.ReviewerName,
				ReviewerWeight: f.ReviewerWeight,
				Findings:       make([]domain.Finding, 0),
			}
			order = append(order, key)
		}
		groups[key].Findings = append(groups[key].Findings, f)
	}

	result := make([]findingGroup, 0, len(order))
	for _, key := range order {
		result = append(result, *groups[key])
	}
	return result
}

// writeFinding writes a single finding in markdown format.
func writeFinding(builder *strings.Builder, finding domain.Finding, caser cases.Caser) {
	_, _ = fmt.Fprintf(builder, "#### %s (%s)\n", finding.Description, caser.String(finding.Severity))
	_, _ = fmt.Fprintf(builder, "- File: %s:%d-%d\n", finding.File, finding.LineStart, finding.LineEnd)
	_, _ = fmt.Fprintf(builder, "- Category: %s\n", finding.Category)
	_, _ = fmt.Fprintf(builder, "- Suggestion: %s\n", finding.Suggestion)
	if finding.Evidence {
		builder.WriteString("- Evidence: Provided\n")
	} else {
		builder.WriteString("- Evidence: Not provided\n")
	}
	builder.WriteString("\n")
}

func sanitise(value string) string {
	if value == "" {
		return "unknown"
	}
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, string(filepath.Separator), "-")
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

// escapeMarkdownInlineCode escapes characters that could break inline code formatting.
// Specifically handles backticks and newlines which would break `code` spans.
func escapeMarkdownInlineCode(s string) string {
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}
