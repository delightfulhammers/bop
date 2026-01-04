# Planning Agent Design - Phase 4

**Status**: Design Complete, Implementation Pending
**Version**: 1.0
**Date**: 2025-10-26
**Related**: ENHANCED_PROMPTING_DESIGN.md, ARCHITECTURE.md

## 1. Overview

The Planning Agent provides an interactive, human-in-the-loop workflow where an LLM analyzes the gathered context and asks clarifying questions before the review begins. This improves review quality by ensuring the right context is gathered and focus areas are identified.

### 1.1 Key Principles

- **Optional**: Only runs with `--interactive` flag, never in CI/CD
- **Fast**: Single LLM call with small, cheap model (gpt-4o-mini)
- **Helpful**: Asks 3-5 targeted questions to improve context
- **Non-Blocking**: Can be skipped with `--no-planning` flag
- **Fail-Safe**: If planning fails, continues with gathered context

### 1.2 User Experience

```bash
$ bop review branch main --interactive

⏳ Gathering context...
✓ Found: ARCHITECTURE.md, README.md, 3 design docs
✓ Detected change types: auth, database
✓ Found relevant docs: SECURITY.md, STORE_INTEGRATION_DESIGN.md

📋 Planning review...

The planner has identified some questions:

1. Is this authentication change for a specific user type?
   • Admin users only
   • All authenticated users
   • Public (unauthenticated) users
   > All authenticated users

2. Should I review the database migration files in db/migrations/?
   • Yes
   • No
   > Yes

3. Are there any specific security concerns to focus on?
   (Enter text or press Enter to skip)
   > SQL injection in query builder

✓ Planning complete. Starting review with enhanced context...

⏳ Reviewing with 3 providers...
```

## 2. Architecture

### 2.1 Component Diagram

```
┌─────────────────────────────────────────────┐
│ CLI (root.go)                               │
│  --interactive flag detected                │
└──────────────────┬──────────────────────────┘
                   ↓
┌─────────────────────────────────────────────┐
│ Orchestrator (orchestrator.go)             │
│  1. Gather context (ContextGatherer)        │
│  2. Check TTY + --interactive flag          │
│  3. Create PlanningAgent                    │
└──────────────────┬──────────────────────────┘
                   ↓
┌─────────────────────────────────────────────┐
│ PlanningAgent (planner.go)                 │
│  • Build planning prompt                    │
│  • Call planning LLM                        │
│  • Parse questions from response            │
│  • Present questions to user (CLI)          │
│  • Incorporate answers into context         │
└──────────────────┬──────────────────────────┘
                   ↓
┌─────────────────────────────────────────────┐
│ Orchestrator (orchestrator.go)             │
│  4. Enhanced context from planning          │
│  5. Build provider prompts                  │
│  6. Execute parallel reviews                │
└─────────────────────────────────────────────┘
```

### 2.2 Data Flow

```
┌──────────────────┐
│ ProjectContext   │  ← Gathered by ContextGatherer
└────────┬─────────┘
         ↓
┌──────────────────────────────────────────────┐
│ Planning Prompt                              │
│  • Context summary                           │
│  • Change types detected                     │
│  • Files changed                             │
│  • Docs found                                │
└────────┬─────────────────────────────────────┘
         ↓
┌──────────────────┐
│ Planning LLM     │  ← gpt-4o-mini, claude-3-5-haiku
└────────┬─────────┘
         ↓
┌──────────────────────────────────────────────┐
│ Planning Response (JSON)                     │
│  {                                           │
│    "questions": [                            │
│      {                                       │
│        "id": 1,                              │
│        "type": "multiple_choice",            │
│        "text": "Is this for admin users?",  │
│        "options": ["Admin", "All", "Public"]│
│      }                                       │
│    ],                                        │
│    "reasoning": "..."                        │
│  }                                           │
└────────┬─────────────────────────────────────┘
         ↓
┌──────────────────┐
│ CLI Interaction  │  ← Present questions to user
└────────┬─────────┘
         ↓
┌──────────────────────────────────────────────┐
│ User Answers                                 │
│  { 1: "All authenticated users",             │
│    2: "Yes",                                 │
│    3: "SQL injection in query builder" }     │
└────────┬─────────────────────────────────────┘
         ↓
┌──────────────────────────────────────────────┐
│ Enhanced ProjectContext                      │
│  • Original context                          │
│  • + CustomInstructions from answers         │
│  • + Additional context files if requested   │
└──────────────────────────────────────────────┘
```

## 3. Implementation Details

### 3.1 PlanningAgent Structure

```go
// internal/usecase/review/planner.go

package review

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/delightfulhammers/bop/internal/domain"
)

// PlanningProvider is the LLM interface for planning operations
type PlanningProvider interface {
	Review(ctx context.Context, req ProviderRequest) (domain.Review, error)
}

// PlanningAgent analyzes context and asks clarifying questions
type PlanningAgent struct {
	provider     PlanningProvider
	config       PlanningConfig
	input        io.Reader  // For reading user input
	output       io.Writer  // For presenting questions
}

// PlanningConfig controls planning behavior
type PlanningConfig struct {
	MaxQuestions int           // Default: 5
	Timeout      time.Duration // Default: 30s
}

// PlanningResult contains the enhanced context after planning
type PlanningResult struct {
	EnhancedContext    ProjectContext
	Questions          []Question
	Answers            map[int]string
	AdditionalContext  []string // Extra files to load
}

// Question represents a planning question
type Question struct {
	ID       int      `json:"id"`
	Type     string   `json:"type"` // "multiple_choice", "yes_no", "text"
	Text     string   `json:"text"`
	Options  []string `json:"options,omitempty"`
	Answer   string   `json:"-"` // Filled in after user response
}

// PlanningResponse is the LLM's structured output
type PlanningResponse struct {
	Questions []Question `json:"questions"`
	Reasoning string     `json:"reasoning"`
}

// Plan analyzes context and presents questions to user
func (p *PlanningAgent) Plan(ctx context.Context, projectCtx ProjectContext, diff domain.Diff) (PlanningResult, error) {
	// 1. Build planning prompt
	prompt := p.buildPlanningPrompt(projectCtx, diff)

	// 2. Call planning LLM
	req := ProviderRequest{
		Prompt:  prompt,
		MaxSize: 2000, // Planning responses should be concise
	}

	review, err := p.provider.Review(ctx, req)
	if err != nil {
		return PlanningResult{EnhancedContext: projectCtx}, fmt.Errorf("planning LLM call failed: %w", err)
	}

	// 3. Parse questions from response
	response, err := p.parseQuestions(review.Summary)
	if err != nil {
		return PlanningResult{EnhancedContext: projectCtx}, fmt.Errorf("failed to parse planning response: %w", err)
	}

	// Limit questions
	if len(response.Questions) > p.config.MaxQuestions {
		response.Questions = response.Questions[:p.config.MaxQuestions]
	}

	// 4. Present questions to user
	answers, err := p.presentQuestions(response.Questions)
	if err != nil {
		return PlanningResult{EnhancedContext: projectCtx}, fmt.Errorf("failed to collect answers: %w", err)
	}

	// 5. Incorporate answers into context
	result := p.incorporateAnswers(projectCtx, response.Questions, answers)

	return result, nil
}

// buildPlanningPrompt creates the prompt for the planning LLM
func (p *PlanningAgent) buildPlanningPrompt(ctx ProjectContext, diff domain.Diff) string {
	var prompt strings.Builder

	prompt.WriteString("You are a code review planning assistant. Analyze the context and changes, then generate 3-5 clarifying questions.\n\n")

	// Context summary
	prompt.WriteString("## Available Context\n\n")
	if ctx.Architecture != "" {
		prompt.WriteString(fmt.Sprintf("- Architecture documentation: %d chars\n", len(ctx.Architecture)))
	}
	if ctx.README != "" {
		prompt.WriteString(fmt.Sprintf("- README: %d chars\n", len(ctx.README)))
	}
	if len(ctx.DesignDocs) > 0 {
		prompt.WriteString(fmt.Sprintf("- Design documents: %d files\n", len(ctx.DesignDocs)))
	}
	if len(ctx.RelevantDocs) > 0 {
		prompt.WriteString(fmt.Sprintf("- Relevant documentation: %d files\n", len(ctx.RelevantDocs)))
	}
	if len(ctx.CustomContextFiles) > 0 {
		prompt.WriteString(fmt.Sprintf("- Custom context: %d files\n", len(ctx.CustomContextFiles)))
	}

	// Change summary
	prompt.WriteString("\n## Changes\n\n")
	if len(ctx.ChangeTypes) > 0 {
		prompt.WriteString(fmt.Sprintf("Change types detected: %s\n", strings.Join(ctx.ChangeTypes, ", ")))
	}
	prompt.WriteString(fmt.Sprintf("Files changed: %d\n", len(ctx.ChangedPaths)))
	if len(ctx.ChangedPaths) > 0 {
		prompt.WriteString("\nChanged files:\n")
		for _, path := range ctx.ChangedPaths {
			prompt.WriteString(fmt.Sprintf("- %s\n", path))
		}
	}

	// Diff summary (first 1000 chars)
	diffStr := fmt.Sprintf("%v", diff)
	if len(diffStr) > 1000 {
		diffStr = diffStr[:1000] + "..."
	}
	prompt.WriteString(fmt.Sprintf("\nDiff preview:\n%s\n", diffStr))

	// Instructions for response
	prompt.WriteString("\n## Task\n\n")
	prompt.WriteString("Generate 3-5 clarifying questions that would help improve the code review quality.\n")
	prompt.WriteString("Focus on:\n")
	prompt.WriteString("1. Missing context (e.g., \"Should I review migration files?\")\n")
	prompt.WriteString("2. Intended audience/scope (e.g., \"Is this for admin users only?\")\n")
	prompt.WriteString("3. Specific concerns (e.g., \"Any security issues to focus on?\")\n")
	prompt.WriteString("4. Related changes (e.g., \"Are there related API changes?\")\n\n")

	prompt.WriteString("Respond in JSON format:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"questions\": [\n")
	prompt.WriteString("    {\n")
	prompt.WriteString("      \"id\": 1,\n")
	prompt.WriteString("      \"type\": \"multiple_choice\",  // or \"yes_no\", \"text\"\n")
	prompt.WriteString("      \"text\": \"Question text?\",\n")
	prompt.WriteString("      \"options\": [\"Option 1\", \"Option 2\"]  // for multiple_choice only\n")
	prompt.WriteString("    }\n")
	prompt.WriteString("  ],\n")
	prompt.WriteString("  \"reasoning\": \"Brief explanation of why these questions matter\"\n")
	prompt.WriteString("}\n")

	return prompt.String()
}

// parseQuestions extracts questions from LLM response
func (p *PlanningAgent) parseQuestions(response string) (PlanningResponse, error) {
	// Try to extract JSON from response (may be wrapped in markdown)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end == -1 || start >= end {
		return PlanningResponse{}, fmt.Errorf("no JSON found in response")
	}

	jsonStr := response[start : end+1]

	var result PlanningResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return PlanningResponse{}, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return result, nil
}

// presentQuestions shows questions to user and collects answers
func (p *PlanningAgent) presentQuestions(questions []Question) (map[int]string, error) {
	answers := make(map[int]string)

	fmt.Fprintln(p.output, "\n📋 Planning review...\n")
	fmt.Fprintln(p.output, "The planner has identified some questions:\n")

	for i, q := range questions {
		fmt.Fprintf(p.output, "%d. %s\n", i+1, q.Text)

		switch q.Type {
		case "multiple_choice":
			for j, opt := range q.Options {
				fmt.Fprintf(p.output, "   %s %s\n", formatOption(j), opt)
			}
			fmt.Fprint(p.output, "   > ")

			var answer string
			fmt.Fscanln(p.input, &answer)
			answers[q.ID] = answer

		case "yes_no":
			fmt.Fprint(p.output, "   (y/n) > ")

			var answer string
			fmt.Fscanln(p.input, &answer)
			answers[q.ID] = answer

		case "text":
			fmt.Fprint(p.output, "   (Enter text or press Enter to skip)\n   > ")

			var answer string
			fmt.Fscanln(p.input, &answer)
			if answer != "" {
				answers[q.ID] = answer
			}
		}

		fmt.Fprintln(p.output)
	}

	return answers, nil
}

// incorporateAnswers updates context with user responses
func (p *PlanningAgent) incorporateAnswers(ctx ProjectContext, questions []Question, answers map[int]string) PlanningResult {
	result := PlanningResult{
		EnhancedContext: ctx,
		Questions:       questions,
		Answers:         answers,
	}

	// Build custom instructions from answers
	var instructions strings.Builder
	if ctx.CustomInstructions != "" {
		instructions.WriteString(ctx.CustomInstructions)
		instructions.WriteString("\n\n")
	}

	instructions.WriteString("Based on planning questions:\n")
	for _, q := range questions {
		if answer, ok := answers[q.ID]; ok && answer != "" {
			instructions.WriteString(fmt.Sprintf("- %s: %s\n", q.Text, answer))
		}
	}

	result.EnhancedContext.CustomInstructions = instructions.String()

	return result
}

// formatOption formats option labels (• for display)
func formatOption(index int) string {
	return "•"
}
```

### 3.2 TTY Detection

```go
// internal/usecase/review/tty.go

package review

import (
	"os"

	"golang.org/x/term"
)

// IsTTY checks if the given file descriptor is a terminal
func IsTTY(fd uintptr) bool {
	return term.IsTerminal(int(fd))
}

// IsInteractive checks if stdin is a TTY (user can provide input)
func IsInteractive() bool {
	return IsTTY(os.Stdin.Fd())
}

// IsOutputTerminal checks if stdout is a TTY (user can see formatted output)
func IsOutputTerminal() bool {
	return IsTTY(os.Stdout.Fd())
}
```

### 3.3 Configuration

```go
// internal/config/config.go

// Add to Config struct:

type Config struct {
	// ... existing fields ...

	Planning PlanningConfig `mapstructure:"planning"`
}

type PlanningConfig struct {
	Enabled      bool   `mapstructure:"enabled"`       // Enable planning agent
	Provider     string `mapstructure:"provider"`      // Provider for planning (e.g., "openai")
	Model        string `mapstructure:"model"`         // Model for planning (e.g., "gpt-4o-mini")
	MaxQuestions int    `mapstructure:"maxQuestions"`  // Max questions to ask (default: 5)
	Timeout      string `mapstructure:"timeout"`       // Timeout for planning phase (default: "30s")
}
```

### 3.4 Orchestrator Integration

```go
// internal/usecase/review/orchestrator.go

// Add to BranchRequest:
type BranchRequest struct {
	// ... existing fields ...

	Interactive bool // Enable interactive planning mode
	NoPlanning  bool // Skip planning even in interactive mode
	PlanOnly    bool // Only show planning, don't execute review
}

// In ReviewBranch method, after context gathering:

// Planning phase (if interactive mode and TTY)
if req.Interactive && !req.NoPlanning && IsInteractive() {
	if o.deps.Planner != nil {
		planningCtx, cancel := context.WithTimeout(ctx, planningTimeout)
		defer cancel()

		planResult, err := o.deps.Planner.Plan(planningCtx, projectContext, diff)
		if err != nil {
			// Log warning but continue with original context
			if o.deps.Logger != nil {
				o.deps.Logger.LogWarning(ctx, "planning failed, continuing with gathered context", map[string]interface{}{
					"error": err.Error(),
				})
			}
		} else {
			// Use enhanced context from planning
			projectContext = planResult.EnhancedContext
		}
	}
}

// Plan-only mode: show context and exit
if req.PlanOnly {
	// Print context summary to stdout
	printContextSummary(projectContext)
	return Result{}, nil
}
```

## 4. Testing Strategy

### 4.1 Unit Tests

```go
// internal/usecase/review/planner_test.go

func TestBuildPlanningPrompt(t *testing.T) {
	// Test prompt generation with various context combinations
}

func TestParseQuestions_ValidJSON(t *testing.T) {
	// Test parsing valid JSON response
}

func TestParseQuestions_JSONInMarkdown(t *testing.T) {
	// Test extracting JSON from markdown code blocks
}

func TestParseQuestions_InvalidJSON(t *testing.T) {
	// Test error handling for invalid JSON
}

func TestPresentQuestions_MultipleChoice(t *testing.T) {
	// Test multiple choice question presentation (mock IO)
}

func TestPresentQuestions_YesNo(t *testing.T) {
	// Test yes/no question presentation
}

func TestPresentQuestions_Text(t *testing.T) {
	// Test free text question presentation
}

func TestIncorporateAnswers(t *testing.T) {
	// Test that answers are correctly added to context
}

func TestPlan_FullFlow(t *testing.T) {
	// Integration test with mock provider and IO
}

func TestPlan_LLMFailure(t *testing.T) {
	// Test graceful failure when LLM call fails
}

func TestPlan_Timeout(t *testing.T) {
	// Test timeout handling
}
```

### 4.2 Integration Tests

```go
// internal/usecase/review/planner_integration_test.go

func TestPlanningAgent_RealProvider(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Test with real OpenAI provider
	// Verify questions are relevant
	// Check cost is within budget (~$0.001)
}
```

### 4.3 TTY Tests

```go
// internal/usecase/review/tty_test.go

func TestIsTTY(t *testing.T) {
	// Test TTY detection
	// Note: May not be TTY in CI
}

func TestIsInteractive(t *testing.T) {
	// Test stdin TTY detection
}
```

## 5. Configuration Examples

### 5.1 Enable Planning

```yaml
# bop.yaml

planning:
  enabled: true
  provider: "openai"
  model: "gpt-4o-mini"
  maxQuestions: 5
  timeout: "30s"

# Optional: Use different provider for planning
planning:
  enabled: true
  provider: "anthropic"
  model: "claude-3-5-haiku-20241022"
  maxQuestions: 3
  timeout: "20s"
```

### 5.2 CLI Usage

```bash
# Interactive mode with planning
cr review branch main --interactive

# Interactive but skip planning
cr review branch main --interactive --no-planning

# Plan only (dry-run, shows what would be gathered)
cr review branch main --plan-only

# In CI/CD (not interactive, planning automatically skipped)
cr review branch main  # Planning won't run even if configured
```

## 6. Error Handling

### 6.1 Graceful Degradation

| Error Scenario | Behavior |
|---|---|
| Planning LLM call fails | Log warning, continue with original context |
| Planning timeout | Log warning, continue with original context |
| Invalid JSON response | Log warning, continue with original context |
| User cancels (Ctrl+C) | Exit gracefully with context cancellation |
| Not a TTY | Skip planning, continue with gathered context |
| No planning provider configured | Skip planning, log info message |

### 6.2 Logging

```go
// Log planning operations
logger.LogInfo(ctx, "planning phase started", map[string]interface{}{
	"provider": config.Provider,
	"model": config.Model,
})

logger.LogWarning(ctx, "planning failed", map[string]interface{}{
	"error": err.Error(),
	"fallback": "using original context",
})

logger.LogInfo(ctx, "planning complete", map[string]interface{}{
	"questions": len(questions),
	"answered": len(answers),
	"duration_ms": elapsed.Milliseconds(),
})
```

## 7. Cost Analysis

### 7.1 Estimated Costs (gpt-4o-mini)

**Input**:
- Context summary: ~500 tokens
- Change types and file paths: ~200 tokens
- Diff preview: ~500 tokens
- Instructions: ~300 tokens
- **Total input**: ~1,500 tokens

**Output**:
- JSON with 5 questions: ~300 tokens

**Cost per review**:
- Input: 1,500 tokens × $0.150/1M = $0.000225
- Output: 300 tokens × $0.600/1M = $0.00018
- **Total**: ~$0.0004 per review

### 7.2 Cost Comparison

| Operation | Cost | Frequency |
|---|---|---|
| Planning agent | $0.0004 | Per review (interactive only) |
| Summary synthesis | $0.0003 | Per review (always) |
| Code review (gpt-4o-mini) | $0.002-0.01 | Per provider per review |
| Code review (claude-sonnet) | $0.01-0.05 | Per provider per review |

**Planning cost is negligible** (~1-4% of typical review cost)

## 8. Implementation Checklist

See [PLANNING_AGENT_CHECKLIST.md](PLANNING_AGENT_CHECKLIST.md) for detailed implementation steps.

## 9. Success Metrics

### 9.1 Quantitative

- Planning completes in < 5 seconds
- Cost per planning call < $0.001
- < 1% of planning calls fail
- Planning increases custom instructions usage by 50%+

### 9.2 Qualitative

- Questions are relevant to changes
- Answers improve review quality
- Users find planning helpful (feedback collection)
- Planning doesn't disrupt workflow

## 10. Future Enhancements

### 10.1 Phase 4+

- **Smart question prioritization**: Learn which questions matter most
- **Context recommendations**: "I found SECURITY.md but it wasn't loaded, should I include it?"
- **Change impact analysis**: "This change affects 3 other modules, should I review them?"
- **Historical learning**: Use past review feedback to improve questions
- **Multi-turn conversation**: Allow follow-up questions based on answers

### 10.2 Research Questions

- Should planning be available in CI/CD with environment variables?
- Can we auto-answer common questions based on patterns?
- Should we persist planning results for similar changes?
- Can we use embeddings to find similar past reviews?

## 11. References

- ENHANCED_PROMPTING_DESIGN.md: Overall enhanced prompting architecture
- ARCHITECTURE.md: System architecture
- CONFIGURATION.md: Configuration options
- golang.org/x/term: TTY detection
