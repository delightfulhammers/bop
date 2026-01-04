# Architecture Skill

Load context for understanding the codebase design and making architectural decisions.

## Instructions

When this skill is invoked:

1. **Load Architecture Doc**: Read `docs/design/02-ARCHITECTURE.md`
2. **Review Project Structure**: Understand the clean architecture layers
3. **For Phase 3 context**: Also read `docs/design/01-PRD.md` for requirements

## Clean Architecture Layers

```
┌─────────────────────────────────────────────────┐
│           cmd/bop/ & cmd/bop-mcp/      │  Entry points
├─────────────────────────────────────────────────┤
│              internal/adapter/                  │  External integrations
│  cli/ git/ llm/ mcp/ output/ store/ github/     │
├─────────────────────────────────────────────────┤
│              internal/usecase/                  │  Business logic
│            review/ merge/ triage/               │
├─────────────────────────────────────────────────┤
│              internal/domain/                   │  Core types
│              (NO external dependencies)         │
└─────────────────────────────────────────────────┘
```

## Key Design Principles

1. **Domain has no dependencies** - Pure Go types only
2. **Adapters depend on domain** - Never the reverse
3. **Use cases orchestrate** - Coordinate adapters and domain logic
4. **Dependency injection** - Interfaces for testability

## Key Components

### Domain (`internal/domain/`)
- `Diff`, `FileDiff` - Git diff representation
- `Finding`, `Review` - Review results
- `Severity`, `Category` - Finding classification
- `TriageStatus`, `TriageFinding` - Triage state (Phase 3.1)

### Use Cases (`internal/usecase/`)
- `review/orchestrator.go` - Main review coordination
- `review/prompt_builder.go` - LLM prompt construction
- `review/context.go` - Context gathering
- `merge/intelligent_merger.go` - Multi-provider consensus
- `triage/service.go` - Triage workflow (Phase 3.1)

### Adapters (`internal/adapter/`)
- `llm/` - Provider clients (OpenAI, Anthropic, Gemini, Ollama)
- `git/` - Git operations via go-git
- `output/` - Formatters (Markdown, JSON, SARIF)
- `store/` - SQLite persistence
- `github/` - GitHub API (PR posting, comments, reviews)
- `mcp/` - MCP tool handlers (Phase 3.1)

## Phase 3 Architecture Additions

Phase 3 introduces:

1. **MCP Server** (`cmd/bop-mcp/`)
   - Separate binary for Model Context Protocol
   - Exposes triage tools for AI assistants
   - Shares libraries with main `cr` CLI

2. **Triage Use Case** (`internal/usecase/triage/`)
   - Finding listing and filtering
   - Suggestion extraction for `str_replace`
   - GitHub thread management

3. **Reviewer Personas** (Phase 3.2)
   - Specialized reviewers with distinct prompts
   - Focus/ignore categories per reviewer
   - Weight-based merge

## Design Decisions

Key decisions are documented in:
- `docs/design/02-ARCHITECTURE.md` (current)
- `docs/archive/ARCHITECTURE.md` (historical)

**Intentional duplication:**
- ID generation exists in both `usecase/review/` and `store/` to maintain clean architecture boundaries

**Template-based prompts:**
- Prompts use Go text/template for provider-specific formatting

**MCP server design:**
- Information + orchestration only; file edits handled by Claude Code native tools
- GitHub comments are source of truth for triage state

## After Loading Context

Read `docs/design/02-ARCHITECTURE.md` for full details, then assist with design questions or codebase understanding.
