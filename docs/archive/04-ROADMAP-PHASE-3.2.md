# Phase 3.2 Implementation Roadmap

## Reviewer Personas

**Target Version:** v0.6.0
**Actual Release:** v0.6.0 (2025-12-31)
**Status:** ✅ Complete

---

## Executive Summary

Phase 3.2 introduces specialized reviewer personas that replace the flat provider model. Each reviewer has a distinct persona, focus areas, and finding weight, enabling targeted reviews that reduce noise and improve relevance.

### Key Deliverables

| Deliverable | Priority | Effort | Status |
|-------------|----------|--------|--------|
| Domain Types & Config | P0 | 5h | ✅ Complete |
| ReviewerRegistry | P0 | 3h | ✅ Complete |
| PersonaPromptBuilder | P0 | 10h | ✅ Complete |
| Orchestrator Integration | P0 | 6h | ✅ Complete |
| Merger Enhancements | P0 | 4h | ✅ Complete |
| GitHub Poster & Triage Fetcher | P0 | 3h | ✅ Complete |
| CLI Changes | P1 | 2h | ✅ Complete |
| Output Formatting | P1 | 4h | ✅ Complete |
| Testing & Documentation | P0 | 5h | ✅ Complete |

---

## Milestone Overview

```
M1: Foundation        M2: Prompt Builder     M3: Integration        M4: Output & Polish
├── Domain types      ├── PersonaPromptBuilder├── Orchestrator       ├── Output formatters
├── Config schema     ├── Persona injection  ├── Reviewer dispatch  ├── CLI enhancements
├── ReviewerRegistry  ├── Focus/ignore       ├── Merger weights     ├── Documentation
│                     ├── Category guidance  │                      ├── Testing
│                     │                      │                      │
│  ✅ Complete        │  ✅ Complete         │  ✅ Complete         │  ✅ Complete
└──────────────────────────────────────────────────────────────────────────────────┘
```

### Dependency Chain

```
M1 (Foundation)
    └── M2 (Prompt Builder)
            └── M3 (Integration)
                    └── M4 (Output & Polish) ← PHASE 3.2 COMPLETE ✅
```

---

## Milestone 1: Foundation (8 hours) ✅

**Goal:** Establish domain types, configuration schema, and reviewer registry.

### Tasks

| Task | Effort | Description | Status |
|------|--------|-------------|--------|
| 1.1 Domain types | 2h | Add `Reviewer` type to `internal/domain/` | ✅ |
| 1.2 Config schema | 3h | Extend config with `reviewers` section | ✅ |
| 1.3 ReviewerRegistry | 3h | Implement registry with resolution logic | ✅ |

### Deliverables

- [x] `internal/domain/reviewer.go` — Reviewer type definition
- [x] `internal/config/config.go` — Config parsing for reviewers
- [x] `internal/usecase/review/reviewer_registry.go` — Registry implementation
- [x] Unit tests for all new code

### Acceptance Criteria

- [x] `Reviewer` type has all required fields (name, provider, model, weight, persona, focus, ignore)
- [x] Config parses `reviewers` section correctly
- [x] Backward compatible - providers section still works
- [x] Registry resolves reviewers by name
- [x] Registry applies defaults for missing fields
- [x] Validation catches invalid configurations
- [x] All tests pass

---

## Milestone 2: Prompt Builder (10 hours) ✅

**Goal:** Implement persona-aware prompt generation with prior findings filtering.

### Tasks

| Task | Effort | Description | Status |
|------|--------|-------------|--------|
| 2.1 PersonaPromptBuilder | 4h | New prompt builder with persona support | ✅ |
| 2.2 Persona injection | 2h | Prepend persona to prompts | ✅ |
| 2.3 Focus/ignore instructions | 1h | Add explicit category directives | ✅ |
| 2.4 Prior findings filter | 2h | Filter prior findings by reviewer name | ✅ |
| 2.5 Unit tests | 1h | Test prompt generation | ✅ |

### Deliverables

- [x] `internal/usecase/review/persona_prompt_builder.go` — Builder implementation
- [x] Unit tests with fixture personas

### Acceptance Criteria

- [x] Personas injected at prompt start
- [x] Focus categories listed as explicit instructions
- [x] Ignore categories listed as exclusions
- [x] Prior findings filtered to show only this reviewer's findings
- [x] Builder integrates with existing EnhancedPromptBuilder
- [x] Provider request includes reviewer name
- [x] Seed propagated for determinism
- [x] All tests pass

---

## Milestone 3: Integration (10 hours) ✅

**Goal:** Wire reviewers into orchestrator and merger.

### Tasks

| Task | Effort | Description | Status |
|------|--------|-------------|--------|
| 3.1 Orchestrator changes | 4h | Dispatch to reviewers instead of providers | ✅ |
| 3.2 Finding attribution | 2h | Tag findings with reviewer metadata | ✅ |
| 3.3 Merger weights | 2h | Score findings by reviewer weight | ✅ |
| 3.4 Focus-aware agreement | 2h | Don't penalize specialized reviewers | ✅ |

### Deliverables

- [x] Modified `internal/usecase/review/orchestrator.go`
- [x] Modified `internal/usecase/merge/intelligent_merger.go`
- [x] Integration tests with multiple reviewers

### Acceptance Criteria

- [x] Orchestrator resolves reviewers from registry
- [x] Each reviewer gets persona-specific prompt
- [x] Findings tagged with `ReviewerName` and `ReviewerWeight`
- [x] Merger applies weight to finding scores
- [x] Focused reviewers not penalized for low agreement
- [x] Parallel dispatch preserved (no performance regression)
- [x] File naming uses reviewer name (not provider) to prevent collisions
- [x] All tests pass

---

## Milestone 4: Output & Polish (14 hours) ✅

**Goal:** Update output formats, GitHub posting, CLI, and documentation.

### Tasks

| Task | Effort | Description | Status |
|------|--------|-------------|--------|
| 4.1 Markdown output | 2h | Group findings by reviewer | ✅ |
| 4.2 JSON output | 1h | Add reviewer metadata to schema | ✅ |
| 4.3 SARIF output | 1h | Add reviewer as tool extension | ✅ |
| 4.4 GitHub comment format | 2h | Add `CR_REVIEWER:` tag to posted comments | ✅ |
| 4.5 Triage fetcher update | 1h | Extract reviewer name when parsing prior findings | ✅ |
| 4.6 CLI flags | 2h | Add `--reviewers` flag | ✅ |
| 4.7 Documentation | 2h | Update README, add examples | ✅ |
| 4.8 End-to-end tests | 1h | Full workflow tests | ✅ |

### Deliverables

- [x] Modified output formatters (markdown, JSON, SARIF)
- [x] Modified GitHub poster (include reviewer tag)
- [x] Modified triage fetcher (parse reviewer tag)
- [x] CLI with `--reviewers` flag
- [x] Updated documentation

### Acceptance Criteria

- [x] Markdown groups findings by reviewer with weight display
- [x] JSON includes `reviewers` array and finding attribution
- [x] SARIF includes reviewer as tool extension
- [x] GitHub comments include `CR_REVIEWER:name` tag
- [x] Triage fetcher extracts reviewer name from prior comments
- [x] CLI `--reviewers security,architecture` selects reviewers
- [x] README documents reviewer configuration
- [x] All tests pass

---

## Bug Fixes During Development

| Bug | Description | Fix |
|-----|-------------|-----|
| Seed propagation | PersonaPromptBuilder dropped baseReq.Seed | Copy Seed to returned ProviderRequest |
| File naming race | Multiple reviewers on same provider wrote to same file | Use reviewer.Name for output filenames |
| Test helper names | Test helpers created reviewers with mismatched names | Use provider name as reviewer name |

---

## Success Criteria ✅

### Functional

- [x] `reviewers` config section works
- [x] `--reviewers` CLI flag works
- [x] Each reviewer gets distinct prompt
- [x] Findings attributed to reviewers
- [x] Weights affect merge ranking
- [x] Legacy configs work unchanged (backward compatible)

### Non-Functional

- [x] No performance regression
- [x] >80% test coverage on new code
- [x] Documentation complete
- [x] Clear error messages

---

## Definition of Done ✅

- [x] All code formatted (`mage format`)
- [x] Lint passes (`mage lint`)
- [x] All tests pass (`mage test`)
- [x] Race detector passes (`mage testRace`)
- [x] Build succeeds (`mage buildAll`)
- [x] Documentation updated
- [x] PR merged (#156)
- [x] Release published (v0.6.0)

---

## Revision History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-30 | Initial roadmap |
| 2.0 | 2025-12-31 | Marked complete, added bug fixes section |
