package verify_test

import (
	"context"
	"errors"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/verify"
)

// mockVerifier implements verify.Verifier for testing.
type mockVerifier struct {
	verifyFunc      func(ctx context.Context, candidate domain.CandidateFinding) (domain.VerificationResult, error)
	verifyBatchFunc func(ctx context.Context, candidates []domain.CandidateFinding) ([]domain.VerificationResult, error)
}

func (m *mockVerifier) Verify(ctx context.Context, candidate domain.CandidateFinding) (domain.VerificationResult, error) {
	if m.verifyFunc != nil {
		return m.verifyFunc(ctx, candidate)
	}
	return domain.VerificationResult{}, nil
}

func (m *mockVerifier) VerifyBatch(ctx context.Context, candidates []domain.CandidateFinding) ([]domain.VerificationResult, error) {
	if m.verifyBatchFunc != nil {
		return m.verifyBatchFunc(ctx, candidates)
	}
	return nil, nil
}

// Compile-time check that mockVerifier implements Verifier.
var _ verify.Verifier = (*mockVerifier)(nil)

func TestVerifier_Interface(t *testing.T) {
	t.Run("Verify returns verification result", func(t *testing.T) {
		verifier := &mockVerifier{
			verifyFunc: func(ctx context.Context, candidate domain.CandidateFinding) (domain.VerificationResult, error) {
				return domain.VerificationResult{
					Verified:        true,
					Classification:  domain.ClassBlockingBug,
					Confidence:      92,
					Evidence:        "The null pointer dereference is confirmed",
					BlocksOperation: true,
				}, nil
			},
		}

		candidate := domain.CandidateFinding{
			Finding: domain.Finding{
				File:        "main.go",
				LineStart:   42,
				Severity:    "high",
				Description: "Null pointer dereference",
			},
			Sources:        []string{"openai", "anthropic"},
			AgreementScore: 1.0,
		}

		result, err := verifier.Verify(context.Background(), candidate)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.Verified {
			t.Error("expected Verified to be true")
		}
		if result.Classification != domain.ClassBlockingBug {
			t.Errorf("got Classification %q, want %q", result.Classification, domain.ClassBlockingBug)
		}
		if result.Confidence != 92 {
			t.Errorf("got Confidence %d, want 92", result.Confidence)
		}
		if !result.BlocksOperation {
			t.Error("expected BlocksOperation to be true")
		}
	})

	t.Run("Verify returns error on failure", func(t *testing.T) {
		verifier := &mockVerifier{
			verifyFunc: func(ctx context.Context, candidate domain.CandidateFinding) (domain.VerificationResult, error) {
				return domain.VerificationResult{}, errors.New("verification failed")
			},
		}

		_, err := verifier.Verify(context.Background(), domain.CandidateFinding{})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("VerifyBatch returns results for all candidates", func(t *testing.T) {
		verifier := &mockVerifier{
			verifyBatchFunc: func(ctx context.Context, candidates []domain.CandidateFinding) ([]domain.VerificationResult, error) {
				results := make([]domain.VerificationResult, len(candidates))
				for i := range candidates {
					results[i] = domain.VerificationResult{
						Verified:   i == 0, // First one verified, rest not
						Confidence: 50 + i*10,
					}
				}
				return results, nil
			},
		}

		candidates := []domain.CandidateFinding{
			{Finding: domain.Finding{Description: "Issue 1"}},
			{Finding: domain.Finding{Description: "Issue 2"}},
			{Finding: domain.Finding{Description: "Issue 3"}},
		}

		results, err := verifier.VerifyBatch(context.Background(), candidates)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("got %d results, want 3", len(results))
		}
		if !results[0].Verified {
			t.Error("expected first result to be verified")
		}
		if results[1].Verified {
			t.Error("expected second result to not be verified")
		}
	})

	t.Run("VerifyBatch respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		verifier := &mockVerifier{
			verifyBatchFunc: func(ctx context.Context, candidates []domain.CandidateFinding) ([]domain.VerificationResult, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
					return nil, nil
				}
			},
		}

		_, err := verifier.VerifyBatch(ctx, []domain.CandidateFinding{{}})
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})
}
