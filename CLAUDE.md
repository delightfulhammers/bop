# Code Reviewer - Claude Context

**Project:** AI-Powered Code Review Tool
**Status:** Phase 3.5 Complete, v0.7.x Stable
**Version:** v0.7.0
**Last Updated:** 2026-01-02

---

## Quick Start

1. **First:** Read Phase 3 design docs in `docs/design/` for current scope
2. **Build:** `mage build` (or `mage buildAll` for both binaries)
3. **Test:** `mage test` (or `mage testRace` for race detection)
4. **Run:** `./bop review branch main` (reviews current branch against main)

> **IMPORTANT:** Always prefer `mage` commands over raw `go` commands. Mage targets ensure consistent builds with version injection and proper flags.

### GitHub Action (CI/CD)

```yaml
- uses: delightfulhammers/bop/action@v0.7.0
  with:
    anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}
```

---

## Current Phase: Phase 3 - Triage & Intelligence

### Phase Status

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1: Foundation | ✅ Complete | Multi-provider LLM, local CLI, basic GitHub workflow |
| Phase 2: GitHub Native | ✅ Complete | First-class reviewer with inline annotations |
| Phase 3.1: Triage | ✅ Complete | MCP server for AI-assisted triage |
| Phase 3.2: Personas | ✅ Complete | Specialized reviewer roles with personas |
| Phase 3.3: Model Selection | Deferred | Right-size models to change characteristics |
| Phase 3.5: Local Mode | ✅ Complete | On-demand PR review, TUI, session storage, MCP review tools |
| Phase 4: Enterprise | Aspirational | Multi-platform, org-wide learning |

### Phase 3 Focus Areas

| Sub-Phase | Focus | Priority | Status |
|-----------|-------|----------|--------|
| **3.1 Triage Automation** | MCP server, CLI, skill for triage workflow | P0 | ✅ Complete |
| **3.2 Reviewer Personas** | Specialized reviewers with distinct prompts | P0 | ✅ Complete |
| **3.3 Dynamic Model Selection** | Right-size models to change characteristics | P2 | Deferred |
| **3.5 Local Mode** | On-demand PR review, TUI, sessions, MCP review tools | P0 | Design |

### Phase 3.1 Deliverables (Triage MCP Server)

| Component | Status |
|-----------|--------|
| `bop-mcp` binary | ✅ Complete |
| `internal/usecase/triage/` | ✅ Complete |
| 12 MCP tool handlers | ✅ Complete |
| Claude Code skill update | ✅ Complete |
| `bop triage` CLI (P2) | 🚧 Planned |

### Phase 3.2 Deliverables (Reviewer Personas)

| Component | Status |
|-----------|--------|
| `internal/domain/reviewer.go` | ✅ Complete |
| `internal/usecase/review/reviewer_registry.go` | ✅ Complete |
| `internal/usecase/review/persona_prompt_builder.go` | ✅ Complete |
| Orchestrator integration | ✅ Complete |
| Merger weight support | ✅ Complete |
| Output formatters (MD/JSON/SARIF) | ✅ Complete |
| CLI `--reviewers` flag | ✅ Complete |

### Phase 3.5 Deliverables (Local Mode)

| Component | Status |
|-----------|--------|
| MCP `review_branch` tool | ✅ Complete |
| MCP `review_pr` tool | ✅ Complete |
| MCP `review_files` tool | ✅ Complete |
| MCP `post_findings` tool | ✅ Complete |
| MCP sampling fallback | ✅ Complete |
| `bop review pr` command | ✅ Complete |
| `bop post` command | ✅ Complete |
| Session-based local storage | ✅ Complete |
| Interactive TUI (Bubble Tea) | 🚧 Planned |

### Key Documents

- **Phase 3 PRD:** `docs/design/01-PRD.md`
- **Phase 3 Architecture:** `docs/design/02-ARCHITECTURE.md`
- **Security:** `docs/SECURITY.md`
- **Archived Docs:** `docs/archive/` (roadmaps, TDDs, phase designs)

---

## Technology Stack

- **Language:** Go 1.21+
- **Architecture:** Clean Architecture (domain → usecase → adapter)
- **LLM Providers:** OpenAI, Anthropic, Gemini, Ollama, MCP Sampling (fallback)
- **Output Formats:** Markdown, JSON, SARIF
- **Persistence:** SQLite
- **Build System:** Mage (preferred)

---

## Development Essentials

### Mage Targets (Preferred)

**Always use `mage` commands** for consistency and proper version injection.

```bash
# List all available targets
mage -l

# Composite targets
mage ci          # Full CI: format, lint, test (race), build all
mage check       # Quick pre-commit: format, vet, test
mage all         # Everything: format, lint, test, coverage, build

# Build targets
mage build       # Build bop binary
mage buildMCP    # Build bop-mcp binary
mage buildAll    # Build both binaries
mage install     # Install to $GOPATH/bin
mage clean       # Remove build artifacts

# Test targets
mage test        # Run all tests
mage testRace    # Run tests with race detector
mage testCoverage # Generate coverage report
mage testUnit    # Run unit tests only (-short)
mage testVerbose # Run tests with verbose output

# Code quality
mage format      # Format code with gofmt
mage lint        # Run golangci-lint
mage lintFix     # Run golangci-lint with auto-fix
mage vet         # Run go vet

# Development helpers
mage deps        # Download and verify dependencies
mage tidy        # Clean up go.mod/go.sum
mage generate    # Run go generate
```

### Fallback (Raw Go Commands)

Only use these if mage is unavailable:

```bash
go build -o bop ./cmd/bop
go build -o bop-mcp ./cmd/bop-mcp
go test ./...
go test -race ./...
gofmt -w .
golangci-lint run
```

### Running Reviews

```bash
# Review current branch against main
./bop review branch main

# Review with specific personas
./bop review branch main --reviewers security,architecture

# Review with output directory
./bop review branch main --output ./review-output

# Review with custom context
./bop review branch main --instructions "Focus on security"
```

### Core Rules

1. **TDD Mandatory:** Write tests first
2. **Clean Architecture:** Domain has no external dependencies
3. **Functional Style:** Prefer immutability, SOLID principles
4. **Use Mage:** Always prefer `mage` commands over raw `go` commands
5. **Fix All Lint Errors:** Fix ANY lint errors you encounter, even if you didn't cause them

### Definition of Done

- [ ] Tests written (TDD)
- [ ] Code formatted (`mage format`)
- [ ] Lint passes (`mage lint`) - fix ALL errors, not just yours
- [ ] All tests pass (`mage test`)
- [ ] Build succeeds (`mage buildAll`)
- [ ] No race conditions (`mage testRace`)

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
  bop-mcp/     # MCP server for triage workflow
action/                  # GitHub Action (composite action)
internal/
  adapter/               # External integrations
    cli/                 # Command-line interface
    git/                 # Git operations
    llm/                 # LLM provider clients
      anthropic/
      gemini/
      ollama/
      openai/
    mcp/                 # MCP tool handlers (12 tools)
    output/              # Output formatters (markdown, json, sarif)
    store/               # SQLite persistence
  config/                # Configuration loading
  domain/                # Core domain types (no dependencies)
  redaction/             # Secret redaction
  usecase/               # Business logic
    dedup/               # Semantic deduplication
    merge/               # Multi-provider merge
    review/              # Review orchestration
    triage/              # Triage workflow service
docs/
  design/                # Active design docs
  archive/               # Historical docs
security-tests/          # Security test cases
```

---

## Common Pitfalls

1. **Don't** skip tests - TDD is mandatory
2. **Don't** import domain from adapters - clean architecture violation
3. **Don't** commit secrets - redaction exists but prevention is better
4. **Don't** use raw `go` commands - use `mage` targets instead
5. **Don't** ignore race detector - `mage testRace` must pass
6. **Don't** ignore lint errors - fix ALL errors you find, even pre-existing ones

### Code Hygiene (IMPORTANT)

When running `mage lint`, fix **every error** you encounter - not just errors in files you modified. This codebase accumulated technical debt from ignoring pre-existing issues. The only way to recover is:

1. Fix issues as you find them
2. Eventually do a full codebase lint cleanup
3. Stay vigilant going forward

**This applies to:** lint errors, test failures, build warnings, race conditions - anything that indicates code quality issues.

---

## PR Triage Protocol

When triaging PR review findings, use the `bop` MCP server tools. The server exposes 12 tools that handle both finding sources:

| Source | MCP Tools |
|--------|-----------|
| **SARIF Annotations** | `list_annotations`, `get_annotation` |
| **PR Comments** | `list_findings`, `get_finding`, `get_thread` |
| **Code Context** | `get_code_context`, `get_diff_context`, `get_suggestion` |
| **Actions** | `reply_to_finding`, `post_comment`, `mark_resolved`, `request_rereview` |

### Triage Workflow

1. **List findings:** `list_annotations` + `list_findings` (always check both)
2. **Analyze:** Use `get_finding`, `get_code_context`, `get_suggestion` for details
3. **Fix:** Apply code fixes for valid findings
4. **Respond:** `reply_to_finding` for PR comments, `post_comment` for SARIF
5. **Resolve:** `mark_resolved` for addressed threads
6. **Re-review:** `request_rereview` after fixes are pushed

See `/skill triage-pr-review` for detailed workflow.

---

## When You're Stuck

**Design documents:**
- PRD: `docs/design/01-PRD.md`
- Architecture: `docs/design/02-ARCHITECTURE.md`

**Reference documentation:**
- Security: `docs/SECURITY.md`
- GitHub Action Setup: `docs/GITHUB_ACTION_SETUP.md`
- Configuration: `docs/CONFIGURATION.md`

**Historical context:**
- Archived docs: `docs/archive/` (TDDs, roadmaps, old designs)

---

**Remember:** This file provides minimal always-on context. Use GitHub Issues for work tracking. Use skills for deeper, task-specific context.
