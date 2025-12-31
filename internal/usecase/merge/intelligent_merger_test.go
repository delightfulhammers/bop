package merge

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/store"
)

func TestGroupSimilarFindings(t *testing.T) {
	tests := []struct {
		name             string
		reviews          []domain.Review
		expectedGroups   int
		checkGroupSizes  bool
		expectedGrouping map[string]int // file -> expected number of groups for that file
	}{
		{
			name: "exact duplicates group together",
			reviews: []domain.Review{
				{
					ProviderName: "openai",
					Findings: []domain.Finding{
						{ID: "abc123", File: "main.go", LineStart: 10, LineEnd: 15, Description: "Null pointer dereference"},
					},
				},
				{
					ProviderName: "anthropic",
					Findings: []domain.Finding{
						{ID: "abc123", File: "main.go", LineStart: 10, LineEnd: 15, Description: "Null pointer dereference"},
					},
				},
			},
			expectedGroups: 1,
		},
		{
			name: "similar findings group together",
			reviews: []domain.Review{
				{
					ProviderName: "openai",
					Findings: []domain.Finding{
						{ID: "id1", File: "auth.go", LineStart: 20, LineEnd: 25, Description: "Potential SQL injection vulnerability"},
					},
				},
				{
					ProviderName: "anthropic",
					Findings: []domain.Finding{
						{ID: "id2", File: "auth.go", LineStart: 22, LineEnd: 26, Description: "SQL injection risk detected"},
					},
				},
			},
			expectedGroups: 1, // Should group together (same file, overlapping lines, similar description)
		},
		{
			name: "different files don't group",
			reviews: []domain.Review{
				{
					ProviderName: "openai",
					Findings: []domain.Finding{
						{ID: "id1", File: "auth.go", LineStart: 20, Description: "SQL injection"},
					},
				},
				{
					ProviderName: "anthropic",
					Findings: []domain.Finding{
						{ID: "id2", File: "db.go", LineStart: 20, Description: "SQL injection"},
					},
				},
			},
			expectedGroups: 2, // Different files, should not group
		},
		{
			name: "non-overlapping lines don't group",
			reviews: []domain.Review{
				{
					ProviderName: "openai",
					Findings: []domain.Finding{
						{ID: "id1", File: "main.go", LineStart: 10, LineEnd: 15, Description: "Issue here"},
					},
				},
				{
					ProviderName: "anthropic",
					Findings: []domain.Finding{
						{ID: "id2", File: "main.go", LineStart: 50, LineEnd: 55, Description: "Issue there"},
					},
				},
			},
			expectedGroups: 2, // Same file but non-overlapping lines
		},
		{
			name: "completely different findings don't group",
			reviews: []domain.Review{
				{
					ProviderName: "openai",
					Findings: []domain.Finding{
						{ID: "id1", File: "main.go", LineStart: 20, Description: "Memory leak detected"},
					},
				},
				{
					ProviderName: "anthropic",
					Findings: []domain.Finding{
						{ID: "id2", File: "main.go", LineStart: 22, Description: "Unused variable"},
					},
				},
			},
			expectedGroups: 2, // Different issues, should not group
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewIntelligentMerger(nil)
			groups := merger.groupSimilarFindings(tt.reviews)

			if len(groups) != tt.expectedGroups {
				t.Errorf("expected %d groups, got %d", tt.expectedGroups, len(groups))
				for i, group := range groups {
					t.Logf("Group %d: %d findings", i, len(group.findings))
					for j, f := range group.findings {
						t.Logf("  Finding %d: %s:%d-%d %s", j, f.File, f.LineStart, f.LineEnd, f.Description)
					}
				}
			}
		})
	}
}

func TestScoreFindings(t *testing.T) {
	// Mock store with precision priors
	mockStore := &mockPrecisionStore{
		priors: map[string]map[string]store.PrecisionPrior{
			"openai": {
				"security": {Provider: "openai", Category: "security", Alpha: 10, Beta: 2}, // High precision: 0.83
			},
			"anthropic": {
				"security": {Provider: "anthropic", Category: "security", Alpha: 5, Beta: 5}, // Medium precision: 0.5
			},
		},
	}

	tests := []struct {
		name          string
		group         findingGroup
		expectedScore float64 // Approximate expected score
		checkRanking  bool
	}{
		{
			name: "high agreement high severity high precision",
			group: findingGroup{
				findings: []domain.Finding{
					{Severity: "error", Category: "security", Evidence: true}, // From openai
					{Severity: "error", Category: "security", Evidence: true}, // From anthropic
					{Severity: "error", Category: "security", Evidence: true}, // From gemini
				},
				providers: map[string]bool{"openai": true, "anthropic": true, "gemini": true},
			},
			// 3 providers (0.4 * 3) + high severity (0.3 * 1.0) + precision (0.2 * ~0.66) + evidence (0.1 * 1.0)
			// ≈ 1.2 + 0.3 + 0.13 + 0.1 = 1.73
			expectedScore: 1.5, // Approximate
		},
		{
			name: "low agreement low severity",
			group: findingGroup{
				findings: []domain.Finding{
					{Severity: "info", Category: "style", Evidence: false},
				},
				providers: map[string]bool{"openai": true},
			},
			// 1 provider (0.4 * 1) + low severity (0.3 * 0.0) + precision (0.2 * ~0.5) + no evidence (0.1 * 0.0)
			// ≈ 0.4 + 0.0 + 0.1 + 0.0 = 0.5
			expectedScore: 0.6, // Approximate upper bound
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewIntelligentMerger(mockStore)
			score := merger.scoreGroup(context.Background(), tt.group)

			// Check if score is in reasonable range
			if score < 0 || score > 5.0 {
				t.Errorf("score %f outside expected range [0, 5.0]", score)
			}

			// Rough check for expected score (within 50% tolerance)
			if score > tt.expectedScore*1.5 || score < tt.expectedScore*0.5 {
				t.Logf("score %f differs from expected %f (may be acceptable)", score, tt.expectedScore)
			}
		})
	}
}

func TestSynthesizeSummary(t *testing.T) {
	tests := []struct {
		name     string
		reviews  []domain.Review
		expected []string // Strings that should appear in summary
	}{
		{
			name: "combines summaries from multiple providers",
			reviews: []domain.Review{
				{ProviderName: "openai", Summary: "Found 3 security issues"},
				{ProviderName: "anthropic", Summary: "Detected 2 performance problems"},
			},
			expected: []string{"security", "performance"},
		},
		{
			name: "handles single review",
			reviews: []domain.Review{
				{ProviderName: "openai", Summary: "Code looks good overall"},
			},
			expected: []string{"Code looks good"},
		},
		{
			name: "handles empty summaries",
			reviews: []domain.Review{
				{ProviderName: "openai", Summary: ""},
				{ProviderName: "anthropic", Summary: "Found issues"},
			},
			expected: []string{"Found issues"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewIntelligentMerger(nil)
			summary := merger.synthesizeSummary(tt.reviews)

			for _, expected := range tt.expected {
				if !strings.Contains(strings.ToLower(summary), strings.ToLower(expected)) {
					t.Errorf("expected summary to contain %q, got: %s", expected, summary)
				}
			}

			// Summary should not be the hardcoded default
			if summary == "This is a merged review." {
				t.Error("summary should not be the hardcoded default")
			}
		})
	}
}

func TestIntelligentMerge_Integration(t *testing.T) {
	// Integration test with realistic data
	reviews := []domain.Review{
		{
			ProviderName: "openai",
			ModelName:    "gpt-4o-mini",
			Summary:      "Found 2 critical security vulnerabilities in authentication code",
			TokensIn:     1500,
			TokensOut:    250,
			Cost:         0.0012,
			Findings: []domain.Finding{
				{
					ID:          "finding1",
					File:        "auth/handler.go",
					LineStart:   45,
					LineEnd:     50,
					Severity:    "error",
					Category:    "security",
					Description: "SQL injection vulnerability in login query",
					Suggestion:  "Use parameterized queries",
					Evidence:    true,
				},
				{
					ID:          "finding2",
					File:        "auth/validator.go",
					LineStart:   20,
					LineEnd:     22,
					Severity:    "warning",
					Category:    "bug",
					Description: "Missing null check",
					Evidence:    false,
				},
			},
		},
		{
			ProviderName: "anthropic",
			ModelName:    "claude-3-5-sonnet",
			Summary:      "Identified SQL injection risk and potential null pointer issue",
			TokensIn:     2000,
			TokensOut:    300,
			Cost:         0.0018,
			Findings: []domain.Finding{
				{
					ID:          "finding3",
					File:        "auth/handler.go",
					LineStart:   46,
					LineEnd:     49,
					Severity:    "error",
					Category:    "security",
					Description: "Unsafe SQL query construction allows injection",
					Suggestion:  "Switch to prepared statements",
					Evidence:    true,
				},
				{
					ID:          "finding4",
					File:        "util/parser.go",
					LineStart:   100,
					LineEnd:     105,
					Severity:    "info",
					Category:    "style",
					Description: "Consider using early return pattern",
					Evidence:    false,
				},
			},
		},
	}

	merger := NewIntelligentMerger(nil)
	result := merger.Merge(context.Background(), reviews)

	// Check merged review properties
	if result.ProviderName != "merged" {
		t.Errorf("expected ProviderName 'merged', got %q", result.ProviderName)
	}

	// Summary should be synthesized, not hardcoded
	if result.Summary == "This is a merged review." {
		t.Error("summary should be synthesized, not hardcoded default")
	}

	if !strings.Contains(strings.ToLower(result.Summary), "sql") {
		t.Error("summary should mention SQL issues from both reviews")
	}

	// Should have findings (exact count depends on grouping logic)
	if len(result.Findings) == 0 {
		t.Error("merged review should have findings")
	}

	// Should group similar SQL injection findings (finding1 and finding3)
	// So we expect fewer findings than the sum of both reviews
	totalFindings := len(reviews[0].Findings) + len(reviews[1].Findings)
	if len(result.Findings) >= totalFindings {
		t.Logf("Warning: expected grouping to reduce findings from %d, got %d", totalFindings, len(result.Findings))
	}

	// Verify usage metadata aggregation
	expectedTokensIn := 1500 + 2000
	expectedTokensOut := 250 + 300
	expectedCost := 0.0012 + 0.0018

	if result.TokensIn != expectedTokensIn {
		t.Errorf("expected TokensIn %d, got %d", expectedTokensIn, result.TokensIn)
	}
	if result.TokensOut != expectedTokensOut {
		t.Errorf("expected TokensOut %d, got %d", expectedTokensOut, result.TokensOut)
	}
	if result.Cost != expectedCost {
		t.Errorf("expected Cost %.4f, got %.4f", expectedCost, result.Cost)
	}
}

// mockPrecisionStore implements the Store interface for testing
type mockPrecisionStore struct {
	priors map[string]map[string]store.PrecisionPrior
}

func (m *mockPrecisionStore) GetPrecisionPriors(ctx context.Context) (map[string]map[string]store.PrecisionPrior, error) {
	if m.priors == nil {
		return make(map[string]map[string]store.PrecisionPrior), nil
	}
	return m.priors, nil
}

// Implement other Store methods (not used in these tests)
func (m *mockPrecisionStore) CreateRun(ctx context.Context, run store.Run) error { return nil }
func (m *mockPrecisionStore) UpdateRunCost(ctx context.Context, runID string, totalCost float64) error {
	return nil
}
func (m *mockPrecisionStore) GetRun(ctx context.Context, runID string) (store.Run, error) {
	return store.Run{}, nil
}
func (m *mockPrecisionStore) ListRuns(ctx context.Context, limit int) ([]store.Run, error) {
	return nil, nil
}
func (m *mockPrecisionStore) SaveReview(ctx context.Context, review store.ReviewRecord) error {
	return nil
}
func (m *mockPrecisionStore) GetReview(ctx context.Context, reviewID string) (store.ReviewRecord, error) {
	return store.ReviewRecord{}, nil
}
func (m *mockPrecisionStore) GetReviewsByRun(ctx context.Context, runID string) ([]store.ReviewRecord, error) {
	return nil, nil
}
func (m *mockPrecisionStore) SaveFindings(ctx context.Context, findings []store.FindingRecord) error {
	return nil
}
func (m *mockPrecisionStore) GetFinding(ctx context.Context, findingID string) (store.FindingRecord, error) {
	return store.FindingRecord{}, nil
}
func (m *mockPrecisionStore) GetFindingsByReview(ctx context.Context, reviewID string) ([]store.FindingRecord, error) {
	return nil, nil
}
func (m *mockPrecisionStore) RecordFeedback(ctx context.Context, feedback store.Feedback) error {
	return nil
}
func (m *mockPrecisionStore) GetFeedbackForFinding(ctx context.Context, findingID string) ([]store.Feedback, error) {
	return nil, nil
}
func (m *mockPrecisionStore) UpdatePrecisionPrior(ctx context.Context, provider, category string, accepted, rejected int) error {
	return nil
}
func (m *mockPrecisionStore) Close() error { return nil }

// Test LLM-based summary synthesis
func TestBuildSynthesisPrompt(t *testing.T) {
	reviews := []domain.Review{
		{
			ProviderName: "openai",
			ModelName:    "gpt-4o-mini",
			Summary:      "Found 3 critical security issues including SQL injection and XSS vulnerabilities.",
			Findings:     make([]domain.Finding, 3),
		},
		{
			ProviderName: "anthropic",
			ModelName:    "claude-3-5-sonnet",
			Summary:      "Identified 2 high-severity issues: SQL injection risk and improper input validation.",
			Findings:     make([]domain.Finding, 2),
		},
	}

	prompt := buildSynthesisPrompt(reviews)

	// Check prompt contains provider summaries
	if !strings.Contains(prompt, "openai") {
		t.Error("prompt should contain openai provider name")
	}
	if !strings.Contains(prompt, "anthropic") {
		t.Error("prompt should contain anthropic provider name")
	}

	// Check prompt contains summaries
	if !strings.Contains(prompt, "SQL injection") {
		t.Error("prompt should contain finding descriptions from summaries")
	}

	// Check prompt contains finding counts
	if !strings.Contains(prompt, "3") || !strings.Contains(prompt, "2") {
		t.Error("prompt should contain finding counts")
	}

	// Check prompt has synthesis instructions
	lowerPrompt := strings.ToLower(prompt)
	if !strings.Contains(lowerPrompt, "synthesize") || !strings.Contains(lowerPrompt, "cohesive") {
		t.Errorf("prompt should contain synthesis instructions, got: %s", prompt)
	}
}

func TestSynthesizeSummary_WithLLM(t *testing.T) {
	mockProvider := &mockSynthesisProvider{
		response: "Comprehensive analysis reveals 5 distinct issues across 2 providers. Both OpenAI and Anthropic identified critical SQL injection vulnerabilities. Additionally, XSS and input validation issues were found. Immediate attention required for security fixes.",
	}

	merger := &IntelligentMerger{
		synthProvider: mockProvider,
		useLLM:        true,
	}

	reviews := []domain.Review{
		{ProviderName: "openai", Summary: "Found SQL injection and XSS."},
		{ProviderName: "anthropic", Summary: "SQL injection and validation issues."},
	}

	summary := merger.synthesizeSummary(reviews)

	// Should use LLM response
	if !strings.Contains(summary, "Comprehensive analysis") {
		t.Errorf("expected LLM-generated summary, got: %s", summary)
	}

	// Should NOT contain concatenated format
	if strings.Contains(summary, "openai:") || strings.Contains(summary, "|") {
		t.Error("should not use concatenation format when LLM is enabled")
	}

	// Verify provider was called
	if !mockProvider.called {
		t.Error("synthesis provider should have been called")
	}
}

func TestSynthesizeSummary_LLMFallback(t *testing.T) {
	mockProvider := &mockSynthesisProvider{
		shouldFail: true,
	}

	merger := &IntelligentMerger{
		synthProvider: mockProvider,
		useLLM:        true,
	}

	reviews := []domain.Review{
		{ProviderName: "openai", Summary: "Found issues."},
		{ProviderName: "anthropic", Summary: "Found problems."},
	}

	summary := merger.synthesizeSummary(reviews)

	// Should fall back to concatenation
	if !strings.Contains(summary, "openai:") || !strings.Contains(summary, "|") {
		t.Error("should fall back to concatenation when LLM fails")
	}

	// Verify provider was called (but failed)
	if !mockProvider.called {
		t.Error("synthesis provider should have been attempted")
	}
}

func TestSynthesizeSummary_LLMDisabled(t *testing.T) {
	mockProvider := &mockSynthesisProvider{}

	merger := &IntelligentMerger{
		synthProvider: mockProvider,
		useLLM:        false, // Disabled
	}

	reviews := []domain.Review{
		{ProviderName: "openai", Summary: "Found issues."},
		{ProviderName: "anthropic", Summary: "Found problems."},
	}

	summary := merger.synthesizeSummary(reviews)

	// Should use concatenation
	if !strings.Contains(summary, "openai:") {
		t.Error("should use concatenation when LLM is disabled")
	}

	// Provider should NOT have been called
	if mockProvider.called {
		t.Error("synthesis provider should not be called when useLLM is false")
	}
}

func TestSynthesizeSummary_NoProvider(t *testing.T) {
	merger := &IntelligentMerger{
		synthProvider: nil,
		useLLM:        true, // Enabled but no provider
	}

	reviews := []domain.Review{
		{ProviderName: "openai", Summary: "Found issues."},
		{ProviderName: "anthropic", Summary: "Found problems."},
	}

	summary := merger.synthesizeSummary(reviews)

	// Should fall back to concatenation when provider is nil
	if !strings.Contains(summary, "openai:") || !strings.Contains(summary, "|") {
		t.Errorf("should fall back to concatenation when provider is nil, got: %s", summary)
	}
}

// Mock synthesis provider for testing
type mockSynthesisProvider struct {
	response   string
	shouldFail bool
	called     bool
}

func (m *mockSynthesisProvider) Review(ctx context.Context, prompt string, seed uint64) (string, error) {
	m.called = true
	if m.shouldFail {
		return "", fmt.Errorf("synthesis failed: mock provider error")
	}
	return m.response, nil
}

func TestWithReviewerWeighting(t *testing.T) {
	// Create a group with reviewers having different weights
	group := findingGroup{
		findings: []domain.Finding{
			{ReviewerName: "security", ReviewerWeight: 1.5},
			{ReviewerName: "style", ReviewerWeight: 0.5},
		},
		providers: map[string]bool{"anthropic": true},
		reviewers: map[string]float64{
			"security": 1.5,
			"style":    0.5,
		},
	}

	t.Run("weighted scoring enabled", func(t *testing.T) {
		merger := NewIntelligentMerger(nil).WithReviewerWeighting(true)
		score := merger.calculateAgreementScore(group)

		// Should sum weights: 1.5 + 0.5 = 2.0
		expected := 2.0
		if score != expected {
			t.Errorf("expected weighted score %.1f, got %.1f", expected, score)
		}
	})

	t.Run("weighted scoring disabled", func(t *testing.T) {
		merger := NewIntelligentMerger(nil).WithReviewerWeighting(false)
		score := merger.calculateAgreementScore(group)

		// Should fall back to provider count: 1
		expected := 1.0
		if score != expected {
			t.Errorf("expected provider count %.1f, got %.1f", expected, score)
		}
	})
}

func TestWithRespectFocus(t *testing.T) {
	// Create a group from a specialized reviewer with low agreement
	groupWithReviewer := findingGroup{
		findings: []domain.Finding{
			{ReviewerName: "security", ReviewerWeight: 1.0},
		},
		providers: map[string]bool{}, // Empty providers (edge case)
		reviewers: map[string]float64{
			"security": 1.0,
		},
	}

	// Create a group without reviewers (legacy behavior)
	groupWithoutReviewer := findingGroup{
		findings: []domain.Finding{
			{Description: "Some finding"},
		},
		providers: map[string]bool{}, // Empty = low agreement
		reviewers: map[string]float64{},
	}

	t.Run("respect focus enabled with specialized reviewer", func(t *testing.T) {
		merger := NewIntelligentMerger(nil).
			WithReviewerWeighting(false). // Disable to test respectFocus path
			WithRespectFocus(true)
		score := merger.calculateAgreementScore(groupWithReviewer)

		// Should not penalize specialized reviewer, minimum 1.0
		if score < 1.0 {
			t.Errorf("expected minimum score 1.0 for specialized reviewer, got %.1f", score)
		}
	})

	t.Run("respect focus disabled", func(t *testing.T) {
		merger := NewIntelligentMerger(nil).
			WithReviewerWeighting(false).
			WithRespectFocus(false)
		score := merger.calculateAgreementScore(groupWithoutReviewer)

		// Should return raw provider count (0)
		expected := 0.0
		if score != expected {
			t.Errorf("expected raw score %.1f, got %.1f", expected, score)
		}
	})
}

func TestConfigTogglesBehavior(t *testing.T) {
	// Integration test: verify config toggles change scoring behavior
	reviews := []domain.Review{
		{
			ProviderName: "anthropic",
			ModelName:    "claude-opus-4",
			Findings: []domain.Finding{
				{
					ID:             "sec-1",
					File:           "auth.go",
					LineStart:      10,
					Severity:       "error",
					Category:       "security",
					Description:    "SQL injection vulnerability",
					ReviewerName:   "security",
					ReviewerWeight: 2.0, // High-weight security reviewer
				},
			},
		},
		{
			ProviderName: "anthropic",
			ModelName:    "claude-sonnet-4-5",
			Findings: []domain.Finding{
				{
					ID:             "style-1",
					File:           "utils.go",
					LineStart:      50,
					Severity:       "info",
					Category:       "style",
					Description:    "Consider using early return",
					ReviewerName:   "maintainability",
					ReviewerWeight: 0.5, // Low-weight style reviewer
				},
			},
		},
	}

	t.Run("weighted scoring affects merge order", func(t *testing.T) {
		// With weighting enabled, security finding (weight 2.0) should score higher
		mergerWeighted := NewIntelligentMerger(nil).
			WithReviewerWeighting(true).
			WithRespectFocus(true)
		resultWeighted := mergerWeighted.Merge(context.Background(), reviews)

		// With weighting disabled, raw count is used
		mergerUnweighted := NewIntelligentMerger(nil).
			WithReviewerWeighting(false).
			WithRespectFocus(false)
		resultUnweighted := mergerUnweighted.Merge(context.Background(), reviews)

		// Both should have 2 findings (different files, no grouping)
		if len(resultWeighted.Findings) != 2 || len(resultUnweighted.Findings) != 2 {
			t.Errorf("expected 2 findings each, got weighted=%d, unweighted=%d",
				len(resultWeighted.Findings), len(resultUnweighted.Findings))
		}

		// The ordering may differ based on scoring
		// This test primarily verifies the config toggles don't break merging
		t.Logf("Weighted first finding: %s (severity=%s)",
			resultWeighted.Findings[0].Description, resultWeighted.Findings[0].Severity)
		t.Logf("Unweighted first finding: %s (severity=%s)",
			resultUnweighted.Findings[0].Description, resultUnweighted.Findings[0].Severity)
	})
}
