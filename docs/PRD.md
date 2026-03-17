# Bop — Product Requirements Document

## Overview

Bop is a multi-persona code review agent written in Go that delivers structured, senior-level code review feedback on GitHub pull requests. It operates as a configurable panel of reviewer personas — each with its own prompt, model, and weighting — that independently analyze a PR diff and produce findings that are then deduplicated, semantically merged, confidence-scored, and synthesized into a single unified review posted to the PR. Bop is distributed as two statically compiled binaries (`bop` CLI and `bop-mcp` MCP server) sharing a common core library. It runs as a GitHub Actions step, as a local CLI tool against local checkouts or remote PRs, and as an MCP server exposable to any MCP-compliant coding assistant. State is persisted across review rounds entirely through GitHub comment metadata — no external database, no long-running service, no infrastructure to manage. Bop is an open source project under the `delightfulhammers` GitHub organization, currently at v0.12.3 and in active beta testing.

---

## User Journeys

### Journey 1: GitHub Actions Automated Review on PR Push

1. A developer pushes commits to a branch and opens (or updates) a pull request on GitHub.
2. A GitHub Actions workflow configured in the repository triggers on the pull request event.
3. The workflow includes a step that invokes the `bop` binary, passing the repository and PR context via environment variables including `GITHUB_TOKEN` and any required LLM API keys.
4. Bop reads the `bop.yaml` file from the repository root (if present) to determine the reviewer panel configuration; if absent, it uses the built-in default reviewer persona.
5. Each configured persona independently reviews the PR diff using its assigned LLM model and prompt configuration, producing a set of weighted findings.
6. Bop runs the findings through its multi-layer deduplication pipeline, including semantic deduplication and confidence scoring, collapsing redundant or contradictory findings across personas.
7. Previous round findings from prior review comments (retrieved via GitHub comment metadata) are injected into the review context, instructing reviewers not to repeat previously surfaced issues.
8. The synthesized findings are posted to the pull request as review comments, attributed to Bop, formatted as a unified review.
9. On subsequent pushes to the same PR, the cycle repeats with prior findings included in context, producing iterative, non-repetitive reviews throughout the PR's lifecycle.

### Journey 2: Local CLI Review of a Remote PR

1. A developer is working locally and wants to get a Bop review on an open PR before pushing additional changes.
2. They invoke `bop` from the command line, providing the target repository and PR identifier, with `GITHUB_TOKEN` and relevant LLM API keys set in their environment.
3. Bop fetches the PR diff and any existing Bop review comments (including persisted metadata) from GitHub via the REST API.
4. The configured reviewer panel runs against the diff, producing findings that are deduplicated and synthesized as in the Actions workflow.
5. The developer receives the synthesized review output in their terminal and/or posted as PR comments, depending on flags provided.
6. The developer can also use the CLI to retrieve prior findings, post a response to a finding, dismiss a review, or re-request a review — all without leaving their local environment.

### Journey 3: MCP Server-Driven Review from a Coding Assistant

1. A developer using an MCP-compliant coding assistant (e.g., an AI IDE or chat tool) has `bop-mcp` running and registered as an MCP server.
2. The developer asks their assistant to run a code review on the current PR.
3. The assistant invokes `bop-mcp` capabilities via the MCP protocol, triggering the full Bop review pipeline against the specified PR.
4. The review findings are returned to the assistant, which presents them in the development environment.
5. The developer instructs the assistant to dismiss a specific finding, post a response to a comment, or re-request a review on an updated diff — all handled by `bop-mcp` relaying operations to the GitHub API.
6. Every action available through the `bop` CLI is equally available through `bop-mcp`, with no capability gap between the two interfaces.

### Journey 4: Custom Reviewer Panel Configuration

1. A project maintainer wants to configure a multi-persona review panel tailored to their project's priorities.
2. They create a `bop.yaml` file in the repository root, defining reviewer personas — for example, a security reviewer, an architecture reviewer, and a performance reviewer — in addition to or replacing the default reviewer.
3. For each persona, the maintainer specifies the prompt, the LLM model to use, the weight given to that reviewer's findings, and whether the persona is included in the default review panel.
4. The `bop.yaml` is committed to the repository, and on the next PR event, Bop uses the project-specific panel rather than the built-in default.
5. The weighted findings from each configured persona flow through the deduplication and synthesis pipeline and are posted as a unified review reflecting the project's review priorities.

---

## Requirements

### Core Review Pipeline

- **R-CRP-1:** Bop must support a multi-persona review model where multiple reviewer personas independently analyze the same PR diff and contribute findings to a unified review.
- **R-CRP-2:** Findings from all active personas must be passed through a multi-layer deduplication pipeline before being posted. Deduplication must include at minimum: exact/near-exact duplicate removal and semantic deduplication (identifying findings that address the same issue in different language).
- **R-CRP-3:** Each finding must be assigned a confidence score as part of the synthesis pipeline. Lower-confidence findings may be surfaced with reduced prominence or filtered, consistent with the synthesis model.
- **R-CRP-4:** The synthesized output posted to the PR must be a single unified review, not separate posts per persona.
- **R-CRP-5:** Prior round findings — retrieved from GitHub comment metadata — must be injected into the per-persona review prompt for subsequent review rounds, instructing each reviewer not to re-surface issues already identified.
- **R-CRP-6:** The review pipeline must operate on the PR diff as its primary input. Full file contents may be retrieved as needed for context, but the diff is the primary unit of analysis.
- **R-CRP-7:** Findings may optionally be passed through a verification agent that independently re-examines each finding against the code context before posting, reducing false positives. Verification must be configurable (enabled/disabled via CLI flags and `bop.yaml`).

### Persona Configuration

- **R-PC-1:** Bop must ship with at least one default reviewer persona that produces useful, substantive reviews on any project without requiring any configuration.
- **R-PC-2:** Each reviewer persona must be independently configurable with the following attributes:
  - **Prompt:** The system/user prompt sent to the LLM for that persona's review.
  - **Model:** The specific LLM model used for that persona's review (e.g., `claude-sonnet-4-5`, `gemini-2.0-flash`, `gpt-4o`, or a locally hosted Ollama model).
  - **Finding weight:** A numeric or categorical weight applied to that persona's findings during synthesis.
  - **Default panel inclusion:** A boolean flag indicating whether the persona is active by default without explicit panel configuration.
- **R-PC-3:** Project-level persona configuration must be specified via a `bop.yaml` file placed in the repository root.
- **R-PC-4:** If no `bop.yaml` is present, Bop must fall back to its built-in default reviewer panel without error.
- **R-PC-5:** The repository must include example persona configurations demonstrating at minimum: a security reviewer, an architecture reviewer, and a performance reviewer.

### LLM Provider Support

- **R-LLM-1:** Bop must support Anthropic Claude as an LLM provider. The default reviewer persona must use Claude Sonnet as its model.
- **R-LLM-2:** Bop must support Google Gemini as an LLM provider.
- **R-LLM-3:** Bop must support OpenAI as an LLM provider.
- **R-LLM-4:** Bop must support Ollama as an LLM provider, enabling locally hosted models to be used as persona backends.
- **R-LLM-5:** LLM provider API keys must be sourced from environment variables. No API keys should be stored in `bop.yaml` or committed to the repository.
- **R-LLM-6:** Bop must surface a clear, actionable error when a required LLM API key is missing from the environment.

### GitHub Integration

- **R-GH-1:** Bop must authenticate with the GitHub API using a `GITHUB_TOKEN` sourced from the environment (either the Actions-injected token or a user-supplied PAT/token for local use).
- **R-GH-2:** Bop must be able to retrieve PR diffs, existing PR comments, and comment metadata from the GitHub REST API.
- **R-GH-3:** Bop must post synthesized review findings as comments on the target pull request.
- **R-GH-4:** Bop must store review round metadata in GitHub comment metadata to enable cross-round context persistence without any external database.
- **R-GH-5:** Bop must support posting responses to existing PR review comments.
- **R-GH-6:** Bop must support dismissing Bop-posted reviews on a PR.
- **R-GH-7:** Bop must support re-requesting a review on a PR (triggering a fresh review round).
- **R-GH-8:** Bop must include an example GitHub Actions workflow in the repository that demonstrates correct configuration, including required workflow permissions.

### GitHub Actions Integration

- **R-GA-1:** The `bop` binary must be invocable as a step in a GitHub Actions workflow without requiring a Docker container or custom action wrapper.
- **R-GA-2:** The example workflow must demonstrate how to supply `GITHUB_TOKEN` and LLM API keys to the Bop step.
- **R-GA-3:** Bop must operate correctly within the GitHub Actions runner environment, reading PR context from standard Actions environment variables.

### CLI (`bop`)

- **R-CLI-1:** `bop` must support local checkout review mode, analyzing a locally checked-out branch or diff without requiring a live GitHub PR.
- **R-CLI-2:** `bop` must support remote PR review mode, fetching a PR from GitHub by repository and PR number and running the full review pipeline.
- **R-CLI-3:** `bop` must support retrieving prior findings from a PR (via GitHub comment metadata).
- **R-CLI-4:** `bop` must support posting a response to a PR comment.
- **R-CLI-5:** `bop` must support dismissing a Bop review on a PR.
- **R-CLI-6:** `bop` must support re-requesting a review on a PR.
- **R-CLI-7:** The CLI must provide clear help output and usage instructions for all commands and flags.

### MCP Server (`bop-mcp`)

- **R-MCP-1:** `bop-mcp` must implement the Model Context Protocol and be usable as an MCP server by any MCP-compliant client.
- **R-MCP-2:** `bop-mcp` must expose all capabilities available in the `bop` CLI, including: running code reviews, retrieving findings, posting responses to comments, dismissing reviews, and re-requesting reviews.
- **R-MCP-3:** There must be no capability gap between `bop` and `bop-mcp`. Any feature added to one binary must be made available in the other.
- **R-MCP-4:** `bop-mcp` must read `GITHUB_TOKEN` and LLM API keys from the environment, consistent with CLI behavior.
- **R-MCP-5:** When LLM API keys are not available in the environment, `bop-mcp` must fall back to MCP sampling — delegating LLM calls to the connected MCP client — enabling zero-configuration usage from MCP-compliant coding assistants.

### Binary and Architecture

- **R-ARCH-1:** Both `bop` and `bop-mcp` must be statically compiled Go binaries with no runtime dependencies beyond the operating system.
- **R-ARCH-2:** Both binaries must share a single core library containing the review pipeline, persona configuration, GitHub API client, and LLM provider integrations.
- **R-ARCH-3:** Bop must operate without external infrastructure: no external database, no long-running daemon process, and no shared state service is required for any supported use case. Local CLI mode may use an embedded SQLite database for session convenience, but this storage must be strictly optional — its absence must not affect review correctness.
- **R-ARCH-4:** State across review rounds must be persisted via GitHub comment metadata on the PR being reviewed. Local session history (for CLI convenience) may additionally be stored in an embedded SQLite database.

---

## Non-Functional Requirements

### Performance

- **NFR-P-1:** Bop must complete a standard review pipeline (multi-persona review, deduplication, synthesis, and posting) within a time envelope acceptable for use in an automated CI step, accounting for LLM API latency. Persona reviews should be parallelized where possible to reduce total wall time.
- **NFR-P-2:** Bop must be capable of handling PRs with large diffs without crashing or producing malformed output. Context window limits of the target LLM should be handled gracefully (e.g., via chunking or summarization).

### Reliability

- **NFR-R-1:** Bop must handle transient GitHub API errors and LLM API errors gracefully, with appropriate retry logic and clear error messaging.
- **NFR-R-2:** A failure in one persona's review must not prevent the remaining personas from completing their reviews. The synthesis pipeline must proceed with the available findings.
- **NFR-R-3:** Bop must not post duplicate review comments to a PR if a review round is re-run against the same diff without new commits.

### Security

- **NFR-S-1:** No secrets (API keys, tokens) should ever be written to logs, standard output, or comment metadata. Bop must treat all credential values as sensitive.
- **NFR-S-2:** The `bop.yaml` configuration file must not be a valid location for specifying secrets. All credentials must come from the environment.
- **NFR-S-3:** Bop must operate within the permissions granted by the provided `GITHUB_TOKEN` and must not request or store elevated permissions beyond what is necessary.

### Usability

- **NFR-U-1:** A developer with no prior experience with Bop must be able to add it to a GitHub Actions workflow and get a working review on their first PR using only the example workflow and README.
- **NFR-U-2:** The `bop.yaml` configuration format must be self-documenting enough that a developer can write a custom persona configuration without reading source code.
- **NFR-U-3:** All error messages must identify the likely cause and suggest a corrective action where applicable.

### Portability

- **NFR-PORT-1:** The `bop` and `bop-mcp` binaries must be distributable for Linux (amd64, arm64), macOS (amd64, arm64), and Windows (amd64) platforms.
- **NFR-PORT-2:** No installation beyond downloading the binary is required for local CLI use. No package manager, runtime, or system library dependency is acceptable.

---

## Success Criteria

- **SC-1:** Bop successfully completes the full review pipeline — multi-persona review, deduplication, synthesis, and comment posting — on a real GitHub PR via GitHub Actions with no manual intervention beyond initial setup.
- **SC-2:** A project using only the default reviewer persona (no `bop.yaml`) receives substantive, actionable review findings that a developer finds useful.
- **SC-3:** A project with a custom `bop.yaml` defining at least three distinct personas (e.g., security, architecture, performance) receives a correctly synthesized unified review reflecting the weighted findings of all configured personas.
- **SC-4:** Subsequent review rounds on the same PR do not re-surface findings already identified and posted in prior rounds, demonstrating correct cross-round context injection.
- **SC-5:** All CLI review and management operations (retrieve findings, post response, dismiss review, re-request review) execute successfully in both local checkout and remote PR modes.
- **SC-6:** All capabilities exposed by `bop` CLI are equally accessible and functional via `bop-mcp` with no observed behavioral differences.
- **SC-7:** Beta testers representing solo developers, small teams, and larger engineering teams report that Bop surfaces relevant, non-repetitive review findings at a pace and quality consistent with a senior code reviewer.
- **SC-8:** The project reaches a stable v1.0 release with no known critical bugs, passing all defined acceptance criteria, and documented upgrade guidance from the current v0.12.3 baseline.

---

## Out of Scope

- **Auto-fixing code:** Bop will not modify source files, open automated fix PRs, apply patches, or take any action that alters code. Bop's output is review commentary only.
- **GitLab support:** Integration with GitLab repositories, merge requests, or CI pipelines is not planned. A tracking issue exists for future consideration.
- **Bitbucket support:** Integration with Bitbucket repositories or pipelines is not planned. A tracking issue exists for future consideration.
- **CI systems other than GitHub Actions:** First-class integration with Jenkins, CircleCI, GitLab CI, Buildkite, or other CI platforms is not planned. A tracking issue exists for future consideration.
