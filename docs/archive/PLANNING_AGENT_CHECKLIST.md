# Planning Agent Implementation Checklist - Phase 4

**Status**: Ready to Start
**Version**: 1.0
**Date**: 2025-10-26
**Design**: [PLANNING_AGENT_DESIGN.md](PLANNING_AGENT_DESIGN.md)

This checklist uses TDD methodology: Write test → Run test (fail) → Implement → Run test (pass) → Refactor → Verify.

---

## Pre-Implementation Setup

- [ ] Review PLANNING_AGENT_DESIGN.md
- [ ] Review ENHANCED_PROMPTING_DESIGN.md (Phase 4 section)
- [ ] Review current `orchestrator.go` implementation
- [ ] Verify all Phase 1-3.5 tests passing
- [ ] Create feature branch: `git checkout -b feature/planning-agent`

---

## Step 1: TTY Detection (30 minutes)

### 1.1 Test - TTY Detection

- [ ] Create `internal/usecase/review/tty_test.go`
- [ ] Test: `TestIsTTY`
  ```go
  func TestIsTTY(t *testing.T) {
      // Should return bool without error
      // Note: May return false in CI environment
  }
  ```
- [ ] Test: `TestIsInteractive`
  ```go
  func TestIsInteractive(t *testing.T) {
      // Should check stdin
  }
  ```
- [ ] Test: `TestIsOutputTerminal`
  ```go
  func TestIsOutputTerminal(t *testing.T) {
      // Should check stdout
  }
  ```
- [ ] Run tests: `go test ./internal/usecase/review -v -run TTY`
- [ ] **Verify**: Tests fail (function not implemented)

### 1.2 Implement - TTY Detection

- [ ] Create `internal/usecase/review/tty.go`
- [ ] Add dependency: `go get golang.org/x/term`
- [ ] Implement `IsTTY(fd uintptr) bool`
- [ ] Implement `IsInteractive() bool`
- [ ] Implement `IsOutputTerminal() bool`
- [ ] Run tests: `go test ./internal/usecase/review -v -run TTY`
- [ ] **Verify**: Tests pass

### 1.3 Verify - TTY Detection

- [ ] Run formatter: `mage format`
- [ ] Run linter: `go vet ./internal/usecase/review`
- [ ] Run all tests: `mage test`
- [ ] Manual test: Run in terminal vs piped (e.g., `echo "" | go run ./cmd/bop`)

---

## Step 2: Planning Types and Interfaces (1 hour)

### 2.1 Test - Question Parsing

- [ ] Create `internal/usecase/review/planner_test.go`
- [ ] Test: `TestParseQuestions_ValidJSON`
  ```go
  func TestParseQuestions_ValidJSON(t *testing.T) {
      response := `{
          "questions": [
              {
                  "id": 1,
                  "type": "multiple_choice",
                  "text": "Is this for admin users?",
                  "options": ["Yes", "No", "All users"]
              }
          ],
          "reasoning": "Need to understand scope"
      }`

      // Should parse successfully
      // Should have 1 question
      // Question should have correct fields
  }
  ```
- [ ] Test: `TestParseQuestions_JSONInMarkdown`
  ```go
  func TestParseQuestions_JSONInMarkdown(t *testing.T) {
      response := "Here are the questions:\n```json\n{...}\n```"
      // Should extract JSON from markdown
  }
  ```
- [ ] Test: `TestParseQuestions_InvalidJSON`
  ```go
  func TestParseQuestions_InvalidJSON(t *testing.T) {
      response := "not valid json"
      // Should return error
  }
  ```
- [ ] Run tests: `go test ./internal/usecase/review -v -run ParseQuestions`
- [ ] **Verify**: Tests fail (function not implemented)

### 2.2 Implement - Question Parsing

- [ ] Create `internal/usecase/review/planner.go`
- [ ] Define types:
  - [ ] `Question` struct
  - [ ] `PlanningResponse` struct
  - [ ] `PlanningResult` struct
  - [ ] `PlanningConfig` struct
  - [ ] `PlanningProvider` interface
  - [ ] `PlanningAgent` struct
- [ ] Implement `parseQuestions(response string) (PlanningResponse, error)`
  - [ ] Find JSON in response (handle markdown wrapping)
  - [ ] Unmarshal to `PlanningResponse`
  - [ ] Validate structure
- [ ] Run tests: `go test ./internal/usecase/review -v -run ParseQuestions`
- [ ] **Verify**: Tests pass

### 2.3 Verify - Question Parsing

- [ ] Run formatter: `mage format`
- [ ] Run linter: `go vet ./internal/usecase/review`
- [ ] Run all tests: `mage test`

---

## Step 3: Planning Prompt Generation (1 hour)

### 3.1 Test - Prompt Building

- [ ] Test: `TestBuildPlanningPrompt_MinimalContext`
  ```go
  func TestBuildPlanningPrompt_MinimalContext(t *testing.T) {
      ctx := ProjectContext{}
      diff := domain.Diff{Files: []domain.FileDiff{{Path: "main.go"}}}

      prompt := buildPlanningPrompt(ctx, diff)

      // Should contain context summary section
      // Should contain changes section
      // Should contain task instructions
      // Should contain JSON schema
  }
  ```
- [ ] Test: `TestBuildPlanningPrompt_FullContext`
  ```go
  func TestBuildPlanningPrompt_FullContext(t *testing.T) {
      ctx := ProjectContext{
          Architecture: "# Architecture...",
          README: "# README...",
          ChangeTypes: []string{"auth", "database"},
          ChangedPaths: []string{"auth.go", "db.go"},
      }

      // Should include all context elements
      // Should list change types
      // Should list changed paths
  }
  ```
- [ ] Run tests: `go test ./internal/usecase/review -v -run BuildPlanning`
- [ ] **Verify**: Tests fail

### 3.2 Implement - Prompt Building

- [ ] Implement `buildPlanningPrompt(ctx ProjectContext, diff domain.Diff) string`
  - [ ] Add context summary section
  - [ ] Add change detection section
  - [ ] Add diff preview (truncated to 1000 chars)
  - [ ] Add task instructions
  - [ ] Add JSON response schema
- [ ] Run tests: `go test ./internal/usecase/review -v -run BuildPlanning`
- [ ] **Verify**: Tests pass

### 3.3 Verify - Prompt Building

- [ ] Manual inspection of generated prompts
- [ ] Run formatter: `mage format`
- [ ] Run all tests: `mage test`

---

## Step 4: Question Presentation (1.5 hours)

### 4.1 Test - Question Presentation

- [ ] Create mock IO readers/writers for testing
- [ ] Test: `TestPresentQuestions_MultipleChoice`
  ```go
  func TestPresentQuestions_MultipleChoice(t *testing.T) {
      input := strings.NewReader("All users\n")
      output := &bytes.Buffer{}

      agent := &PlanningAgent{input: input, output: output}
      questions := []Question{{
          ID: 1,
          Type: "multiple_choice",
          Text: "Who is this for?",
          Options: []string{"Admin", "All users", "Public"},
      }}

      answers, err := agent.presentQuestions(questions)

      // Should not error
      // Should collect answer
      // Should format output correctly
  }
  ```
- [ ] Test: `TestPresentQuestions_YesNo`
- [ ] Test: `TestPresentQuestions_Text`
- [ ] Test: `TestPresentQuestions_Multiple`
  ```go
  func TestPresentQuestions_Multiple(t *testing.T) {
      // Test with 3-5 questions of different types
  }
  ```
- [ ] Run tests: `go test ./internal/usecase/review -v -run PresentQuestions`
- [ ] **Verify**: Tests fail

### 4.2 Implement - Question Presentation

- [ ] Implement `presentQuestions(questions []Question) (map[int]string, error)`
  - [ ] Print header
  - [ ] Loop through questions
  - [ ] Handle multiple_choice type
  - [ ] Handle yes_no type
  - [ ] Handle text type
  - [ ] Collect answers in map
- [ ] Implement `formatOption(index int) string` helper
- [ ] Run tests: `go test ./internal/usecase/review -v -run PresentQuestions`
- [ ] **Verify**: Tests pass

### 4.3 Verify - Question Presentation

- [ ] Manual test with real terminal input
- [ ] Test with different question types
- [ ] Run formatter: `mage format`
- [ ] Run all tests: `mage test`

---

## Step 5: Answer Incorporation (45 minutes)

### 5.1 Test - Answer Incorporation

- [ ] Test: `TestIncorporateAnswers_EmptyContext`
  ```go
  func TestIncorporateAnswers_EmptyContext(t *testing.T) {
      ctx := ProjectContext{}
      questions := []Question{{ID: 1, Text: "Security focus?"}}
      answers := map[int]string{1: "SQL injection"}

      result := incorporateAnswers(ctx, questions, answers)

      // Should add custom instructions
      // Should format Q&A in instructions
  }
  ```
- [ ] Test: `TestIncorporateAnswers_ExistingInstructions`
  ```go
  func TestIncorporateAnswers_ExistingInstructions(t *testing.T) {
      ctx := ProjectContext{
          CustomInstructions: "Focus on performance",
      }

      // Should append to existing instructions
      // Should preserve original instructions
  }
  ```
- [ ] Test: `TestIncorporateAnswers_MultipleAnswers`
- [ ] Run tests: `go test ./internal/usecase/review -v -run IncorporateAnswers`
- [ ] **Verify**: Tests fail

### 5.2 Implement - Answer Incorporation

- [ ] Implement `incorporateAnswers(ctx ProjectContext, questions []Question, answers map[int]string) PlanningResult`
  - [ ] Copy original context
  - [ ] Build enhanced instructions from Q&A
  - [ ] Append to existing instructions if present
  - [ ] Return PlanningResult
- [ ] Run tests: `go test ./internal/usecase/review -v -run IncorporateAnswers`
- [ ] **Verify**: Tests pass

### 5.3 Verify - Answer Incorporation

- [ ] Check instruction formatting is clear
- [ ] Run formatter: `mage format`
- [ ] Run all tests: `mage test`

---

## Step 6: Planning Agent Integration (2 hours)

### 6.1 Test - Full Planning Flow

- [ ] Create mock planning provider
  ```go
  type mockPlanningProvider struct {
      response string
      err      error
  }

  func (m *mockPlanningProvider) Review(ctx context.Context, req ProviderRequest) (domain.Review, error) {
      if m.err != nil {
          return domain.Review{}, m.err
      }
      return domain.Review{Summary: m.response}, nil
  }
  ```
- [ ] Test: `TestPlan_SuccessfulFlow`
  ```go
  func TestPlan_SuccessfulFlow(t *testing.T) {
      mockProvider := &mockPlanningProvider{
          response: `{"questions": [...], "reasoning": "..."}`,
      }

      input := strings.NewReader("Answer 1\nAnswer 2\n")
      output := &bytes.Buffer{}

      agent := &PlanningAgent{
          provider: mockProvider,
          config: PlanningConfig{MaxQuestions: 5},
          input: input,
          output: output,
      }

      result, err := agent.Plan(ctx, projectCtx, diff)

      // Should not error
      // Should have enhanced context
      // Should have collected answers
  }
  ```
- [ ] Test: `TestPlan_LLMFailure`
  ```go
  func TestPlan_LLMFailure(t *testing.T) {
      // Mock provider returns error
      // Should return original context (graceful degradation)
  }
  ```
- [ ] Test: `TestPlan_Timeout`
  ```go
  func TestPlan_Timeout(t *testing.T) {
      ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
      defer cancel()

      // Should handle context cancellation
  }
  ```
- [ ] Test: `TestPlan_MaxQuestionsLimit`
  ```go
  func TestPlan_MaxQuestionsLimit(t *testing.T) {
      // LLM returns 10 questions
      // Config maxQuestions = 5
      // Should only present 5 questions
  }
  ```
- [ ] Run tests: `go test ./internal/usecase/review -v -run "^TestPlan"`
- [ ] **Verify**: Tests fail

### 6.2 Implement - Planning Agent

- [ ] Implement `Plan(ctx context.Context, projectCtx ProjectContext, diff domain.Diff) (PlanningResult, error)`
  - [ ] Build planning prompt
  - [ ] Call planning LLM with timeout
  - [ ] Parse questions
  - [ ] Limit to maxQuestions
  - [ ] Present questions to user
  - [ ] Incorporate answers
  - [ ] Handle all errors gracefully
- [ ] Implement constructor `NewPlanningAgent(provider PlanningProvider, config PlanningConfig, input io.Reader, output io.Writer) *PlanningAgent`
- [ ] Run tests: `go test ./internal/usecase/review -v -run "^TestPlan"`
- [ ] **Verify**: Tests pass

### 6.3 Verify - Planning Agent

- [ ] Run formatter: `mage format`
- [ ] Run all review tests: `go test ./internal/usecase/review -v`
- [ ] Check test coverage: `go test ./internal/usecase/review -cover`
- [ ] **Target**: 80%+ coverage on planner.go

---

## Step 7: Configuration (1 hour)

### 7.1 Test - Configuration

- [ ] Update `internal/config/config_test.go`
- [ ] Test: `TestPlanningConfig_Defaults`
  ```go
  func TestPlanningConfig_Defaults(t *testing.T) {
      cfg := LoadConfig("testdata/minimal.yaml")

      // Planning should have sensible defaults
      assert.Equal(t, 5, cfg.Planning.MaxQuestions)
      assert.Equal(t, "30s", cfg.Planning.Timeout)
  }
  ```
- [ ] Test: `TestPlanningConfig_FromYAML`
  ```go
  func TestPlanningConfig_FromYAML(t *testing.T) {
      yaml := `
      planning:
        enabled: true
        provider: "openai"
        model: "gpt-4o-mini"
        maxQuestions: 3
        timeout: "20s"
      `

      // Should parse correctly
  }
  ```
- [ ] Test: `TestPlanningConfig_EnvVarExpansion`
- [ ] Run tests: `go test ./internal/config -v -run Planning`
- [ ] **Verify**: Tests fail

### 7.2 Implement - Configuration

- [ ] Update `internal/config/config.go`
  - [ ] Add `Planning PlanningConfig` to `Config` struct
  - [ ] Add `PlanningConfig` struct with all fields
  - [ ] Add default values in `setDefaults()`
- [ ] Update testdata with planning config examples
- [ ] Run tests: `go test ./internal/config -v -run Planning`
- [ ] **Verify**: Tests pass

### 7.3 Verify - Configuration

- [ ] Test with real bop.yaml file
- [ ] Test environment variable overrides
- [ ] Run formatter: `mage format`
- [ ] Run all config tests: `go test ./internal/config -v`

---

## Step 8: Orchestrator Integration (2 hours)

### 8.1 Test - Orchestrator with Planning

- [ ] Update `internal/usecase/review/orchestrator_test.go`
- [ ] Test: `TestReviewBranch_WithPlanning`
  ```go
  func TestReviewBranch_WithPlanning(t *testing.T) {
      // Mock planning agent
      // Interactive = true
      // NoPlanning = false
      // IsInteractive() = true

      // Should call planning agent
      // Should use enhanced context
  }
  ```
- [ ] Test: `TestReviewBranch_InteractiveNoPlanningFlag`
  ```go
  func TestReviewBranch_InteractiveNoPlanningFlag(t *testing.T) {
      // Interactive = true
      // NoPlanning = true

      // Should skip planning
  }
  ```
- [ ] Test: `TestReviewBranch_NotTTY`
  ```go
  func TestReviewBranch_NotTTY(t *testing.T) {
      // Interactive = true
      // But IsInteractive() = false (CI environment)

      // Should skip planning automatically
  }
  ```
- [ ] Test: `TestReviewBranch_PlanOnly`
  ```go
  func TestReviewBranch_PlanOnly(t *testing.T) {
      // PlanOnly = true

      // Should gather context
      // Should run planning
      // Should NOT execute review
  }
  ```
- [ ] Run tests: `go test ./internal/usecase/review -v -run ReviewBranch.*Planning`
- [ ] **Verify**: Tests fail

### 8.2 Implement - Orchestrator Integration

- [ ] Update `internal/usecase/review/orchestrator.go`
  - [ ] Add `Planner *PlanningAgent` to `OrchestratorDeps` (optional)
  - [ ] Add `Interactive bool`, `NoPlanning bool`, `PlanOnly bool` to `BranchRequest`
  - [ ] In `ReviewBranch()`, after context gathering:
    ```go
    // Planning phase (if interactive and TTY)
    if req.Interactive && !req.NoPlanning && IsInteractive() {
        if o.deps.Planner != nil {
            planningCtx, cancel := context.WithTimeout(ctx, planningTimeout)
            defer cancel()

            planResult, err := o.deps.Planner.Plan(planningCtx, projectContext, diff)
            if err != nil {
                // Log warning, continue with original context
            } else {
                projectContext = planResult.EnhancedContext
            }
        }
    }

    // Plan-only mode
    if req.PlanOnly {
        printContextSummary(projectContext)
        return Result{}, nil
    }
    ```
- [ ] Implement `printContextSummary(ctx ProjectContext)` helper
- [ ] Run tests: `go test ./internal/usecase/review -v -run ReviewBranch.*Planning`
- [ ] **Verify**: Tests pass

### 8.3 Verify - Orchestrator Integration

- [ ] Run all orchestrator tests: `go test ./internal/usecase/review -v -run Orchestrator`
- [ ] Run formatter: `mage format`
- [ ] Run all tests: `mage test`

---

## Step 9: CLI Integration (1.5 hours)

### 9.1 Test - CLI Flags

- [ ] Update `internal/adapter/cli/root_test.go`
- [ ] Test: `TestBranchCommand_InteractiveFlag`
  ```go
  func TestBranchCommand_InteractiveFlag(t *testing.T) {
      // --interactive flag should set Interactive=true
  }
  ```
- [ ] Test: `TestBranchCommand_NoPlanningFlag`
- [ ] Test: `TestBranchCommand_PlanOnlyFlag`
- [ ] Run tests: `go test ./internal/adapter/cli -v`
- [ ] **Verify**: Tests still pass (flags already exist, just need wiring)

### 9.2 Implement - CLI Flags

- [ ] Update `internal/adapter/cli/root.go`
  - [ ] Un-hide `--interactive`, `--no-planning`, `--plan-only` flags
  - [ ] Wire flags to `BranchRequest`:
    ```go
    _, err := branchReviewer.ReviewBranch(ctx, review.BranchRequest{
        // ... existing fields ...
        Interactive: interactive,
        NoPlanning:  noPlanning,
        PlanOnly:    planOnly,
    })
    ```
- [ ] Run tests: `go test ./internal/adapter/cli -v`
- [ ] **Verify**: Tests pass

### 9.3 Implement - Main.go Wiring

- [ ] Update `cmd/bop/main.go`
  - [ ] Check if planning is configured
  - [ ] If enabled, create planning provider:
    ```go
    var planningAgent *review.PlanningAgent
    if cfg.Planning.Enabled {
        if planProvider, ok := providers[cfg.Planning.Provider]; ok {
            planningAgent = review.NewPlanningAgent(
                planProvider,
                review.PlanningConfig{
                    MaxQuestions: cfg.Planning.MaxQuestions,
                    Timeout: parseTimeout(cfg.Planning.Timeout),
                },
                os.Stdin,
                os.Stdout,
            )
        }
    }
    ```
  - [ ] Add to orchestrator deps:
    ```go
    orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
        // ... existing fields ...
        Planner: planningAgent,
    })
    ```
- [ ] Run: `go build ./cmd/bop`
- [ ] **Verify**: Builds successfully

### 9.4 Verify - CLI Integration

- [ ] Test help output: `./bop review branch --help`
- [ ] Verify `--interactive`, `--no-planning`, `--plan-only` appear
- [ ] Run formatter: `mage format`
- [ ] Run all tests: `mage test`
- [ ] Run build: `mage build`

---

## Step 10: Manual Testing (1 hour)

### 10.1 Interactive Mode Testing

- [ ] Test with minimal config (planning disabled)
  ```bash
  bop review branch main --interactive
  # Should work without planning (graceful degradation)
  ```

- [ ] Test with planning enabled
  ```yaml
  # bop.yaml
  planning:
    enabled: true
    provider: "openai"
    model: "gpt-4o-mini"
  ```
  ```bash
  bop review branch main --interactive
  # Should present planning questions
  # Should collect answers
  # Should enhance context
  ```

- [ ] Test skipping planning
  ```bash
  bop review branch main --interactive --no-planning
  # Should skip planning phase
  ```

- [ ] Test plan-only mode
  ```bash
  bop review branch main --plan-only
  # Should show gathered context
  # Should show planning questions
  # Should NOT execute review
  ```

### 10.2 CI/CD Mode Testing

- [ ] Test in non-TTY environment
  ```bash
  echo "" | bop review branch main --interactive
  # Should automatically skip planning
  # Should log info message
  ```

- [ ] Test in CI/CD (no stdin)
  ```bash
  bop review branch main --interactive < /dev/null
  # Should skip planning gracefully
  ```

### 10.3 Error Handling Testing

- [ ] Test with invalid planning provider
  ```yaml
  planning:
    enabled: true
    provider: "invalid"
  ```
  ```bash
  bop review branch main --interactive
  # Should skip planning with warning
  # Should continue with review
  ```

- [ ] Test with planning timeout
  ```yaml
  planning:
    timeout: "1ms"
  ```
  ```bash
  bop review branch main --interactive
  # Should timeout gracefully
  # Should log warning
  # Should continue with original context
  ```

---

## Step 11: Documentation (2 hours)

### 11.1 Update Design Documents

- [ ] Update `docs/ENHANCED_PROMPTING_DESIGN.md`
  - [ ] Mark Phase 4 as "Complete"
  - [ ] Add implementation notes
  - [ ] Update diagrams if needed

- [ ] Update `docs/ENHANCED_PROMPTING_CHECKLIST.md`
  - [ ] Mark all Phase 4 tasks as complete
  - [ ] Update test counts
  - [ ] Update progress summary

- [ ] Update `ROADMAP.md`
  - [ ] Move Phase 4 to "Completed" section
  - [ ] Update v0.2.0 status
  - [ ] Add release notes template

### 11.2 Update User Documentation

- [ ] Update `docs/CONFIGURATION.md`
  - [ ] Add "Planning Agent" section
  - [ ] Document all planning config fields
  - [ ] Add configuration examples
  - [ ] Add environment variable examples

- [ ] Create or update `docs/USAGE.md`
  - [ ] Add "Interactive Mode" section
  - [ ] Document `--interactive` flag
  - [ ] Document `--no-planning` flag
  - [ ] Document `--plan-only` flag
  - [ ] Add workflow examples
  - [ ] Add screenshots/examples of planning questions

### 11.3 Update Code Documentation

- [ ] Add package-level documentation to `planner.go`
- [ ] Add godoc comments to all exported types
- [ ] Add godoc comments to all exported functions
- [ ] Add usage examples in comments

---

## Step 12: Final Verification (1 hour)

### 12.1 Test Suite

- [ ] Run all tests: `mage test`
  - [ ] Verify all tests pass
  - [ ] Check for data races: `go test -race ./...`
  - [ ] **Target**: 0 failures, 0 data races

- [ ] Run test coverage: `go test ./internal/usecase/review -cover`
  - [ ] **Target**: 80%+ coverage on planner.go
  - [ ] **Target**: 85%+ coverage on orchestrator.go

- [ ] Run integration tests: `go test ./... -tags=integration`
  - [ ] Verify end-to-end flow works

### 12.2 Code Quality

- [ ] Run formatter: `mage format`
- [ ] Run linter: `mage lint` (if available) or `go vet ./...`
- [ ] Check for unused imports: `goimports -l .`
- [ ] Check for code smells: `golangci-lint run` (if available)

### 12.3 Build and Run

- [ ] Build application: `mage build`
- [ ] Run help: `./bop review branch --help`
  - [ ] Verify all flags appear correctly
  - [ ] Verify no hidden flags are shown
  - [ ] Verify descriptions are clear

- [ ] Run with planning: `./bop review branch main --interactive`
  - [ ] Verify questions are relevant
  - [ ] Verify answers enhance context
  - [ ] Verify review quality improves

- [ ] Check cost: Verify planning adds < $0.001 per review

---

## Step 13: Commit and Tag (30 minutes)

### 13.1 Pre-Commit Checks

- [ ] Run formatter: `mage format`
- [ ] Run all tests: `mage test`
- [ ] Run build: `mage build`
- [ ] Review all changes: `git status`, `git diff`

### 13.2 Commit

- [ ] Stage all changes: `git add -A`
- [ ] Create commit:
  ```bash
  git commit -m "Implement Phase 4: Planning Agent (Interactive Mode)

  - Add TTY detection for interactive mode
  - Implement planning agent with LLM-powered questions
  - Wire --interactive, --no-planning, --plan-only flags
  - Add planning configuration
  - 15+ new tests for planning functionality
  - Update documentation

  Phase 4 provides optional interactive workflow where an LLM
  analyzes context and asks clarifying questions before review.
  Planning adds ~$0.001 per review and improves context quality.
  "
  ```

### 13.3 Tag and Push

- [ ] Create tag: `git tag -a v0.2.0 -m "v0.2.0 - Interactive Mode with Planning Agent"`
- [ ] Push: `git push && git push --tags`
- [ ] Verify on GitHub

---

## Success Criteria

### Functional Requirements

- [x] Planning agent asks 3-5 relevant questions
- [x] Questions are based on gathered context and changes
- [x] User can provide answers via CLI
- [x] Answers enhance custom instructions
- [x] Planning only runs in interactive mode with TTY
- [x] Planning can be skipped with `--no-planning`
- [x] Plan-only mode shows gathered context
- [x] Planning fails gracefully if LLM call fails
- [x] Cost is < $0.001 per planning call

### Non-Functional Requirements

- [x] All tests pass (195+ tests → 210+ tests)
- [x] Zero data races
- [x] Test coverage > 80% on new code
- [x] Code formatted with `mage format`
- [x] No linter warnings
- [x] Documentation complete and accurate
- [x] Backward compatible (planning is optional)

### Quality Metrics

- [x] Planning completes in < 5 seconds
- [x] Questions are relevant to changes (manual verification)
- [x] Planning improves review quality (manual verification)
- [x] User experience is smooth (manual verification)

---

## Estimated Time

| Step | Estimated Time |
|---|---|
| Pre-Implementation Setup | 15 min |
| Step 1: TTY Detection | 30 min |
| Step 2: Planning Types | 1 hour |
| Step 3: Prompt Generation | 1 hour |
| Step 4: Question Presentation | 1.5 hours |
| Step 5: Answer Incorporation | 45 min |
| Step 6: Planning Agent | 2 hours |
| Step 7: Configuration | 1 hour |
| Step 8: Orchestrator Integration | 2 hours |
| Step 9: CLI Integration | 1.5 hours |
| Step 10: Manual Testing | 1 hour |
| Step 11: Documentation | 2 hours |
| Step 12: Final Verification | 1 hour |
| Step 13: Commit and Tag | 30 min |
| **Total** | **~16 hours** |

**Recommended approach**: Spread over 2-3 days, completing 5-6 hours per day.

---

## Notes

- TDD methodology: Always write test first, see it fail, then implement
- Run tests frequently during development
- Commit after each major step (not just at the end)
- Keep PRs focused: Phase 4 only, no unrelated changes
- Manual testing is critical for interactive features
- Get user feedback early (dogfood the tool yourself!)

---

## Next Steps After Completion

1. **Phase 5 Completion**: End-to-end testing with all features
2. **Performance Optimization**: Cache ProjectContext
3. **Documentation**: User guide with examples
4. **Feedback Collection**: Gather user feedback on planning questions
5. **v0.2.0 Release**: Tag and announce release
