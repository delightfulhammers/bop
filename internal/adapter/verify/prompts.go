package verify

import (
	"fmt"
	"strings"

	"github.com/delightfulhammers/bop/internal/config"
	"github.com/delightfulhammers/bop/internal/domain"
)

// VerificationPrompt generates the system prompt for verification.
func VerificationPrompt(tools []Tool) string {
	var sb strings.Builder

	sb.WriteString(`You are a code verification agent. Your task is to verify whether a reported code issue actually exists in the codebase.

## Your Goal
Determine if the candidate finding is:
1. A real issue that exists in the code (verified = true)
2. A false positive or incorrect claim (verified = false)

## Classification Criteria
If the finding is verified, classify it as:

- **blocking_bug**: Code that will crash, fail, or produce incorrect results at runtime
  - Null pointer dereferences
  - Array out of bounds
  - Type mismatches
  - Logic errors that cause incorrect behavior
  - Unhandled error conditions that will crash

- **security**: Security vulnerabilities
  - SQL injection, XSS, command injection
  - Authentication/authorization bypasses
  - Cryptographic weaknesses
  - Sensitive data exposure
  - Path traversal

- **performance**: Resource exhaustion or performance issues
  - Unbounded loops or recursion
  - Memory leaks
  - N+1 query patterns
  - Blocking operations in hot paths

- **style**: Style preferences or opinions (these will be discarded)
  - Naming conventions
  - Code formatting
  - Comment style
  - Subjective "best practices"

## Confidence Scoring
Your confidence score (0-100) should reflect:

- **90-100**: Issue is definitively confirmed with concrete evidence
  - You read the exact code and see the bug
  - Build/test failures prove the issue
  - The problematic pattern is unambiguous

- **70-89**: Issue is very likely based on strong evidence
  - Code patterns strongly suggest the issue
  - Similar issues confirmed elsewhere
  - Missing safety checks observed

- **50-69**: Issue is plausible but not certain
  - Code is suspicious but edge cases unclear
  - Depends on runtime behavior
  - Context suggests potential issue

- **Below 50**: Insufficient evidence or likely false positive
  - Cannot find the reported code
  - Code looks correct
  - Report appears to misunderstand the code

## Available Tools
`)

	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name(), tool.Description()))
	}

	sb.WriteString(`
## Response Format
After investigating, respond with a JSON object:

` + "```json" + `
{
  "verified": true,
  "classification": "blocking_bug",
  "confidence": 85,
  "evidence": "The null check at line 42 of handler.go is missing. The function dereferences req.User without checking if it's nil, which will panic when called with an unauthenticated request.",
  "blocks_operation": true
}
` + "```" + `

## Tool Usage
To use a tool, respond with:

` + "```tool" + `
TOOL: tool_name
INPUT: your input here
` + "```" + `

After receiving the tool result, continue your investigation or provide your final verdict.

## Important Notes
- Always read the relevant file(s) before making a determination
- Do NOT assume the report is correct - verify it yourself
- Style issues should always be marked as NOT blocking
- If you cannot find sufficient evidence, return low confidence
- Be specific in your evidence - cite exact lines and code

## Common False Positive Patterns - DO NOT flag these as issues:

**Short-circuit null guards**: When a null/nil/None check is combined with
a dereference using && (AND), this is SAFE. The logical AND operator
short-circuits in virtually all languages - if the left side is false,
the right side is never evaluated. Examples:
- ` + "`x != nil && x.field`" + ` (Go)
- ` + "`obj !== null && obj.prop`" + ` (JavaScript/TypeScript)
- ` + "`x is not None and x.attr`" + ` (Python)
- ` + "`!is.null(x) && x$field`" + ` (R)
- ` + "`!isnothing(x) && x.field`" + ` (Julia)

**Short-circuit OR guards**: Similarly, ` + "`x == nil || ...`" + ` patterns
short-circuit when the left side is true, so subsequent code is safe.

**Optional chaining operators**: ` + "`?.`" + ` in JavaScript/TypeScript,
` + "`&.`" + ` in Ruby, etc. are designed specifically for safe null access.

**Guard clauses with early return**: When a function checks for null and
returns/throws early, subsequent code that dereferences the value is safe.

If the reported issue matches one of these patterns, mark it as:
- verified: false
- confidence: 90+ (high confidence it's a false positive)
- evidence: Explain which safe pattern applies
`)

	return sb.String()
}

// CandidatePrompt generates the prompt for a specific candidate finding.
func CandidatePrompt(candidate domain.CandidateFinding) string {
	var sb strings.Builder

	sb.WriteString("## Candidate Finding to Verify\n\n")
	sb.WriteString(fmt.Sprintf("**File**: %s\n", candidate.Finding.File))

	if candidate.Finding.LineStart > 0 {
		if candidate.Finding.LineEnd > 0 && candidate.Finding.LineEnd != candidate.Finding.LineStart {
			sb.WriteString(fmt.Sprintf("**Lines**: %d-%d\n", candidate.Finding.LineStart, candidate.Finding.LineEnd))
		} else {
			sb.WriteString(fmt.Sprintf("**Line**: %d\n", candidate.Finding.LineStart))
		}
	}

	sb.WriteString(fmt.Sprintf("**Severity**: %s\n", candidate.Finding.Severity))
	sb.WriteString(fmt.Sprintf("**Description**: %s\n", candidate.Finding.Description))

	if candidate.Finding.Category != "" {
		sb.WriteString(fmt.Sprintf("**Category**: %s\n", candidate.Finding.Category))
	}

	if candidate.Finding.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("**Suggestion**: %s\n", candidate.Finding.Suggestion))
	}

	sb.WriteString(fmt.Sprintf("\n**Agreement Score**: %.0f%% of reviewers reported this issue\n", candidate.AgreementScore*100))
	sb.WriteString(fmt.Sprintf("**Sources**: %s\n", strings.Join(candidate.Sources, ", ")))

	sb.WriteString("\nPlease verify this finding by reading the relevant code and determining if the issue actually exists.\n")

	return sb.String()
}

// ToolResultPrompt wraps a tool result for the agent.
func ToolResultPrompt(toolName, input, output string) string {
	return fmt.Sprintf(`## Tool Result

**Tool**: %s
**Input**: %s

**Output**:
%s

Continue your investigation or provide your final verdict.
`, toolName, input, output)
}

// ConfidenceThreshold returns the minimum confidence for a given severity.
func ConfidenceThreshold(severity string, thresholds config.ConfidenceThresholds) int {
	switch strings.ToLower(severity) {
	case "critical":
		if thresholds.Critical > 0 {
			return thresholds.Critical
		}
	case "high":
		if thresholds.High > 0 {
			return thresholds.High
		}
	case "medium":
		if thresholds.Medium > 0 {
			return thresholds.Medium
		}
	case "low":
		if thresholds.Low > 0 {
			return thresholds.Low
		}
	}

	if thresholds.Default > 0 {
		return thresholds.Default
	}

	// Fallback defaults if nothing is configured
	switch strings.ToLower(severity) {
	case "critical":
		return 50 // Lower threshold for critical issues
	case "high":
		return 60
	case "medium":
		return 70
	case "low":
		return 80 // Higher threshold for low severity
	default:
		return 70
	}
}

// ShouldBlockOperation determines if a verified finding should block.
func ShouldBlockOperation(result domain.VerificationResult) bool {
	if !result.Verified {
		return false
	}

	// Style findings never block
	if result.Classification == domain.ClassStyle {
		return false
	}

	// Blocking bugs and security issues always block if verified
	if result.Classification == domain.ClassBlockingBug || result.Classification == domain.ClassSecurity {
		return true
	}

	// Performance issues block only at high confidence
	if result.Classification == domain.ClassPerformance {
		return result.Confidence >= 80
	}

	return false
}
