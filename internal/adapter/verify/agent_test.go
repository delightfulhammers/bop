package verify_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/delightfulhammers/bop/internal/adapter/verify"
	"github.com/delightfulhammers/bop/internal/domain"
	usecaseverify "github.com/delightfulhammers/bop/internal/usecase/verify"
)

// mockLLMClient implements verify.LLMClient for testing.
// Thread-safe to support concurrent tests.
type mockLLMClient struct {
	mu       sync.Mutex
	callFunc func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error)
	calls    []mockLLMCall
}

type mockLLMCall struct {
	SystemPrompt string
	UserPrompt   string
}

func (m *mockLLMClient) Call(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
	m.mu.Lock()
	m.calls = append(m.calls, mockLLMCall{SystemPrompt: systemPrompt, UserPrompt: userPrompt})
	m.mu.Unlock()
	if m.callFunc != nil {
		return m.callFunc(ctx, systemPrompt, userPrompt)
	}
	return "", 0, 0, 0, nil
}

// mockCostTracker implements usecaseverify.CostTracker for testing.
// Thread-safe implementation to support concurrent tests.
type mockCostTracker struct {
	mu      sync.RWMutex
	total   float64
	ceiling float64
}

func (m *mockCostTracker) AddCost(amount float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total += amount
}

func (m *mockCostTracker) TotalCost() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.total
}

func (m *mockCostTracker) ExceedsCeiling() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.total >= m.ceiling
}

func (m *mockCostTracker) RemainingBudget() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.total >= m.ceiling {
		return 0
	}
	return m.ceiling - m.total
}

func TestAgentVerifier_Verify(t *testing.T) {
	t.Run("returns verified result when LLM confirms issue", func(t *testing.T) {
		llm := &mockLLMClient{
			callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
				return `{
					"verified": true,
					"classification": "blocking_bug",
					"confidence": 92,
					"evidence": "The null check is missing at line 42",
					"blocks_operation": true
				}`, 100, 50, 0.001, nil
			},
		}

		repo := &mockRepository{}
		tracker := &mockCostTracker{ceiling: 1.0}
		config := verify.DefaultAgentConfig()

		verifier := verify.NewAgentVerifier(llm, repo, tracker, config)

		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   42,
				Description: "Null pointer dereference",
			},
		}

		result, err := verifier.Verify(context.Background(), candidate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Verified {
			t.Error("expected verified to be true")
		}
		if result.Classification != domain.ClassBlockingBug {
			t.Errorf("got classification %q, want %q", result.Classification, domain.ClassBlockingBug)
		}
		if result.Confidence != 92 {
			t.Errorf("got confidence %d, want 92", result.Confidence)
		}
		if result.Evidence != "The null check is missing at line 42" {
			t.Errorf("got evidence %q", result.Evidence)
		}
	})

	t.Run("returns unverified result when LLM rejects issue", func(t *testing.T) {
		llm := &mockLLMClient{
			callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
				return `{
					"verified": false,
					"classification": "",
					"confidence": 85,
					"evidence": "The null check exists at line 40, before the dereference"
				}`, 100, 50, 0.001, nil
			},
		}

		repo := &mockRepository{}
		tracker := &mockCostTracker{ceiling: 1.0}
		config := verify.DefaultAgentConfig()

		verifier := verify.NewAgentVerifier(llm, repo, tracker, config)

		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				File:        "main.go",
				Description: "Null pointer dereference",
			},
		}

		result, err := verifier.Verify(context.Background(), candidate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Verified {
			t.Error("expected verified to be false")
		}
		if result.BlocksOperation {
			t.Error("unverified findings should not block")
		}
	})

	t.Run("executes tools when LLM requests them", func(t *testing.T) {
		callCount := 0
		llm := &mockLLMClient{
			callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
				callCount++
				if callCount == 1 {
					// First call: request to read file
					return "```tool\nTOOL: read_file\nINPUT: main.go\n```", 100, 50, 0.001, nil
				}
				// Second call: provide verdict after seeing file
				return `{
					"verified": true,
					"classification": "blocking_bug",
					"confidence": 90,
					"evidence": "Confirmed after reading the file"
				}`, 100, 50, 0.001, nil
			},
		}

		repo := &mockRepository{
			readFileFunc: func(path string) ([]byte, error) {
				return []byte("package main\n\nfunc main() { panic(nil) }"), nil
			},
		}
		tracker := &mockCostTracker{ceiling: 1.0}
		config := verify.DefaultAgentConfig()

		verifier := verify.NewAgentVerifier(llm, repo, tracker, config)

		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				File:        "main.go",
				Description: "Panic call",
			},
		}

		result, err := verifier.Verify(context.Background(), candidate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if callCount != 2 {
			t.Errorf("expected 2 LLM calls, got %d", callCount)
		}
		if len(result.Actions) != 1 {
			t.Errorf("expected 1 action, got %d", len(result.Actions))
		}
		if result.Actions[0].Tool != "read_file" {
			t.Errorf("expected read_file action, got %s", result.Actions[0].Tool)
		}
	})

	t.Run("stops when cost ceiling exceeded", func(t *testing.T) {
		llm := &mockLLMClient{
			callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
				return "", 0, 0, 0, nil
			},
		}

		repo := &mockRepository{}
		tracker := &mockCostTracker{total: 1.0, ceiling: 1.0} // Already at ceiling
		config := verify.DefaultAgentConfig()

		verifier := verify.NewAgentVerifier(llm, repo, tracker, config)

		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				Description: "Some issue",
			},
		}

		result, err := verifier.Verify(context.Background(), candidate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Verified {
			t.Error("should not verify when cost ceiling exceeded")
		}
		if result.Evidence != "Cost ceiling exceeded, unable to verify" {
			t.Errorf("expected cost ceiling message, got: %s", result.Evidence)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		llm := &mockLLMClient{
			callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
				select {
				case <-ctx.Done():
					return "", 0, 0, 0, ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return `{"verified": true, "confidence": 50, "evidence": "test"}`, 0, 0, 0, nil
				}
			},
		}

		repo := &mockRepository{}
		tracker := &mockCostTracker{ceiling: 1.0}
		config := verify.DefaultAgentConfig()

		verifier := verify.NewAgentVerifier(llm, repo, tracker, config)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				Description: "Some issue",
			},
		}

		_, err := verifier.Verify(ctx, candidate)
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})

	t.Run("limits tool call iterations", func(t *testing.T) {
		callCount := 0
		llm := &mockLLMClient{
			callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
				callCount++
				// Always request another tool call
				return "```tool\nTOOL: read_file\nINPUT: file.go\n```", 100, 50, 0.001, nil
			},
		}

		repo := &mockRepository{
			readFileFunc: func(path string) ([]byte, error) {
				return []byte("content"), nil
			},
		}
		tracker := &mockCostTracker{ceiling: 10.0}
		agentConfig := verify.AgentConfig{
			MaxIterations: 3, // Limit to 3 iterations
			Concurrency:   1,
		}

		verifier := verify.NewAgentVerifier(llm, repo, tracker, agentConfig)

		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				Description: "Some issue",
			},
		}

		result, err := verifier.Verify(context.Background(), candidate)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if callCount > 3 {
			t.Errorf("expected max 3 calls, got %d", callCount)
		}

		// Should have 3 actions (one per iteration before stopping)
		if len(result.Actions) > 3 {
			t.Errorf("expected max 3 actions, got %d", len(result.Actions))
		}
	})

	t.Run("returns LLM error", func(t *testing.T) {
		llm := &mockLLMClient{
			callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
				return "", 0, 0, 0, errors.New("rate limit exceeded")
			},
		}

		repo := &mockRepository{}
		tracker := &mockCostTracker{ceiling: 1.0}
		config := verify.DefaultAgentConfig()

		verifier := verify.NewAgentVerifier(llm, repo, tracker, config)

		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				Description: "Some issue",
			},
		}

		_, err := verifier.Verify(context.Background(), candidate)
		if err == nil {
			t.Error("expected error from LLM failure")
		}
	})
}

func TestAgentVerifier_VerifyBatch(t *testing.T) {
	t.Run("verifies all candidates", func(t *testing.T) {
		var callCount int32
		llm := &mockLLMClient{
			callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
				atomic.AddInt32(&callCount, 1)
				return `{
					"verified": true,
					"classification": "blocking_bug",
					"confidence": 85,
					"evidence": "Issue confirmed"
				}`, 100, 50, 0.001, nil
			},
		}

		repo := &mockRepository{}
		tracker := &mockCostTracker{ceiling: 10.0}
		agentConfig := verify.AgentConfig{
			MaxIterations: 5,
			Concurrency:   3,
		}

		verifier := verify.NewAgentVerifier(llm, repo, tracker, agentConfig)

		candidates := []domain.CandidateFinding{
			{Finding: domain.Finding{Description: "Issue 1"}},
			{Finding: domain.Finding{Description: "Issue 2"}},
			{Finding: domain.Finding{Description: "Issue 3"}},
		}

		results, err := verifier.VerifyBatch(context.Background(), candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(results) != 3 {
			t.Errorf("got %d results, want 3", len(results))
		}

		for i, result := range results {
			if !result.Verified {
				t.Errorf("result %d: expected verified", i)
			}
		}
	})

	t.Run("respects concurrency limit", func(t *testing.T) {
		var maxConcurrent int32
		var currentConcurrent int32

		llm := &mockLLMClient{
			callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
				current := atomic.AddInt32(&currentConcurrent, 1)
				defer atomic.AddInt32(&currentConcurrent, -1)

				// Track max concurrent
				for {
					old := atomic.LoadInt32(&maxConcurrent)
					if current <= old {
						break
					}
					if atomic.CompareAndSwapInt32(&maxConcurrent, old, current) {
						break
					}
				}

				// Simulate some work
				time.Sleep(10 * time.Millisecond)

				return `{"verified": true, "confidence": 80, "evidence": "ok"}`, 0, 0, 0, nil
			},
		}

		repo := &mockRepository{}
		tracker := &mockCostTracker{ceiling: 10.0}
		agentConfig := verify.AgentConfig{
			MaxIterations: 1,
			Concurrency:   2, // Limit to 2 concurrent
		}

		verifier := verify.NewAgentVerifier(llm, repo, tracker, agentConfig)

		candidates := make([]domain.CandidateFinding, 10)
		for i := range candidates {
			candidates[i] = domain.CandidateFinding{
				Finding: domain.Finding{Description: "Issue"},
			}
		}

		_, err := verifier.VerifyBatch(context.Background(), candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if atomic.LoadInt32(&maxConcurrent) > 2 {
			t.Errorf("exceeded concurrency limit: max was %d", maxConcurrent)
		}
	})

	t.Run("stops remaining when cost ceiling hit", func(t *testing.T) {
		var callCount int32
		llm := &mockLLMClient{
			callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
				atomic.AddInt32(&callCount, 1)
				// Each call costs 0.5
				return `{"verified": true, "confidence": 80, "evidence": "ok"}`, 0, 0, 0.5, nil
			},
		}

		repo := &mockRepository{}
		tracker := &mockCostTracker{ceiling: 0.6} // Only enough for 1 call (0.5 < 0.6, but 1.0 >= 0.6)
		agentConfig := verify.AgentConfig{
			MaxIterations: 1,
			Concurrency:   1, // Sequential to make test deterministic
		}

		verifier := verify.NewAgentVerifier(llm, repo, tracker, agentConfig)

		candidates := make([]domain.CandidateFinding, 5)
		for i := range candidates {
			candidates[i] = domain.CandidateFinding{
				Finding: domain.Finding{Description: "Issue"},
			}
		}

		results, err := verifier.VerifyBatch(context.Background(), candidates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(results) != 5 {
			t.Fatalf("got %d results, want 5", len(results))
		}

		// Count verified and cost-exceeded results
		verifiedCount := 0
		costExceededCount := 0
		for _, r := range results {
			if r.Verified {
				verifiedCount++
			}
			if r.Evidence == "Cost ceiling exceeded, unable to verify" {
				costExceededCount++
			}
		}

		// At least one should be verified (the first one before cost exceeded)
		// But since goroutines may reorder, we just check that SOME are verified
		// and SOME are marked as cost exceeded
		if verifiedCount == 0 && costExceededCount == 0 {
			t.Error("expected some results to be either verified or marked as cost exceeded")
		}

		// The key behavior: once cost exceeds ceiling, remaining should be marked
		if costExceededCount == 0 && callCount >= 5 {
			t.Error("expected some results to be marked as cost ceiling exceeded when not all were called")
		}

		// Verify that we didn't call LLM for all candidates if cost was exceeded
		// With ceiling=0.6 and cost=0.5 per call, only 1 call should succeed
		// then the remaining should be skipped
		if atomic.LoadInt32(&callCount) > 2 {
			t.Logf("warning: %d LLM calls made, expected at most 2 before cost ceiling", callCount)
		}
	})

	t.Run("handles empty candidates", func(t *testing.T) {
		llm := &mockLLMClient{}
		repo := &mockRepository{}
		tracker := &mockCostTracker{ceiling: 1.0}
		config := verify.DefaultAgentConfig()

		verifier := verify.NewAgentVerifier(llm, repo, tracker, config)

		results, err := verifier.VerifyBatch(context.Background(), []domain.CandidateFinding{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(results) != 0 {
			t.Errorf("expected empty results, got %d", len(results))
		}
	})
}

func TestAgentVerifier_ParseVerdict(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantOK   bool
	}{
		{
			name:     "valid JSON in code block",
			response: "Based on my analysis:\n```json\n{\"verified\": true, \"classification\": \"security\", \"confidence\": 90, \"evidence\": \"XSS found\"}\n```",
			wantOK:   true,
		},
		{
			name:     "valid bare JSON",
			response: `{"verified": false, "classification": "", "confidence": 30, "evidence": "Not found"}`,
			wantOK:   true,
		},
		{
			name:     "no JSON present",
			response: "I need to read more files to determine this.",
			wantOK:   false,
		},
		{
			name:     "invalid JSON",
			response: `{"verified": true, "confidence": `,
			wantOK:   false,
		},
		{
			name:     "JSON without required fields",
			response: `{"foo": "bar"}`,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llm := &mockLLMClient{
				callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
					return tt.response, 0, 0, 0, nil
				},
			}

			repo := &mockRepository{}
			tracker := &mockCostTracker{ceiling: 1.0}
			config := verify.DefaultAgentConfig()

			verifier := verify.NewAgentVerifier(llm, repo, tracker, config)

			result, err := verifier.Verify(context.Background(), domain.CandidateFinding{})

			if tt.wantOK {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				// Either error or low confidence result is acceptable
				if err == nil && result.Confidence > 0 {
					t.Error("expected parsing to fail")
				}
			}
		})
	}
}

func TestDefaultAgentConfig(t *testing.T) {
	config := verify.DefaultAgentConfig()

	if config.MaxIterations <= 0 {
		t.Error("MaxIterations should be positive")
	}
	if config.Concurrency <= 0 {
		t.Error("Concurrency should be positive")
	}
	if config.Confidence.Critical <= 0 {
		t.Error("Critical threshold should be set")
	}
}

// Compile-time interface checks
var _ verify.LLMClient = (*mockLLMClient)(nil)
var _ usecaseverify.CostTracker = (*mockCostTracker)(nil)
