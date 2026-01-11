// Package dedup provides semantic deduplication of code review findings.
// It implements a two-stage deduplication process:
//   - Stage 1: Fingerprint matching (exact, handled elsewhere)
//   - Stage 2: Semantic comparison using LLM (this package)
package dedup

import (
	"context"

	"github.com/delightfulhammers/bop/internal/domain"
)

// CandidatePair represents two findings that may be semantic duplicates.
// The existing finding is from a previous review; the new finding is from the current review.
type CandidatePair struct {
	// Existing is the finding from a previous review (already posted as a comment).
	Existing ExistingFinding

	// New is the finding from the current review that may be a duplicate.
	New domain.Finding
}

// ExistingFinding represents a finding that was previously posted.
type ExistingFinding struct {
	// Fingerprint is the unique identifier from the comment.
	Fingerprint domain.FindingFingerprint

	// File is the file path.
	File string

	// LineStart is the starting line number.
	LineStart int

	// LineEnd is the ending line number.
	LineEnd int

	// Description is the finding description from the comment.
	Description string

	// Severity is the severity level.
	Severity string

	// Category is the finding category.
	Category string
}

// DuplicateMatch indicates that a new finding is a semantic duplicate of an existing one.
type DuplicateMatch struct {
	// NewFinding is the finding from the current review.
	NewFinding domain.Finding

	// ExistingFingerprint is the fingerprint of the existing finding it duplicates.
	ExistingFingerprint domain.FindingFingerprint

	// Reason explains why the LLM determined these are duplicates.
	Reason string
}

// ComparisonResult contains the results of semantic comparison.
type ComparisonResult struct {
	// Duplicates are findings determined to be semantic duplicates.
	Duplicates []DuplicateMatch

	// Unique are findings that are not duplicates of any existing finding.
	Unique []domain.Finding
}

// SemanticComparer compares findings for semantic similarity using an LLM.
type SemanticComparer interface {
	// Compare takes candidate pairs and determines which new findings are
	// semantic duplicates of existing findings.
	//
	// The implementation should batch candidates into a single LLM call for efficiency.
	// If the LLM call fails, all new findings should be returned as Unique
	// (fail-open to avoid silently dropping valid feedback).
	Compare(ctx context.Context, candidates []CandidatePair) (*ComparisonResult, error)
}

// Usage contains token consumption metrics from semantic comparison.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// UsageProvider is an optional interface for comparers that track token usage.
// Callers can check if a SemanticComparer implements this interface to retrieve
// accumulated usage for cost accounting.
type UsageProvider interface {
	// TotalUsage returns the accumulated token usage.
	TotalUsage() Usage
	// ResetUsage clears the accumulated usage.
	ResetUsage()
}

// Config holds configuration for semantic deduplication.
type Config struct {
	// Provider is the LLM provider name (e.g., "anthropic").
	Provider string

	// Model is the model to use (e.g., "claude-haiku-4-5-latest").
	Model string

	// MaxTokens is the maximum output tokens for the LLM response.
	// Default: 64000 (works for all current Claude/GPT/Gemini models)
	MaxTokens int

	// LineThreshold is the max line distance for candidates.
	LineThreshold int

	// MaxCandidates limits candidates per review (cost guard).
	MaxCandidates int
}

// DefaultConfig returns the default semantic deduplication configuration.
func DefaultConfig() Config {
	return Config{
		Provider:      "anthropic",
		Model:         "claude-haiku-4-5",
		MaxTokens:     64000,
		LineThreshold: 50, // Increased from 10 to catch more duplicates (Issue #165)
		MaxCandidates: 50,
	}
}
