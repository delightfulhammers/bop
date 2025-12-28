# Code Reviewer - Claude Context

**Project:** AI-Powered Code Review Tool
**Status:** Phase 3 In Development
**Version:** v0.4.2 (targeting v0.5.0)
**Last Updated:** 2025-12-28

---

## Quick Start

1. **First:** Read Phase 3 design docs in `docs/design/` for current scope
2. **Build:** `go build -o cr ./cmd/cr`
3. **Test:** `go test ./...`
4. **Run:** `./cr review branch main` (reviews current branch against main)

---

## Current Phase: Phase 3 - Triage & Intelligence

### Phase Status

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1: Foundation | ✅ Complete | Multi-provider LLM, local CLI, basic GitHub workflow |
| Phase 2: GitHub Native | ✅ Complete | First-class reviewer with inline annotations |
| **Phase 3: Production** | 🚧 In Progress | Triage automation, reviewer personas, model selection |
| Phase 4: Enterprise | Planned | Multi-platform, org-wide learning |

### Phase 3 Focus Areas

| Sub-Phase | Focus | Priority |
|-----------|-------|----------|
| **3.1 Triage Automation** | MCP server, CLI, skill for triage workflow | P0 |
| **3.2 Reviewer Personas** | Specialized reviewers with distinct prompts | P0 |
| **3.3 Dynamic Model Selection** | Right-size models to change characteristics | P1 |

### Phase 3.1 Deliverables (Triage MCP Server)

| Component | Status |
|-----------|--------|
| `code-reviewer-mcp` binary | 🚧 Planned |
| `internal/usecase/triage/` | 🚧 Planned |
| 9 MCP tool handlers | 🚧 Planned |
| Claude Code skill update | 🚧 Planned |
| `cr triage` CLI (P2) | 🚧 Planned |

### Key Documents

- **Phase 3 PRD:** `docs/design/01-PRD.md`
- **Phase 3 Architecture:** `docs/design/02-ARCHITECTURE.md`
- **MCP Server TDD:** `docs/design/03-TDD-TRIAGE-MCP-SERVER.md`
- **Phase 3.1 Roadmap:** `docs/design/04-ROADMAP-PHASE-3.1.md`
- **Security:** `docs/SECURITY.md`
- **Archived Docs:** `docs/archive/` (historical reference)

---

## Technology Stack

- **Language:** Go 1.21+
- **Architecture:** Clean Architecture (domain → usecase → adapter)
- **LLM Providers:** OpenAI, Anthropic, Gemini, Ollama
- **Output Formats:** Markdown, JSON, SARIF
- **Persistence:** SQLite
- **Build:** `go build` (Mage available but optional)

---

## Development Essentials

### Build & Test

```bash
# Build
go build -o cr ./cmd/cr

# Test all
go test ./...

# Test with race detector
go test -race ./...

# Format
gofmt -w .

# Lint (if golangci-lint installed)
golangci-lint run
```

### Running Reviews

```bash
# Review current branch against main
./cr review branch main

# Review with output directory
./cr review branch main --output ./review-output

# Review with custom context
./cr review branch main --instructions "Focus on security"
```

### Core Rules

1. **TDD Mandatory:** Write tests first
2. **Clean Architecture:** Domain has no external dependencies
3. **Functional Style:** Prefer immutability, SOLID principles
4. **Format Before Commit:** `gofmt -w .`
5. **All Tests Pass:** `go test ./...` must succeed
6. **Fix All Lint Errors:** Fix ANY lint errors you encounter, even if you didn't cause them

### Definition of Done

- [ ] Tests written (TDD)
- [ ] Code formatted (`gofmt`)
- [ ] Lint passes (`golangci-lint run`) - fix ALL errors, not just yours
- [ ] All tests pass
- [ ] Build succeeds
- [ ] No race conditions (`go test -race`)

---

## Skills (Load Context On-Demand)

Use these skills for targeted context instead of reading docs manually:

| Skill | Use When | Invoke |
|-------|----------|--------|
| **development** | Building, testing, debugging | `/skill development` |
| **github-workflow** | Commits, PRs, issues | `/skill github-workflow` |
| **architecture** | Design questions, understanding codebase | `/skill architecture` |
| **review** | Using the code reviewer itself | `/skill review` |
| **triage-pr-review** | Triage PR code review feedback & SARIF alerts | `/skill triage-pr-review` |

---

## Project Structure

```
cmd/
  cr/                    # CLI entry point
  code-reviewer-mcp/     # MCP server (Phase 3.1 - planned)
internal/
  adapter/               # External integrations
    cli/                 # Command-line interface
    git/                 # Git operations
    llm/                 # LLM provider clients
      anthropic/
      gemini/
      ollama/
      openai/
    mcp/                 # MCP tool handlers (Phase 3.1 - planned)
    output/              # Output formatters (markdown, json, sarif)
    store/               # SQLite persistence
  config/                # Configuration loading
  domain/                # Core domain types (no dependencies)
  redaction/             # Secret redaction
  usecase/               # Business logic
    merge/               # Multi-provider merge
    review/              # Review orchestration
    triage/              # Triage workflow (Phase 3.1 - planned)
docs/
  design/                # Active design docs (Phase 3)
  archive/               # Historical docs
security-tests/          # Security test cases
```

---

## Common Pitfalls

1. **Don't** skip tests - TDD is mandatory
2. **Don't** import domain from adapters - clean architecture violation
3. **Don't** commit secrets - redaction exists but prevention is better
4. **Don't** ignore race detector - `go test -race` must pass
5. **Don't** forget to format - `gofmt -w .` before committing
6. **Don't** ignore lint errors - fix ALL errors you find, even pre-existing ones

### Code Hygiene (IMPORTANT)

When running `golangci-lint run`, fix **every error** you encounter - not just errors in files you modified. This codebase accumulated technical debt from ignoring pre-existing issues. The only way to recover is:

1. Fix issues as you find them
2. Eventually do a full codebase lint cleanup
3. Stay vigilant going forward

**This applies to:** lint errors, test failures, build warnings, race conditions - anything that indicates code quality issues.

---

## When You're Stuck

**Current work (Phase 3):**
- PRD: `docs/design/01-PRD.md`
- Architecture: `docs/design/02-ARCHITECTURE.md`
- MCP Server TDD: `docs/design/03-TDD-TRIAGE-MCP-SERVER.md`
- Roadmap: `docs/design/04-ROADMAP-PHASE-3.1.md`

**Reference documentation:**
- Security: `docs/SECURITY.md`
- GitHub Setup: `docs/GITHUB_ACTION_SETUP.md`

**Historical context:**
- Archived docs: `docs/archive/` (old architecture, phase docs, checklists)
- Session summaries: `docs/session-summaries/`

---

**Remember:** This file provides minimal always-on context. Use GitHub Issues for work tracking. Use skills for deeper, task-specific context.
