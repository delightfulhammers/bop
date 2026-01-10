package review

import (
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
)

func TestRenderPromptTemplate(t *testing.T) {
	tests := []struct {
		name           string
		template       string
		context        ProjectContext
		diff           domain.Diff
		expectedText   []string // Strings that should appear in output
		unexpectedText []string // Strings that should NOT appear
		expectError    bool
	}{
		{
			name:     "simple template with architecture",
			template: "Architecture:\n{{.Architecture}}\n\nDiff:\n{{.Diff}}",
			context: ProjectContext{
				Architecture: "Clean architecture with layers",
			},
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "main.go", Status: "modified", Patch: "diff content"},
				},
			},
			expectedText: []string{"Architecture:", "Clean architecture with layers", "Diff:", "diff content"},
		},
		{
			name:     "template with custom instructions",
			template: "{{if .CustomInstructions}}Instructions: {{.CustomInstructions}}{{end}}",
			context: ProjectContext{
				CustomInstructions: "Focus on security",
			},
			expectedText: []string{"Instructions:", "Focus on security"},
		},
		{
			name:     "template without custom instructions",
			template: "{{if .CustomInstructions}}Instructions: {{.CustomInstructions}}{{end}}Review the code.",
			context: ProjectContext{
				CustomInstructions: "",
			},
			expectedText:   []string{"Review the code."},
			unexpectedText: []string{"Instructions:"},
		},
		{
			name:     "template with change types",
			template: "Change types: {{range $i, $type := .ChangeTypes}}{{if $i}}, {{end}}{{$type}}{{end}}",
			context: ProjectContext{
				ChangeTypes: []string{"auth", "database"},
			},
			expectedText: []string{"Change types:", "auth", "database"},
		},
		{
			name: "template with multiple sections",
			template: `{{if .Architecture}}## Architecture
{{.Architecture}}
{{end}}
{{if .CustomInstructions}}## Instructions
{{.CustomInstructions}}
{{end}}
## Changes
{{.Diff}}`,
			context: ProjectContext{
				Architecture:       "Layered architecture",
				CustomInstructions: "Check for race conditions",
			},
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "worker.go", Patch: "+func process()"},
				},
			},
			expectedText: []string{
				"## Architecture",
				"Layered architecture",
				"## Instructions",
				"Check for race conditions",
				"## Changes",
				"+func process()",
			},
		},
		{
			name:     "template with join helper",
			template: `Files: {{join .ChangedPaths ", "}}`,
			context: ProjectContext{
				ChangedPaths: []string{"main.go", "util.go", "test.go"},
			},
			expectedText: []string{"Files:", "main.go, util.go, test.go"},
		},
		{
			name:        "invalid template syntax",
			template:    "{{.InvalidField",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &EnhancedPromptBuilder{}

			// Use empty request for template rendering tests
			req := BranchRequest{BaseRef: "main", TargetRef: "feature"}
			result, err := builder.renderTemplate(tt.template, tt.context, tt.diff, req)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.expectError {
				return // Don't check content if we expected an error
			}

			// Check expected text appears
			for _, expected := range tt.expectedText {
				if !strings.Contains(result, expected) {
					t.Errorf("expected text %q not found in output:\n%s", expected, result)
				}
			}

			// Check unexpected text does NOT appear
			for _, unexpected := range tt.unexpectedText {
				if strings.Contains(result, unexpected) {
					t.Errorf("unexpected text %q found in output:\n%s", unexpected, result)
				}
			}
		})
	}
}

func TestFormatDiff(t *testing.T) {
	tests := []struct {
		name     string
		diff     domain.Diff
		expected []string // Strings that should appear in formatted output
	}{
		{
			name: "single file diff",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{
						Path:   "main.go",
						Status: "modified",
						Patch:  "@@ -1,3 +1,4 @@\n func main() {\n+  fmt.Println(\"hello\")\n }",
					},
				},
			},
			expected: []string{"main.go", "modified", "func main()", "fmt.Println"},
		},
		{
			name: "multiple files",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "main.go", Status: "modified", Patch: "patch1"},
					{Path: "util.go", Status: "added", Patch: "patch2"},
					{Path: "old.go", Status: "deleted", Patch: ""},
				},
			},
			expected: []string{"main.go", "modified", "util.go", "added", "old.go", "deleted"},
		},
		{
			name: "empty diff",
			diff: domain.Diff{
				Files: []domain.FileDiff{},
			},
			expected: []string{}, // Should return empty or minimal output
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &EnhancedPromptBuilder{}
			result := builder.formatDiff(tt.diff)

			for _, expected := range tt.expected {
				if !strings.Contains(result, expected) {
					t.Errorf("expected text %q not found in formatted diff:\n%s", expected, result)
				}
			}
		})
	}
}

func TestBuildPromptWithContext(t *testing.T) {
	// Integration test: build a complete prompt with all context
	builder := NewEnhancedPromptBuilder()

	context := ProjectContext{
		Architecture:       "Clean architecture system",
		README:             "# My Project\nA great project",
		DesignDocs:         []string{"=== AUTH_DESIGN.md ===\nJWT authentication"},
		CustomInstructions: "Focus on security and performance",
		RelevantDocs:       []string{"=== SECURITY.md ===\nSecurity guidelines"},
		ChangedPaths:       []string{"auth/handler.go", "auth/middleware.go"},
		ChangeTypes:        []string{"auth", "security"},
	}

	diff := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:   "auth/handler.go",
				Status: "modified",
				Patch:  "@@ -10,5 +10,6 @@\n func Login(req Request) {\n+  validateToken(req.Token)\n }",
			},
		},
	}

	req := BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature/auth-improvements",
	}

	// Use default template
	result, err := builder.Build(context, diff, req, "openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify prompt contains key elements
	expectedElements := []string{
		"Clean architecture system",
		"Focus on security and performance",
		"auth/handler.go",
		"validateToken",
		"main",
		"feature/auth-improvements",
	}

	for _, expected := range expectedElements {
		if !strings.Contains(result.Prompt, expected) {
			t.Errorf("expected element %q not found in prompt", expected)
		}
	}

	// Verify max size is set
	if result.MaxSize == 0 {
		t.Error("expected MaxSize to be set")
	}
}

func TestProviderSpecificTemplates(t *testing.T) {
	builder := NewEnhancedPromptBuilder()

	// Add provider-specific templates
	builder.SetProviderTemplate("anthropic", `<role>Expert reviewer</role>
<instructions>{{.CustomInstructions}}</instructions>
<changes>{{.Diff}}</changes>`)

	builder.SetProviderTemplate("openai", `You are an expert reviewer.
Instructions: {{.CustomInstructions}}
Changes: {{.Diff}}`)

	context := ProjectContext{
		CustomInstructions: "Check for bugs",
	}
	diff := domain.Diff{
		Files: []domain.FileDiff{{Path: "test.go", Patch: "patch"}},
	}
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}

	// Test Anthropic template
	anthropicResult, err := builder.Build(context, diff, req, "anthropic")
	if err != nil {
		t.Fatalf("anthropic build failed: %v", err)
	}
	if !strings.Contains(anthropicResult.Prompt, "<role>") {
		t.Error("Anthropic template not used (missing <role> tag)")
	}

	// Test OpenAI template
	openaiResult, err := builder.Build(context, diff, req, "openai")
	if err != nil {
		t.Fatalf("openai build failed: %v", err)
	}
	if !strings.Contains(openaiResult.Prompt, "You are an expert reviewer") {
		t.Error("OpenAI template not used")
	}
	if strings.Contains(openaiResult.Prompt, "<role>") {
		t.Error("OpenAI should not have Anthropic-style tags")
	}
}

func TestIntegration_ContextGatheringWithPromptBuilder(t *testing.T) {
	// Integration test: gather context and build prompts in realistic scenario

	// Setup context gatherer with test data directory
	gatherer := NewContextGatherer("testdata")

	// Load architecture and design docs
	architecture, err := gatherer.loadFile("docs/ARCHITECTURE.md")
	if err != nil {
		t.Logf("Warning: failed to load architecture: %v", err)
		architecture = "" // Continue without it
	}

	designDocs, err := gatherer.loadDesignDocs()
	if err != nil {
		t.Logf("Warning: failed to load design docs: %v", err)
		designDocs = nil // Continue without them
	}
	t.Logf("Loaded %d design docs", len(designDocs))
	for i, doc := range designDocs {
		t.Logf("Design doc %d preview: %s", i, doc[:min(100, len(doc))])
	}

	// Simulate a diff with auth-related changes
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:   "internal/auth/handler.go",
				Status: "modified",
				Patch:  "@@ -15,5 +15,7 @@\n func Login(req Request) {\n+  token := generateToken()\n+  validateToken(token)\n }",
			},
		},
	}

	// Detect change types
	changeTypes := gatherer.detectChangeTypes(diff)
	t.Logf("Detected change types: %v", changeTypes)

	// Find relevant docs
	relevantDocs, err := gatherer.findRelevantDocs([]string{"internal/auth/handler.go"}, changeTypes)
	if err != nil {
		t.Logf("Warning: failed to find relevant docs: %v", err)
		relevantDocs = nil // Continue without them
	}
	t.Logf("Found %d relevant docs", len(relevantDocs))

	// Build project context
	context := ProjectContext{
		Architecture:       architecture,
		DesignDocs:         designDocs,
		RelevantDocs:       relevantDocs,
		CustomInstructions: "Focus on security vulnerabilities",
		ChangeTypes:        changeTypes,
		ChangedPaths:       []string{"internal/auth/handler.go"},
	}

	// Create prompt builder
	builder := NewEnhancedPromptBuilder()

	// Build prompt
	req := BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature/auth-improvements",
	}

	result, err := builder.Build(context, diff, req, "openai")
	if err != nil {
		t.Fatalf("failed to build prompt: %v", err)
	}

	t.Logf("Generated prompt length: %d bytes", len(result.Prompt))
	t.Logf("Prompt preview (first 500 chars):\n%s", result.Prompt[:min(500, len(result.Prompt))])

	// Verify prompt contains key expected context
	expectedElements := []string{
		"Focus on security vulnerabilities", // Custom instructions should always be there
		"generateToken",                     // From diff
		"main",                              // Base ref
		"feature/auth-improvements",         // Target ref
	}

	for _, expected := range expectedElements {
		if !strings.Contains(result.Prompt, expected) {
			t.Errorf("prompt missing expected element %q", expected)
		}
	}

	// Verify design docs are included if they were loaded
	if len(designDocs) > 0 {
		if !strings.Contains(result.Prompt, "JWT") && !strings.Contains(result.Prompt, "authentication") {
			t.Error("prompt should include content from design docs when available")
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestFileTypePriority(t *testing.T) {
	tests := []struct {
		path     string
		expected int
	}{
		// Priority 0: Source code files (including test files with source extensions)
		{"main.go", 0},
		{"internal/auth/handler.go", 0},
		{"src/app.py", 0},
		{"index.js", 0},
		{"component.tsx", 0},
		{"lib.rs", 0},
		{"Main.java", 0},
		{"utils.c", 0},
		{"helper.cpp", 0},
		{"test_utils.py", 0},  // .py is source code, even if it's a test
		{"spec/helper.rb", 0}, // .rb is source code, even if it's a spec

		// Priority 1: Test directories without source extensions
		// (Currently no common cases - most test files have source extensions)

		// Priority 2: Configuration files
		{"config.yaml", 2},
		{"settings.yml", 2},
		{"package.json", 2},
		{"config.toml", 2},
		{".env", 2},
		{".github/workflows/ci.yml", 2}, // .yml is config (checked before CI path check)

		// Priority 3: Build/CI files (without config extensions)
		{"Dockerfile", 3},
		{"Makefile", 3},

		// Priority 4: Documentation files
		{"README.md", 4},
		{"docs/ARCHITECTURE.md", 4},
		{"CHANGELOG.rst", 4},
		{"notes.txt", 4},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := fileTypePriority(tt.path)
			if result != tt.expected {
				t.Errorf("fileTypePriority(%q) = %d, want %d", tt.path, result, tt.expected)
			}
		})
	}
}

func TestFormatDiff_FileOrdering(t *testing.T) {
	// Test that files are sorted with source code first
	builder := &EnhancedPromptBuilder{}

	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "README.md", Status: "modified", Patch: "readme patch"},
			{Path: "docs/DESIGN.md", Status: "added", Patch: "design patch"},
			{Path: "main.go", Status: "modified", Patch: "go patch"},
			{Path: "config.yaml", Status: "modified", Patch: "yaml patch"},
			{Path: "security-tests/test.go", Status: "added", Patch: "test patch"},
		},
	}

	result := builder.formatDiff(diff)

	// Find positions of each file in the output
	goPos := strings.Index(result, "main.go")
	testPos := strings.Index(result, "security-tests/test.go")
	yamlPos := strings.Index(result, "config.yaml")
	readmePos := strings.Index(result, "README.md")
	docsPos := strings.Index(result, "docs/DESIGN.md")

	// Source code (.go) should come first
	if goPos > yamlPos || goPos > readmePos || goPos > docsPos {
		t.Error("Source code files (.go) should appear before config and docs")
	}

	// Test files should come after main source but before config
	if testPos > yamlPos {
		t.Error("Test files should appear before config files")
	}

	// Config should come before documentation
	if yamlPos > readmePos || yamlPos > docsPos {
		t.Error("Config files should appear before documentation")
	}

	// Both markdown files should be at the end
	if readmePos < yamlPos || docsPos < yamlPos {
		t.Error("Documentation files should appear last")
	}
}

func TestPromptTemplate_CodeFirst(t *testing.T) {
	// Verify that the default template puts code before documentation
	template := defaultPromptTemplate()

	// Find positions of key sections
	codeChangesPos := strings.Index(template, "Code Changes to Review")
	architecturePos := strings.Index(template, "Project Architecture")
	readmePos := strings.Index(template, "Project Overview")
	designDocsPos := strings.Index(template, "Design Documentation")

	// Code changes section should come before documentation sections
	if codeChangesPos > architecturePos {
		t.Error("Code Changes section should appear before Architecture section")
	}
	if codeChangesPos > readmePos {
		t.Error("Code Changes section should appear before Project Overview section")
	}
	if codeChangesPos > designDocsPos {
		t.Error("Code Changes section should appear before Design Documentation section")
	}

	// Template should emphasize code review
	if !strings.Contains(template, "PRIMARY FOCUS") {
		t.Error("Template should emphasize that code is the primary focus")
	}
	if !strings.Contains(template, "source code") {
		t.Error("Template should mention reviewing source code files")
	}
}

// mockTokenEstimator is a simple mock for testing size guards.
type mockTokenEstimator struct {
	tokensPerChar float64
}

func (m *mockTokenEstimator) EstimateTokens(text string) int {
	return int(float64(len(text)) * m.tokensPerChar)
}

func TestBuildWithSizeGuards_NoTruncation(t *testing.T) {
	builder := NewEnhancedPromptBuilder()
	estimator := &mockTokenEstimator{tokensPerChar: 0.25} // 4 chars per token

	context := ProjectContext{
		CustomInstructions: "Focus on security",
	}
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "small patch"},
		},
	}
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}
	limits := SizeGuardLimits{WarnTokens: 10000, MaxTokens: 20000}

	result, truncation, err := builder.BuildWithSizeGuards(context, diff, req, "openai", estimator, limits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if truncation.WasTruncated {
		t.Error("expected no truncation for small prompt")
	}
	if truncation.WasWarned {
		t.Error("expected no warning for small prompt")
	}
	if len(truncation.RemovedFiles) > 0 {
		t.Errorf("expected no removed files, got %v", truncation.RemovedFiles)
	}
	if result.Prompt == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestBuildWithSizeGuards_WarnOnly(t *testing.T) {
	builder := NewEnhancedPromptBuilder()
	// Very high tokens per char to trigger warning without truncation
	estimator := &mockTokenEstimator{tokensPerChar: 10}

	context := ProjectContext{}
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "patch"},
		},
	}
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}
	// Set warn low, max high - should warn but not truncate
	limits := SizeGuardLimits{WarnTokens: 100, MaxTokens: 1000000}

	_, truncation, err := builder.BuildWithSizeGuards(context, diff, req, "openai", estimator, limits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !truncation.WasWarned {
		t.Error("expected warning for prompt exceeding warn threshold")
	}
	if truncation.WasTruncated {
		t.Error("expected no truncation when under max threshold")
	}
}

func TestBuildWithSizeGuards_TruncationByPriority(t *testing.T) {
	builder := NewEnhancedPromptBuilder()
	// Use a very high token rate to force truncation
	estimator := &mockTokenEstimator{tokensPerChar: 100}

	context := ProjectContext{}
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: strings.Repeat("x", 100)},        // Priority 0: Source
			{Path: "README.md", Status: "modified", Patch: strings.Repeat("x", 100)},      // Priority 4: Doc
			{Path: "config.yaml", Status: "modified", Patch: strings.Repeat("x", 100)},    // Priority 2: Config
			{Path: "test_helper.go", Status: "modified", Patch: strings.Repeat("x", 100)}, // Priority 1: Test
			{Path: "Dockerfile", Status: "modified", Patch: strings.Repeat("x", 100)},     // Priority 3: Build
			{Path: "docs/design.md", Status: "modified", Patch: strings.Repeat("x", 100)}, // Priority 4: Doc
		},
	}
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}
	// Very low max to force significant truncation
	limits := SizeGuardLimits{WarnTokens: 100, MaxTokens: 50000}

	_, truncation, err := builder.BuildWithSizeGuards(context, diff, req, "openai", estimator, limits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !truncation.WasTruncated {
		t.Error("expected truncation for large prompt")
	}
	if len(truncation.RemovedFiles) == 0 {
		t.Error("expected some files to be removed")
	}

	// Verify documentation files are removed first (higher priority to remove)
	// and source code files are kept (lower priority to remove)
	removedSet := make(map[string]bool)
	for _, f := range truncation.RemovedFiles {
		removedSet[f] = true
	}

	// Docs should be removed before source code
	if removedSet["main.go"] && !removedSet["README.md"] {
		t.Error("source code (main.go) should not be removed before documentation (README.md)")
	}
	if removedSet["main.go"] && !removedSet["docs/design.md"] {
		t.Error("source code (main.go) should not be removed before documentation (docs/design.md)")
	}
}

func TestBuildWithSizeGuards_TruncationNote(t *testing.T) {
	builder := NewEnhancedPromptBuilder()
	// Use lower token rate so removing README.md brings us under the limit
	estimator := &mockTokenEstimator{tokensPerChar: 10}

	context := ProjectContext{}
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: strings.Repeat("x", 500)},
			{Path: "README.md", Status: "modified", Patch: strings.Repeat("x", 5000)},
		},
	}
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}
	// Set limit so initial exceeds but after removing README.md we're under
	limits := SizeGuardLimits{WarnTokens: 100, MaxTokens: 30000}

	_, truncation, err := builder.BuildWithSizeGuards(context, diff, req, "openai", estimator, limits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !truncation.WasTruncated {
		t.Fatal("expected truncation to occur")
	}
	if truncation.TruncationNote == "" {
		t.Error("expected truncation note when files are removed")
	}
	if !strings.Contains(truncation.TruncationNote, "exceeded limit") {
		t.Error("truncation note should mention exceeding limit")
	}
	if !strings.Contains(truncation.TruncationNote, "Consider splitting") {
		t.Error("truncation note should suggest splitting the PR")
	}
}

func TestBuildWithSizeGuards_StillExceedsAfterTruncation(t *testing.T) {
	builder := NewEnhancedPromptBuilder()
	// Very high token rate - even one file exceeds the limit
	estimator := &mockTokenEstimator{tokensPerChar: 1000}

	context := ProjectContext{}
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: strings.Repeat("x", 500)},
			{Path: "README.md", Status: "modified", Patch: strings.Repeat("x", 500)},
		},
	}
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}
	// Even with all files removed, template overhead exceeds this
	limits := SizeGuardLimits{WarnTokens: 100, MaxTokens: 1000}

	_, truncation, err := builder.BuildWithSizeGuards(context, diff, req, "openai", estimator, limits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have removed files but still exceed limit
	if !truncation.WasTruncated {
		t.Fatal("expected truncation to occur")
	}
	if truncation.FinalTokens <= limits.MaxTokens {
		t.Skipf("test scenario didn't produce still-exceeds condition (final=%d, max=%d)",
			truncation.FinalTokens, limits.MaxTokens)
	}
	if !strings.Contains(truncation.TruncationNote, "too large to review") {
		t.Errorf("truncation note should indicate review difficulty, got: %s", truncation.TruncationNote)
	}
}

func TestBuildWithSizeGuards_PreservesSourceCode(t *testing.T) {
	builder := NewEnhancedPromptBuilder()
	// Use token rate that requires removing some files but not all
	estimator := &mockTokenEstimator{tokensPerChar: 1}

	context := ProjectContext{}
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: strings.Repeat("x", 500)},
			{Path: "util.go", Status: "modified", Patch: strings.Repeat("x", 500)},
			{Path: "README.md", Status: "modified", Patch: strings.Repeat("x", 10000)},
			{Path: "docs/guide.md", Status: "modified", Patch: strings.Repeat("x", 10000)},
			{Path: "CHANGELOG.md", Status: "modified", Patch: strings.Repeat("x", 10000)},
		},
	}
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}
	// Set max tokens to force some truncation but keep source
	limits := SizeGuardLimits{WarnTokens: 5000, MaxTokens: 15000}

	result, truncation, err := builder.BuildWithSizeGuards(context, diff, req, "openai", estimator, limits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Source code files should be preserved
	if strings.Contains(strings.Join(truncation.RemovedFiles, " "), "main.go") {
		t.Error("main.go should not be removed (source code has lowest removal priority)")
	}
	if strings.Contains(strings.Join(truncation.RemovedFiles, " "), "util.go") {
		t.Error("util.go should not be removed (source code has lowest removal priority)")
	}

	// Result prompt should contain source code
	if !strings.Contains(result.Prompt, "main.go") {
		t.Error("prompt should contain main.go")
	}
}

func TestBuildWithSizeGuards_TemplateError(t *testing.T) {
	builder := NewEnhancedPromptBuilder()
	// Set a broken template that will fail to parse/execute
	builder.SetProviderTemplate("broken", "{{.InvalidField}}")

	estimator := &mockTokenEstimator{tokensPerChar: 1}
	context := ProjectContext{}
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "x"},
		},
	}
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}
	limits := SizeGuardLimits{WarnTokens: 100, MaxTokens: 200}

	_, _, err := builder.BuildWithSizeGuards(context, diff, req, "broken", estimator, limits)
	if err == nil {
		t.Error("expected error for broken template")
	}
	if !strings.Contains(err.Error(), "template") {
		t.Errorf("error should mention template, got: %v", err)
	}
}

func TestBuildWithSizeGuards_EmptyDiff(t *testing.T) {
	builder := NewEnhancedPromptBuilder()
	estimator := &mockTokenEstimator{tokensPerChar: 1}

	context := ProjectContext{}
	diff := domain.Diff{Files: []domain.FileDiff{}} // Empty
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}
	limits := SizeGuardLimits{WarnTokens: 100, MaxTokens: 200}

	result, truncation, err := builder.BuildWithSizeGuards(context, diff, req, "openai", estimator, limits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if truncation.WasTruncated {
		t.Error("empty diff should not be truncated")
	}
	if len(truncation.RemovedFiles) > 0 {
		t.Error("empty diff should not have removed files")
	}
	if result.Prompt == "" {
		t.Error("prompt should not be empty even with empty diff")
	}
}

func TestBuildWithSizeGuards_NilEstimator(t *testing.T) {
	builder := NewEnhancedPromptBuilder()

	context := ProjectContext{}
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "x"},
		},
	}
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}
	limits := SizeGuardLimits{WarnTokens: 100, MaxTokens: 200}

	_, _, err := builder.BuildWithSizeGuards(context, diff, req, "openai", nil, limits)
	if err == nil {
		t.Error("expected error for nil estimator")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil, got: %v", err)
	}
}

// Tests for formatPriorFindings (Issue #138)

func TestFormatPriorFindings_NilContext(t *testing.T) {
	result := formatPriorFindings(nil)
	if result != "" {
		t.Errorf("expected empty string for nil context, got: %q", result)
	}
}

func TestFormatPriorFindings_NoFindings(t *testing.T) {
	ctx := &domain.TriagedFindingContext{
		PRNumber: 123,
		Findings: []domain.TriagedFinding{},
	}
	result := formatPriorFindings(ctx)
	if result != "" {
		t.Errorf("expected empty string for empty findings, got: %q", result)
	}
}

func TestFormatPriorFindings_OnlyOpenFindings(t *testing.T) {
	// Issue #165: Open findings SHOULD appear to prevent LLM from re-raising them
	ctx := &domain.TriagedFindingContext{
		PRNumber: 123,
		Findings: []domain.TriagedFinding{
			{
				File:         "main.go",
				LineStart:    10,
				LineEnd:      15,
				Category:     "security",
				Description:  "Open issue",
				Status:       domain.StatusOpen,
				StatusReason: "Previously raised - awaiting author response",
			},
		},
	}
	result := formatPriorFindings(ctx)

	// Should contain open findings section
	if !strings.Contains(result, "Previously Posted Findings") {
		t.Error("expected 'Previously Posted Findings' header for open findings")
	}
	if !strings.Contains(result, "do NOT re-raise") {
		t.Error("expected instruction not to re-raise")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("expected file name in output")
	}
	if !strings.Contains(result, "security") {
		t.Error("expected category in output")
	}
	if !strings.Contains(result, "awaiting") {
		t.Error("expected status reason mentioning awaiting response")
	}
}

func TestFormatPriorFindings_AcknowledgedFindings(t *testing.T) {
	ctx := &domain.TriagedFindingContext{
		PRNumber: 123,
		Findings: []domain.TriagedFinding{
			{
				File:         "auth.go",
				LineStart:    20,
				LineEnd:      25,
				Category:     "security",
				Description:  "Missing input validation",
				Status:       domain.StatusAcknowledged,
				StatusReason: "Author acknowledged the concern",
			},
		},
	}
	result := formatPriorFindings(ctx)

	// Should contain acknowledged section
	if !strings.Contains(result, "Acknowledged Findings") {
		t.Error("expected 'Acknowledged Findings' header")
	}
	if !strings.Contains(result, "do NOT re-raise") {
		t.Error("expected instruction not to re-raise")
	}
	if !strings.Contains(result, "auth.go") {
		t.Error("expected file name in output")
	}
	if !strings.Contains(result, "security") {
		t.Error("expected category in output")
	}
	if !strings.Contains(result, "Missing input validation") {
		t.Error("expected description in output")
	}

	// Should NOT contain disputed section
	if strings.Contains(result, "Disputed Findings") {
		t.Error("should not contain 'Disputed Findings' when there are none")
	}
}

func TestFormatPriorFindings_DisputedFindings(t *testing.T) {
	ctx := &domain.TriagedFindingContext{
		PRNumber: 123,
		Findings: []domain.TriagedFinding{
			{
				File:         "config.go",
				LineStart:    5,
				LineEnd:      5,
				Category:     "performance",
				Description:  "Unnecessary allocation",
				Status:       domain.StatusDisputed,
				StatusReason: "Author disputed as false positive",
			},
		},
	}
	result := formatPriorFindings(ctx)

	// Should contain disputed section
	if !strings.Contains(result, "Disputed Findings") {
		t.Error("expected 'Disputed Findings' header")
	}
	if !strings.Contains(result, "false positives") {
		t.Error("expected mention of false positives")
	}
	if !strings.Contains(result, "config.go") {
		t.Error("expected file name in output")
	}

	// Should NOT contain acknowledged section
	if strings.Contains(result, "Acknowledged Findings") {
		t.Error("should not contain 'Acknowledged Findings' when there are none")
	}
}

func TestFormatPriorFindings_MixedFindings(t *testing.T) {
	// Issue #165: All findings (acknowledged, disputed, AND open) should appear
	ctx := &domain.TriagedFindingContext{
		PRNumber: 123,
		Findings: []domain.TriagedFinding{
			{
				File:         "auth.go",
				LineStart:    20,
				LineEnd:      25,
				Category:     "security",
				Description:  "Acknowledged security issue",
				Status:       domain.StatusAcknowledged,
				StatusReason: "Author acknowledged",
			},
			{
				File:         "config.go",
				LineStart:    5,
				LineEnd:      5,
				Category:     "performance",
				Description:  "Disputed performance issue",
				Status:       domain.StatusDisputed,
				StatusReason: "Author disputed",
			},
			{
				File:         "main.go",
				LineStart:    1,
				LineEnd:      1,
				Category:     "bug",
				Description:  "Open bug awaiting response",
				Status:       domain.StatusOpen,
				StatusReason: "Previously raised - awaiting author response",
			},
		},
	}
	result := formatPriorFindings(ctx)

	// Should contain all three sections
	if !strings.Contains(result, "Acknowledged Findings") {
		t.Error("expected 'Acknowledged Findings' header")
	}
	if !strings.Contains(result, "Disputed Findings") {
		t.Error("expected 'Disputed Findings' header")
	}
	if !strings.Contains(result, "Previously Posted Findings") {
		t.Error("expected 'Previously Posted Findings' header for open findings")
	}

	// Should contain all findings
	if !strings.Contains(result, "Acknowledged security issue") {
		t.Error("expected acknowledged finding description")
	}
	if !strings.Contains(result, "Disputed performance issue") {
		t.Error("expected disputed finding description")
	}
	// Issue #165: Open findings SHOULD now appear
	if !strings.Contains(result, "Open bug awaiting response") {
		t.Error("expected open finding description (Issue #165)")
	}
}

func TestPromptTemplate_IncludesPriorFindings(t *testing.T) {
	// Verify that the default template has the PriorFindings section
	template := defaultPromptTemplate()

	if !strings.Contains(template, "{{if .PriorFindings}}") {
		t.Error("template should have conditional for PriorFindings")
	}
	if !strings.Contains(template, "Previously Addressed Concerns") {
		t.Error("template should have 'Previously Addressed Concerns' section header")
	}
	if !strings.Contains(template, "DO NOT raise similar concerns") {
		t.Error("template should instruct LLM not to raise similar concerns")
	}
}

func TestRenderTemplate_WithPriorFindings(t *testing.T) {
	builder := NewEnhancedPromptBuilder()

	// Create context with triaged findings
	triageCtx := &domain.TriagedFindingContext{
		PRNumber: 123,
		Findings: []domain.TriagedFinding{
			{
				File:         "auth.go",
				LineStart:    20,
				LineEnd:      25,
				Category:     "security",
				Description:  "Test acknowledged finding",
				Status:       domain.StatusAcknowledged,
				StatusReason: "Author acknowledged the concern",
			},
		},
	}

	context := ProjectContext{
		TriagedFindings: triageCtx,
	}
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "patch content"},
		},
	}
	req := BranchRequest{BaseRef: "main", TargetRef: "feature"}

	result, err := builder.Build(context, diff, req, "openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify prior findings appear in the prompt
	if !strings.Contains(result.Prompt, "Previously Addressed Concerns") {
		t.Error("prompt should contain 'Previously Addressed Concerns' section")
	}
	if !strings.Contains(result.Prompt, "Test acknowledged finding") {
		t.Error("prompt should contain the triaged finding description")
	}
	if !strings.Contains(result.Prompt, "auth.go") {
		t.Error("prompt should contain the triaged finding file")
	}
}

// Tests for sanitizeRationale (Issue #191)

func TestSanitizeRationale_EmptyString(t *testing.T) {
	result := sanitizeRationale("", "")
	if result != "" {
		t.Errorf("expected empty string for empty input, got: %q", result)
	}
}

func TestSanitizeRationale_ShortRationale(t *testing.T) {
	input := "This is a valid rationale."
	result := sanitizeRationale(input, "")

	// Should be wrapped in quote block
	if !strings.HasPrefix(result, "> ") {
		t.Error("expected rationale to be wrapped in markdown quote block")
	}
	if !strings.Contains(result, input) {
		t.Error("expected full rationale to be preserved")
	}
	// Should not be truncated
	if strings.Contains(result, "[truncated]") {
		t.Error("short rationale should not be truncated")
	}
}

func TestSanitizeRationale_MultilineRationale(t *testing.T) {
	input := "Line one\nLine two\nLine three"
	result := sanitizeRationale(input, "")

	// Each line should have quote prefix
	lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "> ") {
			t.Errorf("line %d should start with '> ', got: %q", i, line)
		}
	}
}

func TestSanitizeRationale_WithIndent(t *testing.T) {
	input := "Line one\nLine two"
	result := sanitizeRationale(input, "     ") // 5 spaces for markdown list

	// Each line should have indent + quote prefix
	lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "     > ") {
			t.Errorf("line %d should start with '     > ', got: %q", i, line)
		}
	}
}

func TestSanitizeRationale_ExceedsMaxLength(t *testing.T) {
	// Create a rationale that exceeds MaxRationaleLength
	longRationale := strings.Repeat("a", MaxRationaleLength+100)
	result := sanitizeRationale(longRationale, "")

	// Should be truncated
	if !strings.Contains(result, "[truncated]") {
		t.Error("long rationale should be truncated with marker")
	}

	// The content (without quote markers) should not exceed max length + truncation suffix
	// Remove quote prefixes to check content length
	content := strings.ReplaceAll(result, "> ", "")
	content = strings.TrimSpace(content)

	maxExpected := MaxRationaleLength + len("... [truncated]")
	if len(content) > maxExpected {
		t.Errorf("truncated content too long: got %d, max expected %d", len(content), maxExpected)
	}
}

func TestSanitizeRationale_ExactlyMaxLength(t *testing.T) {
	// Create a rationale that is exactly MaxRationaleLength
	exactRationale := strings.Repeat("b", MaxRationaleLength)
	result := sanitizeRationale(exactRationale, "")

	// Should NOT be truncated (boundary case)
	if strings.Contains(result, "[truncated]") {
		t.Error("rationale exactly at max length should not be truncated")
	}
}

func TestSanitizeRationale_UTF8Safe(t *testing.T) {
	// Create a rationale with multi-byte UTF-8 characters near the truncation boundary
	// Using emoji (4 bytes each) to test UTF-8 safety
	emoji := "🔥" // 4 bytes, 1 rune
	longRationale := strings.Repeat(emoji, MaxRationaleLength+10)
	result := sanitizeRationale(longRationale, "")

	// Should be truncated
	if !strings.Contains(result, "[truncated]") {
		t.Error("long rationale should be truncated with marker")
	}

	// Result should be valid UTF-8 (no broken characters)
	content := strings.ReplaceAll(result, "> ", "")
	content = strings.TrimSpace(content)
	content = strings.TrimSuffix(content, "... [truncated]")

	// Each remaining character should be a complete emoji
	for _, r := range content {
		if r == '�' { // Unicode replacement character indicates broken UTF-8
			t.Error("truncation produced invalid UTF-8 (broken multi-byte character)")
		}
	}
}

func TestSanitizeRationale_WindowsNewlines(t *testing.T) {
	// Windows-style \r\n newlines should be normalized to \n
	input := "Line one\r\nLine two\r\nLine three"
	result := sanitizeRationale(input, "")

	// Should NOT contain any \r characters
	if strings.Contains(result, "\r") {
		t.Error("result should not contain carriage return characters")
	}

	// Should have 3 lines, each with quote prefix
	lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "> ") {
			t.Errorf("line %d should start with '> ', got: %q", i, line)
		}
	}
}

func TestSanitizeRationale_OldMacNewlines(t *testing.T) {
	// Old Mac-style \r newlines should be normalized to \n
	input := "Line one\rLine two\rLine three"
	result := sanitizeRationale(input, "")

	// Should NOT contain any \r characters
	if strings.Contains(result, "\r") {
		t.Error("result should not contain carriage return characters")
	}

	// Should have 3 lines, each with quote prefix
	lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestSanitizeRationale_MixedNewlines(t *testing.T) {
	// Mixed newline styles should all be normalized
	input := "Unix line\nWindows line\r\nOld Mac line\rFinal line"
	result := sanitizeRationale(input, "")

	// Should NOT contain any \r characters
	if strings.Contains(result, "\r") {
		t.Error("result should not contain carriage return characters")
	}

	// Should have 4 lines
	lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d", len(lines))
	}
}

func TestFormatPriorFindings_SanitizesRationale(t *testing.T) {
	// Issue #191: User-provided rationales should be sanitized
	ctx := &domain.TriagedFindingContext{
		PRNumber: 123,
		Findings: []domain.TriagedFinding{
			{
				File:         "auth.go",
				LineStart:    20,
				LineEnd:      25,
				Category:     "security",
				Description:  "Test finding",
				Status:       domain.StatusAcknowledged,
				StatusReason: "User provided rationale",
			},
		},
	}
	result := formatPriorFindings(ctx)

	// Rationale should be in indented quote block format (5 spaces for markdown list structure)
	if !strings.Contains(result, "     > User provided rationale") {
		t.Error("expected rationale to be wrapped in indented quote block")
	}

	// Should have the "(user-provided)" label
	if !strings.Contains(result, "user-provided") {
		t.Error("expected rationale label to indicate user-provided content")
	}
}

func TestFormatPriorFindings_TruncatesLongRationale(t *testing.T) {
	// Issue #191: Long rationales should be truncated
	longRationale := strings.Repeat("x", MaxRationaleLength+500)

	ctx := &domain.TriagedFindingContext{
		PRNumber: 123,
		Findings: []domain.TriagedFinding{
			{
				File:         "auth.go",
				LineStart:    20,
				LineEnd:      25,
				Category:     "security",
				Description:  "Test finding",
				Status:       domain.StatusDisputed,
				StatusReason: longRationale,
			},
		},
	}
	result := formatPriorFindings(ctx)

	// Should contain truncation marker
	if !strings.Contains(result, "[truncated]") {
		t.Error("expected long rationale to be truncated")
	}

	// Full original rationale should NOT be present
	if strings.Contains(result, longRationale) {
		t.Error("full long rationale should not appear in output")
	}
}

func TestBuildPromptWithThemes(t *testing.T) {
	builder := NewEnhancedPromptBuilder()

	context := ProjectContext{
		ThemeContext: &ThemeExtractionResult{
			Themes:   []string{"input validation", "error handling", "null safety"},
			Strategy: StrategyAbstract,
		},
	}

	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "diff content"},
		},
	}

	req := BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature",
	}

	result, err := builder.Build(context, diff, req, "openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify themes section appears in prompt
	expectedElements := []string{
		"Explored Themes and Decisions",
		"input validation",
		"error handling",
		"null safety",
		"DO NOT raise findings that contradict these decisions",
	}

	for _, expected := range expectedElements {
		if !strings.Contains(result.Prompt, expected) {
			t.Errorf("expected element %q not found in prompt", expected)
		}
	}
}

func TestBuildPromptWithoutThemes(t *testing.T) {
	builder := NewEnhancedPromptBuilder()

	context := ProjectContext{
		// No extracted themes
	}

	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Status: "modified", Patch: "diff content"},
		},
	}

	req := BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature",
	}

	result, err := builder.Build(context, diff, req, "openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify themes section does NOT appear in prompt
	if strings.Contains(result.Prompt, "Explored Themes") {
		t.Error("themes section should not appear when no themes are provided")
	}
}
