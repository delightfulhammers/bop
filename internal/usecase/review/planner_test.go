package review

import (
	"context"
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPlanningProvider mocks the LLM provider for planning tests.
type mockPlanningProvider struct {
	response string
	err      error
}

func (m *mockPlanningProvider) Review(ctx context.Context, req ProviderRequest) (domain.Review, error) {
	if m.err != nil {
		return domain.Review{}, m.err
	}

	return domain.Review{
		Summary:  m.response,
		Findings: []domain.Finding{},
	}, nil
}

func TestParseQuestions_ValidJSON(t *testing.T) {
	response := `{
		"questions": [
			{
				"id": 1,
				"type": "multiple_choice",
				"text": "Is this for admin users only?",
				"options": ["Yes", "No", "All users"]
			},
			{
				"id": 2,
				"type": "yes_no",
				"text": "Should I review migration files?"
			},
			{
				"id": 3,
				"type": "text",
				"text": "Any specific security concerns?"
			}
		],
		"reasoning": "Need to understand scope and focus areas"
	}`

	agent := &PlanningAgent{}
	result, err := agent.parseQuestions(response)

	require.NoError(t, err, "parsing valid JSON should not error")
	assert.Len(t, result.Questions, 3, "should parse all 3 questions")

	// Check first question (multiple choice)
	q1 := result.Questions[0]
	assert.Equal(t, 1, q1.ID)
	assert.Equal(t, "multiple_choice", q1.Type)
	assert.Equal(t, "Is this for admin users only?", q1.Text)
	assert.Equal(t, []string{"Yes", "No", "All users"}, q1.Options)

	// Check second question (yes/no)
	q2 := result.Questions[1]
	assert.Equal(t, 2, q2.ID)
	assert.Equal(t, "yes_no", q2.Type)
	assert.Equal(t, "Should I review migration files?", q2.Text)

	// Check third question (text)
	q3 := result.Questions[2]
	assert.Equal(t, 3, q3.ID)
	assert.Equal(t, "text", q3.Type)
	assert.Equal(t, "Any specific security concerns?", q3.Text)

	// Check reasoning
	assert.Equal(t, "Need to understand scope and focus areas", result.Reasoning)
}

func TestParseQuestions_JSONInMarkdown(t *testing.T) {
	response := "Here are the planning questions:\n\n```json\n" +
		`{
	"questions": [
		{
			"id": 1,
			"type": "yes_no",
			"text": "Is this a breaking change?"
		}
	],
	"reasoning": "Need to understand impact"
}` +
		"\n```\n\nLet me know if you need clarification!"

	agent := &PlanningAgent{}
	result, err := agent.parseQuestions(response)

	require.NoError(t, err, "should extract JSON from markdown")
	assert.Len(t, result.Questions, 1, "should parse question")
	assert.Equal(t, "Is this a breaking change?", result.Questions[0].Text)
}

func TestParseQuestions_JSONWithWhitespace(t *testing.T) {
	response := `


	{
		"questions": [{"id": 1, "type": "text", "text": "What is the purpose?"}],
		"reasoning": "Context needed"
	}


	`

	agent := &PlanningAgent{}
	result, err := agent.parseQuestions(response)

	require.NoError(t, err, "should handle whitespace around JSON")
	assert.Len(t, result.Questions, 1)
}

func TestParseQuestions_InvalidJSON(t *testing.T) {
	response := `This is not valid JSON at all!`

	agent := &PlanningAgent{}
	_, err := agent.parseQuestions(response)

	assert.Error(t, err, "should error on invalid JSON")
	assert.Contains(t, err.Error(), "no JSON found", "error should mention missing JSON")
}

func TestParseQuestions_MalformedJSON(t *testing.T) {
	response := `{"questions": [{"id": 1, "type": "text"` // missing closing braces

	agent := &PlanningAgent{}
	_, err := agent.parseQuestions(response)

	assert.Error(t, err, "should error on malformed JSON")
}

func TestParseQuestions_EmptyQuestionsList(t *testing.T) {
	response := `{
		"questions": [],
		"reasoning": "No questions needed"
	}`

	agent := &PlanningAgent{}
	result, err := agent.parseQuestions(response)

	require.NoError(t, err, "empty questions list is valid")
	assert.Len(t, result.Questions, 0)
	assert.Equal(t, "No questions needed", result.Reasoning)
}

func TestParseQuestions_MissingOptionalFields(t *testing.T) {
	// Options are only required for multiple_choice
	response := `{
		"questions": [
			{
				"id": 1,
				"type": "text",
				"text": "What's the goal?"
			}
		],
		"reasoning": "Testing optional fields"
	}`

	agent := &PlanningAgent{}
	result, err := agent.parseQuestions(response)

	require.NoError(t, err, "should handle missing optional fields")
	assert.Len(t, result.Questions, 1)
	assert.Nil(t, result.Questions[0].Options, "options should be nil for text question")
}

func TestParseQuestions_MultipleJSONBlocks(t *testing.T) {
	// If response contains multiple JSON blocks, should use the first one
	response := `{"questions": [{"id": 1, "type": "text", "text": "First"}], "reasoning": "First block"}

	And here's another: {"questions": [], "reasoning": "Second block"}`

	agent := &PlanningAgent{}
	result, err := agent.parseQuestions(response)

	require.NoError(t, err, "should parse first JSON block")
	assert.Len(t, result.Questions, 1, "should use first JSON block")
	assert.Equal(t, "First", result.Questions[0].Text)
}

func TestQuestion_TypeValidation(t *testing.T) {
	tests := []struct {
		name         string
		questionType string
		expectValid  bool
	}{
		{"multiple_choice is valid", "multiple_choice", true},
		{"yes_no is valid", "yes_no", true},
		{"text is valid", "text", true},
		{"unknown type", "invalid_type", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := `{
				"questions": [{
					"id": 1,
					"type": "` + tt.questionType + `",
					"text": "Test question"
				}],
				"reasoning": "Test"
			}`

			agent := &PlanningAgent{}
			result, err := agent.parseQuestions(response)

			if tt.expectValid {
				require.NoError(t, err)
				assert.Equal(t, tt.questionType, result.Questions[0].Type)
			}
			// Note: We're not strictly validating question types yet,
			// just ensuring they parse. Validation can be added later if needed.
		})
	}
}

func TestParseQuestions_PreservesQuestionOrder(t *testing.T) {
	response := `{
		"questions": [
			{"id": 3, "type": "text", "text": "Third"},
			{"id": 1, "type": "text", "text": "First"},
			{"id": 2, "type": "text", "text": "Second"}
		],
		"reasoning": "Order test"
	}`

	agent := &PlanningAgent{}
	result, err := agent.parseQuestions(response)

	require.NoError(t, err)
	assert.Len(t, result.Questions, 3)

	// Should preserve order from JSON, not sort by ID
	assert.Equal(t, 3, result.Questions[0].ID)
	assert.Equal(t, 1, result.Questions[1].ID)
	assert.Equal(t, 2, result.Questions[2].ID)
}

// --- Planning Prompt Generation Tests ---

func TestBuildPlanningPrompt_MinimalContext(t *testing.T) {
	agent := &PlanningAgent{}

	ctx := ProjectContext{}
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go"},
		},
	}

	prompt := agent.buildPlanningPrompt(ctx, diff)

	// Should contain required sections
	assert.Contains(t, prompt, "planning assistant", "should describe agent role")
	assert.Contains(t, prompt, "Available Context", "should have context section")
	assert.Contains(t, prompt, "Changes", "should have changes section")
	assert.Contains(t, prompt, "Task", "should have task section")
	assert.Contains(t, prompt, "JSON format", "should specify JSON output")

	// Should mention file count
	assert.Contains(t, prompt, "Files changed: 1", "should show file count")
}

func TestBuildPlanningPrompt_FullContext(t *testing.T) {
	agent := &PlanningAgent{}

	ctx := ProjectContext{
		Architecture:       "# Architecture\n\nClean architecture...",
		README:             "# README\n\nProject overview...",
		DesignDocs:         []string{"=== DESIGN.md ===\nDesign doc"},
		RelevantDocs:       []string{"=== SECURITY.md ===\nSecurity doc"},
		CustomContextFiles: []string{"=== custom.md ===\nCustom context"},
		ChangeTypes:        []string{"auth", "database"},
		ChangedPaths:       []string{"auth.go", "db.go"},
	}

	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "auth.go"},
			{Path: "db.go"},
		},
	}

	prompt := agent.buildPlanningPrompt(ctx, diff)

	// Should mention all context elements
	assert.Contains(t, prompt, "Architecture documentation", "should mention architecture")
	assert.Contains(t, prompt, "README", "should mention README")
	assert.Contains(t, prompt, "Design documents: 1", "should count design docs")
	assert.Contains(t, prompt, "Relevant documentation: 1", "should count relevant docs")
	assert.Contains(t, prompt, "Custom context: 1", "should count custom files")

	// Should show change types
	assert.Contains(t, prompt, "auth, database", "should list change types")

	// Should show changed files
	assert.Contains(t, prompt, "auth.go", "should list changed files")
	assert.Contains(t, prompt, "db.go", "should list changed files")
}

func TestBuildPlanningPrompt_DiffTruncation(t *testing.T) {
	agent := &PlanningAgent{}

	// Create a large diff
	largeDiff := strings.Repeat("line of diff content\n", 100)
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:  "large.go",
				Patch: largeDiff,
			},
		},
	}

	prompt := agent.buildPlanningPrompt(ProjectContext{}, diff)

	// Diff should be truncated to ~1000 chars
	// Count occurrences of "Diff preview" section
	assert.Contains(t, prompt, "Diff preview", "should include diff preview")

	// If diff is very long, should be truncated with "..."
	if len(largeDiff) > 1000 {
		assert.Contains(t, prompt, "...", "large diff should be truncated")
	}
}

func TestBuildPlanningPrompt_JSONSchema(t *testing.T) {
	agent := &PlanningAgent{}

	prompt := agent.buildPlanningPrompt(ProjectContext{}, domain.Diff{})

	// Should include JSON schema
	assert.Contains(t, prompt, `"questions"`, "should show questions field")
	assert.Contains(t, prompt, `"id"`, "should show id field")
	assert.Contains(t, prompt, `"type"`, "should show type field")
	assert.Contains(t, prompt, `"text"`, "should show text field")
	assert.Contains(t, prompt, `"options"`, "should show options field")
	assert.Contains(t, prompt, `"reasoning"`, "should show reasoning field")

	// Should explain question types
	assert.Contains(t, prompt, "multiple_choice", "should mention multiple_choice type")
	assert.Contains(t, prompt, "yes_no", "should mention yes_no type")
	assert.Contains(t, prompt, "text", "should mention text type")
}

func TestBuildPlanningPrompt_GuidanceForQuestions(t *testing.T) {
	agent := &PlanningAgent{}

	prompt := agent.buildPlanningPrompt(ProjectContext{}, domain.Diff{})

	// Should provide guidance on what to ask
	assert.Contains(t, prompt, "Missing context", "should ask about missing context")
	assert.Contains(t, prompt, "scope", "should ask about scope")
	assert.Contains(t, prompt, "concerns", "should ask about concerns")
}

func TestBuildPlanningPrompt_EmptyChangedPaths(t *testing.T) {
	agent := &PlanningAgent{}

	ctx := ProjectContext{
		ChangedPaths: []string{}, // Empty
	}

	prompt := agent.buildPlanningPrompt(ctx, domain.Diff{})

	// Should handle empty gracefully
	assert.Contains(t, prompt, "Files changed: 0", "should show 0 files")
	assert.NotPanics(t, func() {
		agent.buildPlanningPrompt(ctx, domain.Diff{})
	}, "should not panic on empty paths")
}

// --- Question Presentation Tests ---

func TestPresentQuestions_YesNoQuestion(t *testing.T) {
	// Simulate user input: "yes"
	input := strings.NewReader("yes\n")
	output := &strings.Builder{}

	agent := &PlanningAgent{
		input:  input,
		output: output,
	}

	questions := []Question{
		{
			ID:   1,
			Type: "yes_no",
			Text: "Should I review test files?",
		},
	}

	answers, err := agent.presentQuestions(questions)

	require.NoError(t, err, "should handle yes/no question")
	assert.Equal(t, "yes", answers[1], "should capture yes answer")

	// Check output format
	outputStr := output.String()
	assert.Contains(t, outputStr, "Should I review test files?", "should display question text")
	assert.Contains(t, outputStr, "[y/n]", "should show yes/no options")
}

func TestPresentQuestions_YesNoQuestion_NoAnswer(t *testing.T) {
	input := strings.NewReader("n\n")
	output := &strings.Builder{}

	agent := &PlanningAgent{
		input:  input,
		output: output,
	}

	questions := []Question{
		{
			ID:   1,
			Type: "yes_no",
			Text: "Is this a breaking change?",
		},
	}

	answers, err := agent.presentQuestions(questions)

	require.NoError(t, err)
	assert.Equal(t, "no", answers[1], "should normalize 'n' to 'no'")
}

func TestPresentQuestions_MultipleChoice(t *testing.T) {
	// Simulate user selecting option 2
	input := strings.NewReader("2\n")
	output := &strings.Builder{}

	agent := &PlanningAgent{
		input:  input,
		output: output,
	}

	questions := []Question{
		{
			ID:      1,
			Type:    "multiple_choice",
			Text:    "What is the primary focus?",
			Options: []string{"Security", "Performance", "Quality"},
		},
	}

	answers, err := agent.presentQuestions(questions)

	require.NoError(t, err)
	assert.Equal(t, "Performance", answers[1], "should capture selected option")

	// Check output format
	outputStr := output.String()
	assert.Contains(t, outputStr, "What is the primary focus?", "should display question")
	assert.Contains(t, outputStr, "1. Security", "should show option 1")
	assert.Contains(t, outputStr, "2. Performance", "should show option 2")
	assert.Contains(t, outputStr, "3. Quality", "should show option 3")
}

func TestPresentQuestions_MultipleChoice_InvalidThenValid(t *testing.T) {
	// First invalid (out of range), then valid
	input := strings.NewReader("5\n2\n")
	output := &strings.Builder{}

	agent := &PlanningAgent{
		input:  input,
		output: output,
	}

	questions := []Question{
		{
			ID:      1,
			Type:    "multiple_choice",
			Text:    "Select option:",
			Options: []string{"A", "B", "C"},
		},
	}

	answers, err := agent.presentQuestions(questions)

	require.NoError(t, err)
	assert.Equal(t, "B", answers[1], "should accept valid input after invalid")

	// Should show error message
	outputStr := output.String()
	assert.Contains(t, outputStr, "Invalid", "should show error for invalid input")
}

func TestPresentQuestions_TextQuestion(t *testing.T) {
	input := strings.NewReader("Check authentication logic carefully\n")
	output := &strings.Builder{}

	agent := &PlanningAgent{
		input:  input,
		output: output,
	}

	questions := []Question{
		{
			ID:   1,
			Type: "text",
			Text: "Any specific concerns?",
		},
	}

	answers, err := agent.presentQuestions(questions)

	require.NoError(t, err)
	assert.Equal(t, "Check authentication logic carefully", answers[1], "should capture text answer")
}

func TestPresentQuestions_TextQuestion_Empty(t *testing.T) {
	// User presses enter without typing anything
	input := strings.NewReader("\n")
	output := &strings.Builder{}

	agent := &PlanningAgent{
		input:  input,
		output: output,
	}

	questions := []Question{
		{
			ID:   1,
			Type: "text",
			Text: "Additional notes?",
		},
	}

	answers, err := agent.presentQuestions(questions)

	require.NoError(t, err)
	assert.Equal(t, "", answers[1], "should accept empty text answer")
}

func TestPresentQuestions_MultipleQuestions(t *testing.T) {
	// Three questions: yes/no, multiple choice, text
	input := strings.NewReader("yes\n2\nFocus on edge cases\n")
	output := &strings.Builder{}

	agent := &PlanningAgent{
		input:  input,
		output: output,
	}

	questions := []Question{
		{
			ID:   1,
			Type: "yes_no",
			Text: "Review tests?",
		},
		{
			ID:      2,
			Type:    "multiple_choice",
			Text:    "Priority?",
			Options: []string{"High", "Medium", "Low"},
		},
		{
			ID:   3,
			Type: "text",
			Text: "Notes?",
		},
	}

	answers, err := agent.presentQuestions(questions)

	require.NoError(t, err)
	assert.Len(t, answers, 3, "should collect all three answers")
	assert.Equal(t, "yes", answers[1])
	assert.Equal(t, "Medium", answers[2])
	assert.Equal(t, "Focus on edge cases", answers[3])
}

func TestPresentQuestions_EmptyQuestionsList(t *testing.T) {
	input := strings.NewReader("")
	output := &strings.Builder{}

	agent := &PlanningAgent{
		input:  input,
		output: output,
	}

	answers, err := agent.presentQuestions([]Question{})

	require.NoError(t, err)
	assert.Empty(t, answers, "should return empty map for no questions")
}

func TestPresentQuestions_UnknownQuestionType(t *testing.T) {
	input := strings.NewReader("")
	output := &strings.Builder{}

	agent := &PlanningAgent{
		input:  input,
		output: output,
	}

	questions := []Question{
		{
			ID:   1,
			Type: "unknown_type",
			Text: "Invalid question",
		},
	}

	_, err := agent.presentQuestions(questions)

	assert.Error(t, err, "should error on unknown question type")
	assert.Contains(t, err.Error(), "unknown question type", "should mention unknown type")
}

// --- Answer Incorporation Tests ---

func TestIncorporateAnswers_WithAnswers(t *testing.T) {
	agent := &PlanningAgent{}

	ctx := ProjectContext{
		Architecture: "Original architecture doc",
		README:       "Original README",
	}

	questions := []Question{
		{
			ID:   1,
			Type: "yes_no",
			Text: "Should I review test files?",
		},
		{
			ID:   2,
			Type: "text",
			Text: "Any specific concerns?",
		},
	}

	answers := map[int]string{
		1: "yes",
		2: "Focus on authentication",
	}

	enhanced := agent.incorporateAnswers(ctx, questions, answers)

	// Original context should be preserved
	assert.Equal(t, ctx.Architecture, enhanced.Architecture)
	assert.Equal(t, ctx.README, enhanced.README)

	// Planning answers should be added
	assert.NotEmpty(t, enhanced.PlanningAnswers, "should have planning answers")

	// Check format of planning answers
	assert.Contains(t, enhanced.PlanningAnswers, "Should I review test files?")
	assert.Contains(t, enhanced.PlanningAnswers, "yes")
	assert.Contains(t, enhanced.PlanningAnswers, "Any specific concerns?")
	assert.Contains(t, enhanced.PlanningAnswers, "Focus on authentication")
}

func TestIncorporateAnswers_EmptyAnswers(t *testing.T) {
	agent := &PlanningAgent{}

	ctx := ProjectContext{
		Architecture: "Architecture doc",
	}

	enhanced := agent.incorporateAnswers(ctx, []Question{}, map[int]string{})

	// Should preserve original context
	assert.Equal(t, ctx.Architecture, enhanced.Architecture)

	// Planning answers should be empty
	assert.Empty(t, enhanced.PlanningAnswers, "should have no planning answers")
}

func TestIncorporateAnswers_PreservesOtherFields(t *testing.T) {
	agent := &PlanningAgent{}

	ctx := ProjectContext{
		Architecture:       "Architecture",
		README:             "README",
		DesignDocs:         []string{"doc1", "doc2"},
		RelevantDocs:       []string{"doc3"},
		CustomContextFiles: []string{"custom1"},
		ChangeTypes:        []string{"auth", "db"},
		ChangedPaths:       []string{"file1.go", "file2.go"},
	}

	questions := []Question{
		{ID: 1, Type: "text", Text: "Notes?"},
	}
	answers := map[int]string{1: "Test notes"}

	enhanced := agent.incorporateAnswers(ctx, questions, answers)

	// All original fields should be preserved
	assert.Equal(t, ctx.Architecture, enhanced.Architecture)
	assert.Equal(t, ctx.README, enhanced.README)
	assert.Equal(t, ctx.DesignDocs, enhanced.DesignDocs)
	assert.Equal(t, ctx.RelevantDocs, enhanced.RelevantDocs)
	assert.Equal(t, ctx.CustomContextFiles, enhanced.CustomContextFiles)
	assert.Equal(t, ctx.ChangeTypes, enhanced.ChangeTypes)
	assert.Equal(t, ctx.ChangedPaths, enhanced.ChangedPaths)

	// Plus new planning answers
	assert.Contains(t, enhanced.PlanningAnswers, "Notes?")
	assert.Contains(t, enhanced.PlanningAnswers, "Test notes")
}

func TestIncorporateAnswers_FormatsMultipleAnswers(t *testing.T) {
	agent := &PlanningAgent{}

	questions := []Question{
		{ID: 1, Type: "yes_no", Text: "Q1?"},
		{ID: 2, Type: "text", Text: "Q2?"},
		{ID: 3, Type: "multiple_choice", Text: "Q3?"},
	}

	answers := map[int]string{
		1: "yes",
		2: "Some text",
		3: "Option B",
	}

	enhanced := agent.incorporateAnswers(ProjectContext{}, questions, answers)

	// All Q&A pairs should be present
	assert.Contains(t, enhanced.PlanningAnswers, "Q1?")
	assert.Contains(t, enhanced.PlanningAnswers, "yes")
	assert.Contains(t, enhanced.PlanningAnswers, "Q2?")
	assert.Contains(t, enhanced.PlanningAnswers, "Some text")
	assert.Contains(t, enhanced.PlanningAnswers, "Q3?")
	assert.Contains(t, enhanced.PlanningAnswers, "Option B")
}

func TestIncorporateAnswers_HandlesEmptyTextAnswers(t *testing.T) {
	agent := &PlanningAgent{}

	questions := []Question{
		{ID: 1, Type: "text", Text: "Additional notes?"},
	}

	answers := map[int]string{
		1: "", // Empty text answer
	}

	enhanced := agent.incorporateAnswers(ProjectContext{}, questions, answers)

	// Should still include the question even with empty answer
	assert.Contains(t, enhanced.PlanningAnswers, "Additional notes?")
}

func TestIncorporateAnswers_OrderPreserved(t *testing.T) {
	agent := &PlanningAgent{}

	questions := []Question{
		{ID: 3, Type: "text", Text: "Third question"},
		{ID: 1, Type: "text", Text: "First question"},
		{ID: 2, Type: "text", Text: "Second question"},
	}

	answers := map[int]string{
		1: "Answer 1",
		2: "Answer 2",
		3: "Answer 3",
	}

	enhanced := agent.incorporateAnswers(ProjectContext{}, questions, answers)

	// Should preserve order from questions array, not sorted by ID
	firstIdx := strings.Index(enhanced.PlanningAnswers, "First")
	secondIdx := strings.Index(enhanced.PlanningAnswers, "Second")
	thirdIdx := strings.Index(enhanced.PlanningAnswers, "Third")

	assert.Less(t, thirdIdx, firstIdx, "Third should appear before First")
	assert.Less(t, firstIdx, secondIdx, "First should appear before Second")
}

// --- Full Planning Integration Tests ---

func TestPlan_FullWorkflow(t *testing.T) {
	// Mock provider returns questions
	llmResponse := `{
		"questions": [
			{
				"id": 1,
				"type": "yes_no",
				"text": "Review tests?"
			}
		],
		"reasoning": "Need to know scope"
	}`

	provider := &mockPlanningProvider{response: llmResponse}

	// User answers "yes"
	input := strings.NewReader("y\n")
	output := &strings.Builder{}

	agent := NewPlanningAgent(provider, PlanningConfig{}, input, output)

	ctx := ProjectContext{Architecture: "Test arch"}
	diff := domain.Diff{Files: []domain.FileDiff{{Path: "test.go"}}}

	result, err := agent.Plan(context.Background(), ctx, diff)

	require.NoError(t, err, "plan should succeed")

	// Check that questions were asked
	assert.Len(t, result.Questions, 1, "should have 1 question")
	assert.Equal(t, "Review tests?", result.Questions[0].Text)

	// Check that answer was captured
	assert.Contains(t, result.Answers, 1, "should have answer for question 1")
	assert.Equal(t, "yes", result.Answers[1])

	// Check that enhanced context includes the answer
	assert.Contains(t, result.EnhancedContext.PlanningAnswers, "Review tests?")
	assert.Contains(t, result.EnhancedContext.PlanningAnswers, "yes")

	// Original context should be preserved
	assert.Equal(t, "Test arch", result.EnhancedContext.Architecture)

	// Output should contain the question
	assert.Contains(t, output.String(), "Review tests?")
}

func TestPlan_NoQuestions(t *testing.T) {
	// LLM returns empty questions list
	llmResponse := `{
		"questions": [],
		"reasoning": "Context is sufficient"
	}`

	provider := &mockPlanningProvider{response: llmResponse}
	agent := NewPlanningAgent(provider, PlanningConfig{}, nil, nil)

	ctx := ProjectContext{}
	diff := domain.Diff{}

	result, err := agent.Plan(context.Background(), ctx, diff)

	require.NoError(t, err, "plan should succeed with no questions")
	assert.Empty(t, result.Questions, "should have no questions")
	assert.Empty(t, result.Answers, "should have no answers")
	assert.Empty(t, result.EnhancedContext.PlanningAnswers, "should have no planning answers")
}

func TestPlan_LLMError(t *testing.T) {
	// Provider returns error
	provider := &mockPlanningProvider{
		err: assert.AnError,
	}

	agent := NewPlanningAgent(provider, PlanningConfig{}, nil, nil)

	ctx := ProjectContext{}
	diff := domain.Diff{}

	_, err := agent.Plan(context.Background(), ctx, diff)

	assert.Error(t, err, "should propagate LLM error")
}

func TestPlan_InvalidJSONResponse(t *testing.T) {
	// LLM returns invalid JSON
	provider := &mockPlanningProvider{
		response: "This is not JSON at all",
	}

	agent := NewPlanningAgent(provider, PlanningConfig{}, nil, nil)

	ctx := ProjectContext{}
	diff := domain.Diff{}

	_, err := agent.Plan(context.Background(), ctx, diff)

	assert.Error(t, err, "should error on invalid JSON")
	assert.Contains(t, err.Error(), "parse", "error should mention parsing")
}

func TestPlan_MultipleQuestions(t *testing.T) {
	llmResponse := `{
		"questions": [
			{"id": 1, "type": "yes_no", "text": "Q1?"},
			{"id": 2, "type": "text", "text": "Q2?"}
		],
		"reasoning": "Need details"
	}`

	provider := &mockPlanningProvider{response: llmResponse}

	// User answers both questions
	input := strings.NewReader("yes\nAnswer 2\n")
	output := &strings.Builder{}

	agent := NewPlanningAgent(provider, PlanningConfig{}, input, output)

	result, err := agent.Plan(context.Background(), ProjectContext{}, domain.Diff{})

	require.NoError(t, err)
	assert.Len(t, result.Questions, 2)
	assert.Len(t, result.Answers, 2)
	assert.Equal(t, "yes", result.Answers[1])
	assert.Equal(t, "Answer 2", result.Answers[2])
}

func TestPlan_ContextCancellation(t *testing.T) {
	provider := &mockPlanningProvider{response: "{}"}
	agent := NewPlanningAgent(provider, PlanningConfig{}, nil, nil)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := agent.Plan(ctx, ProjectContext{}, domain.Diff{})

	assert.Error(t, err, "should error on cancelled context")
}
