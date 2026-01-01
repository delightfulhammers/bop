# Product Requirements Document: Code-Reviewer Phase 3

**Version:** 0.3
**Date:** 2025-12-31
**Author:** Brandon Young
**Status:** Phase 3.1 & 3.2 Complete (v0.6.0-v0.6.3)

---

## 1. Executive Summary

Code-reviewer provides high-quality, automated code reviews using the LLM-as-judge pattern. The discovery pipeline is mature: diffs are analyzed by multiple LLM providers in parallel, findings are merged into consensus reviews, and results are posted to GitHub PRs with inline comments and review statuses.

**Phase 3 focused on two strategic gaps:**

1. **Resolution Pipeline** — Findings are surfaced but acting on them is manual and friction-heavy. We need tools that enable efficient triage and resolution of review feedback.

2. **Review Intelligence** — All providers receive identical prompts regardless of the change characteristics. We need specialized reviewers with distinct personas, and intelligent routing to match changes with the right models.

### Phase Summary

| Phase | Name | Focus | Priority | Status | Release |
|-------|------|-------|----------|--------|---------|
| **3.1** | Triage Automation | MCP server, skill for triage workflow | P0 | ✅ Complete | v0.5.0 |
| **3.2** | Reviewer Personas | Specialized reviewers with distinct prompts | P0 | ✅ Complete | v0.6.0 |
| **—** | GitHub Action | CI/CD integration for automated reviews | P0 | ✅ Complete | v0.6.2 |
| **3.3** | Dynamic Model Selection | Right-size models to change characteristics | P1 | Deferred | — |

### v0.6.x Releases

- **v0.6.0** — Reviewer personas with weighted findings, focus/ignore areas, CLI selection
- **v0.6.1** — Observability improvements, concurrency limiting, verbose logging
- **v0.6.2** — Official GitHub Action, semantic deduplication improvements
- **v0.6.3** — Defense-in-depth input validation, token budget reservation for personas

### Phase 3.3 Decision (2025-12-31)

Analyzed cost/benefit of dynamic model routing. Conclusion: current per-PR costs (~$2 worst case) don't justify the complexity of intelligent routing. The routing layer would add latency and cost while unlikely to eliminate enough reviewer calls to break even. At 3-5 PRs/day, the daily ceiling is ~$10 — well within acceptable range for the value provided. Deferred indefinitely in favor of using the tool on real projects rather than over-optimizing.

---

## 2. Problem Statement

### Problem 1: The Triage Bottleneck

**Current State:**  
When a PR receives automated review feedback, developers must:
- Read each finding manually
- Decide whether to accept, dispute, or acknowledge each one
- Make code changes using their IDE or editor
- Reply to comments on GitHub
- Push changes and wait for re-review

This loop is slow, especially when triaging with an AI assistant (e.g., Claude Code), because:
- The `gh` CLI is slow for bulk operations
- Context switching between GitHub UI, terminal, and editor adds friction
- There's no structured way to batch-process findings
- Feedback responses (acknowledge/dispute/won't-fix) require manual comment composition

**Desired State:**  
An agent (or human with good tooling) can efficiently:
- List and filter findings by severity, category, or status
- View finding context (code, diff, thread history) in one operation
- Apply suggested fixes (individually or in batches)
- Respond to findings with standard statuses
- Request re-review after changes

### Problem 2: One-Size-Fits-All Reviews

**Current State:**  
Every configured provider receives the same prompt:
```yaml
providers:
  openai:
    model: "gpt-4o"
  anthropic:
    model: "claude-sonnet-4-5"
  gemini:
    model: "gemini-2.5-pro"
```

This means:
- A one-line typo fix gets reviewed by 3 expensive models
- A massive refactor might exceed context limits of the configured models
- Security-critical changes get the same scrutiny as documentation updates
- All models look for everything, leading to redundant findings

**Desired State:**  
Reviews are tailored to the change:
- **Specialized reviewers** with distinct personas (security expert, refactoring advisor, documentation checker)
- **Right-sized models** based on change characteristics (token count, complexity, sensitivity)
- **Cost-aware routing** that balances thoroughness with budget
- **Focused prompts** that tell each reviewer what to look for (and what to ignore)

---

## 3. User Personas

### 3.1 The Solo Developer (Primary)
- Uses code-reviewer on personal/small team projects
- Triages feedback locally, often with Claude Code assistance
- Wants fast, actionable feedback without spending too much on API costs
- Values being able to customize review focus per project

### 3.2 The Team Lead (Secondary)
- Configures code-reviewer for team repositories
- Wants consistent review quality across the team
- Needs cost visibility and budget controls
- Values specialized reviews for different change types

### 3.3 The AI Agent (Emerging)
- Claude Code, Cursor, or other AI-assisted development tools
- Needs programmatic access to triage operations
- Benefits from structured tools over CLI scraping
- Could fully automate triage with human oversight

### 3.4 The Platform Engineer (Future)
- Manages code-reviewer across an organization
- Needs enterprise features: RBAC, audit trails, cross-repo learning
- Values standardized configurations with project-level overrides

---

## 4. Goals & Success Metrics

### Goal 1: Reduce Triage Time by 50%
**Metric:** Time from review posted → all findings addressed  
**Baseline:** Manual triage with Claude Code + gh CLI  
**Target:** Triage via MCP tools or `cr triage` CLI  

### Goal 2: Improve Finding Relevance by 30%
**Metric:** Ratio of accepted findings to total findings  
**Baseline:** Generic prompts, all findings weighted equally  
**Target:** Persona-based reviews with focused prompts  

### Goal 3: Reduce Review Cost by 40% (for small changes)
**Metric:** API cost per review for changes under 500 tokens  
**Baseline:** All configured providers run regardless of change size  
**Target:** Dynamic model selection routes small changes to fast/cheap models  

### Goal 4: Enable Full Agent-Driven Triage
**Metric:** Percentage of triage operations possible without human CLI/UI interaction  
**Baseline:** 0% (all operations require gh CLI or GitHub UI)  
**Target:** 100% (all operations available via MCP tools)  

---

## 5. Feature Requirements

### 5.1 Theme 1: Triage Automation

#### 5.1.1 MCP Server: `code-reviewer-mcp`

**Description:**  
An MCP (Model Context Protocol) server that exposes triage primitives as tools. Works with any MCP-compatible client (Claude Desktop, Claude Code, potentially other AI assistants).

**Core Tools (Read Operations):**

| Tool | Description | Parameters |
|------|-------------|------------|
| `list_findings` | List findings on a PR with filtering | `pr_number`, `status?`, `severity?`, `category?` |
| `get_finding` | Get detailed finding with context | `finding_id` or `comment_id` |
| `get_code_context` | Get current code at finding location | `file`, `line_start`, `line_end` |
| `get_diff_context` | Get diff hunk for finding location | `file`, `line_start`, `line_end` |
| `get_thread` | Get full comment thread for a finding | `comment_id` |

**Core Tools (Write Operations):**

| Tool | Description | Parameters |
|------|-------------|------------|
| `reply_to_finding` | Add reply to finding thread | `comment_id`, `body`, `status?` |
| `apply_suggestion` | Apply a suggested fix | `finding_id`, `create_commit?` |
| `batch_apply` | Apply multiple suggestions | `finding_ids[]`, `commit_message?` |
| `mark_resolved` | Mark finding as resolved | `comment_id` |
| `request_rereview` | Dismiss stale reviews, request fresh | `pr_number` |

**Status Values:**
- `acknowledged` — Will address in future work (synonym: `wont_fix`)
- `accepted` — Implementing the fix
- `disputed` — Disagree with the finding
- `wont_fix` — Valid finding but intentional/acceptable (synonym: `acknowledged`)
- `question` — Need clarification (alias: `clarification_request`)

*Note: Synonyms/aliases are accepted for flexibility; both resolve to the same semantic status.*

**Priority:** P0 (Critical)  
**Phase:** 3.1

---

#### 5.1.2 Claude Code Skill: `triage-pr-review`

**Description:**  
A skill file that teaches Claude Code how to effectively triage PR review feedback using the MCP tools (preferred) or gh CLI (fallback).

**Skill Capabilities:**
- Understand the triage workflow
- Know when to accept vs dispute findings
- Apply fixes safely (with verification)
- Compose appropriate response messages
- Handle batch operations efficiently

**Priority:** P1 (High)  
**Phase:** 3.1 (ships with MCP server)

---

#### 5.1.3 CLI Command: `cr triage`

**Description:**  
Interactive CLI for humans who don't use AI assistants. Provides a guided triage experience.

**Subcommands:**

| Command | Description |
|---------|-------------|
| `cr triage list` | List findings with filters |
| `cr triage show <id>` | Show finding details with context |
| `cr triage accept <id>` | Mark finding as accepted, apply fix if available |
| `cr triage dispute <id> <reason>` | Dispute with explanation |
| `cr triage batch-apply <ids...>` | Apply multiple fixes |
| `cr triage summary` | Show triage progress (X/Y addressed) |

**Priority:** P2 (Medium) — CLI is secondary interface to MCP  
**Phase:** 3.1 (ships with MCP server, but lower priority)

---

### 5.2 Theme 2: Intelligent Review Orchestration

#### 5.2.1 Reviewer Personas

**Description:**  
Replace the flat `providers` config with a `reviewers` concept. Each reviewer has a persona, specialization, and model assignment.

**Configuration Schema:**

```yaml
reviewers:
  security-expert:
    provider: anthropic
    model: claude-opus-4
    weight: 1.5  # Findings weighted higher in merge
    persona: |
      You are a security-focused code reviewer...
    focus:
      - authentication
      - authorization  
      - injection
      - secrets
    ignore:
      - style
      - performance
      - documentation
    
  maintainability-advisor:
    provider: anthropic
    model: claude-sonnet-4-5
    weight: 1.0
    persona: |
      You are a maintainability expert...
    focus:
      - dry-violations
      - solid-principles
      - complexity
      - naming
```

**Merger Changes:**
- Understand that reviewers have different roles
- Weight findings by reviewer weight
- Don't penalize low agreement when reviewers have different focus areas
- Attribute findings to reviewer persona in output

**Priority:** P0 (Critical)  
**Phase:** 3.2

---

#### 5.2.2 Dynamic Model Selection

**Description:**  
Automatically select the appropriate model based on change characteristics.

**Selection Factors:**

| Factor | Logic |
|--------|-------|
| Token count | Small → fast/cheap; Large → big context model |
| Change type | Security → strongest; Docs → cheapest |
| Cost budget | Over budget → downgrade model tier |
| Latency mode | Blocking CI → fast; Async → thorough |

**Configuration:**

```yaml
model_selection:
  default_strategy: "balanced"  # or "cost", "quality", "speed"
  
  token_tiers:
    - max_tokens: 1000
      prefer: ["gemini-flash", "gpt-4o-mini"]
    - max_tokens: 10000
      prefer: ["claude-sonnet", "gpt-4o"]
    - max_tokens: 100000
      prefer: ["gemini-pro", "claude-opus"]
  
  change_type_routing:
    security: ["claude-opus", "gpt-4o"]
    documentation: ["gemini-flash"]
    tests: ["claude-sonnet"]
```

**Priority:** P1 (High)  
**Phase:** 3.3

---

#### 5.2.3 Diff-Aware Prompt Generation

**Description:**  
Generate prompts dynamically based on what changed in the diff.

**Prompt Customizations:**
- Include relevant change type context
- Emphasize security review when auth code changes
- Include test coverage expectations when implementation changes
- Reference codebase conventions detected from context

**Priority:** P2 (Medium)  
**Phase:** 3.3 or later

---

## 6. Non-Goals / Out of Scope (Phase 3)

| Item | Reason |
|------|--------|
| GitLab/Bitbucket support | Platform expansion is Phase 4 |
| Cross-repo learning | Enterprise feature, Phase 4 |
| IDE plugins | Different product surface, future consideration |
| Self-hosted LLM training | Out of scope entirely |
| Real-time collaborative triage | Complex UX, uncertain value |

**Note on Phase 4:** Enterprise features (multi-platform, cross-repo learning) are aspirational. The tool's current scope — GitHub-only, per-repo — is sufficient for solo developers and small teams. No roadmap exists for Phase 4.

---

## 7. Constraints & Dependencies

### Technical Constraints
- Must maintain clean architecture (domain has no external deps)
- MCP server must be a separate binary (not embedded in `cr`)
- Backward compatibility with existing `providers` config

### Dependencies
- MCP SDK (Go implementation or build our own)
- GitHub API rate limits (especially for triage operations)
- LLM provider API stability

### Security Constraints
- MCP server must not expose secrets
- Triage tools must respect repository permissions
- Audit trail for all write operations

---

## 8. Resolved Design Decisions

| Question | Decision | Rationale |
|----------|----------|-----------|
| **MCP SDK** | Use [mcp-go](https://github.com/mark3labs/mcp-go) | Reasonably complete, well-maintained Go implementation |
| **Triage State** | GitHub comments are source of truth | Enables cross-context flexibility (different laptops, sessions, assistants) |
| **Reviewer Inheritance** | Support inheritance/composition | DRY for persona definitions; obvious benefit |
| **Hybrid Configs** | No backward compatibility needed | Solo user currently; just migrate to `reviewers` config |

---

## 9. Open Questions (Deferred)

1. **Cost Attribution & Observability:** Current cost calculation is coarse-grained. Phase 3 should include refined per-reviewer cost attribution as part of a larger observability initiative (telemetry, logging, metrics, tracing, event collection). *Design details TBD.*

---

## 10. Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 0.1 | 2025-12-28 | Brandon | Initial draft |
| 0.2 | 2025-12-31 | Brandon | Phase 3.1 & 3.2 complete |
| 0.3 | 2025-12-31 | Brandon | Added v0.6.x release summary, GitHub Action, deferred Phase 3.3 |