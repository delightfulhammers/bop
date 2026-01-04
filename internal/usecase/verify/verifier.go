package verify

import (
	"context"

	"github.com/delightfulhammers/bop/internal/domain"
)

// Verifier verifies candidate findings against the codebase.
// Implementations may use LLM agents, static analysis, or other techniques
// to determine if a candidate finding is valid.
type Verifier interface {
	// Verify checks a single candidate finding and returns the verification result.
	// The implementation should:
	// 1. Analyze the candidate's claim against the actual codebase
	// 2. Classify the finding (blocking_bug, security, performance, style)
	// 3. Assign a confidence score (0-100)
	// 4. Determine if it blocks the operation
	Verify(ctx context.Context, candidate domain.CandidateFinding) (domain.VerificationResult, error)

	// VerifyBatch verifies multiple candidates, potentially in parallel.
	// Returns results in the same order as the input candidates.
	// Implementations may use concurrency limits and cost ceilings.
	VerifyBatch(ctx context.Context, candidates []domain.CandidateFinding) ([]domain.VerificationResult, error)
}
