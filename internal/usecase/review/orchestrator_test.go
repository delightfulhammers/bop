package review_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/review"
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

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"stub-openai": providerMock,
		},
		Merger:        mergerMock,
		Markdown:      writerMock,
		JSON:          jsonWriterMock,
		SARIF:         sarifWriterMock,
		SeedGenerator: func(baseRef, targetRef string) uint64 { return 42 },
		PromptBuilder: func(ctx review.ProjectContext, d domain.Diff, req review.BranchRequest, providerName string) (review.ProviderRequest, error) {
			if d.ToCommitHash != diff.ToCommitHash {
				t.Fatalf("unexpected diff passed to prompt builder: %+v", d)
			}
			return review.ProviderRequest{
				Prompt:  "prompt",
				Seed:    42,
				MaxSize: 16384,
			}, nil
		},
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

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"provider1": provider1,
			"provider2": provider2,
		},
		Merger:        mergerMock,
		Markdown:      writerMock,
		JSON:          jsonWriterMock,
		SARIF:         sarifWriterMock,
		SeedGenerator: func(_, _ string) uint64 { return 42 },
		PromptBuilder: func(ctx review.ProjectContext, d domain.Diff, req review.BranchRequest, providerName string) (review.ProviderRequest, error) {
			return review.ProviderRequest{Prompt: "prompt", Seed: 42, MaxSize: 16384}, nil
		},
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
	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git:           gitMock,
		Providers:     map[string]review.Provider{"mock": &mockProvider{}},
		Merger:        &mockMerger{},
		Markdown:      &mockMarkdownWriter{},
		JSON:          &mockJSONWriter{},
		SARIF:         &mockSARIFWriter{},
		SeedGenerator: func(_, _ string) uint64 { return 0 },
		PromptBuilder: func(review.ProjectContext, domain.Diff, review.BranchRequest, string) (review.ProviderRequest, error) {
			return review.ProviderRequest{}, nil
		},
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

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"test-provider": providerMock,
		},
		Merger:        mergerMock,
		Markdown:      writerMock,
		JSON:          jsonWriterMock,
		SARIF:         sarifWriterMock,
		Store:         storeMock,
		SeedGenerator: func(baseRef, targetRef string) uint64 { return 12345 },
		PromptBuilder: func(ctx review.ProjectContext, d domain.Diff, req review.BranchRequest, providerName string) (review.ProviderRequest, error) {
			return review.ProviderRequest{
				Prompt:  "test prompt",
				Seed:    12345,
				MaxSize: 16384,
			}, nil
		},
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
	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"test-provider": providerMock,
		},
		Merger:        mergerMock,
		Markdown:      writerMock,
		JSON:          jsonWriterMock,
		SARIF:         sarifWriterMock,
		Store:         nil, // Disabled
		SeedGenerator: func(baseRef, targetRef string) uint64 { return 42 },
		PromptBuilder: func(ctx review.ProjectContext, d domain.Diff, req review.BranchRequest, providerName string) (review.ProviderRequest, error) {
			return review.ProviderRequest{Prompt: "test", Seed: 42, MaxSize: 16384}, nil
		},
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

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"test-provider": providerMock,
		},
		Merger:        mergerMock,
		Markdown:      writerMock,
		JSON:          jsonWriterMock,
		SARIF:         sarifWriterMock,
		Store:         storeMock,
		SeedGenerator: func(baseRef, targetRef string) uint64 { return 42 },
		PromptBuilder: func(ctx review.ProjectContext, d domain.Diff, req review.BranchRequest, providerName string) (review.ProviderRequest, error) {
			return review.ProviderRequest{Prompt: "test", Seed: 42, MaxSize: 16384}, nil
		},
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

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"provider1": provider1,
			"provider2": provider2,
		},
		Merger:        mergerMock,
		Markdown:      writerMock,
		JSON:          jsonWriterMock,
		SARIF:         sarifWriterMock,
		Store:         storeMock,
		SeedGenerator: func(_, _ string) uint64 { return 42 },
		PromptBuilder: func(ctx review.ProjectContext, d domain.Diff, req review.BranchRequest, providerName string) (review.ProviderRequest, error) {
			return review.ProviderRequest{Prompt: "prompt", Seed: 42, MaxSize: 16384}, nil
		},
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

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git: gitMock,
		Providers: map[string]review.Provider{
			"slow-provider": blockingProv,
		},
		Merger:        mergerMock,
		Markdown:      writerMock,
		JSON:          jsonWriterMock,
		SARIF:         sarifWriterMock,
		SeedGenerator: func(_, _ string) uint64 { return 42 },
		PromptBuilder: func(ctx review.ProjectContext, d domain.Diff, req review.BranchRequest, providerName string) (review.ProviderRequest, error) {
			return review.ProviderRequest{Prompt: "prompt", Seed: 42, MaxSize: 8192}, nil
		},
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
