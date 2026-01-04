package verify_test

import (
	"context"
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/verify"
	"github.com/delightfulhammers/bop/internal/domain"
)

func TestBatchVerifier_SingleLLMCall(t *testing.T) {
	// Setup mock LLM that returns valid JSON
	callCount := 0
	mockLLM := &mockLLMClient{
		callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
			callCount++
			return `[
				{"index": 0, "verified": false, "classification": "", "confidence": 95, "evidence": "Import exists at line 4"},
				{"index": 1, "verified": true, "classification": "blocking_bug", "confidence": 80, "evidence": "Nil check missing"}
			]`, 100, 50, 0.001, nil
		},
	}

	mockRepo := &mockRepository{
		readFileFunc: func(path string) ([]byte, error) {
			return []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
}`), nil
		},
	}

	verifier := verify.NewBatchVerifier(mockLLM, mockRepo, nil, verify.DefaultBatchConfig())

	candidates := []domain.CandidateFinding{
		{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   10,
				Severity:    "high",
				Description: "fmt not imported",
			},
			AgreementScore: 1.0,
			Sources:        []string{"provider1"},
		},
		{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   20,
				Severity:    "high",
				Description: "nil pointer dereference",
			},
			AgreementScore: 0.5,
			Sources:        []string{"provider1", "provider2"},
		},
	}

	results, err := verifier.VerifyBatch(context.Background(), candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify only ONE LLM call was made
	if callCount != 1 {
		t.Errorf("expected 1 LLM call, got %d", callCount)
	}

	// Verify results
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Verified {
		t.Error("expected first finding to be NOT verified (false positive)")
	}
	if results[0].Confidence != 95 {
		t.Errorf("expected confidence 95, got %d", results[0].Confidence)
	}

	if !results[1].Verified {
		t.Error("expected second finding to be verified")
	}
	if results[1].Classification != domain.ClassBlockingBug {
		t.Errorf("expected classification blocking_bug, got %s", results[1].Classification)
	}
}

func TestBatchVerifier_EmptyCandidates(t *testing.T) {
	callCount := 0
	mockLLM := &mockLLMClient{
		callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
			callCount++
			return "", 0, 0, 0, nil
		},
	}
	mockRepo := &mockRepository{}

	verifier := verify.NewBatchVerifier(mockLLM, mockRepo, nil, verify.DefaultBatchConfig())

	results, err := verifier.VerifyBatch(context.Background(), []domain.CandidateFinding{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}

	// No LLM call should be made for empty input
	if callCount != 0 {
		t.Errorf("expected 0 LLM calls, got %d", callCount)
	}
}

func TestBatchVerifier_MalformedResponse(t *testing.T) {
	mockLLM := &mockLLMClient{
		callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
			return "This is not valid JSON at all", 100, 50, 0.001, nil
		},
	}
	mockRepo := &mockRepository{
		readFileFunc: func(path string) ([]byte, error) {
			return []byte("package test"), nil
		},
	}

	verifier := verify.NewBatchVerifier(mockLLM, mockRepo, nil, verify.DefaultBatchConfig())

	candidates := []domain.CandidateFinding{
		{
			Finding: domain.Finding{
				File:        "test.go",
				Description: "some issue",
			},
		},
	}

	results, err := verifier.VerifyBatch(context.Background(), candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return unverified results with error message
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Verified {
		t.Error("expected unverified result for malformed response")
	}
	if results[0].Evidence == "" {
		t.Error("expected error evidence in result")
	}
}

func TestBatchVerifier_CostCeilingExceeded(t *testing.T) {
	callCount := 0
	mockLLM := &mockLLMClient{
		callFunc: func(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
			callCount++
			return "[]", 0, 0, 0, nil
		},
	}
	mockRepo := &mockRepository{}

	// Cost tracker that always reports exceeded
	costTracker := &mockCostTracker{
		total:   10.0,
		ceiling: 1.0, // Already exceeded
	}

	verifier := verify.NewBatchVerifier(mockLLM, mockRepo, costTracker, verify.DefaultBatchConfig())

	candidates := []domain.CandidateFinding{
		{Finding: domain.Finding{File: "test.go", Description: "issue"}},
	}

	results, err := verifier.VerifyBatch(context.Background(), candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT call LLM when cost ceiling is exceeded
	if callCount != 0 {
		t.Errorf("expected 0 LLM calls when cost exceeded, got %d", callCount)
	}

	// Should return unverified results
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Verified {
		t.Error("expected unverified when cost ceiling exceeded")
	}
}
