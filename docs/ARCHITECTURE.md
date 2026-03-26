# Bop — Architecture

> This document describes the technical architecture of Bop, a multi-persona code review agent. See [VISION.md](VISION.md) for product principles and [PRD.md](PRD.md) for full feature requirements.

---

## 1. System Overview

Bop is structured as two statically compiled Go binaries — `bop` (CLI) and `bop-mcp` (MCP server) — built on top of a shared core library. Both binaries expose identical capabilities through their respective interfaces; neither is a subset of the other.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Deployment Surfaces                       │
│                                                                  │
│   ┌──────────────────┐   ┌──────────────────┐  ┌─────────────┐ │
│   │  GitHub Actions  │   │   Local Terminal  │  │  MCP Client │ │
│   │    Workflow      │   │   (bop CLI)       │  │  (AI IDE)   │ │
│   └────────┬─────────┘   └────────┬──────────┘  └──────┬──────┘ │
└────────────┼────────────────────── ┼────────────────────┼────────┘
             │                       │                    │
             ▼                       ▼                    ▼
     ┌───────────────┐       ┌───────────────┐   ┌───────────────┐
     │  bop binary   │       │  bop binary   │   │ bop-mcp binary│
     │ (actions mode)│       │ (local/remote)│   │ (MCP server)  │
     └───────┬───────┘       └───────┬───────┘   └───────┬───────┘
             └───────────────────────┴───────────────────┘
                                     │
                         ┌───────────▼────────────┐
                         │      Core Library       │
                         │                         │
                         │  ┌─────────────────┐   │
                         │  │  Review Pipeline │   │
                         │  └────────┬────────┘   │
                         │           │             │
                         │  ┌────────▼────────┐   │
                         │  │ Persona Engine  │   │
                         │  └────────┬────────┘   │
                         │           │             │
                         │  ┌────────▼────────┐   │
                         │  │  LLM Providers  │   │
                         │  └────────┬────────┘   │
                         │           │             │
                         │  ┌────────▼────────┐   │
                         │  │  GitHub Client  │   │
                         │  └─────────────────┘   │
                         └────────────────────────┘
                                     │
                    ┌────────────────┴────────────────┐
                    │                                 │
                    ▼                                 ▼
         ┌──────────────────┐             ┌──────────────────┐
         │   GitHub REST    │             │   LLM Provider   │
         │       API        │             │      APIs        │
         │                  │             │                  │
         │  - PR diffs      │             │  - Anthropic     │
         │  - PR comments   │             │  - Google Gemini │
         │  - Comment meta  │             │  - OpenAI        │
         └──────────────────┘             │  - Ollama        │
                                          └──────────────────┘
```

All state that persists across review rounds is stored in GitHub comment metadata on the PR itself. There is no database, no cache server, and no long-running daemon. Each binary invocation is independent and fully stateless beyond what it reads from GitHub and the local environment.

---

## 2. Components

### 2.1 `bop` — CLI Binary

**Responsibility:** The primary user-facing binary for all non-MCP usage. Handles GitHub Actions invocations, local checkout reviews, and remote PR reviews from the terminal.

The CLI binary wires command-line argument parsing and environment variable ingestion to core library functions. It is the entry point for the GitHub Actions workflow step — the runner downloads and executes `bop` directly, with no container wrapper required.

**Modes of operation:**
- **Actions mode:** Triggered as a GitHub Actions step. Reads PR context from standard Actions environment variables, `GITHUB_TOKEN`, and configured LLM API keys. Runs the full review pipeline and posts findings to the PR.
- **Local checkout mode:** Reviews a locally checked-out branch or diff without requiring a live GitHub PR.
- **Remote PR mode:** Fetches a PR from GitHub by repository identifier and PR number, runs the full review pipeline, and optionally posts findings.

**Management commands (all modes):**
- Retrieve prior findings from a PR
- Post a response to a PR comment
- Dismiss a Bop review on a PR
- Re-request a review on a PR

### 2.2 `bop-mcp` — MCP Server Binary

**Responsibility:** Exposes all Bop capabilities as an MCP (Model Context Protocol) server, allowing any MCP-compliant client — AI IDEs, coding assistants, chat tools — to invoke Bop programmatically.

`bop-mcp` registers MCP tool handlers that map 1:1 onto the capabilities of the `bop` CLI. It handles the MCP protocol framing and delegates all substantive work to the same core library functions used by the CLI. No capability exists in one binary that does not exist in the other.

The MCP server reads `GITHUB_TOKEN` and LLM API keys from the environment identically to the CLI.

### 2.3 Core Library

The shared Go library that both binaries import. All business logic lives here. Neither binary contains review logic, GitHub API logic, or LLM provider logic — they are thin wrappers that handle their respective interface protocols and delegate to this library.

The core library is composed of the following internal subsystems:

#### 2.3.1 Review Pipeline

**Responsibility:** Orchestrates the full review cycle from diff input to synthesized findings output.

The pipeline executes in the following stages:

1. **Context assembly** — Fetches the PR diff, retrieves prior round findings from GitHub comment metadata, and assembles the review context.
2. **Per-persona review** — Dispatches the diff (with prior findings injected) to each active reviewer persona concurrently. Each persona calls its assigned LLM provider and returns a set of raw findings with associated weights.
3. **Multi-layer deduplication** — Passes the aggregated raw findings through the deduplication pipeline (see §2.3.2).
4. **Synthesis** — Produces the unified review output from the deduplicated, confidence-scored findings.
5. **Posting** — Submits the synthesized review as PR comments via the GitHub client, embedding round metadata in comment metadata for future round persistence.

#### 2.3.2 Deduplication Pipeline

**Responsibility:** Reduces the combined output of all personas into a non-redundant finding set before synthesis.

The deduplication pipeline runs in layers:

1. **Exact / near-exact deduplication** — Removes findings that are textually identical or near-identical across personas.
2. **Semantic deduplication** — Identifies findings that address the same underlying issue in different language (using LLM-based semantic comparison or embedding similarity) and collapses them into a single representative finding.
3. **Confidence scoring** — Assigns a confidence score to each surviving finding, informed by the number of personas that independently identified it, their respective weights, and the strength of the LLM's expressed certainty. Low-confidence findings may be surfaced with reduced prominence or filtered.

#### 2.3.3 Persona Engine

**Responsibility:** Loads persona configuration, resolves the active reviewer panel, and dispatches per-persona review requests.

On startup, the Persona Engine:
1. Looks for `bop.yaml` in the repository root.
2. If found, loads the project-defined reviewer panel.
3. If not found, falls back to the built-in default reviewer panel (including the default persona backed by Claude Sonnet).

Each persona record carries:
- **Prompt:** The system/user prompt template for that persona's LLM call.
- **Model:** The LLM provider and model identifier (e.g., `claude-sonnet-4-5`, `gemini-2.0-flash`, `gpt-4o`, or an Ollama model name).
- **Finding weight:** Applied during synthesis to up-weight or down-weight a persona's findings relative to others.
- **Default panel inclusion:** Whether the persona is active without explicit panel configuration in `bop.yaml`.

The engine is also responsible for injecting the prior-round findings context into each persona's prompt before dispatch.

#### 2.3.4 LLM Provider Abstraction

**Responsibility:** Provides a uniform interface for making LLM review calls across all supported providers.

Supported providers:
- **Anthropic Claude** (default; default model: Claude Sonnet)
- **Google Gemini**
- **OpenAI**
- **Ollama** (local model hosting)
- **MCP Sampling** (fallback — delegates LLM calls to the connected MCP client)

Each provider implementation handles authentication (reading the relevant API key from the environment), request construction, response parsing, and error handling. The abstraction layer normalizes provider differences so the Persona Engine and Review Pipeline are provider-agnostic. The MCP sampling provider enables zero-configuration usage when `bop-mcp` is connected to an MCP client that supports sampling.

API keys are always sourced from environment variables. They are never read from `bop.yaml` or any file committed to the repository. The HTTP client layer includes observable clients with logging, metrics, pricing tracking, and retry logic with exponential backoff.

#### 2.3.5 GitHub Client

**Responsibility:** All interactions with the GitHub API (both GraphQL and REST).

Capabilities:
- Fetch PR diff and file contents (GraphQL)
- Fetch existing PR review comments and metadata (REST)
- Read SARIF annotations from check runs
- Read and write Bop-specific comment metadata (used for cross-round persistence)
- Post synthesized review findings as PR comments
- Post responses to existing PR comments
- Dismiss Bop reviews
- Trigger review re-requests
- Build PR summaries

Issue comment listing includes an in-memory cache with TTL-based expiration, automatically invalidated when new comments are posted. Callers can optionally limit pagination depth for performance-sensitive paths. These optimizations reduce API round-trips during triage sessions and on PRs with extensive comment histories.

Authentication is via `GITHUB_TOKEN` sourced from the environment. In GitHub Actions, this is the automatically injected token with permissions extended as shown in the example workflow. In local and MCP usage, the developer supplies a PAT or token with equivalent permissions. GitHub Enterprise Server is supported via `GITHUB_API_URL`.

#### 2.3.6 Verification Agent

**Responsibility:** Validates findings before they are posted, reducing false positives.

The verification agent independently re-examines each finding against the actual code context, applying tool-based checks to confirm or reject the finding. Findings can be verified individually or in batches with configurable concurrency. Verification is optional and controlled via configuration and CLI flags (`--verify` / `--no-verify`).

#### 2.3.7 Session Management

**Responsibility:** Persists local review history for the CLI's on-demand review mode.

When running local reviews (not in CI), Bop stores review sessions in a SQLite database (default: `~/.config/bop/reviews.db`). This enables session history, re-review of prior results, and local state tracking. Session storage is strictly local — it is not used in GitHub Actions or MCP server mode, and its absence does not affect any core capability.

#### 2.3.8 Supporting Modules

Several smaller internal modules support the core subsystems:

- **Diff parsing** (`internal/diff/`): Parses and analyzes unified diffs into structured representations.
- **Redaction** (`internal/redaction/`): Scrubs secrets from log output and review content before external transmission.
- **Path utilities** (`internal/pathutil/`): Validates and normalizes file paths.
- **Determinism** (`internal/determinism/`): Seed generation for reproducible LLM outputs where supported.
- **Theme extraction** (`internal/adapter/theme/`): Extracts code themes and architectural patterns to enrich review context.
- **Platform integration** (`internal/platform/`): Device flow authentication and configuration sync with the Bop platform service (optional).

### 2.4 `bop.yaml` Configuration

**Responsibility:** Project-level definition of the reviewer panel and per-persona configuration.

Located at the repository root. Optional — Bop operates correctly without it using the built-in default panel.

Configurable per persona:
- Prompt (full text or reference to a prompt file)
- Model (provider-qualified model identifier)
- Finding weight (numeric or categorical)
- Default panel inclusion (boolean)

`bop.yaml` is not a valid location for secrets. All credentials must come from the environment.

The repository ships with example configurations demonstrating security, architecture, and performance reviewer personas.

### 2.5 GitHub Comment Metadata (Persistence Layer)

**Responsibility:** Cross-round state persistence with no external infrastructure.

Bop embeds structured metadata into GitHub PR comments when posting review findings. On subsequent review rounds, this metadata is read back to reconstruct the prior findings context. This is the sole persistence mechanism.

This design means:
- No database to provision or maintain
- State is co-located with the PR it belongs to
- State is automatically scoped and cleaned up with the PR lifecycle
- Any environment with `GITHUB_TOKEN` read access to the PR can retrieve prior state

---

## 3. Data Flow

### 3.1 Standard Review Round (GitHub Actions)

```
GitHub Actions trigger
        │
        ▼
bop binary invoked
        │
        ├── Read GITHUB_TOKEN, LLM API keys from environment
        ├── Read PR context from Actions environment variables
        │
        ▼
GitHub Client: fetch PR diff
        │
        ▼
GitHub Client: fetch prior Bop comments + metadata
        │
        ▼
Persona Engine: load bop.yaml (or default panel)
        │
        ▼
Persona Engine: assemble per-persona prompts
  (diff + prior findings injected)
        │
        ▼
Per-persona LLM calls (concurrent)
  ┌─────┴──────────────────────────────┐
  │ Persona A (e.g., default)          │
  │ Persona B (e.g., security)         │
  │ Persona C (e.g., architecture)     │
  └─────┬──────────────────────────────┘
        │  Raw weighted findings from each persona
        ▼
Deduplication Pipeline
  1. Exact/near-exact dedup
  2. Semantic dedup
  3. Confidence scoring
        │
        ▼
Synthesis: unified review output
        │
        ▼
GitHub Client: post review comments to PR
  (with embedded round metadata)
        │
        ▼
Done
```

### 3.2 Cross-Round Persistence

```
Round N:
  Synthesis output
        │
        ▼
  GitHub comment posted
  with embedded metadata
  { round: N, findings: [...] }

Round N+1:
  GitHub Client reads
  prior comments + metadata
        │
        ▼
  Prior findings extracted
  from metadata
        │
        ▼
  Injected into per-persona
  prompts as "do not repeat"
  context before LLM dispatch
```

### 3.3 MCP Operation

```
MCP Client (coding assistant)
        │  MCP protocol call
        ▼
bop-mcp: parse MCP request
        │
        ▼
Core Library: same code path
as CLI for all operations
        │
        ▼
GitHub API / LLM APIs
        │
        ▼
Core Library: result
        │
        ▼
bop-mcp: format MCP response
        │
        ▼
MCP Client: receives findings/confirmation
```

---

## 4. Working Conventions

### 4.1 Language and Tooling

Bop is written in **Go**. All production code, tests, and build tooling use Go. The project is structured as a Go module. Both `bop` and `bop-mcp` are built from the same module, with the core library as an internal package.

### 4.2 Binary Structure

The repository produces two binaries from a single Go module:

- `cmd/bop/` — CLI entry point
- `cmd/bop-mcp/` — MCP server entry point
- `internal/` — Core library, shared by both binaries (clean architecture layers)

Neither `cmd/bop` nor `cmd/bop-mcp` contains business logic. They contain only: argument/flag parsing, environment variable reading, interface-specific protocol handling (CLI UX vs. MCP framing), and calls into the core library. This is a strict convention — any logic that could conceivably be needed by both binaries belongs in the core library.

### 4.3 Execution Model

In CI (GitHub Actions) and MCP server mode, every invocation is fully stateless and self-contained. All state comes from: environment variables, the `bop.yaml` in the repository, and GitHub comment metadata fetched at runtime. No assumption is made about prior invocations on the same machine.

In local CLI mode, Bop optionally persists review sessions to a SQLite database (`~/.config/bop/reviews.db`) for session history and re-review convenience. This local persistence is strictly additive — removing the database has no effect on review correctness.

Code that introduces shared infrastructure dependencies (external databases, long-running services, network-accessible state) is not acceptable. Local-only convenience persistence for the CLI is the only exception to the stateless model.

### 4.4 Secrets Handling

API keys and tokens are **always** sourced from environment variables. They are never:
- Stored in `bop.yaml`
- Logged to stdout or stderr
- Written into comment metadata or any GitHub-posted content
- Committed to the repository in any form

When a required environment variable is missing, Bop must emit a clear error identifying which variable is absent and how to set it.

### 4.5 CLI / MCP Parity

Capability parity between `bop` and `bop-mcp` is a hard constraint, not a goal. When adding a new operation, it must be wired up in both the CLI command layer and the MCP tool handler layer before the feature is considered complete. This is enforced by convention and should be verified in code review.

### 4.6 Configuration Philosophy

`bop.yaml` should be entirely optional. All defaults must be genuinely useful. When adding configuration options, ask whether the feature works well without the option before adding it. Prefer conventions over required configuration.

### 4.7 Error Handling

Errors must be surfaced with enough context to identify the cause and a suggested corrective action where possible. A failure in one persona's LLM call must not abort the entire review pipeline — the pipeline continues with the findings it has and notes the failure. Transient API errors (GitHub, LLM) should be retried with appropriate backoff before surfacing as failures.

### 4.8 Build and Distribution

Binaries are statically compiled for the following targets:
- `linux/amd64`
- `linux/arm64`
- `macos/amd64`
- `macos/arm64`
- `windows/amd64`

No runtime dependencies beyond the OS are acceptable. No CGo. Builds must produce a single executable file that can be downloaded and run without installation.

### 4.9 Repository Layout (Conventions)

```
/
├── cmd/
│   ├── bop/                    # CLI entry point (Cobra wiring, env loading)
│   └── bop-mcp/                # MCP server entry point (MCP protocol, delegation)
├── internal/                    # Core library (clean architecture)
│   ├── domain/                  # Core entities — no external dependencies
│   │   ├── finding.go           #   Finding, annotation, reviewer, session types
│   │   └── ...
│   ├── usecase/                 # Business logic orchestration
│   │   ├── review/              #   Review orchestrator, prompt building, planning
│   │   ├── triage/              #   PR triage service
│   │   ├── merge/               #   Intelligent multi-provider consensus merging
│   │   ├── dedup/               #   Deduplication coordination
│   │   ├── post/                #   Finding posting/publishing
│   │   ├── verify/              #   Verification orchestration
│   │   ├── session/             #   Session management
│   │   ├── github/              #   GitHub-specific usecase logic
│   │   └── skip/                #   Review skip logic
│   ├── adapter/                 # External integrations and I/O boundaries
│   │   ├── cli/                 #   Cobra commands and flag resolution
│   │   ├── git/                 #   Git repository operations (go-git)
│   │   ├── github/              #   GitHub API client (GraphQL + REST)
│   │   ├── llm/                 #   LLM providers (anthropic/, gemini/, openai/, ollama/, sampling/)
│   │   ├── mcp/                 #   MCP server and tool handlers
│   │   ├── output/              #   Format writers (json/, markdown/, sarif/)
│   │   ├── dedup/               #   Dedup adapter (Anthropic-backed semantic dedup)
│   │   ├── store/               #   SQLite persistence
│   │   ├── session/             #   Session storage adapter
│   │   ├── verify/              #   Verification agent and tools
│   │   ├── theme/               #   Theme/context extraction
│   │   ├── repository/          #   Repository abstraction
│   │   └── observability/       #   Structured logging
│   ├── config/                  # Configuration loading, embedding, merging
│   │   └── embed/               #   Embedded default bop.yaml
│   ├── platform/                # Bop platform auth and config sync
│   ├── diff/                    # Diff parsing and analysis
│   ├── redaction/               # Secret redaction
│   ├── pathutil/                # Path validation utilities
│   ├── determinism/             # Seed generation for reproducibility
│   ├── store/                   # Store abstraction layer
│   └── version/                 # Build-time version injection
├── action/                      # GitHub Action (composite action.yml)
├── security-tests/              # Security test cases
├── .github/
│   └── workflows/               # CI, code review, and release workflows
├── bop.yaml                     # Bop's own reviewer panel config
├── magefile.go                  # Mage build targets
└── .goreleaser.yml              # Release automation config
```

---

## 5. Key Decisions

### Decision 1: Two Binaries, One Core Library

**Choice:** `bop` and `bop-mcp` are separate compiled binaries that share all business logic through a common internal library.

**Rationale:** Keeping them as separate binaries allows each to be distributed independently and invoked in the manner natural to its context (direct execution vs. MCP server process). Sharing a core library ensures capability parity is structurally enforced — there is only one implementation of the review pipeline, the persona engine, the dedup pipeline, and the GitHub client. Adding a new capability once in the core library makes it available to both surfaces.

### Decision 2: GitHub Comment Metadata as Persistence Layer

**Choice:** All cross-round state (prior findings context) is stored in metadata embedded in GitHub PR comments. No external database or storage system is used.

**Rationale:** Stateless execution dramatically reduces operational complexity for users. There is nothing to provision, nothing to keep running, and nothing to migrate. State is co-located with the PR it describes and has the same access controls. The tradeoff is that persistence is bounded by what GitHub's comment metadata supports, and state is lost if Bop's comments are deleted — both acceptable tradeoffs given the operational simplicity gained.

### Decision 3: Statically Compiled Go Binaries

**Choice:** Both binaries are statically compiled Go with no CGo, distributed as single executable files.

**Rationale:** A single binary download is the lowest-friction distribution mechanism. Users in GitHub Actions, on developer machines, and in MCP server contexts all benefit from not needing a runtime, package manager, or system dependency. Go's static compilation and cross-compilation support make multi-platform distribution straightforward. This choice directly enables the "just download and run" experience targeted by NFR-PORT-2.

### Decision 4: Multi-Layer Deduplication with Semantic Dedup and Confidence Scoring

**Choice:** Findings are processed through at least two deduplication passes (exact and semantic) and assigned confidence scores before synthesis.

**Rationale:** A naive multi-persona review that concatenates all findings would produce a noisy, repetitive output that is worse than a single-persona review. The deduplication and confidence pipeline is what makes multi-persona review net-positive. This is treated as a core feature of the system, not an optimization — per VISION.md principle 4.

### Decision 5: Per-Persona Model Configuration

**Choice:** Each reviewer persona specifies its own LLM provider and model, independently of other personas.

**Rationale:** Different review tasks benefit from different models. A security reviewer may warrant a more capable (and expensive) model; a style reviewer may not. Teams may have existing contracts or preferences for specific providers. Allowing per-persona model selection also enables hybrid setups — for example, using Ollama for some personas when data privacy is a concern while using a cloud provider for others.

### Decision 6: `bop.yaml` is Optional with Useful Defaults

**Choice:** Bop ships with a default reviewer persona and operates fully without any `bop.yaml` present.

**Rationale:** If Bop required configuration to produce useful output, adoption would be gated behind a configuration authoring step. The default persona must deliver real value on a real project to demonstrate Bop's capabilities before a user invests in customization. This follows VISION.md principle 2: sensible defaults must be genuinely useful, not merely present.

### Decision 7: GITHUB_TOKEN for Authentication

**Choice:** Bop uses `GITHUB_TOKEN` for GitHub API authentication in all deployment contexts.

**Rationale:** In GitHub Actions, `GITHUB_TOKEN` is automatically injected, making setup minimal for the primary use case. For local and MCP usage, the same mechanism works with a user-supplied PAT. This avoids the need for a dedicated GitHub App registration for basic usage, reducing the setup burden, particularly for the target audience of solo developers and small teams.

### Decision 8: MCP Sampling Fallback

**Choice:** When `bop-mcp` is invoked without direct LLM API keys in the environment, it falls back to MCP sampling — delegating LLM calls to the MCP client (e.g., the connected AI assistant) rather than calling provider APIs directly.

**Rationale:** This enables zero-configuration MCP usage. A developer using Claude Code or another MCP-compliant assistant can use `bop-mcp` for code review without setting up separate API keys — the assistant's own model serves as the review backend. Direct API key usage remains available (and preferred for CI) but sampling removes a setup barrier for the interactive local use case.

### Decision 9: SQLite for Local Session Persistence

**Choice:** The CLI stores review sessions in a local SQLite database (`~/.config/bop/reviews.db`) when running in local review mode.

**Rationale:** Local session history improves the developer experience for interactive reviews — enabling re-review, session listing, and result recall without re-running the pipeline. SQLite was chosen because it requires no server process, is embedded in the binary (via `modernc.org/sqlite`, pure Go, no CGo), and aligns with the "just download and run" distribution model. This persistence is strictly local and optional — CI mode and MCP mode do not use it, and deleting the database has no effect on correctness.

### Decision 10: LLM Provider Coverage Including Ollama

**Choice:** Bop supports Anthropic, Google Gemini, OpenAI, and Ollama as LLM providers.

**Rationale:** Provider diversity gives users flexibility on cost, capability, and data residency. Ollama specifically addresses teams and developers with data privacy requirements that prevent sending code to external APIs. The default (Claude Sonnet via Anthropic) is chosen for quality; alternatives exist for teams with different constraints or preferences.
