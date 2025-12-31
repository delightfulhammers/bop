package merge

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/store"
)

// PrecisionStore defines the interface for accessing precision priors.
type PrecisionStore interface {
	GetPrecisionPriors(ctx context.Context) (map[string]map[string]store.PrecisionPrior, error)
}

// SynthesisProvider defines the interface for LLM-based summary synthesis.
// This is intentionally a simple interface to avoid circular dependencies with review.Provider.
type SynthesisProvider interface {
	Review(ctx context.Context, prompt string, seed uint64) (string, error)
}

// IntelligentMerger merges reviews with scoring, grouping, and synthesis.
type IntelligentMerger struct {
	store PrecisionStore

	// Scoring weights (should sum to 1.0)
	agreementWeight float64
	severityWeight  float64
	precisionWeight float64
	evidenceWeight  float64

	// Similarity threshold for grouping (0.0-1.0)
	similarityThreshold float64

	// LLM-based synthesis (optional)
	synthProvider SynthesisProvider // Provider for summary synthesis (can be nil)
	useLLM        bool              // Use LLM for synthesis vs simple concatenation

	// Phase 3.2: Reviewer Personas
	// weightByReviewer applies per-reviewer weights to agreement scoring.
	// When true, agreement score sums reviewer weights instead of counting reviewers.
	weightByReviewer bool

	// respectFocus prevents penalizing low agreement for specialized reviewers.
	// When true, findings with a ReviewerName are not penalized for lack of agreement.
	respectFocus bool
}

// NewIntelligentMerger creates a new intelligent merger with default weights.
func NewIntelligentMerger(store PrecisionStore) *IntelligentMerger {
	return &IntelligentMerger{
		store:               store,
		agreementWeight:     0.4,
		severityWeight:      0.3,
		precisionWeight:     0.2,
		evidenceWeight:      0.1,
		similarityThreshold: 0.3, // Lowered from 0.7 to group similar issues better
		synthProvider:       nil,
		useLLM:              false,
		weightByReviewer:    true, // Phase 3.2: Default to reviewer-weighted scoring
		respectFocus:        true, // Phase 3.2: Default to respecting focused reviewers
	}
}

// WithReviewerWeighting configures whether to apply reviewer weights to scoring.
func (m *IntelligentMerger) WithReviewerWeighting(enabled bool) *IntelligentMerger {
	m.weightByReviewer = enabled
	return m
}

// WithRespectFocus configures whether to skip agreement penalty for specialized reviewers.
func (m *IntelligentMerger) WithRespectFocus(enabled bool) *IntelligentMerger {
	m.respectFocus = enabled
	return m
}

// WithSynthesisProvider configures LLM-based summary synthesis.
func (m *IntelligentMerger) WithSynthesisProvider(provider SynthesisProvider) *IntelligentMerger {
	m.synthProvider = provider
	m.useLLM = true
	return m
}

// findingGroup represents a group of similar findings.
type findingGroup struct {
	findings  []domain.Finding
	providers map[string]bool // Set of providers that found this issue (legacy)

	// Phase 3.2: Reviewer Personas
	// reviewers tracks which reviewers found this issue and their weights.
	// Key is reviewer name, value is reviewer weight.
	reviewers map[string]float64
}

// Merge combines multiple reviews intelligently using scoring and grouping.
func (m *IntelligentMerger) Merge(ctx context.Context, reviews []domain.Review) domain.Review {
	// Group similar findings
	groups := m.groupSimilarFindings(reviews)

	// Score each group
	scoredGroups := make([]scoredGroup, 0, len(groups))
	for _, group := range groups {
		score := m.scoreGroup(ctx, group)
		scoredGroups = append(scoredGroups, scoredGroup{
			group: group,
			score: score,
		})
	}

	// Sort by score (descending)
	sortByScore(scoredGroups)

	// Select representative finding from each group
	findings := make([]domain.Finding, 0, len(scoredGroups))
	for _, sg := range scoredGroups {
		representative := m.selectRepresentative(sg.group)
		findings = append(findings, representative)
	}

	// Synthesize summary
	summary := m.synthesizeSummary(reviews)

	// Aggregate usage metadata from all providers
	var totalTokensIn, totalTokensOut int
	var totalCost float64
	for _, review := range reviews {
		totalTokensIn += review.TokensIn
		totalTokensOut += review.TokensOut
		totalCost += review.Cost
	}

	return domain.Review{
		ProviderName: "merged",
		ModelName:    "consensus",
		Summary:      summary,
		Findings:     findings,
		TokensIn:     totalTokensIn,
		TokensOut:    totalTokensOut,
		Cost:         totalCost,
	}
}

// groupSimilarFindings groups findings that are likely the same issue.
func (m *IntelligentMerger) groupSimilarFindings(reviews []domain.Review) []findingGroup {
	var groups []findingGroup
	processedIDs := make(map[string]bool)

	for _, review := range reviews {
		for _, finding := range review.Findings {
			if processedIDs[finding.ID] {
				continue
			}

			// Create new group or find existing similar group
			var targetGroup *findingGroup
			for i := range groups {
				if m.areSimilar(finding, groups[i].findings[0]) {
					targetGroup = &groups[i]
					break
				}
			}

			if targetGroup == nil {
				// Create new group
				newGroup := findingGroup{
					findings:  []domain.Finding{finding},
					providers: map[string]bool{review.ProviderName: true},
					reviewers: make(map[string]float64),
				}
				// Track reviewer if present (Phase 3.2)
				if finding.ReviewerName != "" {
					newGroup.reviewers[finding.ReviewerName] = finding.ReviewerWeight
				}
				groups = append(groups, newGroup)
			} else {
				// Add to existing group
				targetGroup.findings = append(targetGroup.findings, finding)
				targetGroup.providers[review.ProviderName] = true
				// Track reviewer if present (Phase 3.2)
				if finding.ReviewerName != "" {
					targetGroup.reviewers[finding.ReviewerName] = finding.ReviewerWeight
				}
			}

			processedIDs[finding.ID] = true
		}
	}

	return groups
}

// areSimilar determines if two findings are likely the same issue.
func (m *IntelligentMerger) areSimilar(a, b domain.Finding) bool {
	// Must be same file
	if a.File != b.File {
		return false
	}

	// Check line overlap
	if !linesOverlap(a.LineStart, a.LineEnd, b.LineStart, b.LineEnd) {
		return false
	}

	// Check description similarity
	similarity := stringSimilarity(a.Description, b.Description)
	return similarity >= m.similarityThreshold
}

// linesOverlap checks if two line ranges overlap.
func linesOverlap(start1, end1, start2, end2 int) bool {
	// Handle cases where LineEnd might be 0 (single line)
	if end1 == 0 {
		end1 = start1
	}
	if end2 == 0 {
		end2 = start2
	}

	// Check for overlap
	return start1 <= end2 && start2 <= end1
}

// stringSimilarity computes similarity between two strings (0.0-1.0).
// Uses simple word-based Jaccard similarity.
func stringSimilarity(a, b string) float64 {
	wordsA := strings.Fields(strings.ToLower(a))
	wordsB := strings.Fields(strings.ToLower(b))

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0.0
	}

	// Create word sets
	setA := make(map[string]bool)
	setB := make(map[string]bool)

	for _, word := range wordsA {
		setA[word] = true
	}
	for _, word := range wordsB {
		setB[word] = true
	}

	// Count intersection
	intersection := 0
	for word := range setA {
		if setB[word] {
			intersection++
		}
	}

	// Jaccard similarity: |A ∩ B| / |A ∪ B|
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// scoreGroup calculates a score for a finding group.
func (m *IntelligentMerger) scoreGroup(ctx context.Context, group findingGroup) float64 {
	if len(group.findings) == 0 {
		return 0.0
	}

	// Agreement component: how many providers/reviewers found this
	agreementScore := m.calculateAgreementScore(group)

	// Severity component: average severity score
	severityScore := m.averageSeverityScore(group.findings)

	// Precision component: average precision prior for providers
	precisionScore := m.averagePrecisionScore(ctx, group)

	// Evidence component: ratio of findings with evidence
	evidenceScore := m.evidenceRatio(group.findings)

	// Weighted sum
	totalScore := (m.agreementWeight * agreementScore) +
		(m.severityWeight * severityScore) +
		(m.precisionWeight * precisionScore) +
		(m.evidenceWeight * evidenceScore)

	return totalScore
}

// calculateAgreementScore computes the agreement score for a finding group.
// When weightByReviewer is enabled, it sums reviewer weights instead of counting.
// When respectFocus is enabled, specialized reviewers are not penalized for low agreement.
func (m *IntelligentMerger) calculateAgreementScore(group findingGroup) float64 {
	// If we have reviewers tracked and weightByReviewer is enabled, use weighted scoring
	if m.weightByReviewer && len(group.reviewers) > 0 {
		var weightedSum float64
		for _, weight := range group.reviewers {
			// Use weight of 1.0 if not set (default)
			if weight <= 0 {
				weight = 1.0
			}
			weightedSum += weight
		}
		return weightedSum
	}

	// Fall back to provider count (legacy behavior)
	agreementScore := float64(len(group.providers))

	// If respectFocus is enabled and this is from a specialized reviewer,
	// don't penalize for lack of agreement (give minimum score of 1.0)
	if m.respectFocus && m.hasSpecializedReviewer(group) {
		if agreementScore < 1.0 {
			agreementScore = 1.0
		}
	}

	return agreementScore
}

// hasSpecializedReviewer returns true if any finding in the group has a ReviewerName.
// This indicates the finding came from a specialized reviewer persona.
func (m *IntelligentMerger) hasSpecializedReviewer(group findingGroup) bool {
	return len(group.reviewers) > 0
}

// averageSeverityScore converts severity to numeric score and averages.
func (m *IntelligentMerger) averageSeverityScore(findings []domain.Finding) float64 {
	if len(findings) == 0 {
		return 0.0
	}

	total := 0.0
	for _, f := range findings {
		total += severityToScore(f.Severity)
	}

	return total / float64(len(findings))
}

// severityToScore converts severity string to numeric score.
func severityToScore(severity string) float64 {
	switch strings.ToLower(severity) {
	case "error", "critical":
		return 1.0
	case "warning":
		return 0.6
	case "info":
		return 0.3
	default:
		return 0.0
	}
}

// averagePrecisionScore gets average precision for providers in this group.
func (m *IntelligentMerger) averagePrecisionScore(ctx context.Context, group findingGroup) float64 {
	if m.store == nil || len(group.findings) == 0 {
		return 0.5 // Default to medium precision if no store
	}

	priors, err := m.store.GetPrecisionPriors(ctx)
	if err != nil {
		return 0.5 // Default on error
	}

	// Get average precision across providers
	total := 0.0
	count := 0

	for provider := range group.providers {
		for _, finding := range group.findings {
			if categoryPriors, ok := priors[provider]; ok {
				if prior, ok := categoryPriors[finding.Category]; ok {
					// Beta distribution mean: alpha / (alpha + beta)
					precision := prior.Alpha / (prior.Alpha + prior.Beta)
					total += precision
					count++
				}
			}
		}
	}

	if count == 0 {
		return 0.5 // Default if no priors found
	}

	return total / float64(count)
}

// evidenceRatio computes the ratio of findings with evidence.
func (m *IntelligentMerger) evidenceRatio(findings []domain.Finding) float64 {
	if len(findings) == 0 {
		return 0.0
	}

	count := 0
	for _, f := range findings {
		if f.Evidence {
			count++
		}
	}

	return float64(count) / float64(len(findings))
}

// selectRepresentative chooses the best finding from a group.
func (m *IntelligentMerger) selectRepresentative(group findingGroup) domain.Finding {
	if len(group.findings) == 0 {
		return domain.Finding{}
	}

	// Prefer findings with evidence
	for _, f := range group.findings {
		if f.Evidence {
			return f
		}
	}

	// Prefer higher severity
	best := group.findings[0]
	bestScore := severityToScore(best.Severity)

	for _, f := range group.findings[1:] {
		score := severityToScore(f.Severity)
		if score > bestScore {
			best = f
			bestScore = score
		}
	}

	return best
}

// synthesizeSummary creates a summary from multiple review summaries.
// If useLLM is true and synthProvider is available, uses LLM to generate cohesive narrative.
// Falls back to concatenation if LLM fails or is disabled.
func (m *IntelligentMerger) synthesizeSummary(reviews []domain.Review) string {
	if len(reviews) == 0 {
		return "No reviews to merge."
	}

	if len(reviews) == 1 {
		return reviews[0].Summary
	}

	// Try LLM-based synthesis if enabled
	if m.useLLM && m.synthProvider != nil {
		prompt := buildSynthesisPrompt(reviews)
		ctx := context.Background()

		// Use synthesis provider (typically a cheap, fast model like gpt-4o-mini)
		synthesizedSummary, err := m.synthProvider.Review(ctx, prompt, 0)
		if err == nil && synthesizedSummary != "" {
			return synthesizedSummary
		}
		// Fall through to concatenation on error
	}

	// Fall back to simple concatenation (original behavior)
	return concatenateSummaries(reviews)
}

// buildSynthesisPrompt creates a prompt for LLM-based summary synthesis.
func buildSynthesisPrompt(reviews []domain.Review) string {
	var prompt strings.Builder

	prompt.WriteString("You are synthesizing code review results from multiple AI providers. ")
	prompt.WriteString("Create a cohesive, professional summary (200-300 words) that:\n\n")
	prompt.WriteString("1. Identifies key themes and patterns across all reviews\n")
	prompt.WriteString("2. Highlights areas of agreement between providers\n")
	prompt.WriteString("3. Notes any significant disagreements or unique findings\n")
	prompt.WriteString("4. Prioritizes critical and high-severity issues\n")
	prompt.WriteString("5. Provides actionable recommendations\n\n")
	prompt.WriteString("Input reviews:\n\n")

	for _, review := range reviews {
		prompt.WriteString(fmt.Sprintf("**%s (%s)** - %d findings:\n",
			review.ProviderName, review.ModelName, len(review.Findings)))
		prompt.WriteString(review.Summary)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("Synthesize the above reviews into a cohesive summary. ")
	prompt.WriteString("Focus on the most important issues and provide clear next steps. ")
	prompt.WriteString("Do not repeat individual provider names unless highlighting disagreement.")

	return prompt.String()
}

// concatenateSummaries provides simple concatenation (original behavior).
func concatenateSummaries(reviews []domain.Review) string {
	var parts []string
	for _, review := range reviews {
		if review.Summary != "" {
			// Take first sentence or first 100 chars
			summary := review.Summary
			if idx := strings.Index(summary, "."); idx > 0 && idx < 100 {
				summary = summary[:idx+1]
			} else if len(summary) > 100 {
				summary = summary[:100] + "..."
			}
			parts = append(parts, fmt.Sprintf("%s: %s", review.ProviderName, summary))
		}
	}

	if len(parts) == 0 {
		return "Multiple providers completed the review."
	}

	return strings.Join(parts, " | ")
}

// scoredGroup pairs a group with its score for sorting.
type scoredGroup struct {
	group findingGroup
	score float64
}

// sortByScore sorts scored groups by score (descending).
func sortByScore(groups []scoredGroup) {
	// Simple bubble sort (good enough for small n)
	n := len(groups)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if groups[j].score < groups[j+1].score {
				groups[j], groups[j+1] = groups[j+1], groups[j]
			}
		}
	}
}

// Compile-time check that IntelligentMerger can be used as a Merger
var _ interface {
	Merge(context.Context, []domain.Review) domain.Review
} = (*IntelligentMerger)(nil)

// Helper function to avoid unused import error
func init() {
	_ = math.Abs(0)
}
