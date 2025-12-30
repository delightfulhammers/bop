# Technical Design Document: Reviewer Personas

**Version:** 1.0
**Date:** 2025-12-30
**Author:** Brandon Young
**Status:** Draft
**Phase:** 3.2

---

## 1. Overview

Phase 3.2 introduces **Reviewer Personas** — specialized reviewers with distinct expertise, prompts, and finding weights. This replaces the current flat provider model where all LLMs receive identical prompts.

### Goals

| Goal | Metric | Target |
|------|--------|--------|
| Improve finding relevance | Accepted/Total ratio | +30% improvement |
| Reduce noise | False positive rate | -40% reduction |
| Enable specialization | Persona coverage | 3+ built-in personas |
| Maintain compatibility | Config migration | Zero breaking changes |

### Non-Goals

- Dynamic model selection (Phase 3.3)
- Cross-repo learning (Phase 4)
- Custom persona marketplace

---

## 2. Current State Analysis

### Current Architecture

```
Config (providers)
    ↓
Orchestrator
    ↓ (parallel dispatch)
┌───────────────────────────────────┐
│  Provider A    Provider B    ...  │  ← Same prompt to all
└───────────────────────────────────┘
    ↓
Merger (provider-agnostic)
    ↓
Output
```

### Current Limitations

1. **Identical Prompts**: All providers receive the same prompt regardless of their strengths
2. **No Specialization**: Claude, GPT, and Gemini all look for everything
3. **Flat Weighting**: All findings weighted equally regardless of source expertise
4. **Category Overlap**: Multiple providers flag the same issues, adding noise
5. **No Focus Control**: Can't tell a provider to focus on security and ignore style

### Current Code Paths

| Component | File | Current Behavior |
|-----------|------|------------------|
| Config | `internal/config/config.go` | `providers` map, no personas |
| Orchestrator | `internal/usecase/review/orchestrator.go` | Dispatches same prompt to all |
| PromptBuilder | `internal/usecase/review/prompt_builder.go` | Provider-agnostic templates |
| Merger | `internal/usecase/merge/intelligent_merger.go` | Scores by agreement, not expertise |

---

## 3. Proposed Architecture

### Target Architecture

```
Config (reviewers)
    ↓
ReviewerRegistry
    ↓
Orchestrator
    ↓ (parallel dispatch with personas)
┌─────────────────────────────────────────────────────────────┐
│  Security Expert      Maintainability       Docs Checker   │
│  (claude-opus)        (claude-sonnet)       (gemini-flash) │
│  weight: 1.5          weight: 1.0           weight: 0.5    │
│  focus: security      focus: solid,dry      focus: docs    │
└─────────────────────────────────────────────────────────────┘
    ↓
Merger (role-aware, weighted)
    ↓
Output (attributed to personas)
```

### Key Changes

1. **New Config Section**: `reviewers` alongside/replacing `providers`
2. **ReviewerRegistry**: Manages reviewer definitions with defaults
3. **PersonaPromptBuilder**: Injects persona, focus, ignore into prompts
4. **Orchestrator**: Resolves reviewers, passes persona to prompt builder
5. **Merger**: Weights findings by reviewer weight, understands focus areas

---

## 4. Domain Model

### New Types

```go
// internal/domain/reviewer.go

// Reviewer represents a specialized review perspective.
type Reviewer struct {
    // Name is the unique identifier (e.g., "security-expert")
    Name string

    // Provider is the LLM provider to use (e.g., "anthropic")
    Provider string

    // Model is the specific model (e.g., "claude-opus-4")
    Model string

    // Weight affects finding priority in merge (default: 1.0)
    // Higher weight = findings ranked higher
    Weight float64

    // Persona is the system prompt describing expertise
    // Injected at the start of the review prompt
    Persona string

    // Focus lists categories this reviewer specializes in
    // Findings in these categories are emphasized
    Focus []string

    // Ignore lists categories this reviewer should skip
    // Used to prevent overlap between specialized reviewers
    Ignore []string

    // Enabled controls whether this reviewer runs (default: true)
    Enabled bool
}

// ReviewerDefaults provides default values for optional fields
var ReviewerDefaults = Reviewer{
    Weight:  1.0,
    Enabled: true,
}
```

### Finding Attribution

```go
// internal/domain/finding.go (extension)

// Finding gains reviewer attribution
type Finding struct {
    // ... existing fields ...

    // ReviewerName identifies which reviewer found this
    ReviewerName string

    // ReviewerWeight is the weight at time of finding
    ReviewerWeight float64
}
```

---

## 5. Configuration Schema

### New `reviewers` Section

```yaml
# cr.yaml

# New reviewers configuration (Phase 3.2)
reviewers:
  # Security specialist - uses strongest model, high weight
  security:
    provider: anthropic
    model: claude-opus-4
    weight: 1.5
    persona: |
      You are a security-focused code reviewer with deep expertise in:
      - OWASP Top 10 vulnerabilities
      - Authentication and authorization flaws
      - Injection attacks (SQL, command, XSS, SSTI)
      - Secrets and credential exposure
      - Cryptographic weaknesses
      - Race conditions and TOCTOU bugs

      Focus ONLY on security issues. Ignore style, performance, and
      general code quality unless they have security implications.

      When reporting issues, cite the specific CWE or OWASP category.
      Be precise about the attack vector and exploitability.
    focus:
      - security
      - authentication
      - authorization
      - injection
      - secrets
      - cryptography
    ignore:
      - style
      - naming
      - documentation
      - performance
      - formatting

  # Maintainability expert - balanced model
  maintainability:
    provider: anthropic
    model: claude-sonnet-4-5
    weight: 1.0
    persona: |
      You are a maintainability expert focused on long-term code health.
      Evaluate code for:
      - DRY violations and unnecessary duplication
      - SOLID principle violations
      - Excessive cyclomatic complexity
      - Poor naming that hurts readability
      - Missing or inadequate error handling
      - Unclear control flow

      Do NOT comment on security, performance, or documentation.
      Focus on whether future developers can easily understand and
      modify this code.
    focus:
      - maintainability
      - dry-violations
      - solid-principles
      - complexity
      - error-handling
      - readability
    ignore:
      - security
      - performance
      - documentation
      - style

  # Documentation checker - fast, cheap model
  docs:
    provider: google
    model: gemini-2.0-flash
    weight: 0.5
    enabled: true  # Can be disabled for code-only reviews
    persona: |
      Review ONLY comments, docstrings, and documentation.
      Check for:
      - Spelling and grammar errors
      - Outdated or inaccurate documentation
      - Missing documentation for public APIs
      - Misleading comments that don't match the code

      Do NOT review code logic, structure, or implementation.
      Your only concern is the quality of written documentation.
    focus:
      - documentation
      - comments
      - docstrings
    ignore:
      - security
      - bugs
      - performance
      - maintainability
      - style

# Default reviewers when --reviewers flag not specified
default_reviewers:
  - security
  - maintainability

# Legacy providers section (deprecated but still works)
# If both `providers` and `reviewers` exist, `reviewers` takes precedence
# providers:
#   openai:
#     model: gpt-4o
```

### Config Priority

1. CLI flag `--reviewers security,docs` → use specified reviewers
2. Config `default_reviewers` → use default list
3. Config `reviewers` (all enabled) → use all defined reviewers

---

## 6. Component Design

### 6.1 ReviewerRegistry

```go
// internal/usecase/review/reviewer_registry.go

// ReviewerRegistry manages reviewer definitions and resolution.
type ReviewerRegistry struct {
    reviewers map[string]domain.Reviewer
    defaults  []string
}

// NewReviewerRegistry creates a registry from config.
func NewReviewerRegistry(cfg config.ReviewersConfig) *ReviewerRegistry

// Resolve returns reviewers for a request.
// Priority: CLI flags > config defaults > all enabled
func (r *ReviewerRegistry) Resolve(requested []string) ([]domain.Reviewer, error)

// FromLegacyProviders converts old providers config to reviewers.
func (r *ReviewerRegistry) FromLegacyProviders(providers map[string]config.ProviderConfig) error

// Get returns a single reviewer by name.
func (r *ReviewerRegistry) Get(name string) (domain.Reviewer, bool)

// List returns all registered reviewers.
func (r *ReviewerRegistry) List() []domain.Reviewer

// Validate checks reviewer configurations for errors.
func (r *ReviewerRegistry) Validate() error
```

### 6.2 PersonaPromptBuilder

```go
// internal/usecase/review/persona_prompt_builder.go

// PersonaPromptBuilder generates prompts tailored to reviewer personas.
type PersonaPromptBuilder struct {
    base     *EnhancedPromptBuilder
    registry *ReviewerRegistry
}

// Build creates a prompt for a specific reviewer.
func (b *PersonaPromptBuilder) Build(
    ctx ProjectContext,
    diff domain.Diff,
    req BranchRequest,
    reviewer domain.Reviewer,
) (ProviderRequest, error) {
    // 1. Build base prompt using enhanced builder
    base, err := b.base.Build(ctx, diff, req, reviewer.Provider)
    if err != nil {
        return ProviderRequest{}, err
    }

    // 2. Inject persona at prompt start
    prompt := b.injectPersona(base.Prompt, reviewer.Persona)

    // 3. Add focus/ignore instructions
    prompt = b.addFocusInstructions(prompt, reviewer.Focus, reviewer.Ignore)

    // 4. Add category-specific guidance
    prompt = b.addCategoryGuidance(prompt, reviewer.Focus)

    return ProviderRequest{
        Prompt:       prompt,
        Seed:         base.Seed,
        MaxSize:      base.MaxSize,
        ReviewerName: reviewer.Name,
    }, nil
}

// injectPersona prepends the persona to the prompt.
func (b *PersonaPromptBuilder) injectPersona(prompt, persona string) string

// addFocusInstructions adds explicit focus/ignore directives.
func (b *PersonaPromptBuilder) addFocusInstructions(
    prompt string,
    focus, ignore []string,
) string

// addCategoryGuidance adds category-specific review guidance.
func (b *PersonaPromptBuilder) addCategoryGuidance(
    prompt string,
    categories []string,
) string
```

### 6.3 Orchestrator Changes

```go
// internal/usecase/review/orchestrator.go (modifications)

// OrchestratorDeps gains reviewer support
type OrchestratorDeps struct {
    // ... existing fields ...

    // ReviewerRegistry manages reviewer definitions (new)
    ReviewerRegistry *ReviewerRegistry

    // PersonaPromptBuilder builds persona-specific prompts (new)
    PersonaPromptBuilder *PersonaPromptBuilder
}

// ReviewBranch modifications
func (o *Orchestrator) ReviewBranch(ctx context.Context, req BranchRequest) (Result, error) {
    // ... existing setup ...

    // Resolve reviewers (new)
    reviewers, err := o.resolveReviewers(req)
    if err != nil {
        return Result{}, fmt.Errorf("resolving reviewers: %w", err)
    }

    // Dispatch to each reviewer in parallel (modified)
    for _, reviewer := range reviewers {
        wg.Add(1)
        go func(r domain.Reviewer) {
            defer wg.Done()

            // Build persona-specific prompt (new)
            providerReq, err := o.deps.PersonaPromptBuilder.Build(
                projectContext, textDiff, req, r,
            )
            if err != nil {
                errCh <- fmt.Errorf("building prompt for %s: %w", r.Name, err)
                return
            }

            // Get provider for this reviewer
            provider, ok := o.deps.Providers[r.Provider]
            if !ok {
                errCh <- fmt.Errorf("provider %s not found for reviewer %s", r.Provider, r.Name)
                return
            }

            // Execute review
            review, err := provider.Review(ctx, providerReq)
            if err != nil {
                errCh <- fmt.Errorf("review by %s: %w", r.Name, err)
                return
            }

            // Tag findings with reviewer metadata (new)
            for i := range review.Findings {
                review.Findings[i].ReviewerName = r.Name
                review.Findings[i].ReviewerWeight = r.Weight
            }

            reviewCh <- review
        }(reviewer)
    }

    // ... rest of method ...
}

// resolveReviewers determines which reviewers to use.
func (o *Orchestrator) resolveReviewers(req BranchRequest) ([]domain.Reviewer, error) {
    // CLI flag takes precedence
    if len(req.Reviewers) > 0 {
        return o.deps.ReviewerRegistry.Resolve(req.Reviewers)
    }

    // Fall back to registry defaults
    return o.deps.ReviewerRegistry.Resolve(nil)
}
```

### 6.4 Merger Changes

```go
// internal/usecase/merge/intelligent_merger.go (modifications)

// MergeConfig gains persona awareness
type MergeConfig struct {
    // ... existing fields ...

    // RespectFocus prevents penalizing low agreement for focused reviewers
    RespectFocus bool

    // WeightByReviewer uses reviewer weights in scoring
    WeightByReviewer bool
}

// calculateScore modifications
func (m *IntelligentMerger) calculateScore(group FindingGroup) float64 {
    // ... existing scoring ...

    // Apply reviewer weight boost (new)
    if m.config.WeightByReviewer {
        maxWeight := 0.0
        for _, f := range group.Findings {
            if f.ReviewerWeight > maxWeight {
                maxWeight = f.ReviewerWeight
            }
        }
        score *= maxWeight
    }

    return score
}

// shouldPenalizeAgreement checks if low agreement should reduce score.
func (m *IntelligentMerger) shouldPenalizeAgreement(group FindingGroup) bool {
    if !m.config.RespectFocus {
        return true // Always penalize (current behavior)
    }

    // Check if finding is in a focused category
    // If yes, don't penalize - specialist reviewers won't overlap
    for _, f := range group.Findings {
        if f.ReviewerName != "" {
            // Finding came from a specialized reviewer
            // Don't penalize for lack of agreement
            return false
        }
    }
    return true
}
```

### 6.5 Prior Findings Integration (Issue #138 Interaction)

The existing input deduplication feature injects previously acknowledged/disputed findings into the prompt to prevent LLMs from re-raising them. With reviewer personas, each reviewer should only see prior findings relevant to their focus.

#### Design Decision: Filter by Reviewer Name

Since we're not supporting migration from legacy configs, all findings will have a `ReviewerName` when posted. This allows simple, precise filtering.

```go
// internal/usecase/review/persona_prompt_builder.go

// filterPriorFindings returns only findings from this reviewer.
func (b *PersonaPromptBuilder) filterPriorFindings(
    ctx ProjectContext,
    reviewer domain.Reviewer,
) *domain.TriagedFindingContext {
    if ctx.TriagedFindings == nil || !ctx.TriagedFindings.HasFindings() {
        return nil
    }

    var filtered []domain.TriagedFinding
    for _, f := range ctx.TriagedFindings.Findings {
        if f.ReviewerName == reviewer.Name {
            filtered = append(filtered, f)
        }
    }

    if len(filtered) == 0 {
        return nil
    }

    return &domain.TriagedFindingContext{
        PRNumber: ctx.TriagedFindings.PRNumber,
        Findings: filtered,
    }
}
```

#### Required Changes

1. **Finding Attribution**: Findings must be tagged with reviewer name when posted to GitHub:

```go
// When posting findings, include reviewer name in the comment metadata
type Finding struct {
    // ... existing fields ...
    ReviewerName string  // Tag for filtering on subsequent reviews
}
```

2. **GitHub Comment Format**: The fingerprint comment must include reviewer name:

```
<!-- CR_FP:abc123 CR_REVIEWER:security -->
```

3. **Triage Fetcher**: Extract reviewer name when parsing prior findings:

```go
func extractReviewerFromComment(body string) string {
    // Parse CR_REVIEWER tag from comment
}
```

#### Prompt Builder Integration

```go
func (b *PersonaPromptBuilder) Build(
    ctx ProjectContext,
    diff domain.Diff,
    req BranchRequest,
    reviewer domain.Reviewer,
) (ProviderRequest, error) {
    // Filter prior findings for this reviewer only
    filteredCtx := ctx
    filteredCtx.TriagedFindings = b.filterPriorFindings(ctx, reviewer)

    // Build base prompt with filtered context
    base, err := b.base.Build(filteredCtx, diff, req, reviewer.Provider)
    // ...
}
```

#### Acceptance Criteria

- [ ] Findings tagged with `ReviewerName` when posted
- [ ] Security reviewer only sees prior security findings
- [ ] Maintainability reviewer only sees prior maintainability findings
- [ ] Token count reduced for each focused reviewer

---

## 7. CLI Changes

### New Flags

```bash
# Select specific reviewers
cr review branch main --reviewers security,docs

# List available reviewers
cr review list-reviewers

# Show reviewer details
cr review show-reviewer security
```

### BranchRequest Extension

```go
// internal/usecase/review/types.go

type BranchRequest struct {
    // ... existing fields ...

    // Reviewers specifies which reviewers to use (new)
    // Empty = use defaults from config
    Reviewers []string
}
```

---

## 8. Output Changes

### Markdown Output

```markdown
# Code Review: feature-branch → main

## Summary
- **Security Expert** (claude-opus-4): 3 findings
- **Maintainability** (claude-sonnet-4-5): 5 findings
- **Docs Checker** (gemini-flash): 2 findings

## Findings by Reviewer

### Security Expert (weight: 1.5)
#### Critical: SQL Injection in user input
...

### Maintainability (weight: 1.0)
#### High: DRY violation in validation logic
...
```

### JSON Output

```json
{
  "reviewers": [
    {
      "name": "security",
      "provider": "anthropic",
      "model": "claude-opus-4",
      "weight": 1.5,
      "findingCount": 3
    }
  ],
  "findings": [
    {
      "id": "abc123",
      "reviewer": "security",
      "reviewerWeight": 1.5,
      "severity": "critical",
      "category": "security",
      "...": "..."
    }
  ]
}
```

### SARIF Output

```json
{
  "runs": [
    {
      "tool": {
        "driver": {
          "name": "code-reviewer",
          "informationUri": "https://github.com/bkyoung/code-reviewer"
        },
        "extensions": [
          {
            "name": "security-expert",
            "version": "claude-opus-4"
          }
        ]
      },
      "results": [
        {
          "ruleId": "security/sql-injection",
          "extensions": {
            "reviewerName": "security",
            "reviewerWeight": 1.5
          }
        }
      ]
    }
  ]
}
```

---

## 9. Testing Strategy

### Unit Tests

| Component | Test Focus |
|-----------|------------|
| ReviewerRegistry | Resolution priority, legacy conversion, validation |
| PersonaPromptBuilder | Persona injection, focus/ignore formatting |
| Orchestrator | Reviewer dispatch, error handling, metadata tagging |
| Merger | Weight-based scoring, focus-aware agreement |

### Integration Tests

| Test | Validates |
|------|-----------|
| Config loading | YAML parsing, default application |
| End-to-end review | Multi-reviewer dispatch, merge, output |
| Legacy migration | Old config works unchanged |

### Fixture Reviewers

```go
// internal/testutil/reviewers.go

var TestSecurityReviewer = domain.Reviewer{
    Name:     "test-security",
    Provider: "static",
    Model:    "test",
    Weight:   1.5,
    Persona:  "You are a security expert.",
    Focus:    []string{"security"},
    Ignore:   []string{"style"},
}
```

---

## 10. Breaking Changes

Phase 3.2 introduces breaking changes to configuration:

1. **`providers` section removed** — Use `reviewers` instead
2. **No automatic migration** — Users must update config manually
3. **Finding format change** — Comments include `CR_REVIEWER:` tag

### Migration Guide

```yaml
# Before (v0.5.x)
providers:
  openai:
    model: gpt-4o
  anthropic:
    model: claude-sonnet-4-5

# After (v0.6.x)
reviewers:
  general:
    provider: openai
    model: gpt-4o
    weight: 1.0
  # Or use built-in personas:
  security:
    provider: anthropic
    model: claude-opus-4
```

---

## 11. Security Considerations

| Risk | Mitigation |
|------|------------|
| Persona prompt injection | Validate persona content, no code execution |
| Focus/ignore bypass | Validate against known categories |
| Weight manipulation | Cap weight range (0.1 - 5.0) |
| Provider impersonation | Reviewer names are config-controlled |

---

## 12. Performance Considerations

| Concern | Mitigation |
|---------|------------|
| More reviewers = more API calls | User controls reviewer count |
| Persona prompts add tokens | Keep personas concise (<500 tokens) |
| Focus instructions add overhead | Minimal token cost (~50 tokens) |
| Merge complexity increases | O(n) where n = findings, not reviewers |

---

## 13. Open Questions

| Question | Proposed Answer | Status |
|----------|-----------------|--------|
| Should personas be in separate files? | No, keep in cr.yaml for simplicity | Decided |
| Allow runtime persona override? | No, config-only for v1 | Decided |
| Built-in persona library? | Yes, ship 3 default personas | Decided |
| Persona inheritance/composition? | Defer to Phase 3.3 or later | Deferred |

---

## 14. Success Criteria

### Functional

- [ ] Config accepts `reviewers` section with all fields
- [ ] CLI `--reviewers` flag selects specific reviewers
- [ ] Each reviewer gets persona-specific prompt
- [ ] Findings tagged with reviewer name and weight
- [ ] Merger weights findings by reviewer weight
- [ ] Output attributes findings to reviewers
- [ ] Legacy `providers` config still works

### Non-Functional

- [ ] No performance regression (parallel dispatch preserved)
- [ ] Config validation catches errors early
- [ ] Clear error messages for misconfiguration
- [ ] Documentation covers all new options

---

## 15. Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-30 | Brandon Young | Initial draft |
