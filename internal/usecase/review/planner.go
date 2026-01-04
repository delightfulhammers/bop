package review

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/delightfulhammers/bop/internal/domain"
)

// scanner wraps bufio.Scanner for easier mocking and testing.
type scanner struct {
	*bufio.Scanner
}

// newScanner creates a new scanner from an io.Reader.
func newScanner(r io.Reader) *scanner {
	return &scanner{bufio.NewScanner(r)}
}

// PlanningProvider is the LLM interface for planning operations.
// This is typically a lightweight, fast model like gpt-4o-mini or claude-3-5-haiku.
type PlanningProvider interface {
	Review(ctx context.Context, req ProviderRequest) (domain.Review, error)
}

// PlanningAgent analyzes gathered context and asks clarifying questions
// before the review begins. This improves review quality by ensuring
// the right context is gathered and focus areas are identified.
//
// The planning agent only runs in interactive mode (--interactive flag)
// and requires a TTY (terminal). It automatically skips in CI/CD environments.
type PlanningAgent struct {
	provider PlanningProvider
	config   PlanningConfig
	input    io.Reader // For reading user input
	output   io.Writer // For presenting questions
}

// PlanningConfig controls planning behavior.
type PlanningConfig struct {
	MaxQuestions int           // Maximum number of questions to ask (default: 5)
	Timeout      time.Duration // Maximum time for planning phase (default: 30s)
}

// PlanningResult contains the enhanced context after planning.
type PlanningResult struct {
	EnhancedContext   ProjectContext // Original context plus user answers
	Questions         []Question     // Questions that were asked
	Answers           map[int]string // User's answers by question ID
	AdditionalContext []string       // Extra files to load (future)
}

// Question represents a single planning question.
type Question struct {
	ID      int      `json:"id"`                // Question identifier
	Type    string   `json:"type"`              // "multiple_choice", "yes_no", or "text"
	Text    string   `json:"text"`              // The question text
	Options []string `json:"options,omitempty"` // Options for multiple_choice questions
	Answer  string   `json:"-"`                 // User's answer (filled after presentation)
}

// PlanningResponse is the LLM's structured output containing questions
// and reasoning for why those questions were generated.
type PlanningResponse struct {
	Questions []Question `json:"questions"` // List of questions to ask
	Reasoning string     `json:"reasoning"` // Why these questions matter
}

// NewPlanningAgent creates a new planning agent with the given configuration.
//
// Parameters:
//   - provider: LLM provider for generating planning questions
//   - config: Planning configuration (max questions, timeout)
//   - input: Input reader for user responses (typically os.Stdin)
//   - output: Output writer for presenting questions (typically os.Stdout)
func NewPlanningAgent(provider PlanningProvider, config PlanningConfig, input io.Reader, output io.Writer) *PlanningAgent {
	return &PlanningAgent{
		provider: provider,
		config:   config,
		input:    input,
		output:   output,
	}
}

// parseQuestions extracts questions from the LLM's response.
// The response may be plain JSON or JSON wrapped in markdown code blocks.
//
// Expected JSON format:
//
//	{
//	  "questions": [
//	    {
//	      "id": 1,
//	      "type": "multiple_choice",
//	      "text": "Question text?",
//	      "options": ["Option 1", "Option 2"]
//	    }
//	  ],
//	  "reasoning": "Why these questions matter"
//	}
//
// Returns error if:
//   - No JSON found in response
//   - JSON is malformed
//   - Required fields are missing
func (p *PlanningAgent) parseQuestions(response string) (PlanningResponse, error) {
	// Try to extract JSON from response (may be wrapped in markdown code blocks)
	start := strings.Index(response, "{")
	if start == -1 {
		return PlanningResponse{}, fmt.Errorf("no JSON found in response")
	}

	// Find the matching closing brace for the first opening brace
	// This handles cases where there are multiple JSON blocks
	depth := 0
	end := -1
	for i := start; i < len(response); i++ {
		if response[i] == '{' {
			depth++
		} else if response[i] == '}' {
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
	}

	if end == -1 || start >= end {
		return PlanningResponse{}, fmt.Errorf("no JSON found in response")
	}

	jsonStr := response[start : end+1]

	var result PlanningResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return PlanningResponse{}, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return result, nil
}

// buildPlanningPrompt constructs the prompt for the planning LLM.
// It includes:
//   - Available context summary (architecture, README, docs)
//   - Change information (types, files, diff preview)
//   - JSON schema for questions
//   - Guidance on what types of questions to ask
//
// The diff is truncated to approximately 1000 characters to keep
// the prompt size manageable for fast, cheap models.
func (p *PlanningAgent) buildPlanningPrompt(ctx ProjectContext, diff domain.Diff) string {
	var b strings.Builder

	// Role and purpose
	b.WriteString("You are a planning assistant for a code review system. ")
	b.WriteString("Your job is to analyze the available context and the changes being reviewed, ")
	b.WriteString("then generate clarifying questions that will help improve the review quality.\n\n")

	// Available Context Section
	b.WriteString("## Available Context\n\n")

	hasContext := false
	if ctx.Architecture != "" {
		b.WriteString("- Architecture documentation: Available\n")
		hasContext = true
	}
	if ctx.README != "" {
		b.WriteString("- README: Available\n")
		hasContext = true
	}
	if len(ctx.DesignDocs) > 0 {
		b.WriteString(fmt.Sprintf("- Design documents: %d\n", len(ctx.DesignDocs)))
		hasContext = true
	}
	if len(ctx.RelevantDocs) > 0 {
		b.WriteString(fmt.Sprintf("- Relevant documentation: %d\n", len(ctx.RelevantDocs)))
		hasContext = true
	}
	if len(ctx.CustomContextFiles) > 0 {
		b.WriteString(fmt.Sprintf("- Custom context: %d files\n", len(ctx.CustomContextFiles)))
		hasContext = true
	}
	if !hasContext {
		b.WriteString("- No project context available\n")
	}

	// Change Information
	b.WriteString("\n## Changes\n\n")

	// Change types
	if len(ctx.ChangeTypes) > 0 {
		b.WriteString(fmt.Sprintf("Change types detected: %s\n", strings.Join(ctx.ChangeTypes, ", ")))
	}

	// File count and paths
	fileCount := len(ctx.ChangedPaths)
	if fileCount == 0 {
		fileCount = len(diff.Files)
	}
	b.WriteString(fmt.Sprintf("Files changed: %d\n", fileCount))

	if len(ctx.ChangedPaths) > 0 {
		b.WriteString("\nChanged files:\n")
		for _, path := range ctx.ChangedPaths {
			b.WriteString(fmt.Sprintf("- %s\n", path))
		}
	}

	// Diff preview (truncated)
	b.WriteString("\n### Diff preview\n\n")
	diffContent := formatDiffForPreview(diff)
	if len(diffContent) > 1000 {
		b.WriteString(diffContent[:1000])
		b.WriteString("\n...\n[Diff truncated for planning]\n")
	} else {
		b.WriteString(diffContent)
	}

	// Task description
	b.WriteString("\n## Task\n\n")
	b.WriteString("Based on the available context and changes:\n\n")
	b.WriteString("1. Identify gaps in understanding or context\n")
	b.WriteString("2. Generate 1-5 clarifying questions that would help improve the review\n")
	b.WriteString("3. Focus on:\n")
	b.WriteString("   - Missing context that would be valuable for review\n")
	b.WriteString("   - Understanding the scope and intent of changes\n")
	b.WriteString("   - Specific concerns or focus areas (security, performance, etc.)\n")
	b.WriteString("   - Additional files or context that should be loaded\n\n")
	b.WriteString("If you have no questions and the available context is sufficient, return an empty questions array.\n\n")

	// JSON Schema - Use code review format for provider compatibility
	b.WriteString("## Output Format\n\n")
	b.WriteString("Return your response in this EXACT JSON format:\n\n")
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"summary\": \"{\\\"questions\\\": [...], \\\"reasoning\\\": \\\"...\\\"}\",\n")
	b.WriteString("  \"findings\": []\n")
	b.WriteString("}\n")
	b.WriteString("```\n\n")
	b.WriteString("IMPORTANT: The planning questions JSON must be embedded as a string in the 'summary' field.\n")
	b.WriteString("This format ensures compatibility with the provider's expected code review response structure.\n")
	b.WriteString("The parseQuestions() function will extract and parse the embedded JSON from the summary field.\n\n")
	b.WriteString("The questions object (embedded in summary) should have this structure:\n\n")
	b.WriteString("```\n")
	b.WriteString("{\n")
	b.WriteString("  \"questions\": [\n")
	b.WriteString("    {\n")
	b.WriteString("      \"id\": 1,\n")
	b.WriteString("      \"type\": \"multiple_choice\",\n")
	b.WriteString("      \"text\": \"What is the primary focus for this review?\",\n")
	b.WriteString("      \"options\": [\"Security\", \"Performance\", \"General quality\", \"All areas\"]\n")
	b.WriteString("    },\n")
	b.WriteString("    {\n")
	b.WriteString("      \"id\": 2,\n")
	b.WriteString("      \"type\": \"yes_no\",\n")
	b.WriteString("      \"text\": \"Should I review test files in detail?\"\n")
	b.WriteString("    },\n")
	b.WriteString("    {\n")
	b.WriteString("      \"id\": 3,\n")
	b.WriteString("      \"type\": \"text\",\n")
	b.WriteString("      \"text\": \"Any specific concerns or areas to focus on?\"\n")
	b.WriteString("    }\n")
	b.WriteString("  ],\n")
	b.WriteString("  \"reasoning\": \"Brief explanation of why these questions matter\"\n")
	b.WriteString("}\n")
	b.WriteString("```\n\n")
	b.WriteString("Question types:\n")
	b.WriteString("- `multiple_choice`: Provides 2-5 options for the user to select from\n")
	b.WriteString("- `yes_no`: Simple yes/no question\n")
	b.WriteString("- `text`: Free-form text response from the user\n")

	return b.String()
}

// formatDiffForPreview creates a compact diff representation for the planning prompt.
// This is lighter weight than the full diff used for the actual review.
func formatDiffForPreview(diff domain.Diff) string {
	var b strings.Builder

	for i, file := range diff.Files {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("--- %s\n", file.Path))
		if file.Status != "" {
			b.WriteString(fmt.Sprintf("Status: %s\n", file.Status))
		}
		if file.Patch != "" {
			b.WriteString(file.Patch)
		}
	}

	return b.String()
}

// presentQuestions displays questions to the user and collects their answers.
// This is an interactive process that happens in the terminal.
//
// For each question:
//   - yes_no: Prompts with [y/n], normalizes to "yes" or "no"
//   - multiple_choice: Shows numbered options, user enters number
//   - text: Free-form text input (can be empty)
//
// Invalid input causes the question to be re-prompted.
//
// Returns a map of question ID to answer string.
func (p *PlanningAgent) presentQuestions(questions []Question) (map[int]string, error) {
	answers := make(map[int]string)

	if len(questions) == 0 {
		return answers, nil
	}

	scanner := newScanner(p.input)

	for _, q := range questions {
		var answer string
		var err error

		switch q.Type {
		case "yes_no":
			answer, err = p.promptYesNo(scanner, q)
		case "multiple_choice":
			answer, err = p.promptMultipleChoice(scanner, q)
		case "text":
			answer, err = p.promptText(scanner, q)
		default:
			return nil, fmt.Errorf("unknown question type: %s", q.Type)
		}

		if err != nil {
			return nil, fmt.Errorf("error presenting question %d: %w", q.ID, err)
		}

		answers[q.ID] = answer
	}

	return answers, nil
}

// promptYesNo prompts for a yes/no answer.
// Accepts: y, yes, n, no (case insensitive)
// Returns: "yes" or "no"
func (p *PlanningAgent) promptYesNo(scanner *scanner, q Question) (string, error) {
	for {
		_, _ = fmt.Fprintf(p.output, "\n%s [y/n]: ", q.Text)

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("failed to read input: %w", err)
			}
			return "", fmt.Errorf("unexpected end of input")
		}

		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))

		switch answer {
		case "y", "yes":
			return "yes", nil
		case "n", "no":
			return "no", nil
		default:
			_, _ = fmt.Fprintf(p.output, "Invalid input. Please enter 'y' or 'n'.\n")
		}
	}
}

// promptMultipleChoice prompts for a numbered choice.
// User enters a number (1-based), returns the selected option text.
func (p *PlanningAgent) promptMultipleChoice(scanner *scanner, q Question) (string, error) {
	// Display question and options
	_, _ = fmt.Fprintf(p.output, "\n%s\n", q.Text)
	for i, option := range q.Options {
		_, _ = fmt.Fprintf(p.output, "%d. %s\n", i+1, option)
	}

	for {
		_, _ = fmt.Fprintf(p.output, "Enter number (1-%d): ", len(q.Options))

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("failed to read input: %w", err)
			}
			return "", fmt.Errorf("unexpected end of input")
		}

		answer := strings.TrimSpace(scanner.Text())

		// Parse number
		var choice int
		_, err := fmt.Sscanf(answer, "%d", &choice)
		if err != nil || choice < 1 || choice > len(q.Options) {
			_, _ = fmt.Fprintf(p.output, "Invalid choice. Please enter a number between 1 and %d.\n", len(q.Options))
			continue
		}

		return q.Options[choice-1], nil
	}
}

// promptText prompts for free-form text input.
// Empty input is allowed.
func (p *PlanningAgent) promptText(scanner *scanner, q Question) (string, error) {
	_, _ = fmt.Fprintf(p.output, "\n%s\n> ", q.Text)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		return "", fmt.Errorf("unexpected end of input")
	}

	answer := strings.TrimSpace(scanner.Text())
	return answer, nil
}

// incorporateAnswers takes the user's answers and integrates them into
// the ProjectContext as PlanningAnswers. This enhanced context will be
// used for the actual code review.
//
// The Q&A pairs are formatted as a readable text block that preserves
// the order of questions as they were asked.
//
// Returns a new ProjectContext with all original fields plus PlanningAnswers.
func (p *PlanningAgent) incorporateAnswers(ctx ProjectContext, questions []Question, answers map[int]string) ProjectContext {
	// If no answers, return context as-is
	if len(answers) == 0 {
		return ctx
	}

	// Format Q&A pairs in order
	var b strings.Builder
	b.WriteString("Planning Phase - User Input:\n\n")

	for _, q := range questions {
		answer, ok := answers[q.ID]
		if !ok {
			continue // Skip questions with no answer
		}

		b.WriteString(fmt.Sprintf("Q: %s\n", q.Text))
		b.WriteString(fmt.Sprintf("A: %s\n\n", answer))
	}

	// Create enhanced context
	enhanced := ctx
	enhanced.PlanningAnswers = b.String()

	return enhanced
}

// Plan executes the full planning workflow:
//  1. Builds a planning prompt from context and diff
//  2. Calls the LLM to generate clarifying questions
//  3. Parses the LLM's response
//  4. Presents questions to the user interactively (if any)
//  5. Incorporates answers into the context
//  6. Returns enhanced context and planning metadata
//
// If the LLM generates no questions, the original context is returned unchanged.
// Errors from the LLM or parsing are propagated to the caller.
//
// Context cancellation is respected - if the context is cancelled, Plan returns immediately.
func (p *PlanningAgent) Plan(ctx context.Context, projectCtx ProjectContext, diff domain.Diff) (PlanningResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return PlanningResult{}, ctx.Err()
	default:
	}

	// Step 1: Build planning prompt
	prompt := p.buildPlanningPrompt(projectCtx, diff)

	// Step 2: Call LLM provider
	req := ProviderRequest{
		Prompt: prompt,
	}

	llmResponse, err := p.provider.Review(ctx, req)
	if err != nil {
		return PlanningResult{}, fmt.Errorf("planning LLM call failed: %w", err)
	}

	// Step 3: Parse questions from response
	planningResponse, err := p.parseQuestions(llmResponse.Summary)
	if err != nil {
		return PlanningResult{}, fmt.Errorf("failed to parse planning response: %w", err)
	}

	// Step 4: If no questions, return original context
	if len(planningResponse.Questions) == 0 {
		return PlanningResult{
			EnhancedContext: projectCtx,
			Questions:       []Question{},
			Answers:         map[int]string{},
		}, nil
	}

	// Step 5: Present questions to user and collect answers
	answers, err := p.presentQuestions(planningResponse.Questions)
	if err != nil {
		return PlanningResult{}, fmt.Errorf("failed to present questions: %w", err)
	}

	// Step 6: Incorporate answers into context
	enhancedCtx := p.incorporateAnswers(projectCtx, planningResponse.Questions, answers)

	// Return result
	return PlanningResult{
		EnhancedContext: enhancedCtx,
		Questions:       planningResponse.Questions,
		Answers:         answers,
	}, nil
}
