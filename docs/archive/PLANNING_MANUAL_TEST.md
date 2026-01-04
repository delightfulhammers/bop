# Planning Agent Manual Testing Guide

This document provides step-by-step instructions for manually testing the interactive planning feature.

## Prerequisites

1. Go 1.21 or later installed
2. Valid API key for at least one LLM provider (OpenAI, Anthropic, Gemini, or Ollama)
3. A git repository with some changes to review

## Setup

### 1. Build the application

```bash
cd /Users/brandon/Development/personal/code-reviewer
mage build
```

### 2. Create configuration file

Copy the example configuration:

```bash
cp .cr-planning-example.yaml ~/.config/bop/bop.yaml
```

Edit `~/.config/bop/bop.yaml` and set your API keys:

```yaml
providers:
  openai:
    enabled: true
    apiKey: sk-... # Your actual OpenAI API key

planning:
  enabled: true
  provider: openai
  maxQuestions: 3  # Start with 3 for testing
  timeout: 60s
```

## Test Scenarios

### Test 1: Basic Interactive Planning (Happy Path)

**Purpose**: Verify planning agent asks questions and incorporates answers

**Steps**:
1. Create a test branch with some code changes:
   ```bash
   git checkout -b test-planning
   # Make some code changes
   git add .
   git commit -m "test changes"
   ```

2. Run review with interactive flag:
   ```bash
   ./bop review branch --base main --interactive
   ```

3. **Expected behavior**:
   - Planning phase starts
   - Agent asks 1-3 clarifying questions about:
     - Purpose of changes
     - Specific concerns
     - Focus areas
   - Presents questions one at a time with appropriate prompts:
     - Yes/No questions show `[y/n]`
     - Multiple choice shows numbered options
     - Text questions allow free-form input
   - After answering, review proceeds normally
   - Review output incorporates your answers

4. **Verification**:
   - Check that questions are clear and relevant
   - Verify input validation (e.g., typing "x" for y/n shows error)
   - Inspect generated review files in `reviews/` directory
   - Planning answers should influence the review focus

### Test 2: Non-TTY Environment (CI/CD Simulation)

**Purpose**: Verify planning is skipped when not in interactive terminal

**Steps**:
1. Run review through a pipe (simulates CI/CD):
   ```bash
   echo "" | ./bop review branch --base main --interactive
   ```

2. **Expected behavior**:
   - Planning phase is skipped (TTY detection returns false)
   - Warning logged: "Interactive mode requested but not running in TTY"
   - Review proceeds without planning
   - No interactive prompts displayed

3. **Verification**:
   - Review completes successfully
   - No blocking prompts
   - Check logs for TTY detection message

### Test 3: Planning Provider Failure

**Purpose**: Verify graceful degradation when LLM fails

**Steps**:
1. Temporarily use invalid API key in config:
   ```yaml
   providers:
     openai:
       apiKey: invalid-key
   planning:
     provider: openai
   ```

2. Run review:
   ```bash
   ./bop review branch --base main --interactive
   ```

3. **Expected behavior**:
   - Planning phase starts
   - LLM call fails with authentication error
   - Warning logged about planning failure
   - Review continues without planning
   - Review still completes successfully

4. **Verification**:
   - Review generates output files
   - Error message is user-friendly
   - No panic or crash
   - Review uses standard prompts without planning context

### Test 4: Planning Timeout

**Purpose**: Verify timeout handling

**Steps**:
1. Set very short timeout in config:
   ```yaml
   planning:
     timeout: 1s  # Very short timeout
   ```

2. Run review on large diff:
   ```bash
   ./bop review branch --base main --interactive
   ```

3. **Expected behavior**:
   - Planning phase starts
   - LLM call times out after 1 second
   - Warning logged about timeout
   - Review continues without planning

4. **Verification**:
   - Timeout occurs within expected timeframe
   - No hanging or blocking
   - Review completes normally

### Test 5: Invalid JSON Response

**Purpose**: Verify handling of malformed LLM responses

**Steps**:
1. This is difficult to test manually (requires mocking)
2. Instead, verify unit tests cover this:
   ```bash
   go test ./internal/usecase/review/... -v -run TestParseQuestions_InvalidJSON
   ```

3. **Expected tests**:
   - `TestParseQuestions_InvalidJSON`
   - `TestParseQuestions_InvalidQuestionType`
   - `TestParseQuestions_EmptyResponse`

4. **Verification**:
   - All tests pass
   - Error messages are descriptive

### Test 6: All Question Types

**Purpose**: Verify all three question types work correctly

**Steps**:
1. Run multiple reviews and observe question types:
   ```bash
   ./bop review branch --base main --interactive
   ```

2. **Expected question types**:

   **Yes/No Question**:
   ```
   Q: Should the review focus on security vulnerabilities? [y/n]:
   ```
   - Type `y` → normalized to "yes"
   - Type `n` → normalized to "no"
   - Type `invalid` → error and retry

   **Multiple Choice**:
   ```
   Q: What is your primary concern?
   1. Performance
   2. Security
   3. Maintainability
   4. Correctness
   Enter choice (1-4):
   ```
   - Type `1-4` → accepts choice
   - Type `5` → error and retry
   - Type `abc` → error and retry

   **Text Input**:
   ```
   Q: Please describe the purpose of these changes:
   ```
   - Any text input accepted
   - Empty input accepted
   - Multi-word input accepted

3. **Verification**:
   - All question types display correctly
   - Input validation works as expected
   - Answers are captured and incorporated

### Test 7: Context Cancellation (Ctrl+C)

**Purpose**: Verify graceful handling of user interruption

**Steps**:
1. Run review:
   ```bash
   ./bop review branch --base main --interactive
   ```

2. When planning questions appear, press `Ctrl+C`

3. **Expected behavior**:
   - Application exits gracefully
   - No panic or stack trace
   - Partial state cleaned up
   - Database connections closed (if store enabled)

4. **Verification**:
   - Clean exit (exit code 130 or 1)
   - No corrupted files
   - No hung processes

### Test 8: Planning Without Interactive Flag

**Purpose**: Verify planning doesn't run without --interactive

**Steps**:
1. Ensure config has planning enabled:
   ```yaml
   planning:
     enabled: true
   ```

2. Run review WITHOUT --interactive flag:
   ```bash
   ./bop review branch --base main
   ```

3. **Expected behavior**:
   - No planning phase
   - No interactive prompts
   - Review proceeds with standard prompts
   - Review completes normally

4. **Verification**:
   - No questions asked
   - Review output generated
   - Execution time faster (no planning overhead)

## Test Results Checklist

After completing all tests, verify:

- [ ] Planning runs successfully in TTY environment
- [ ] Questions are clear, relevant, and grammatically correct
- [ ] All three question types (yes/no, multiple choice, text) work
- [ ] Input validation provides helpful error messages
- [ ] Planning is skipped in non-TTY environments
- [ ] LLM failures don't block the review
- [ ] Timeouts are handled gracefully
- [ ] Planning answers are incorporated into review prompts
- [ ] Ctrl+C exits cleanly
- [ ] Planning can be disabled via config or CLI flag
- [ ] No sensitive data (API keys) in logs or error messages

## Automated Test Coverage

The following scenarios are covered by unit tests:

- TTY detection (5 tests in `tty_test.go`)
- JSON parsing with various formats (11 tests in `planner_test.go`)
- Prompt generation (7 tests)
- Question presentation (9 tests)
- Answer incorporation (6 tests)
- Full planning workflow (6 tests)

Run automated tests:

```bash
go test ./internal/usecase/review/... -v -run Plan
```

## Cost Estimation

Interactive planning adds minimal cost:

- **OpenAI gpt-4o-mini**: ~$0.001 per planning session
- **Anthropic claude-3-5-haiku**: ~$0.002 per planning session
- **Input tokens**: ~500-1500 (context summary + diff preview)
- **Output tokens**: ~100-300 (questions in JSON format)

Total overhead: **< $0.01 per review** with planning enabled.

## Troubleshooting

### Issue: "Planning phase failed" warning

**Possible causes**:
- Invalid API key
- Network connectivity issues
- LLM service outage
- Invalid provider configuration

**Solution**: Check logs for specific error, verify API key, test provider manually

### Issue: Questions not appearing

**Possible causes**:
- Not running in TTY (piped input/output)
- Planning disabled in config
- --interactive flag not provided

**Solution**: Verify TTY detection with `test -t 0 && echo "TTY" || echo "Not TTY"`, check config and CLI flags

### Issue: Review takes too long

**Possible causes**:
- Planning timeout too high
- Too many questions (maxQuestions > 5)
- Slow LLM response

**Solution**: Reduce timeout and maxQuestions in config

## Next Steps

After manual testing is complete:
1. Document any issues found
2. Update CHANGELOG.md
3. Update README.md with planning feature
4. Update configuration documentation
5. Create release notes for v0.2.0
