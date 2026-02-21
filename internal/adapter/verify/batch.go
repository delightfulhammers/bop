package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/delightfulhammers/bop/internal/config"
	"github.com/delightfulhammers/bop/internal/domain"
	usecaseverify "github.com/delightfulhammers/bop/internal/usecase/verify"
)

// BatchConfig configures the batch verifier behavior.
type BatchConfig struct {
	// Confidence thresholds per severity level.
	Confidence config.ConfidenceThresholds
}

// DefaultBatchConfig returns sensible defaults.
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		Confidence: config.ConfidenceThresholds{
			Critical: 50,
			High:     60,
			Medium:   70,
			Low:      80,
		},
	}
}

// BatchVerifier verifies findings in a single LLM call with full file context.
// This is much more efficient than the agent-based approach for large batches.
type BatchVerifier struct {
	llm         LLMClient
	repo        usecaseverify.Repository
	config      BatchConfig
	costTracker usecaseverify.CostTracker
}

// NewBatchVerifier creates a new batch-based verifier.
func NewBatchVerifier(
	llm LLMClient,
	repo usecaseverify.Repository,
	costTracker usecaseverify.CostTracker,
	config BatchConfig,
) *BatchVerifier {
	return &BatchVerifier{
		llm:         llm,
		repo:        repo,
		config:      config,
		costTracker: costTracker,
	}
}

// Verify checks a single candidate - delegates to VerifyBatch.
func (v *BatchVerifier) Verify(ctx context.Context, candidate domain.CandidateFinding) (domain.VerificationResult, error) {
	results, err := v.VerifyBatch(ctx, []domain.CandidateFinding{candidate})
	if err != nil {
		return domain.VerificationResult{}, err
	}
	if len(results) == 0 {
		return domain.VerificationResult{}, fmt.Errorf("no results returned")
	}
	return results[0], nil
}

// VerifyBatch verifies all candidates in a single LLM call.
func (v *BatchVerifier) VerifyBatch(ctx context.Context, candidates []domain.CandidateFinding) ([]domain.VerificationResult, error) {
	if len(candidates) == 0 {
		return []domain.VerificationResult{}, nil
	}

	// Check cost ceiling
	if v.costTracker != nil && v.costTracker.ExceedsCeiling() {
		results := make([]domain.VerificationResult, len(candidates))
		for i := range results {
			results[i] = domain.VerificationResult{
				Verified:   false,
				Confidence: 0,
				Evidence:   "Cost ceiling exceeded, unable to verify",
			}
		}
		return results, nil
	}

	// Gather unique files and their contents
	fileContents, err := v.gatherFileContents(candidates)
	if err != nil {
		return nil, fmt.Errorf("gathering file contents: %w", err)
	}

	// Build prompts
	systemPrompt := batchVerificationSystemPrompt()
	userPrompt := batchVerificationUserPrompt(candidates, fileContents)

	// Single LLM call
	response, _, _, cost, err := v.llm.Call(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("llm call: %w", err)
	}

	// Track cost
	if v.costTracker != nil {
		v.costTracker.AddCost(cost)
	}

	// Parse response
	results, err := v.parseResponse(response, len(candidates))
	if err != nil {
		// If parsing fails, return unverified results with low confidence
		results = make([]domain.VerificationResult, len(candidates))
		for i := range results {
			results[i] = domain.VerificationResult{
				Verified:   false,
				Confidence: 0,
				Evidence:   fmt.Sprintf("Failed to parse verification response: %v", err),
			}
		}
	}

	return results, nil
}

// gatherFileContents reads the content of all files referenced by candidates.
func (v *BatchVerifier) gatherFileContents(candidates []domain.CandidateFinding) (map[string]string, error) {
	// Collect unique file paths
	files := make(map[string]struct{})
	for _, c := range candidates {
		if c.Finding.File != "" {
			files[c.Finding.File] = struct{}{}
		}
	}

	// Read each file
	contents := make(map[string]string, len(files))
	for file := range files {
		content, err := v.repo.ReadFile(file)
		if err != nil {
			// Include error message instead of failing entirely
			contents[file] = fmt.Sprintf("[Error reading file: %v]", err)
			continue
		}
		contents[file] = string(content)
	}

	return contents, nil
}

// batchVerificationSystemPrompt returns the system prompt for batch verification.
func batchVerificationSystemPrompt() string {
	return `You are a code verification expert. Your task is to verify whether reported code issues actually exist.

## Your Goal
For each finding, determine if it is:
1. A real issue that exists in the code (verified = true)
2. A false positive or incorrect claim (verified = false)

## Classification (for verified findings)
- **blocking_bug**: Code that will crash, fail, or produce incorrect results
- **security**: Security vulnerabilities (injection, auth bypass, etc.)
- **performance**: Resource exhaustion or performance issues
- **style**: Style preferences (these should be marked verified=false)

## Confidence Scoring (0-100)
- **90-100**: Issue definitively confirmed with concrete evidence
- **70-89**: Issue very likely based on strong evidence
- **50-69**: Issue plausible but not certain
- **Below 50**: Insufficient evidence or likely false positive

## Critical Rules
1. READ THE FULL FILE CONTENT provided - do not assume the finding is correct
2. Check imports, nil guards, error handling - verify claims specifically
3. If a finding claims something is missing (import, check, etc.), verify by searching the file
4. Style issues should always be verified=false
5. Be specific in evidence - cite exact lines

## Response Format
Return a JSON array with one object per finding, in the same order as the input:

` + "```json" + `
[
  {
    "index": 0,
    "verified": false,
    "classification": "",
    "confidence": 95,
    "evidence": "The finding claims 'fmt' is not imported, but line 4 shows 'import \"fmt\"'. This is a false positive."
  },
  {
    "index": 1,
    "verified": true,
    "classification": "blocking_bug",
    "confidence": 85,
    "evidence": "Line 42 dereferences req.User without nil check. The function can receive nil from unauthenticated requests per line 38."
  }
]
` + "```" + `

IMPORTANT: Return ONLY the JSON array, no other text.`
}

// batchVerificationUserPrompt builds the user prompt with findings and file contents.
func batchVerificationUserPrompt(candidates []domain.CandidateFinding, fileContents map[string]string) string {
	var sb strings.Builder

	// Include file contents first
	sb.WriteString("## Source Files\n\n")
	for file, content := range fileContents {
		fmt.Fprintf(&sb, "### File: %s\n", file)
		sb.WriteString("```\n")
		// Add line numbers for easy reference
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			fmt.Fprintf(&sb, "%4d | %s\n", i+1, line)
		}
		sb.WriteString("```\n\n")
	}

	// Include findings to verify
	sb.WriteString("## Findings to Verify\n\n")
	for i, c := range candidates {
		fmt.Fprintf(&sb, "### Finding %d\n", i)
		fmt.Fprintf(&sb, "- **File**: %s\n", c.Finding.File)
		if c.Finding.LineStart > 0 {
			if c.Finding.LineEnd > 0 && c.Finding.LineEnd != c.Finding.LineStart {
				fmt.Fprintf(&sb, "- **Lines**: %d-%d\n", c.Finding.LineStart, c.Finding.LineEnd)
			} else {
				fmt.Fprintf(&sb, "- **Line**: %d\n", c.Finding.LineStart)
			}
		}
		fmt.Fprintf(&sb, "- **Severity**: %s\n", c.Finding.Severity)
		fmt.Fprintf(&sb, "- **Category**: %s\n", c.Finding.Category)
		fmt.Fprintf(&sb, "- **Description**: %s\n", c.Finding.Description)
		if c.Finding.Suggestion != "" {
			fmt.Fprintf(&sb, "- **Suggestion**: %s\n", c.Finding.Suggestion)
		}
		fmt.Fprintf(&sb, "- **Agreement**: %.0f%% of reviewers\n", c.AgreementScore*100)
		sb.WriteString("\n")
	}

	sb.WriteString("Verify each finding against the source files provided above. Return a JSON array with your verdicts.\n")

	return sb.String()
}

// batchVerdict represents a single verdict in the batch response.
type batchVerdict struct {
	Index          int    `json:"index"`
	Verified       bool   `json:"verified"`
	Classification string `json:"classification"`
	Confidence     int    `json:"confidence"`
	Evidence       string `json:"evidence"`
}

// parseResponse parses the LLM response into verification results.
func (v *BatchVerifier) parseResponse(response string, expectedCount int) ([]domain.VerificationResult, error) {
	// Extract JSON array from response (might be in code blocks)
	jsonStr := extractJSONArray(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON array found in response")
	}

	var verdicts []batchVerdict
	if err := json.Unmarshal([]byte(jsonStr), &verdicts); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	// Build results array, handling missing/out-of-order indices
	results := make([]domain.VerificationResult, expectedCount)
	for i := range results {
		results[i] = domain.VerificationResult{
			Verified:   false,
			Confidence: 0,
			Evidence:   "No verdict returned for this finding",
		}
	}

	for _, verdict := range verdicts {
		if verdict.Index < 0 || verdict.Index >= expectedCount {
			continue
		}
		result := domain.VerificationResult{
			Verified:       verdict.Verified,
			Classification: domain.Classification(verdict.Classification),
			Confidence:     verdict.Confidence,
			Evidence:       verdict.Evidence,
		}
		result.BlocksOperation = ShouldBlockOperation(result)
		results[verdict.Index] = result
	}

	return results, nil
}

// jsonArrayPattern matches JSON arrays
var jsonArrayPattern = regexp.MustCompile(`(?s)\[.+\]`)

// extractJSONArray finds and extracts a JSON array from text, handling code blocks.
func extractJSONArray(text string) string {
	// Try to find JSON in code blocks first
	if matches := codeBlockPattern.FindStringSubmatch(text); len(matches) >= 2 {
		candidate := strings.TrimSpace(matches[1])
		if isValidJSON(candidate) && strings.HasPrefix(candidate, "[") {
			return candidate
		}
	}

	// Try to find bare JSON array
	if matches := jsonArrayPattern.FindString(text); matches != "" {
		if isValidJSON(matches) {
			return matches
		}
	}

	return ""
}

// Compile-time interface check
var _ usecaseverify.Verifier = (*BatchVerifier)(nil)
