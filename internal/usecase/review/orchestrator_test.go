package review_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

type mockGitEngine struct {
	baseRef            string
	targetRef          string
	includeUncommitted bool
	diff               domain.Diff
	err                error
	branch             string
	branchErr          error
	incrementalDiff    domain.Diff
	incrementalDiffErr error
	commitExistsMap    map[string]bool
}

func (m *mockGitEngine) GetCumulativeDiff(ctx context.Context, baseRef, targetRef string, includeUncommitted bool) (domain.Diff, error) {
	m.baseRef = baseRef
	m.targetRef = targetRef
	m.includeUncommitted = includeUncommitted
	return m.diff, m.err
}

func (m *mockGitEngine) GetIncrementalDiff(ctx context.Context, fromCommit, toCommit string) (domain.Diff, error) {
	return m.incrementalDiff, m.incrementalDiffErr
}

func (m *mockGitEngine) CommitExists(ctx context.Context, commitSHA string) (bool, error) {
	if m.commitExistsMap == nil {
		return false, nil
	}
	return m.commitExistsMap[commitSHA], nil
}

func (m *mockGitEngine) CurrentBranch(ctx context.Context) (string, error) {
	return m.branch, m.branchErr
}

type mockProvider struct {
	mu       sync.Mutex
	requests []review.ProviderRequest
	response domain.Review
	err      error
}

type mockMerger struct {
	mu    sync.Mutex
	calls [][]domain.Review
}

func (m *mockMerger) Merge(ctx context.Context, reviews []domain.Review) domain.Review {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, reviews)
	return domain.Review{ProviderName: "merged", ModelName: "consensus"}
}

func (m *mockProvider) Review(ctx context.Context, req review.ProviderRequest) (domain.Review, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	return m.response, m.err
}

func (m *mockProvider) EstimateTokens(text string) int {
	return len(text) / 4 // Simple estimate for testing
}

type mockMarkdownWriter struct {
	mu    sync.Mutex
	calls []domain.MarkdownArtifact
	err   error
}

type mockJSONWriter struct {
	mu    sync.Mutex
	calls []domain.JSONArtifact
	err   error
}

type mockSARIFWriter struct {
	mu    sync.Mutex
	calls []review.SARIFArtifact
	err   error
}

func (m *mockJSONWriter) Write(ctx context.Context, artifact domain.JSONArtifact) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, artifact)
	if m.err != nil {
		return "", m.err
	}
	return filepath.Join(artifact.OutputDir, "review-"+artifact.ProviderName+".json"), nil
}

func (m *mockMarkdownWriter) Write(ctx context.Context, artifact domain.MarkdownArtifact) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, artifact)
	if m.err != nil {
		return "", m.err
	}
	return filepath.Join(artifact.OutputDir, "review-"+artifact.ProviderName+".md"), nil
}

func (m *mockSARIFWriter) Write(ctx context.Context, artifact review.SARIFArtifact) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, artifact)
	if m.err != nil {
		return "", m.err
	}
	return filepath.Join(artifact.OutputDir, "review-"+artifact.ProviderName+".sarif"), nil
}

func TestReviewBranchWithSingleProvider(t *testing.T) {
	ctx := context.Background()
	diff := domain.Diff{
		FromCommitHash: "abc",
		ToCommitHash:   "def",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,2 @@\n+package main\n+func main() {}"},
		},
	}
	expectedReview := domain.Review{
		ProviderName: "stub-openai",
		ModelName:    "gpt-4o",
		Summary:      "No issues found.",
		Findings: []domain.Finding{
			{
				ID:          "hash",
				File:        "main.go",
				LineStart:   1,
				LineEnd:     1,
				Severity:    "low",
				Category:    "style",
				Description: "Example finding",
				Suggestion:  "Refactor main",
				Evidence:    true,
			},
		},
	}

	gitMock := &mockGitEngine{diff: diff}
	providerMock := &mockProvider{response: expectedReview}
	writerMock := &mockMarkdownWriter{}
	mergerMock := &mockMerger{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}

	registry, personaBuilder := createTestReviewerDeps("stub-openai")

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"stub-openai": providerMock,
		},
		Merger:               mergerMock,
		Markdown:             writerMock,
		JSON:                 jsonWriterMock,
		SARIF:                sarifWriterMock,
		SeedGenerator:        func(baseRef, targetRef string) uint64 { return 42 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	result, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
		BaseRef:            "main",
		TargetRef:          "feature",
		OutputDir:          t.TempDir(),
		IncludeUncommitted: true,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if gitMock.baseRef != "main" || gitMock.targetRef != "feature" || !gitMock.includeUncommitted {
		t.Fatalf("git engine received unexpected inputs: base=%s target=%s include=%t", gitMock.baseRef, gitMock.targetRef, gitMock.includeUncommitted)
	}

	if len(providerMock.requests) != 1 {
		t.Fatalf("expected provider to be called once, got %d", len(providerMock.requests))
	}

	if providerMock.requests[0].Seed != 42 {
		t.Fatalf("expected seed of 42, got %d", providerMock.requests[0].Seed)
	}

	if len(writerMock.calls) != 2 {
		t.Fatalf("expected markdown writer to be called twice (1 provider + 1 merged), got %d", len(writerMock.calls))
	}

	if writerMock.calls[0].Review.Summary != expectedReview.Summary {
		t.Fatalf("markdown writer received wrong review summary: %s", writerMock.calls[0].Review.Summary)
	}

	if result.MarkdownPaths["stub-openai"] == "" {
		t.Fatalf("expected markdown path to be populated for stub-openai")
	}

	if result.MarkdownPaths["merged"] == "" {
		t.Fatalf("expected markdown path to be populated for merged review")
	}

	if len(jsonWriterMock.calls) != 2 {
		t.Fatalf("expected json writer to be called twice (1 provider + 1 merged), got %d", len(jsonWriterMock.calls))
	}

	if result.JSONPaths["stub-openai"] == "" {
		t.Fatalf("expected json path to be populated for stub-openai")
	}

	if result.JSONPaths["merged"] == "" {
		t.Fatalf("expected json path to be populated for merged review")
	}

	if len(result.Reviews) != 2 {
		t.Fatalf("expected 2 reviews (1 provider + 1 merged), got %d", len(result.Reviews))
	}

	if result.Reviews[0].ProviderName != "stub-openai" {
		t.Fatalf("expected review from stub-openai, got %s", result.Reviews[0].ProviderName)
	}
}

func TestReviewBranchWithMultipleProviders(t *testing.T) {
	ctx := context.Background()
	diff := domain.Diff{
		FromCommitHash: "abc",
		ToCommitHash:   "def",
		Files:          []domain.FileDiff{{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,2 @@\n+package main\n+func main() {}"}},
	}

	review1 := domain.Review{ProviderName: "provider1", ModelName: "model1", Summary: "Review 1"}
	review2 := domain.Review{ProviderName: "provider2", ModelName: "model2", Summary: "Review 2"}

	provider1 := &mockProvider{response: review1}
	provider2 := &mockProvider{response: review2}
	gitMock := &mockGitEngine{diff: diff}
	writerMock := &mockMarkdownWriter{}
	mergerMock := &mockMerger{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}

	registry, personaBuilder := createMultiProviderTestDeps([]string{"provider1", "provider2"})

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"provider1": provider1,
			"provider2": provider2,
		},
		Merger:               mergerMock,
		Markdown:             writerMock,
		JSON:                 jsonWriterMock,
		SARIF:                sarifWriterMock,
		SeedGenerator:        func(_, _ string) uint64 { return 42 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	result, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature",
		OutputDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(provider1.requests) != 1 {
		t.Fatalf("expected provider1 to be called once, got %d", len(provider1.requests))
	}
	if len(provider2.requests) != 1 {
		t.Fatalf("expected provider2 to be called once, got %d", len(provider2.requests))
	}

	if len(writerMock.calls) != 3 {
		t.Fatalf("expected markdown writer to be called three times (2 providers + 1 merged), got %d", len(writerMock.calls))
	}

	if len(result.Reviews) != 3 {
		t.Fatalf("expected 3 reviews (2 provider + 1 merged), got %d", len(result.Reviews))
	}

	if result.MarkdownPaths["provider1"] == "" {
		t.Fatalf("expected markdown path for provider1")
	}
	if result.MarkdownPaths["provider2"] == "" {
		t.Fatalf("expected markdown path for provider2")
	}
	if result.MarkdownPaths["merged"] == "" {
		t.Fatalf("expected markdown path for merged review")
	}

	if len(jsonWriterMock.calls) != 3 {
		t.Fatalf("expected json writer to be called three times (2 providers + 1 merged), got %d", len(jsonWriterMock.calls))
	}

	if result.JSONPaths["provider1"] == "" {
		t.Fatalf("expected json path for provider1")
	}
	if result.JSONPaths["provider2"] == "" {
		t.Fatalf("expected json path for provider2")
	}
	if result.JSONPaths["merged"] == "" {
		t.Fatalf("expected json path for merged review")
	}

	if len(mergerMock.calls) != 1 {
		t.Fatalf("expected merger to be called once, got %d", len(mergerMock.calls))
	}
}

func TestCurrentBranchDelegatesToGitEngine(t *testing.T) {
	ctx := context.Background()
	gitMock := &mockGitEngine{branch: "main"}
	registry, personaBuilder := createTestReviewerDeps("mock")
	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git:                  gitMock,
		Providers:            map[string]review.Provider{"mock": &mockProvider{}},
		Merger:               &mockMerger{},
		Markdown:             &mockMarkdownWriter{},
		JSON:                 &mockJSONWriter{},
		SARIF:                &mockSARIFWriter{},
		SeedGenerator:        func(_, _ string) uint64 { return 0 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	branch, err := orchestrator.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected branch main, got %s", branch)
	}
}

// Integration tests for store functionality

func TestReviewBranch_StoreIntegration_SavesRunAndReviews(t *testing.T) {
	ctx := context.Background()
	diff := domain.Diff{
		FromCommitHash: "abc123",
		ToCommitHash:   "def456",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,5 @@\n+package main\n+func main() {}"},
		},
	}

	expectedReview := domain.Review{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		Summary:      "Test review summary",
		Findings: []domain.Finding{
			{
				ID:          "finding-1",
				File:        "main.go",
				LineStart:   10,
				LineEnd:     15,
				Severity:    "high",
				Category:    "security",
				Description: "SQL injection vulnerability",
				Suggestion:  "Use parameterized queries",
				Evidence:    true,
			},
			{
				ID:          "finding-2",
				File:        "utils.go",
				LineStart:   5,
				LineEnd:     8,
				Severity:    "medium",
				Category:    "performance",
				Description: "Inefficient loop",
				Suggestion:  "Use map lookup",
				Evidence:    false,
			},
		},
	}

	gitMock := &mockGitEngine{diff: diff}
	providerMock := &mockProvider{response: expectedReview}
	writerMock := &mockMarkdownWriter{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}
	mergerMock := &mockMerger{}
	storeMock := &mockStore{}

	registry, personaBuilder := createTestReviewerDeps("test-provider")

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"test-provider": providerMock,
		},
		Merger:               mergerMock,
		Markdown:             writerMock,
		JSON:                 jsonWriterMock,
		SARIF:                sarifWriterMock,
		Store:                storeMock,
		SeedGenerator:        func(baseRef, targetRef string) uint64 { return 12345 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	result, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
		BaseRef:    "main",
		TargetRef:  "feature",
		OutputDir:  t.TempDir(),
		Repository: "test-repo",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify run record was created
	if len(storeMock.runs) != 1 {
		t.Fatalf("expected 1 run record, got %d", len(storeMock.runs))
	}
	run := storeMock.runs[0]
	if run.Repository != "test-repo" {
		t.Errorf("expected repository test-repo, got %s", run.Repository)
	}
	if run.BaseRef != "main" {
		t.Errorf("expected base ref main, got %s", run.BaseRef)
	}
	if run.TargetRef != "feature" {
		t.Errorf("expected target ref feature, got %s", run.TargetRef)
	}
	if run.Scope != "main..feature" {
		t.Errorf("expected scope main..feature, got %s", run.Scope)
	}

	// Verify review records were saved (1 provider + 1 merged = 2 total)
	if len(storeMock.reviews) != 2 {
		t.Fatalf("expected 2 review records (1 provider + 1 merged), got %d", len(storeMock.reviews))
	}

	// Verify provider review
	providerReview := storeMock.reviews[0]
	if providerReview.Provider != "test-provider" {
		t.Errorf("expected provider test-provider, got %s", providerReview.Provider)
	}
	if providerReview.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", providerReview.Model)
	}
	if providerReview.RunID != run.RunID {
		t.Errorf("expected review to reference run ID %s, got %s", run.RunID, providerReview.RunID)
	}

	// Verify merged review
	mergedReview := storeMock.reviews[1]
	if mergedReview.Provider != "merged" {
		t.Errorf("expected merged provider, got %s", mergedReview.Provider)
	}
	if mergedReview.RunID != run.RunID {
		t.Errorf("expected merged review to reference run ID %s, got %s", run.RunID, mergedReview.RunID)
	}

	// Verify findings were saved (2 findings from provider review only, merged review has no findings from mockMerger)
	if len(storeMock.findings) != 2 {
		t.Fatalf("expected 2 findings (from provider review), got %d", len(storeMock.findings))
	}

	// Verify finding details
	finding := storeMock.findings[0]
	if finding.File != "main.go" {
		t.Errorf("expected file main.go, got %s", finding.File)
	}
	if finding.LineStart != 10 {
		t.Errorf("expected line start 10, got %d", finding.LineStart)
	}
	if finding.LineEnd != 15 {
		t.Errorf("expected line end 15, got %d", finding.LineEnd)
	}
	if finding.Severity != "high" {
		t.Errorf("expected severity high, got %s", finding.Severity)
	}
	if finding.Category != "security" {
		t.Errorf("expected category security, got %s", finding.Category)
	}
	if !finding.Evidence {
		t.Errorf("expected evidence to be true")
	}

	// Verify finding hash is populated
	if finding.FindingHash == "" {
		t.Errorf("expected finding hash to be populated")
	}

	// Verify result is returned correctly
	if len(result.Reviews) != 2 {
		t.Errorf("expected 2 reviews in result, got %d", len(result.Reviews))
	}
}

func TestReviewBranch_StoreDisabled_ContinuesWithoutStore(t *testing.T) {
	ctx := context.Background()
	diff := domain.Diff{
		FromCommitHash: "abc",
		ToCommitHash:   "def",
		Files:          []domain.FileDiff{{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,2 @@\n+package main\n+func main() {}"}},
	}

	expectedReview := domain.Review{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		Summary:      "Test review",
	}

	gitMock := &mockGitEngine{diff: diff}
	providerMock := &mockProvider{response: expectedReview}
	writerMock := &mockMarkdownWriter{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}
	mergerMock := &mockMerger{}

	// Store is nil - simulating disabled store
	registry, personaBuilder := createTestReviewerDeps("test-provider")
	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"test-provider": providerMock,
		},
		Merger:               mergerMock,
		Markdown:             writerMock,
		JSON:                 jsonWriterMock,
		SARIF:                sarifWriterMock,
		Store:                nil, // Disabled
		SeedGenerator:        func(baseRef, targetRef string) uint64 { return 42 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	result, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
		BaseRef:    "main",
		TargetRef:  "feature",
		OutputDir:  t.TempDir(),
		Repository: "test-repo",
	})

	// Should succeed even with nil store
	if err != nil {
		t.Fatalf("expected no error with nil store, got %v", err)
	}

	if len(result.Reviews) != 2 {
		t.Errorf("expected 2 reviews (1 provider + 1 merged), got %d", len(result.Reviews))
	}
}

func TestReviewBranch_StoreErrors_ContinueGracefully(t *testing.T) {
	ctx := context.Background()
	diff := domain.Diff{
		FromCommitHash: "abc",
		ToCommitHash:   "def",
		Files:          []domain.FileDiff{{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,2 @@\n+package main\n+func main() {}"}},
	}

	expectedReview := domain.Review{
		ProviderName: "test-provider",
		ModelName:    "test-model",
		Summary:      "Test review",
		Findings: []domain.Finding{
			{
				ID:          "finding-1",
				File:        "main.go",
				LineStart:   1,
				LineEnd:     1,
				Severity:    "low",
				Category:    "style",
				Description: "Minor issue",
				Suggestion:  "Fix it",
				Evidence:    true,
			},
		},
	}

	gitMock := &mockGitEngine{diff: diff}
	providerMock := &mockProvider{response: expectedReview}
	writerMock := &mockMarkdownWriter{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}
	mergerMock := &mockMerger{}

	// Store that returns errors
	storeMock := &mockStore{
		createRunErr:    nil, // CreateRun succeeds
		saveReviewErr:   &testError{msg: "database connection failed"},
		saveFindingsErr: &testError{msg: "database write failed"},
	}

	registry, personaBuilder := createTestReviewerDeps("test-provider")
	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"test-provider": providerMock,
		},
		Merger:               mergerMock,
		Markdown:             writerMock,
		JSON:                 jsonWriterMock,
		SARIF:                sarifWriterMock,
		Store:                storeMock,
		SeedGenerator:        func(baseRef, targetRef string) uint64 { return 42 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	result, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
		BaseRef:    "main",
		TargetRef:  "feature",
		OutputDir:  t.TempDir(),
		Repository: "test-repo",
	})

	// Should succeed despite store errors (graceful degradation)
	if err != nil {
		t.Fatalf("expected no error despite store failures, got %v", err)
	}

	if len(result.Reviews) != 2 {
		t.Errorf("expected 2 reviews despite store errors, got %d", len(result.Reviews))
	}

	// Verify run was created successfully
	if len(storeMock.runs) != 1 {
		t.Errorf("expected run to be created, got %d runs", len(storeMock.runs))
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// TestReviewBranch_CostTracking verifies cost aggregation from provider reviews
func TestReviewBranch_CostTracking(t *testing.T) {
	ctx := context.Background()
	diff := domain.Diff{
		FromCommitHash: "abc123",
		ToCommitHash:   "def456",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,2 @@\n+package main\n+func main() {}"},
		},
	}

	// Provider 1 costs $0.05
	review1 := domain.Review{
		ProviderName: "provider1",
		ModelName:    "model1",
		Summary:      "Review 1",
		Cost:         0.05,
	}

	// Provider 2 costs $0.03
	review2 := domain.Review{
		ProviderName: "provider2",
		ModelName:    "model2",
		Summary:      "Review 2",
		Cost:         0.03,
	}

	provider1 := &mockProvider{response: review1}
	provider2 := &mockProvider{response: review2}
	gitMock := &mockGitEngine{diff: diff}
	writerMock := &mockMarkdownWriter{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}
	mergerMock := &mockMerger{}
	storeMock := &mockStore{}

	registry, personaBuilder := createMultiProviderTestDeps([]string{"provider1", "provider2"})
	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"provider1": provider1,
			"provider2": provider2,
		},
		Merger:               mergerMock,
		Markdown:             writerMock,
		JSON:                 jsonWriterMock,
		SARIF:                sarifWriterMock,
		Store:                storeMock,
		SeedGenerator:        func(_, _ string) uint64 { return 42 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	_, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
		BaseRef:    "main",
		TargetRef:  "feature",
		OutputDir:  t.TempDir(),
		Repository: "test-repo",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify run record has correct total cost ($0.05 + $0.03 = $0.08)
	if len(storeMock.runs) != 1 {
		t.Fatalf("expected 1 run record, got %d", len(storeMock.runs))
	}
	run := storeMock.runs[0]
	expectedTotalCost := 0.08
	if run.TotalCost != expectedTotalCost {
		t.Errorf("expected total cost $%.4f, got $%.4f", expectedTotalCost, run.TotalCost)
	}
}

// blockingProvider simulates a slow HTTP request that blocks until context is cancelled
type blockingProvider struct {
	mu               sync.Mutex
	contextCancelled bool
}

func (b *blockingProvider) Review(ctx context.Context, req review.ProviderRequest) (domain.Review, error) {
	// Wait for context cancellation
	<-ctx.Done()

	b.mu.Lock()
	b.contextCancelled = true
	b.mu.Unlock()

	return domain.Review{}, ctx.Err()
}

func (b *blockingProvider) EstimateTokens(text string) int {
	return len(text) / 4 // Simple estimate for testing
}

func TestOrchestrator_ContextCancellation(t *testing.T) {
	// Test that when context is cancelled (e.g., CTRL+C), all in-flight operations abort promptly
	ctx, cancel := context.WithCancel(context.Background())

	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "test.go", Status: "modified", Patch: "diff content"},
		},
	}

	blockingProv := &blockingProvider{}
	gitMock := &mockGitEngine{diff: diff}
	writerMock := &mockMarkdownWriter{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}
	mergerMock := &mockMerger{}

	registry, personaBuilder := createTestReviewerDeps("slow-provider")
	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"slow-provider": blockingProv,
		},
		Merger:               mergerMock,
		Markdown:             writerMock,
		JSON:                 jsonWriterMock,
		SARIF:                sarifWriterMock,
		SeedGenerator:        func(_, _ string) uint64 { return 42 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	// Run ReviewBranch in a goroutine
	done := make(chan struct{})
	var reviewErr error
	go func() {
		_, reviewErr = orchestrator.ReviewBranch(ctx, review.BranchRequest{
			BaseRef:    "main",
			TargetRef:  "feature",
			OutputDir:  t.TempDir(),
			Repository: "test-repo",
		})
		close(done)
	}()

	// Give the goroutine a moment to start
	time.Sleep(50 * time.Millisecond)

	// Cancel the context (simulating CTRL+C)
	cancel()

	// Wait for ReviewBranch to complete (should be prompt)
	select {
	case <-done:
		// Good! The operation completed promptly
	case <-time.After(2 * time.Second):
		t.Fatal("ReviewBranch did not abort within 2 seconds of context cancellation")
	}

	// Verify the provider detected the cancellation
	blockingProv.mu.Lock()
	cancelled := blockingProv.contextCancelled
	blockingProv.mu.Unlock()

	if !cancelled {
		t.Error("expected provider to detect context cancellation")
	}

	// Verify the error is context-related
	if reviewErr == nil {
		t.Error("expected an error after context cancellation")
	}
}

func TestFilterBinaryFiles(t *testing.T) {
	tests := []struct {
		name            string
		input           domain.Diff
		wantTextCount   int
		wantBinaryCount int
		wantTextPaths   []string
		wantBinaryPaths []string
	}{
		{
			name: "mixed text and binary files",
			input: domain.Diff{
				FromCommitHash: "abc123",
				ToCommitHash:   "def456",
				Files: []domain.FileDiff{
					{Path: "main.go", IsBinary: false},
					{Path: "image.png", IsBinary: true},
					{Path: "utils.go", IsBinary: false},
					{Path: "data.bin", IsBinary: true},
				},
			},
			wantTextCount:   2,
			wantBinaryCount: 2,
			wantTextPaths:   []string{"main.go", "utils.go"},
			wantBinaryPaths: []string{"image.png", "data.bin"},
		},
		{
			name: "all text files",
			input: domain.Diff{
				FromCommitHash: "abc",
				ToCommitHash:   "def",
				Files: []domain.FileDiff{
					{Path: "a.go", IsBinary: false},
					{Path: "b.go", IsBinary: false},
				},
			},
			wantTextCount:   2,
			wantBinaryCount: 0,
			wantTextPaths:   []string{"a.go", "b.go"},
			wantBinaryPaths: []string{},
		},
		{
			name: "all binary files",
			input: domain.Diff{
				FromCommitHash: "abc",
				ToCommitHash:   "def",
				Files: []domain.FileDiff{
					{Path: "a.png", IsBinary: true},
					{Path: "b.jpg", IsBinary: true},
				},
			},
			wantTextCount:   0,
			wantBinaryCount: 2,
			wantTextPaths:   []string{},
			wantBinaryPaths: []string{"a.png", "b.jpg"},
		},
		{
			name: "empty diff",
			input: domain.Diff{
				FromCommitHash: "abc",
				ToCommitHash:   "def",
				Files:          []domain.FileDiff{},
			},
			wantTextCount:   0,
			wantBinaryCount: 0,
			wantTextPaths:   []string{},
			wantBinaryPaths: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			textDiff, binaryFiles := review.FilterBinaryFiles(tt.input)

			// Verify counts
			if len(textDiff.Files) != tt.wantTextCount {
				t.Errorf("text file count = %d, want %d", len(textDiff.Files), tt.wantTextCount)
			}
			if len(binaryFiles) != tt.wantBinaryCount {
				t.Errorf("binary file count = %d, want %d", len(binaryFiles), tt.wantBinaryCount)
			}

			// Verify commit hashes are preserved
			if textDiff.FromCommitHash != tt.input.FromCommitHash {
				t.Errorf("FromCommitHash = %s, want %s", textDiff.FromCommitHash, tt.input.FromCommitHash)
			}
			if textDiff.ToCommitHash != tt.input.ToCommitHash {
				t.Errorf("ToCommitHash = %s, want %s", textDiff.ToCommitHash, tt.input.ToCommitHash)
			}

			// Verify text file paths
			for i, wantPath := range tt.wantTextPaths {
				if i < len(textDiff.Files) && textDiff.Files[i].Path != wantPath {
					t.Errorf("text file[%d].Path = %s, want %s", i, textDiff.Files[i].Path, wantPath)
				}
			}

			// Verify binary file paths
			for i, wantPath := range tt.wantBinaryPaths {
				if i < len(binaryFiles) && binaryFiles[i].Path != wantPath {
					t.Errorf("binary file[%d].Path = %s, want %s", i, binaryFiles[i].Path, wantPath)
				}
			}
		})
	}
}

// mockReviewerRegistry is a mock implementation of ReviewerRegistry for testing.
type mockReviewerRegistry struct {
	reviewers        map[string]domain.Reviewer
	defaultReviewers []string
	resolveErr       error
}

// createTestReviewerDeps creates a ReviewerRegistry and PersonaPromptBuilder for testing.
// It creates a single reviewer named "default" that uses the specified provider.
func createTestReviewerDeps(providerName string) (review.ReviewerRegistry, *review.PersonaPromptBuilder) {
	// Use provider name as reviewer name to maintain backward-compatible path keys.
	// This ensures result.MarkdownPaths[providerName] works as expected.
	reviewer := domain.Reviewer{
		Name:     providerName,
		Provider: providerName,
		Model:    "test-model",
		Weight:   1.0,
		Enabled:  true,
	}
	registry := &mockReviewerRegistry{
		reviewers: map[string]domain.Reviewer{
			providerName: reviewer,
		},
		defaultReviewers: []string{providerName},
	}
	baseBuilder := review.NewEnhancedPromptBuilder()
	personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)
	return registry, personaBuilder
}

// createMultiProviderTestDeps creates a ReviewerRegistry with multiple reviewers for testing.
// Uses provider names as reviewer names to maintain backward-compatible path keys.
func createMultiProviderTestDeps(providerNames []string) (review.ReviewerRegistry, *review.PersonaPromptBuilder) {
	reviewers := make(map[string]domain.Reviewer)
	defaults := make([]string, 0, len(providerNames))
	for _, name := range providerNames {
		// Use provider name as reviewer name for consistent path keys
		reviewers[name] = domain.Reviewer{
			Name:     name,
			Provider: name,
			Model:    "test-model",
			Weight:   1.0,
			Enabled:  true,
		}
		defaults = append(defaults, name)
	}
	registry := &mockReviewerRegistry{
		reviewers:        reviewers,
		defaultReviewers: defaults,
	}
	baseBuilder := review.NewEnhancedPromptBuilder()
	personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)
	return registry, personaBuilder
}

func (m *mockReviewerRegistry) Get(name string) (domain.Reviewer, error) {
	reviewer, ok := m.reviewers[name]
	if !ok {
		return domain.Reviewer{}, review.ErrReviewerNotFound
	}
	return reviewer, nil
}

func (m *mockReviewerRegistry) List() []domain.Reviewer {
	var result []domain.Reviewer
	for _, r := range m.reviewers {
		result = append(result, r)
	}
	return result
}

func (m *mockReviewerRegistry) ListEnabled() []domain.Reviewer {
	var result []domain.Reviewer
	for _, r := range m.reviewers {
		if r.Enabled {
			result = append(result, r)
		}
	}
	return result
}

func (m *mockReviewerRegistry) Resolve(names []string) ([]domain.Reviewer, error) {
	if m.resolveErr != nil {
		return nil, m.resolveErr
	}

	// Use defaults if no names provided
	if len(names) == 0 {
		names = m.defaultReviewers
	}

	var reviewers []domain.Reviewer
	for _, name := range names {
		if r, ok := m.reviewers[name]; ok && r.Enabled {
			reviewers = append(reviewers, r)
		}
	}

	if len(reviewers) == 0 {
		return nil, review.ErrNoEnabledReviewers
	}

	return reviewers, nil
}

func TestReviewBranchWithReviewerDispatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	diff := domain.Diff{
		FromCommitHash: "abc",
		ToCommitHash:   "def",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,2 @@\n+package main\n+func main() {}"},
		},
	}

	// Create reviewer with weight
	securityReviewer := domain.Reviewer{
		Name:     "security",
		Provider: "stub-provider",
		Model:    "test-model",
		Weight:   1.5,
		Persona:  "You are a security expert.",
		Focus:    []string{"security"},
		Enabled:  true,
	}

	expectedReview := domain.Review{
		ProviderName: "stub-provider",
		ModelName:    "test-model",
		Summary:      "Security review complete.",
		Findings: []domain.Finding{
			{
				ID:          "hash1",
				File:        "main.go",
				LineStart:   1,
				LineEnd:     2,
				Severity:    "high",
				Category:    "security",
				Description: "Potential vulnerability",
				Suggestion:  "Add validation",
				Evidence:    true,
			},
		},
	}

	gitMock := &mockGitEngine{diff: diff}
	providerMock := &mockProvider{response: expectedReview}
	writerMock := &mockMarkdownWriter{}
	mergerMock := &mockMerger{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}

	registry := &mockReviewerRegistry{
		reviewers: map[string]domain.Reviewer{
			"security": securityReviewer,
		},
		defaultReviewers: []string{"security"},
	}

	baseBuilder := review.NewEnhancedPromptBuilder()
	personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"stub-provider": providerMock,
		},
		Merger:               mergerMock,
		Markdown:             writerMock,
		JSON:                 jsonWriterMock,
		SARIF:                sarifWriterMock,
		SeedGenerator:        func(baseRef, targetRef string) uint64 { return 42 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	result, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature",
		OutputDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify provider was called
	if len(providerMock.requests) != 1 {
		t.Fatalf("expected provider to be called once, got %d", len(providerMock.requests))
	}

	// Verify the request has the reviewer name
	if providerMock.requests[0].ReviewerName != "security" {
		t.Errorf("expected reviewer name 'security', got '%s'", providerMock.requests[0].ReviewerName)
	}

	// Verify findings are tagged with reviewer metadata
	if len(result.Reviews) < 1 {
		t.Fatalf("expected at least 1 review, got %d", len(result.Reviews))
	}

	// Find the review from stub-provider (not merged)
	var reviewFromProvider domain.Review
	for _, r := range result.Reviews {
		if r.ProviderName == "stub-provider" {
			reviewFromProvider = r
			break
		}
	}

	if len(reviewFromProvider.Findings) == 0 {
		t.Fatalf("expected findings in review, got 0")
	}

	// Verify findings have reviewer metadata
	finding := reviewFromProvider.Findings[0]
	if finding.ReviewerName != "security" {
		t.Errorf("expected finding.ReviewerName = 'security', got '%s'", finding.ReviewerName)
	}
	if finding.ReviewerWeight != 1.5 {
		t.Errorf("expected finding.ReviewerWeight = 1.5, got %f", finding.ReviewerWeight)
	}
}

func TestReviewBranchWithMultipleReviewers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	diff := domain.Diff{
		FromCommitHash: "abc",
		ToCommitHash:   "def",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,2 @@\n+package main\n+func main() {}"},
		},
	}

	// Create two reviewers using different providers
	securityReviewer := domain.Reviewer{
		Name:     "security",
		Provider: "provider-a",
		Model:    "model-a",
		Weight:   1.5,
		Persona:  "Security expert",
		Enabled:  true,
	}

	maintainabilityReviewer := domain.Reviewer{
		Name:     "maintainability",
		Provider: "provider-b",
		Model:    "model-b",
		Weight:   1.0,
		Persona:  "Maintainability expert",
		Enabled:  true,
	}

	reviewA := domain.Review{
		ProviderName: "provider-a",
		ModelName:    "model-a",
		Summary:      "Security review",
		Findings: []domain.Finding{
			{ID: "a1", File: "main.go", LineStart: 1, LineEnd: 1, Severity: "high", Category: "security", Description: "Issue A"},
		},
	}

	reviewB := domain.Review{
		ProviderName: "provider-b",
		ModelName:    "model-b",
		Summary:      "Maintainability review",
		Findings: []domain.Finding{
			{ID: "b1", File: "main.go", LineStart: 1, LineEnd: 1, Severity: "medium", Category: "complexity", Description: "Issue B"},
		},
	}

	gitMock := &mockGitEngine{diff: diff}
	providerA := &mockProvider{response: reviewA}
	providerB := &mockProvider{response: reviewB}
	writerMock := &mockMarkdownWriter{}
	mergerMock := &mockMerger{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}

	registry := &mockReviewerRegistry{
		reviewers: map[string]domain.Reviewer{
			"security":        securityReviewer,
			"maintainability": maintainabilityReviewer,
		},
		defaultReviewers: []string{"security", "maintainability"},
	}

	baseBuilder := review.NewEnhancedPromptBuilder()
	personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"provider-a": providerA,
			"provider-b": providerB,
		},
		Merger:               mergerMock,
		Markdown:             writerMock,
		JSON:                 jsonWriterMock,
		SARIF:                sarifWriterMock,
		SeedGenerator:        func(baseRef, targetRef string) uint64 { return 42 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	result, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature",
		OutputDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify both providers were called
	if len(providerA.requests) != 1 {
		t.Errorf("expected provider-a to be called once, got %d", len(providerA.requests))
	}
	if len(providerB.requests) != 1 {
		t.Errorf("expected provider-b to be called once, got %d", len(providerB.requests))
	}

	// Verify we have 3 reviews (2 from reviewers + 1 merged)
	if len(result.Reviews) != 3 {
		t.Fatalf("expected 3 reviews (2 reviewers + 1 merged), got %d", len(result.Reviews))
	}

	// Verify merger was called with both reviews
	if len(mergerMock.calls) != 1 {
		t.Fatalf("expected merger to be called once, got %d", len(mergerMock.calls))
	}
	if len(mergerMock.calls[0]) != 2 {
		t.Errorf("expected merger to receive 2 reviews, got %d", len(mergerMock.calls[0]))
	}

	// Verify findings have correct reviewer metadata
	for _, rev := range result.Reviews {
		if rev.ProviderName == "merged" {
			continue // Skip merged review
		}

		for _, finding := range rev.Findings {
			switch rev.ProviderName {
			case "provider-a":
				if finding.ReviewerName != "security" {
					t.Errorf("expected finding from provider-a to have ReviewerName='security', got '%s'", finding.ReviewerName)
				}
				if finding.ReviewerWeight != 1.5 {
					t.Errorf("expected finding from provider-a to have ReviewerWeight=1.5, got %f", finding.ReviewerWeight)
				}
			case "provider-b":
				if finding.ReviewerName != "maintainability" {
					t.Errorf("expected finding from provider-b to have ReviewerName='maintainability', got '%s'", finding.ReviewerName)
				}
				if finding.ReviewerWeight != 1.0 {
					t.Errorf("expected finding from provider-b to have ReviewerWeight=1.0, got %f", finding.ReviewerWeight)
				}
			}
		}
	}
}

func TestReviewBranchReviewerDispatchMissingProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	diff := domain.Diff{
		FromCommitHash: "abc",
		ToCommitHash:   "def",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,2 @@\n+package main\n+func main() {}"},
		},
	}

	// Reviewer references a provider that doesn't exist
	reviewer := domain.Reviewer{
		Name:     "test-reviewer",
		Provider: "nonexistent-provider",
		Model:    "test-model",
		Weight:   1.0,
		Enabled:  true,
	}

	gitMock := &mockGitEngine{diff: diff}
	writerMock := &mockMarkdownWriter{}
	mergerMock := &mockMerger{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}

	registry := &mockReviewerRegistry{
		reviewers: map[string]domain.Reviewer{
			"test-reviewer": reviewer,
		},
		defaultReviewers: []string{"test-reviewer"},
	}

	baseBuilder := review.NewEnhancedPromptBuilder()
	personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)

	// Provide a different provider so validation passes, but not the one the reviewer needs
	dummyProvider := &mockProvider{response: domain.Review{Summary: "dummy"}}
	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"some-other-provider": dummyProvider, // Not "nonexistent-provider"
		},
		Merger:               mergerMock,
		Markdown:             writerMock,
		JSON:                 jsonWriterMock,
		SARIF:                sarifWriterMock,
		SeedGenerator:        func(baseRef, targetRef string) uint64 { return 42 },
		ReviewerRegistry:     registry,
		PersonaPromptBuilder: personaBuilder,
	})

	_, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature",
		OutputDir: t.TempDir(),
	})

	// Should return an error about missing provider
	if err == nil {
		t.Fatal("expected error for missing provider, got nil")
	}

	if !strings.Contains(err.Error(), "nonexistent-provider") {
		t.Errorf("expected error to mention missing provider, got: %v", err)
	}
}

func TestReviewBranchConcurrencyLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	diff := domain.Diff{
		FromCommitHash: "abc",
		ToCommitHash:   "def",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,2 @@\n+package main\n+func main() {}"},
		},
	}

	// Create 5 reviewers to test concurrency limiting
	reviewers := make(map[string]domain.Reviewer)
	defaultReviewers := make([]string, 5)
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("reviewer-%d", i)
		reviewers[name] = domain.Reviewer{
			Name:     name,
			Provider: "shared-provider",
			Model:    "test-model",
			Weight:   1.0,
			Enabled:  true,
		}
		defaultReviewers[i] = name
	}

	// Track concurrent executions
	var (
		mu             sync.Mutex
		currentCount   int
		maxConcurrent  int
		callTimestamps []time.Time
	)

	concurrencyTrackingProvider := &concurrencyTrackingMockProvider{
		response: domain.Review{
			ProviderName: "shared-provider",
			ModelName:    "test-model",
			Summary:      "Test review",
			Findings:     []domain.Finding{},
		},
		delay: 50 * time.Millisecond, // Small delay to allow concurrency measurement
		onCall: func() {
			mu.Lock()
			currentCount++
			if currentCount > maxConcurrent {
				maxConcurrent = currentCount
			}
			callTimestamps = append(callTimestamps, time.Now())
			mu.Unlock()
		},
		onReturn: func() {
			mu.Lock()
			currentCount--
			mu.Unlock()
		},
	}

	gitMock := &mockGitEngine{diff: diff}
	writerMock := &mockMarkdownWriter{}
	mergerMock := &mockMerger{}
	jsonWriterMock := &mockJSONWriter{}
	sarifWriterMock := &mockSARIFWriter{}

	registry := &mockReviewerRegistry{
		reviewers:        reviewers,
		defaultReviewers: defaultReviewers,
	}

	baseBuilder := review.NewEnhancedPromptBuilder()
	personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)

	t.Run("unlimited concurrency", func(t *testing.T) {
		mu.Lock()
		currentCount = 0
		maxConcurrent = 0
		callTimestamps = nil
		mu.Unlock()

		orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
			Git: gitMock,
			Providers: map[string]review.Provider{
				"shared-provider": concurrencyTrackingProvider,
			},
			Merger:                 mergerMock,
			Markdown:               writerMock,
			JSON:                   jsonWriterMock,
			SARIF:                  sarifWriterMock,
			SeedGenerator:          func(baseRef, targetRef string) uint64 { return 42 },
			ReviewerRegistry:       registry,
			PersonaPromptBuilder:   personaBuilder,
			MaxConcurrentReviewers: 0, // Unlimited
		})

		_, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
			BaseRef:   "main",
			TargetRef: "feature",
			OutputDir: t.TempDir(),
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		mu.Lock()
		observed := maxConcurrent
		mu.Unlock()

		// With unlimited concurrency, all 5 should run at once
		if observed < 3 {
			t.Errorf("expected at least 3 concurrent reviewers with unlimited concurrency, got %d", observed)
		}
	})

	t.Run("limited to 2 concurrent", func(t *testing.T) {
		mu.Lock()
		currentCount = 0
		maxConcurrent = 0
		callTimestamps = nil
		mu.Unlock()

		orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
			Git: gitMock,
			Providers: map[string]review.Provider{
				"shared-provider": concurrencyTrackingProvider,
			},
			Merger:                 mergerMock,
			Markdown:               writerMock,
			JSON:                   jsonWriterMock,
			SARIF:                  sarifWriterMock,
			SeedGenerator:          func(baseRef, targetRef string) uint64 { return 42 },
			ReviewerRegistry:       registry,
			PersonaPromptBuilder:   personaBuilder,
			MaxConcurrentReviewers: 2, // Limited to 2
		})

		_, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
			BaseRef:   "main",
			TargetRef: "feature",
			OutputDir: t.TempDir(),
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		mu.Lock()
		observed := maxConcurrent
		mu.Unlock()

		// With limit of 2, should never exceed 2 concurrent
		if observed > 2 {
			t.Errorf("expected max 2 concurrent reviewers, got %d", observed)
		}
	})
}

// concurrencyTrackingMockProvider tracks concurrent invocations
type concurrencyTrackingMockProvider struct {
	response domain.Review
	delay    time.Duration
	onCall   func()
	onReturn func()
}

func (m *concurrencyTrackingMockProvider) Review(ctx context.Context, req review.ProviderRequest) (domain.Review, error) {
	if m.onCall != nil {
		m.onCall()
	}
	defer func() {
		if m.onReturn != nil {
			m.onReturn()
		}
	}()

	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	return m.response, nil
}

func (m *concurrencyTrackingMockProvider) EstimateTokens(text string) int {
	return len(text) / 4 // Simple estimate for testing
}

// mockRepoAccessChecker implements review.RepoAccessChecker for testing
type mockRepoAccessChecker struct {
	err       error
	called    bool
	isPrivate bool
	ownerType string
}

func (m *mockRepoAccessChecker) CanAccessRepo(isPrivate bool, ownerType string) error {
	m.called = true
	m.isPrivate = isPrivate
	m.ownerType = ownerType
	return m.err
}

// mockRemoteGitHubClientForEntitlements implements review.RemoteGitHubClient for entitlement tests
type mockRemoteGitHubClientForEntitlements struct {
	metadata    *domain.PRMetadata
	metadataErr error
}

func (m *mockRemoteGitHubClientForEntitlements) GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error) {
	return m.metadata, m.metadataErr
}

func (m *mockRemoteGitHubClientForEntitlements) GetPRDiff(ctx context.Context, owner, repo string, prNumber int) (domain.Diff, error) {
	return domain.Diff{}, nil
}

func (m *mockRemoteGitHubClientForEntitlements) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	return "", nil
}

func TestReviewBranchEntitlementCheck(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	diff := domain.Diff{
		FromCommitHash: "abc",
		ToCommitHash:   "def",
		Files:          []domain.FileDiff{{Path: "main.go", Status: "modified", Patch: "@@ -0,0 +1,2 @@\n+package main\n+func main() {}"}},
	}

	t.Run("blocks access when RepoAccessChecker denies", func(t *testing.T) {
		gitMock := &mockGitEngine{diff: diff}
		writerMock := &mockMarkdownWriter{}
		mergerMock := &mockMerger{}
		jsonWriterMock := &mockJSONWriter{}
		sarifWriterMock := &mockSARIFWriter{}
		baseBuilder := review.NewEnhancedPromptBuilder()
		personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)

		mockGitHubClient := &mockRemoteGitHubClientForEntitlements{
			metadata: &domain.PRMetadata{
				HeadSHA:   "abc123",
				BaseSHA:   "def456",
				IsPrivate: true,
				OwnerType: "Organization",
			},
		}

		mockChecker := &mockRepoAccessChecker{
			err: fmt.Errorf("Private repository access requires Solo plan or higher"),
		}

		orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
			Git:                  gitMock,
			Providers:            map[string]review.Provider{"test": &mockProvider{response: domain.Review{}}},
			Merger:               mergerMock,
			Markdown:             writerMock,
			JSON:                 jsonWriterMock,
			SARIF:                sarifWriterMock,
			SeedGenerator:        func(baseRef, targetRef string) uint64 { return 42 },
			PersonaPromptBuilder: personaBuilder,
			RemoteGitHubClient:   mockGitHubClient,
		})

		_, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
			BaseRef:           "main",
			TargetRef:         "feature",
			OutputDir:         t.TempDir(),
			PostToGitHub:      true,
			GitHubOwner:       "owner",
			GitHubRepo:        "repo",
			PRNumber:          123,
			CommitSHA:         "abc123",
			RepoAccessChecker: mockChecker,
		})

		if err == nil {
			t.Fatal("expected error when RepoAccessChecker denies access")
		}
		if !strings.Contains(err.Error(), "Private repository access") {
			t.Errorf("expected entitlement error, got: %v", err)
		}
		if !mockChecker.called {
			t.Error("expected RepoAccessChecker to be called")
		}
		if !mockChecker.isPrivate {
			t.Error("expected isPrivate to be true")
		}
		if mockChecker.ownerType != "Organization" {
			t.Errorf("expected ownerType 'Organization', got %q", mockChecker.ownerType)
		}
	})

	t.Run("skips check when RepoAccessChecker is nil (legacy mode)", func(t *testing.T) {
		gitMock := &mockGitEngine{diff: diff}
		writerMock := &mockMarkdownWriter{}
		mergerMock := &mockMerger{}
		jsonWriterMock := &mockJSONWriter{}
		sarifWriterMock := &mockSARIFWriter{}
		baseBuilder := review.NewEnhancedPromptBuilder()
		personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)

		mockGitHubClient := &mockRemoteGitHubClientForEntitlements{
			metadata: &domain.PRMetadata{
				HeadSHA:   "abc123",
				BaseSHA:   "def456",
				IsPrivate: true,
				OwnerType: "Organization",
			},
		}

		orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
			Git:                  gitMock,
			Providers:            map[string]review.Provider{"test": &mockProvider{response: domain.Review{}}},
			Merger:               mergerMock,
			Markdown:             writerMock,
			JSON:                 jsonWriterMock,
			SARIF:                sarifWriterMock,
			SeedGenerator:        func(baseRef, targetRef string) uint64 { return 42 },
			PersonaPromptBuilder: personaBuilder,
			RemoteGitHubClient:   mockGitHubClient,
		})

		// Should succeed even with private org repo when no checker is set
		_, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
			BaseRef:           "main",
			TargetRef:         "feature",
			OutputDir:         t.TempDir(),
			PostToGitHub:      true,
			GitHubOwner:       "owner",
			GitHubRepo:        "repo",
			PRNumber:          123,
			CommitSHA:         "abc123",
			RepoAccessChecker: nil, // Legacy mode - no checker
		})

		// Should succeed (entitlement check skipped)
		if err != nil {
			t.Fatalf("expected no error in legacy mode, got: %v", err)
		}
	})

	t.Run("skips check when PostToGitHub is false", func(t *testing.T) {
		gitMock := &mockGitEngine{diff: diff}
		writerMock := &mockMarkdownWriter{}
		mergerMock := &mockMerger{}
		jsonWriterMock := &mockJSONWriter{}
		sarifWriterMock := &mockSARIFWriter{}
		baseBuilder := review.NewEnhancedPromptBuilder()
		personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)

		// Checker that would deny if called
		mockChecker := &mockRepoAccessChecker{
			err: fmt.Errorf("should not be called"),
		}

		orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
			Git:                  gitMock,
			Providers:            map[string]review.Provider{"test": &mockProvider{response: domain.Review{}}},
			Merger:               mergerMock,
			Markdown:             writerMock,
			JSON:                 jsonWriterMock,
			SARIF:                sarifWriterMock,
			SeedGenerator:        func(baseRef, targetRef string) uint64 { return 42 },
			PersonaPromptBuilder: personaBuilder,
			// No RemoteGitHubClient needed when not posting
		})

		_, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
			BaseRef:           "main",
			TargetRef:         "feature",
			OutputDir:         t.TempDir(),
			PostToGitHub:      false, // Not posting to GitHub
			RepoAccessChecker: mockChecker,
		})

		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if mockChecker.called {
			t.Error("RepoAccessChecker should not be called when PostToGitHub is false")
		}
	})

	t.Run("allows access when RepoAccessChecker permits", func(t *testing.T) {
		gitMock := &mockGitEngine{diff: diff}
		writerMock := &mockMarkdownWriter{}
		mergerMock := &mockMerger{}
		jsonWriterMock := &mockJSONWriter{}
		sarifWriterMock := &mockSARIFWriter{}
		baseBuilder := review.NewEnhancedPromptBuilder()
		personaBuilder := review.NewPersonaPromptBuilder(baseBuilder)

		mockGitHubClient := &mockRemoteGitHubClientForEntitlements{
			metadata: &domain.PRMetadata{
				HeadSHA:   "abc123",
				BaseSHA:   "def456",
				IsPrivate: true,
				OwnerType: "User",
			},
		}

		mockChecker := &mockRepoAccessChecker{
			err: nil, // Access allowed
		}

		orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
			Git:                  gitMock,
			Providers:            map[string]review.Provider{"test": &mockProvider{response: domain.Review{}}},
			Merger:               mergerMock,
			Markdown:             writerMock,
			JSON:                 jsonWriterMock,
			SARIF:                sarifWriterMock,
			SeedGenerator:        func(baseRef, targetRef string) uint64 { return 42 },
			PersonaPromptBuilder: personaBuilder,
			RemoteGitHubClient:   mockGitHubClient,
		})

		_, err := orchestrator.ReviewBranch(ctx, review.BranchRequest{
			BaseRef:           "main",
			TargetRef:         "feature",
			OutputDir:         t.TempDir(),
			PostToGitHub:      true,
			GitHubOwner:       "owner",
			GitHubRepo:        "repo",
			PRNumber:          123,
			CommitSHA:         "abc123",
			RepoAccessChecker: mockChecker,
		})

		if err != nil {
			t.Fatalf("expected no error when access is allowed, got: %v", err)
		}
		if !mockChecker.called {
			t.Error("expected RepoAccessChecker to be called")
		}
	})
}
