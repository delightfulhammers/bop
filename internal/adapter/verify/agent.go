package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/delightfulhammers/bop/internal/config"
	"github.com/delightfulhammers/bop/internal/domain"
	usecaseverify "github.com/delightfulhammers/bop/internal/usecase/verify"
)

// LLMClient defines the interface for LLM interactions.
// This abstracts over different providers (OpenAI, Anthropic, etc.).
type LLMClient interface {
	// Call sends a prompt to the LLM and returns the response text.
	// The system prompt should be included in the implementation.
	Call(ctx context.Context, systemPrompt, userPrompt string) (response string, tokensIn, tokensOut int, cost float64, err error)
}

// AgentConfig configures the verification agent behavior.
type AgentConfig struct {
	// MaxIterations limits the number of tool calls per verification.
	MaxIterations int

	// Concurrency limits parallel verifications.
	Concurrency int

	// Confidence thresholds per severity level.
	Confidence config.ConfidenceThresholds

	// Depth controls verification thoroughness: "quick", "medium", "deep".
	Depth string
}

// DefaultAgentConfig returns sensible defaults.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		MaxIterations: 10,
		Concurrency:   5,
		Confidence: config.ConfidenceThresholds{
			Critical: 50,
			High:     60,
			Medium:   70,
			Low:      80,
		},
		Depth: "medium",
	}
}

// AgentVerifier implements the verify.Verifier interface using an LLM agent.
type AgentVerifier struct {
	llm         LLMClient
	repo        usecaseverify.Repository
	tools       []Tool
	toolMap     map[string]Tool
	config      AgentConfig
	costTracker usecaseverify.CostTracker
}

// NewAgentVerifier creates a new agent-based verifier.
func NewAgentVerifier(
	llm LLMClient,
	repo usecaseverify.Repository,
	costTracker usecaseverify.CostTracker,
	config AgentConfig,
) *AgentVerifier {
	tools := NewToolRegistry(repo)
	toolMap := make(map[string]Tool, len(tools))
	for _, t := range tools {
		toolMap[t.Name()] = t
	}

	return &AgentVerifier{
		llm:         llm,
		repo:        repo,
		tools:       tools,
		toolMap:     toolMap,
		config:      config,
		costTracker: costTracker,
	}
}

// Verify checks a single candidate finding and returns the verification result.
func (v *AgentVerifier) Verify(ctx context.Context, candidate domain.CandidateFinding) (domain.VerificationResult, error) {
	// Check cost ceiling before starting
	if v.costTracker != nil && v.costTracker.ExceedsCeiling() {
		return domain.VerificationResult{
			Verified:   false,
			Confidence: 0,
			Evidence:   "Cost ceiling exceeded, unable to verify",
		}, nil
	}

	// Build prompts
	systemPrompt := VerificationPrompt(v.tools)
	userPrompt := CandidatePrompt(candidate)

	var actions []domain.VerificationAction
	var lastResponse string

	// Agent loop - let LLM use tools until it provides a verdict
	for i := 0; i < v.config.MaxIterations; i++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return domain.VerificationResult{}, ctx.Err()
		}

		// Check cost ceiling
		if v.costTracker != nil && v.costTracker.ExceedsCeiling() {
			break
		}

		// Call LLM
		response, tokensIn, tokensOut, cost, err := v.llm.Call(ctx, systemPrompt, userPrompt)
		if err != nil {
			return domain.VerificationResult{}, fmt.Errorf("llm call: %w", err)
		}

		// Track cost
		if v.costTracker != nil {
			v.costTracker.AddCost(cost)
		}

		lastResponse = response

		// Try to parse as final verdict
		if result, ok := v.parseVerdict(response); ok {
			result.Actions = actions
			return result, nil
		}

		// Try to parse tool call
		toolName, toolInput, ok := v.parseToolCall(response)
		if !ok {
			// No tool call and no verdict - treat as final response
			break
		}

		// Execute tool
		tool, exists := v.toolMap[toolName]
		if !exists {
			userPrompt = fmt.Sprintf("Unknown tool: %s. Available tools: %v", toolName, v.toolNames())
			continue
		}

		output, err := tool.Execute(ctx, toolInput)
		if err != nil {
			output = fmt.Sprintf("Error: %v", err)
		}

		// Record action
		actions = append(actions, domain.VerificationAction{
			Tool:   toolName,
			Input:  toolInput,
			Output: truncateOutput(output),
		})

		// Build next prompt with tool result
		userPrompt = ToolResultPrompt(toolName, toolInput, output)

		// For logging/debugging
		_ = tokensIn
		_ = tokensOut
	}

	// Max iterations reached or no verdict parsed - try to extract verdict from last response
	if result, ok := v.parseVerdict(lastResponse); ok {
		result.Actions = actions
		return result, nil
	}

	// Fallback: Unable to determine
	return domain.VerificationResult{
		Verified:       false,
		Classification: "",
		Confidence:     0,
		Evidence:       "Unable to determine verification status after investigation",
		Actions:        actions,
	}, nil
}

// VerifyBatch verifies multiple candidates, potentially in parallel.
//
// Note on cost ceiling enforcement: The cost ceiling check is best-effort under
// concurrency. Multiple goroutines may pass the check before any of them add their
// costs. This is acceptable as the ceiling is a soft limit to prevent runaway costs,
// not a hard budget guarantee. For strict budget enforcement, use Concurrency=1.
func (v *AgentVerifier) VerifyBatch(ctx context.Context, candidates []domain.CandidateFinding) ([]domain.VerificationResult, error) {
	if len(candidates) == 0 {
		return []domain.VerificationResult{}, nil
	}

	results := make([]domain.VerificationResult, len(candidates))
	errors := make([]error, len(candidates))

	// Use semaphore for concurrency control
	sem := make(chan struct{}, v.config.Concurrency)
	var wg sync.WaitGroup

	for i, candidate := range candidates {
		// Check context before starting new verification
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		wg.Add(1)
		go func(idx int, cand domain.CandidateFinding) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check context after acquiring semaphore
			if ctx.Err() != nil {
				errors[idx] = ctx.Err()
				return
			}

			// Check cost ceiling after acquiring semaphore to ensure
			// accurate cost tracking when running sequentially
			if v.costTracker != nil && v.costTracker.ExceedsCeiling() {
				results[idx] = domain.VerificationResult{
					Verified:   false,
					Confidence: 0,
					Evidence:   "Cost ceiling exceeded, unable to verify",
				}
				return
			}

			result, err := v.Verify(ctx, cand)
			if err != nil {
				errors[idx] = err
				return
			}
			results[idx] = result
		}(i, candidate)
	}

	wg.Wait()

	// Check for errors
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("verifying candidate %d: %w", i, err)
		}
	}

	return results, nil
}

// verdictResponse represents the JSON structure of a verification verdict.
type verdictResponse struct {
	Verified        bool   `json:"verified"`
	Classification  string `json:"classification"`
	Confidence      int    `json:"confidence"`
	Evidence        string `json:"evidence"`
	BlocksOperation bool   `json:"blocks_operation"`
}

// parseVerdict attempts to extract a JSON verdict from the response.
func (v *AgentVerifier) parseVerdict(response string) (domain.VerificationResult, bool) {
	// Look for JSON in the response (might be in code blocks)
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return domain.VerificationResult{}, false
	}

	var verdict verdictResponse
	if err := json.Unmarshal([]byte(jsonStr), &verdict); err != nil {
		return domain.VerificationResult{}, false
	}

	// Validate required fields are present
	if verdict.Evidence == "" && verdict.Confidence == 0 {
		return domain.VerificationResult{}, false
	}

	result := domain.VerificationResult{
		Verified:       verdict.Verified,
		Classification: domain.Classification(verdict.Classification),
		Confidence:     verdict.Confidence,
		Evidence:       verdict.Evidence,
	}

	// Determine if it blocks operation
	result.BlocksOperation = ShouldBlockOperation(result)

	return result, true
}

// toolCallPattern matches tool invocations like "TOOL: read_file\nINPUT: main.go"
var toolCallPattern = regexp.MustCompile(`(?s)TOOL:\s*(\w+)\s*\nINPUT:\s*(.+?)(?:\n|$)`)

// parseToolCall attempts to extract a tool call from the response.
func (v *AgentVerifier) parseToolCall(response string) (toolName, input string, ok bool) {
	matches := toolCallPattern.FindStringSubmatch(response)
	if len(matches) >= 3 {
		return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2]), true
	}
	return "", "", false
}

// toolNames returns the list of available tool names.
func (v *AgentVerifier) toolNames() []string {
	names := make([]string, len(v.tools))
	for i, t := range v.tools {
		names[i] = t.Name()
	}
	return names
}

// codeBlockPattern matches markdown code blocks (with optional json language tag)
var codeBlockPattern = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.+?)\\n?```") //nolint:gocritic // Using double quotes for backticks in pattern

// jsonObjectPattern matches JSON objects
var jsonObjectPattern = regexp.MustCompile(`(?s)\{.+\}`)

// extractJSON finds and extracts JSON from text, handling code blocks.
func extractJSON(text string) string {
	// Try to find JSON in code blocks first
	if matches := codeBlockPattern.FindStringSubmatch(text); len(matches) >= 2 {
		candidate := strings.TrimSpace(matches[1])
		if isValidJSON(candidate) {
			return candidate
		}
	}

	// Try to find bare JSON object
	if matches := jsonObjectPattern.FindString(text); matches != "" {
		if isValidJSON(matches) {
			return matches
		}
	}

	return ""
}

// isValidJSON checks if a string is valid JSON.
func isValidJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}

// Compile-time interface check
var _ usecaseverify.Verifier = (*AgentVerifier)(nil)
