# Enhanced Prompting and Context System - Design Document

**Status**: Draft
**Version**: 1.0
**Date**: 2025-10-26
**Related**: ARCHITECTURE.md, TECHNICAL_DESIGN_SPEC.md

## 1. Overview

This document describes enhancements to the code review system's prompting and context gathering capabilities. The goal is to provide richer context to LLMs for higher-quality code reviews while maintaining compatibility with CI/CD pipelines.

### 1.1 Problems with Current System

1. **Minimal Context**: Prompts contain only raw diffs with no project context
2. **Generic Prompting**: All providers receive identical generic instructions
3. **No User Customization**: Cannot provide additional instructions at invocation time
4. **Poor Merge Quality**: Merge service only deduplicates, doesn't synthesize
5. **Unused Infrastructure**: Precision priors stored in database but never used

### 1.2 Goals

- **Rich Context**: Include architecture, design docs, and relevant documentation
- **Provider-Specific Prompts**: Leverage each LLM's strengths with tailored instructions
- **User Control**: Allow custom instructions and context at invocation time
- **Intelligent Merging**: Synthesize findings using precision priors and agreement scoring
- **CI/CD Compatible**: All enhancements work in automated pipelines
- **Interactive Mode**: Optional planning agent for human-in-the-loop scenarios

## 2. Architecture

### 2.1 Three-Tier Context System

```
┌─────────────────────────────────────────────────────────┐
│ Tier 1: Smart Context Gathering (Always, Rule-Based)   │
│                                                         │
│  - Analyzes changed file paths                         │
│  - Detects change types (auth, database, api, etc.)    │
│  - Loads relevant documentation automatically          │
│  - Finds related test files                            │
│  - Loads user-provided context files                   │
│  - No LLM calls (fast, free, deterministic)            │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ Tier 2: Planning Agent (Optional, Interactive Only)    │
│                                                         │
│  - Reviews gathered context                            │
│  - Identifies potential gaps                           │
│  - Asks clarifying questions to user                   │
│  - Can trigger additional context loading              │
│  - Skipped in CI/CD or with --no-planning flag        │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ Tier 3: Enhanced Prompt Building (Always)              │
│                                                         │
│  - Selects provider-specific template                  │
│  - Renders template with context                       │
│  - Includes custom instructions                        │
│  - Formats appropriately for each LLM                  │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ Tier 4: Review Orchestration (Existing, Enhanced)      │
│                                                         │
│  - Parallel LLM execution with enhanced prompts        │
│  - Cost tracking and observability                     │
│  - Store persistence                                   │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ Tier 5: Intelligent Merge (New)                        │
│                                                         │
│  - Loads precision priors from database                │
│  - Groups similar findings                             │
│  - Ranks by agreement, severity, precision             │
│  - Synthesizes summaries from all providers            │
│  - Resolves conflicts using precision scores           │
└─────────────────────────────────────────────────────────┘
```

### 2.2 Component Responsibilities

#### 2.2.1 ContextGatherer (`internal/usecase/review/context.go`)

**Purpose**: Rule-based context collection (no LLM calls)

**Inputs**:
- `domain.Diff`: Changed files and their diffs
- `ContextConfig`: Configuration for what to gather

**Outputs**:
- `ProjectContext`: Structured context including docs, tests, custom instructions

**Operations**:
1. Extract changed file paths from diff
2. Detect change types (auth, database, api, security, config, testing, docs, frontend)
3. Load core documentation (ARCHITECTURE.md, README.md)
4. Load design documents matching pattern (e.g., `docs/*_DESIGN.md`)
5. Find relevant docs based on change types (map-based lookup)
6. Find related test files (by naming convention)
7. Load user-provided context files
8. Capture custom instructions

**Design Decisions**:
- Pure Go, no LLM calls for speed and reliability
- Map-based change type detection (keywords in file paths)
- Graceful degradation (missing docs don't fail the review)
- Smart defaults with full configurability

#### 2.2.2 PlanningAgent (`internal/usecase/review/planner.go`)

**Purpose**: Interactive planning with user feedback (optional)

**Inputs**:
- `ProjectContext`: Already-gathered context
- `domain.Diff`: The code changes
- `io.Reader`, `io.Writer`: For user interaction

**Outputs**:
- `PlanningResult`: Approved context + additional context + custom instructions

**Operations**:
1. Send context summary + diff summary to planning LLM
2. Planning LLM identifies:
   - Missing context (e.g., "Are there security guidelines?")
   - Clarifying questions (e.g., "Is this a breaking change?")
   - Additional files to review (e.g., "Should I review the migration files?")
3. Present questions to user via CLI
4. Incorporate user responses into final context
5. Return enhanced context for review

**Design Decisions**:
- Uses small, fast model (e.g., gpt-4o-mini, claude-3-5-haiku)
- Only runs in `--interactive` mode
- Can be skipped with `--no-planning`
- Not available in CI/CD (detected by checking if stdin is a TTY)
- Cost is minimal (one LLM call with small context)

#### 2.2.3 PromptBuilder (`internal/usecase/review/prompt_builder.go`)

**Purpose**: Render provider-specific prompts with context

**Inputs**:
- `ProjectContext`: Gathered context
- `domain.Diff`: Code changes
- `PromptTemplate`: Provider-specific template
- `providerName`: Which provider this prompt is for

**Outputs**:
- `ProviderRequest`: Rendered prompt ready for LLM

**Operations**:
1. Select provider-specific template from configuration
2. Render template with all context (architecture, docs, instructions, diff)
3. Apply token budget limits (truncate context if needed)
4. Format appropriately for provider (system message vs user message)

**Template Variables**:
```go
type TemplateData struct {
    Architecture       string
    README             string
    DesignDocs         string   // Concatenated
    RelevantDocs       string   // Concatenated
    RelatedTests       string   // Concatenated
    CustomInstructions string
    CustomContext      string   // User-provided files
    ChangeTypes        []string
    ChangedPaths       []string
    Diff               string   // Formatted diffs
    BaseRef            string
    TargetRef          string
}
```

**Design Decisions**:
- Go's `text/template` for rendering
- Provider-specific templates in configuration (overridable)
- Fallback to default template if provider template not found
- Token budgeting: truncate least-important context first (order: custom files, tests, design docs, relevant docs, architecture, README, instructions, diff)
- Different template styles for different providers (e.g., XML tags for Claude)

#### 2.2.4 IntelligentMerger (`internal/usecase/merge/intelligent_merger.go`)

**Purpose**: Synthesize findings from multiple providers

**Inputs**:
- `[]domain.Review`: Individual provider reviews
- `Store`: Access to precision priors

**Outputs**:
- `domain.Review`: Merged and synthesized review

**Operations**:
1. Load precision priors from database: `map[provider][category]Prior`
2. Group findings by similarity:
   - Exact match: Same `finding.ID` (already implemented)
   - Similar match: Same file + overlapping line range + similar description (new)
3. Score each finding group:
   ```
   score = (agreement_weight * num_providers_found_it) +
           (severity_weight * avg_severity_score) +
           (precision_weight * max_precision_prior) +
           (evidence_weight * evidence_bool)
   ```
4. Rank findings by score (descending)
5. For each group, select representative finding:
   - If conflict (different suggestions), prefer highest precision provider
   - If agreement, combine suggestions
6. Synthesize summary:
   - Extract key points from each provider's summary
   - Weight by provider precision
   - Generate meta-summary describing agreement and disagreements

**Design Decisions**:
- Configurable weights in `merge.strategy` config
- Precision priors use Beta distribution: `precision = alpha / (alpha + beta)`
- Default weights: `agreement=0.4, severity=0.3, precision=0.2, evidence=0.1`
- Similarity threshold for grouping: 0.7 (Levenshtein distance on descriptions)
- Summary synthesis: Concatenate weighted summaries (for now, later could use LLM)

## 3. Configuration Schema

### 3.1 Context Configuration

```yaml
# New top-level config section
context:
  # Core documentation
  architecture:
    enabled: true
    path: "docs/ARCHITECTURE.md"

  readme:
    enabled: true
    path: "README.md"

  designDocs:
    enabled: true
    glob: "docs/*_DESIGN.md"

  # Smart gathering
  autoDetectRelevantDocs: true
  autoDetectRelatedTests: true

  # Documentation mapping (change type -> doc files)
  docMapping:
    auth:
      - "docs/SECURITY.md"
      - "docs/AUTHENTICATION.md"
    database:
      - "docs/DATABASE.md"
      - "docs/STORE_INTEGRATION_DESIGN.md"
    api:
      - "docs/API.md"
      - "docs/HTTP_CLIENT_DESIGN.md"

  # Token budgeting
  maxContextTokens: 50000  # Reserve tokens for context
  truncationOrder:         # What to remove first if over budget
    - "customFiles"
    - "relatedTests"
    - "designDocs"
    - "relevantDocs"
    - "architecture"
    - "readme"
    - "customInstructions"
    # diff and instructions never truncated
```

### 3.2 Planning Agent Configuration

```yaml
# New top-level config section
planning:
  enabled: false  # Only available with --interactive flag
  provider: "openai"
  model: "gpt-4o-mini"  # Fast, cheap model for planning
  maxQuestions: 5       # Limit questions to avoid fatigue
  timeout: "30s"        # Max time for planning phase
```

### 3.3 Provider-Specific Templates

```yaml
providers:
  openai:
    enabled: true
    model: "gpt-4o-mini"
    apiKey: "${OPENAI_API_KEY}"

    # New: Provider-specific prompt template
    promptTemplate: |
      You are an expert code reviewer specializing in finding bugs, security vulnerabilities, and performance issues.

      {{if .Architecture}}
      ## Project Architecture
      {{.Architecture}}
      {{end}}

      {{if .CustomInstructions}}
      ## Review Focus
      {{.CustomInstructions}}
      {{end}}

      ## Changes to Review
      Change types: {{join .ChangeTypes ", "}}
      Files modified: {{len .ChangedPaths}}

      {{.Diff}}

      Provide detailed findings in JSON format with severity, category, file, line numbers, description, and actionable suggestions.

  anthropic:
    enabled: true
    model: "claude-3-5-sonnet-20241022"
    apiKey: "${ANTHROPIC_API_KEY}"

    # Claude performs better with XML-style structure
    promptTemplate: |
      <role>
      You are a senior software architect reviewing code for design quality, maintainability, and adherence to clean architecture principles.
      </role>

      <architecture>
      {{.Architecture}}
      </architecture>

      {{if .CustomInstructions}}
      <instructions>
      {{.CustomInstructions}}
      </instructions>
      {{end}}

      <changes>
      Change types: {{join .ChangeTypes ", "}}
      {{.Diff}}
      </changes>

      Analyze the code changes and provide structured feedback in JSON format.
```

### 3.4 Merge Strategy Configuration

```yaml
merge:
  enabled: true
  strategy: "intelligent"  # New: "simple", "intelligent", "consensus"

  # Weights for intelligent merging
  weights:
    agreement: 0.4    # How many providers found this issue
    severity: 0.3     # How severe is the issue
    precision: 0.2    # Historical precision of provider
    evidence: 0.1     # Did provider cite specific code

  # Similarity threshold for grouping findings
  similarityThreshold: 0.7  # 0.0-1.0, higher = stricter matching

  # Summary synthesis
  synthesizeSummary: true   # Generate meta-summary vs concatenate

  # LLM-based synthesis (when synthesizeSummary: true and useLLM: true)
  useLLM: true              # Use LLM for summary synthesis vs simple concatenation
  provider: "openai"        # Which provider to use for synthesis
  model: "gpt-4o-mini"      # Fast, cheap model for meta-operations
```

### 3.5 Meta-Operation Model Selection

**Problem**: Some operations (summary synthesis, planning agent, context analysis) require LLM calls but aren't primary code reviews. We need to control which models are used for these "meta" operations separately from review providers.

**Design Principles**:
1. **Separate Configuration**: Meta-operations should use separate provider/model config
2. **Cost Optimization**: Default to cheap, fast models (gpt-4o-mini, claude-3-5-haiku)
3. **Reuse Infrastructure**: Use existing provider abstraction and clients
4. **Fail Gracefully**: If meta-operation LLM call fails, fall back to rule-based approach

**Configuration Strategy**:
```yaml
# Planning agent (future Phase 4)
planning:
  enabled: false
  provider: "openai"
  model: "gpt-4o-mini"      # Fast, cheap model for planning
  maxQuestions: 5
  timeout: "30s"

# Merge synthesis (Phase 3.5 - LLM-based)
merge:
  enabled: true
  strategy: "intelligent"
  synthesizeSummary: true
  useLLM: true              # Enable LLM-based synthesis
  provider: "openai"        # Separate from review providers
  model: "gpt-4o-mini"      # ~$0.0001-0.0005 per synthesis

# Future: Context analysis
context:
  intelligentPruning: false
  provider: "openai"
  model: "gpt-4o-mini"
```

**Model Selection Guidelines**:
- **Summary Synthesis**: gpt-4o-mini (cheap, good at summarization)
- **Planning Agent**: gpt-4o-mini or claude-3-5-haiku (conversational, fast)
- **Context Pruning**: gpt-4o-mini (cheap, can be run multiple times)
- **Relevance Scoring**: claude-3-5-haiku (fast, good at classification)

**Cost Impact**:
- Summary synthesis: ~500 tokens in, ~200 tokens out = $0.0003 per review
- Planning agent: ~2000 tokens in, ~500 tokens out = $0.001 per review
- Total meta-operation cost: < $0.002 per review (negligible vs review cost)

**Rationale**:
- Review providers (gpt-4o, claude-3-5-sonnet, gemini-1.5-pro) are for deep code analysis
- Meta-operations need speed and low cost, not maximum intelligence
- Separate config allows users to optimize cost/quality trade-offs independently

## 4. CLI Design

### 4.1 New Flags

```bash
# Custom instructions (works in all modes)
cr review branch main --instructions "Focus on security in the auth module"

# Additional context files (multiple allowed)
cr review branch main --context ./SECURITY_GUIDELINES.md --context ./team-conventions.txt

# Interactive mode with planning (requires TTY)
cr review branch main --interactive

# Skip planning even in interactive mode
cr review branch main --interactive --no-planning

# Planning-only mode (dry-run, shows what would be gathered)
cr review branch main --plan-only

# Override context config
cr review branch main --no-architecture  # Skip architecture doc
cr review branch main --no-auto-context  # Disable smart doc detection
```

### 4.2 Interactive Mode Flow

```
$ bop review branch main --interactive

⏳ Gathering context...
✓ Found: ARCHITECTURE.md, README.md, 3 design docs
✓ Detected change types: auth, database
✓ Found relevant docs: SECURITY.md, STORE_INTEGRATION_DESIGN.md
✓ Found 2 related test files

📋 Planning review...

The planner has identified some questions:

1. Are these auth changes intended for a specific user role?
   [ ] Yes, admin only
   [ ] Yes, all users
   [x] No specific role

2. Should I review the database migration files in db/migrations/?
   [x] Yes
   [ ] No

3. Any specific security concerns to focus on?
   [Enter]: SQL injection in the new query builder

⏳ Reviewing code with 3 providers...
   ✓ OpenAI (gpt-4o-mini): 12 findings ($0.0023)
   ✓ Anthropic (claude-3-5-sonnet): 15 findings ($0.0156)
   ✓ Gemini (gemini-1.5-pro): 10 findings ($0.0089)

🔄 Merging findings...
   ✓ Identified 18 unique findings
   ✓ Grouped 5 similar findings
   ✓ Ranked by agreement and severity

📄 Reviews written to ./reviews/
   - review-openai-20241026T120000Z.md
   - review-anthropic-20241026T120000Z.md
   - review-gemini-20241026T120000Z.md
   - review-merged-20241026T120000Z.md (⭐ recommended)

Total cost: $0.0268
```

## 5. Implementation Plan

### 5.1 Phase 1: Context Gathering (Week 1)

**Deliverables**:
- `internal/usecase/review/context.go`: ContextGatherer implementation
- `internal/usecase/review/context_test.go`: Comprehensive tests
- Configuration schema in `internal/config/config.go`
- Update `docs/CONFIGURATION.md`

**TDD Approach**:
1. Write tests for change type detection
2. Implement change type detection
3. Write tests for document loading
4. Implement document loading
5. Write tests for smart doc discovery
6. Implement smart doc discovery
7. Write integration tests with real repo

**Success Criteria**:
- All tests pass
- `mage format`, `mage lint`, `mage test`, `mage build` pass
- Can gather context from this project (code-reviewer itself)

### 5.2 Phase 2: Enhanced Prompt Building (Week 1)

**Deliverables**:
- `internal/usecase/review/prompt_builder.go`: Template-based prompt builder
- `internal/usecase/review/prompt_builder_test.go`: Template rendering tests
- Provider-specific default templates
- Token budgeting logic

**TDD Approach**:
1. Write tests for template rendering
2. Implement template system
3. Write tests for token budgeting
4. Implement truncation logic
5. Write tests for provider-specific formatting
6. Implement provider adapters

**Success Criteria**:
- Templates render correctly with all context
- Token budgeting truncates appropriately
- Provider-specific formatting works

### 5.3 Phase 3: Intelligent Merge (Week 1)

**Deliverables**:
- `internal/usecase/merge/intelligent_merger.go`: Intelligent merge implementation
- `internal/usecase/merge/intelligent_merger_test.go`: Merge logic tests
- Finding similarity algorithms
- Precision prior loading from store

**TDD Approach**:
1. Write tests for finding grouping
2. Implement similarity detection
3. Write tests for scoring algorithm
4. Implement weighted scoring
5. Write tests for summary synthesis
6. Implement summary generation
7. Write integration tests with store

**Success Criteria**:
- Findings group correctly
- Scoring weights work as expected
- Precision priors influence results
- Merged review is more useful than simple deduplication

### 5.3.5 Phase 3.5: LLM-Based Summary Synthesis (Quick Win)

**Status**: In Progress

**Deliverables**:
- Update `internal/usecase/merge/intelligent_merger.go`: Add LLM-based synthesis
- Update `internal/usecase/merge/intelligent_merger_test.go`: Test LLM synthesis
- Update `internal/config/config.go`: Add merge.useLLM, merge.provider, merge.model
- Update `cmd/bop/main.go`: Wire synthesis provider

**TDD Approach**:
1. Write tests for LLM synthesis prompt generation
2. Implement synthesis prompt builder
3. Write tests for synthesis with mock provider
4. Implement LLM call in synthesizeSummary
5. Write tests for graceful fallback on LLM failure
6. Implement fallback to concatenation
7. Write integration tests with real provider

**Success Criteria**:
- LLM-based synthesis produces cohesive narrative
- Falls back gracefully if LLM call fails
- Configuration controls which provider/model used
- Cost is minimal (~$0.0003 per review)
- All tests pass

**Implementation Notes**:
- Reuse existing provider abstraction (no new HTTP clients needed)
- Use small, cheap model (gpt-4o-mini by default)
- Synthesis prompt includes: all provider summaries, finding counts, key themes
- Graceful fallback: if LLM synthesis fails, use concatenation (current behavior)
- Optional: can be disabled with `merge.useLLM: false`

### 5.4 Phase 4: Planning Agent (Week 2)

**Deliverables**:
- `internal/usecase/review/planner.go`: Planning agent implementation
- `internal/usecase/review/planner_test.go`: Planning logic tests
- Interactive CLI prompts
- TTY detection

**TDD Approach**:
1. Write tests for context analysis prompt generation
2. Implement planning prompt builder
3. Write tests for question parsing
4. Implement question extraction
5. Write tests for user interaction (mock IO)
6. Implement CLI question presenter
7. Write integration tests

**Success Criteria**:
- Planning agent asks relevant questions
- User responses update context appropriately
- Works only in TTY mode
- Can be skipped with flag

### 5.5 Phase 5: CLI Integration (Week 2)

**Deliverables**:
- Updated `internal/adapter/cli/root.go`: New flags
- Updated `cmd/bop/main.go`: Wire new components
- Updated documentation

**TDD Approach**:
1. Write tests for flag parsing
2. Implement new flags
3. Write tests for component wiring
4. Update dependency injection
5. Write end-to-end tests
6. Update documentation

**Success Criteria**:
- All new flags work
- Context flows through to prompts
- Intelligent merge produces better results
- Documentation updated
- All existing tests still pass

## 6. Testing Strategy

### 6.1 Unit Tests

- **ContextGatherer**: Mock file system with testdata directory
- **PromptBuilder**: Test template rendering with known inputs
- **IntelligentMerger**: Test with synthetic review data
- **PlanningAgent**: Test with mock LLM responses

### 6.2 Integration Tests

- **End-to-End**: Review actual branch in code-reviewer repo
- **Store Integration**: Test precision prior loading and updates
- **Multi-Provider**: Test with all 4 providers in parallel

### 6.3 Test Data

Create `testdata/` directory with:
- Sample repository structure
- Sample documentation files
- Sample diffs
- Expected outputs

## 7. Backward Compatibility

### 7.1 Configuration

- All new config sections optional with sensible defaults
- If no templates provided, use simple default prompt (current behavior)
- If context gathering fails, continue with just diff (current behavior)

### 7.2 CLI

- Existing commands work unchanged
- New flags are optional
- Interactive mode requires explicit `--interactive` flag

### 7.3 API

- `OrchestratorDeps` extended with optional fields
- `PromptBuilder` signature unchanged, new builder is drop-in replacement
- `Merger` interface unchanged, intelligent merger implements same interface

## 8. Performance Considerations

### 8.1 Context Gathering

- File I/O is fast (milliseconds)
- Caching: Consider caching docs between reviews (future)
- Token counting: Use approximate counting (chars / 4) to avoid overhead

### 8.2 Planning Agent

- Single LLM call with small context (~2k tokens)
- Cost: ~$0.001 per review
- Time: 1-3 seconds
- Only runs in interactive mode

### 8.3 Intelligent Merge

- In-memory operations, no LLM calls
- O(n²) for finding similarity (acceptable for typical review sizes)
- Store query: Single query for all precision priors

## 9. Success Metrics

### 9.1 Quantitative

- **Review Quality**: User feedback on finding relevance (future TUI)
- **Context Usage**: % of reviews that include project context
- **Merge Improvement**: Merged review finding count vs simple dedupe
- **Performance**: Context gathering adds <1s overhead
- **Cost**: Planning agent adds <$0.01 per review

### 9.2 Qualitative

- **User Satisfaction**: Reviews are more actionable
- **Precision Improvement**: Fewer false positives over time
- **Developer Adoption**: % of reviews using custom instructions

## 10. Future Enhancements

### 10.1 Phase 3+ Features

- **Smart Context Caching**: Cache docs between reviews
- **LLM-Based Summary Synthesis**: Use LLM to synthesize merged summary
- **Vector Search**: Find relevant docs by semantic similarity
- **Learning from Feedback**: Update prompt templates based on feedback
- **Context Pruning**: Remove irrelevant sections from large docs
- **Multi-Language Support**: Different templates for different project languages

### 10.2 Research Questions

- Should we use embeddings for doc similarity?
- Can we learn optimal prompt templates from feedback?
- Should planning agent be available in CI with environment variables?
- Can we automate finding grouping with ML?

## 11. References

- ARCHITECTURE.md: Overall system architecture
- TECHNICAL_DESIGN_SPEC.md: Original technical design
- STORE_INTEGRATION_DESIGN.md: Precision priors and feedback
- OBSERVABILITY.md: Logging and metrics for new components
