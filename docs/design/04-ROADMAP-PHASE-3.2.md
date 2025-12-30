# Phase 3.2 Implementation Roadmap

## Reviewer Personas

**Target Version:** v0.6.0
**Estimated Effort:** ~42 hours
**Status:** Planning

---

## Executive Summary

Phase 3.2 introduces specialized reviewer personas that replace the flat provider model. Each reviewer has a distinct persona, focus areas, and finding weight, enabling targeted reviews that reduce noise and improve relevance.

### Key Deliverables

| Deliverable | Priority | Effort | Status |
|-------------|----------|--------|--------|
| Domain Types & Config | P0 | 5h | Pending |
| ReviewerRegistry | P0 | 3h | Pending |
| PersonaPromptBuilder | P0 | 10h | Pending |
| Orchestrator Integration | P0 | 6h | Pending |
| Merger Enhancements | P0 | 4h | Pending |
| GitHub Poster & Triage Fetcher | P0 | 3h | Pending |
| CLI Changes | P1 | 2h | Pending |
| Output Formatting | P1 | 4h | Pending |
| Testing & Documentation | P0 | 5h | Pending |

---

## Milestone Overview

```
M1: Foundation        M2: Prompt Builder     M3: Integration        M4: Output & Polish
├── Domain types      ├── PersonaPromptBuilder├── Orchestrator       ├── Output formatters
├── Config schema     ├── Persona injection  ├── Reviewer dispatch  ├── CLI enhancements
├── ReviewerRegistry  ├── Focus/ignore       ├── Merger weights     ├── Documentation
│                     ├── Category guidance  │                      ├── Testing
│                     │                      │                      │
│  Pending            │  Pending             │  Pending             │  Pending
└──────────────────────────────────────────────────────────────────────────────────┘
```

### Dependency Chain

```
M1 (Foundation)
    └── M2 (Prompt Builder)
            └── M3 (Integration)
                    └── M4 (Output & Polish) ← PHASE 3.2 COMPLETE
```

---

## Milestone 1: Foundation (8 hours)

**Goal:** Establish domain types, configuration schema, and reviewer registry.

### Tasks

| Task | Effort | Description |
|------|--------|-------------|
| 1.1 Domain types | 2h | Add `Reviewer` type to `internal/domain/` |
| 1.2 Config schema | 3h | Extend config with `reviewers` section (replaces `providers`) |
| 1.3 ReviewerRegistry | 3h | Implement registry with resolution logic |

### Deliverables

- [ ] `internal/domain/reviewer.go` — Reviewer type definition
- [ ] `internal/config/reviewers.go` — Config parsing for reviewers
- [ ] `internal/usecase/review/reviewer_registry.go` — Registry implementation
- [ ] Unit tests for all new code

### Acceptance Criteria

- [ ] `Reviewer` type has all required fields (name, provider, model, weight, persona, focus, ignore)
- [ ] Config parses `reviewers` section correctly
- [ ] Config rejects old `providers` section with clear error message
- [ ] Registry resolves reviewers by name
- [ ] Registry applies defaults for missing fields
- [ ] Validation catches invalid configurations
- [ ] All tests pass

### Domain Type Specification

```go
// internal/domain/reviewer.go

type Reviewer struct {
    Name     string   // Unique identifier
    Provider string   // LLM provider (anthropic, openai, etc.)
    Model    string   // Model name
    Weight   float64  // Finding weight (default: 1.0)
    Persona  string   // System prompt describing expertise
    Focus    []string // Categories to emphasize
    Ignore   []string // Categories to skip
    Enabled  bool     // Whether reviewer is active
}
```

### Config Schema

```yaml
reviewers:
  security:
    provider: anthropic
    model: claude-opus-4
    weight: 1.5
    persona: |
      You are a security expert...
    focus: [security, authentication]
    ignore: [style, documentation]
    enabled: true

default_reviewers:
  - security
  - maintainability
```

---

## Milestone 2: Prompt Builder (10 hours)

**Goal:** Implement persona-aware prompt generation with prior findings filtering.

**Depends On:** M1

### Tasks

| Task | Effort | Description |
|------|--------|-------------|
| 2.1 PersonaPromptBuilder | 4h | New prompt builder with persona support |
| 2.2 Persona injection | 2h | Prepend persona to prompts |
| 2.3 Focus/ignore instructions | 1h | Add explicit category directives |
| 2.4 Prior findings filter | 2h | Filter prior findings by reviewer name |
| 2.5 Unit tests | 1h | Test prompt generation |

### Deliverables

- [ ] `internal/usecase/review/persona_prompt_builder.go` — Builder implementation
- [ ] Unit tests with fixture personas

### Acceptance Criteria

- [ ] Personas injected at prompt start
- [ ] Focus categories listed as explicit instructions
- [ ] Ignore categories listed as exclusions
- [ ] Prior findings filtered to show only this reviewer's findings
- [ ] Builder integrates with existing EnhancedPromptBuilder
- [ ] Provider request includes reviewer name
- [ ] All tests pass

### Prompt Structure

```
[PERSONA]
You are a security-focused code reviewer with expertise in...

[FOCUS INSTRUCTIONS]
Focus on these categories: security, authentication, injection
Ignore these categories: style, documentation, formatting

[STANDARD PROMPT]
Review the following code changes...

[DIFF]
...
```

---

## Milestone 3: Integration (10 hours)

**Goal:** Wire reviewers into orchestrator and merger.

**Depends On:** M2

### Tasks

| Task | Effort | Description |
|------|--------|-------------|
| 3.1 Orchestrator changes | 4h | Dispatch to reviewers instead of providers |
| 3.2 Finding attribution | 2h | Tag findings with reviewer metadata |
| 3.3 Merger weights | 2h | Score findings by reviewer weight |
| 3.4 Focus-aware agreement | 2h | Don't penalize specialized reviewers |

### Deliverables

- [ ] Modified `internal/usecase/review/orchestrator.go`
- [ ] Modified `internal/usecase/merge/intelligent_merger.go`
- [ ] Integration tests with multiple reviewers

### Acceptance Criteria

- [ ] Orchestrator resolves reviewers from registry
- [ ] Each reviewer gets persona-specific prompt
- [ ] Findings tagged with `ReviewerName` and `ReviewerWeight`
- [ ] Merger applies weight to finding scores
- [ ] Focused reviewers not penalized for low agreement
- [ ] Parallel dispatch preserved (no performance regression)
- [ ] All tests pass

### Orchestrator Flow

```
1. resolveReviewers(req) → []Reviewer
2. For each reviewer (parallel):
   a. PersonaPromptBuilder.Build(reviewer)
   b. provider.Review(prompt)
   c. Tag findings with reviewer metadata
3. Merger.MergeWithRoles(reviews)
4. Output
```

---

## Milestone 4: Output & Polish (14 hours)

**Goal:** Update output formats, GitHub posting, CLI, and documentation.

**Depends On:** M3

### Tasks

| Task | Effort | Description |
|------|--------|-------------|
| 4.1 Markdown output | 2h | Group findings by reviewer |
| 4.2 JSON output | 1h | Add reviewer metadata to schema |
| 4.3 SARIF output | 1h | Add reviewer as tool extension |
| 4.4 GitHub comment format | 2h | Add `CR_REVIEWER:` tag to posted comments |
| 4.5 Triage fetcher update | 1h | Extract reviewer name when parsing prior findings |
| 4.6 CLI flags | 2h | Add `--reviewers` flag |
| 4.7 Built-in personas | 2h | Ship 3 default persona templates |
| 4.8 Documentation | 2h | Update README, add persona guide |
| 4.9 End-to-end tests | 1h | Full workflow tests |

### Deliverables

- [ ] Modified output formatters (markdown, JSON, SARIF)
- [ ] Modified GitHub poster (include reviewer tag)
- [ ] Modified triage fetcher (parse reviewer tag)
- [ ] CLI with `--reviewers` flag
- [ ] Built-in persona definitions
- [ ] Updated documentation
- [ ] End-to-end test suite

### Acceptance Criteria

- [ ] Markdown groups findings by reviewer with weight display
- [ ] JSON includes `reviewers` array and finding attribution
- [ ] SARIF includes reviewer as tool extension
- [ ] GitHub comments include `CR_REVIEWER:name` tag
- [ ] Triage fetcher extracts reviewer name from prior comments
- [ ] CLI `--reviewers security,docs` selects reviewers
- [ ] 3 built-in personas: security, maintainability, docs
- [ ] README documents reviewer configuration
- [ ] All tests pass including E2E

### Built-in Personas

| Persona | Provider | Model | Weight | Focus |
|---------|----------|-------|--------|-------|
| security | anthropic | claude-opus-4 | 1.5 | security, auth, injection |
| maintainability | anthropic | claude-sonnet-4-5 | 1.0 | solid, dry, complexity |
| documentation | google | gemini-2.0-flash | 0.5 | docs, comments |

---

## Risk Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing configs | High | Legacy `providers` auto-converts |
| Performance regression | Medium | Parallel dispatch unchanged |
| Prompt token bloat | Low | Keep personas <500 tokens |
| Category confusion | Medium | Document valid categories |

---

## Success Criteria

### Functional

- [ ] `reviewers` config section works
- [ ] `--reviewers` CLI flag works
- [ ] Each reviewer gets distinct prompt
- [ ] Findings attributed to reviewers
- [ ] Weights affect merge ranking
- [ ] Legacy configs work unchanged
- [ ] 3 built-in personas available

### Non-Functional

- [ ] No performance regression
- [ ] >80% test coverage on new code
- [ ] Documentation complete
- [ ] Clear error messages

---

## Definition of Done

- [ ] All code formatted (`mage format`)
- [ ] Lint passes (`mage lint`)
- [ ] All tests pass (`mage test`)
- [ ] Race detector passes (`mage testRace`)
- [ ] Build succeeds (`mage buildAll`)
- [ ] Documentation updated
- [ ] CHANGELOG updated

---

## Revision History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-30 | Initial roadmap |
