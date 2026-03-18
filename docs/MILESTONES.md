# Bop — Milestones

> This document defines the development roadmap from the current v0.12.4 beta state through stable v1.0 release and beyond. Each milestone has a clear goal, testable acceptance criteria, and an ordered build sequence. See [PRD.md](PRD.md) for full feature requirements and [ARCHITECTURE.md](ARCHITECTURE.md) for structural context.

---

## Current State

**Version:** v0.12.4
**Status:** Feature-complete beta — all core capabilities are implemented and the project is in active beta testing with real users across solo developer, small team, and enterprise contexts.

**Recent additions (post-beta baseline):**
- CLI-to-platform bridge with device flow auth and config sync (`bop auth`)
- GitHub Enterprise Server support via `GITHUB_API_URL`
- MCP sampling fallback for zero-config MCP usage
- Finding verification agent

The milestones below are organized around hardening, stabilization, and release readiness rather than net-new feature development.

---

## Milestone 1 — Beta Stabilization

**Status:** 🔄 In Progress

### Goal
Resolve all critical and high-severity issues surfaced by beta testers and establish a reliable, well-documented baseline that consistently passes defined quality gates.

### Acceptance Criteria

1. All issues labeled `critical` or `high` in the GitHub issue tracker are closed or triaged with a documented resolution decision.
2. The full review pipeline — multi-persona review, multi-layer deduplication, semantic dedup, confidence scoring, synthesis, and PR comment posting — completes successfully on a PR with at least three active personas (e.g., default, security, architecture) in a GitHub Actions workflow without manual intervention.
3. Cross-round context injection is verified: a finding surfaced in round N is not re-surfaced in round N+1 when the same issue remains unresolved, confirmed by manual inspection of at least two consecutive review rounds on a live PR.
4. All four LLM providers (Anthropic, Google Gemini, OpenAI, Ollama) are tested end-to-end in at least one persona configuration each, with no provider-specific crashes or malformed outputs.
5. A persona failure (e.g., LLM API error for one persona) does not abort the review pipeline; the synthesis proceeds with the remaining personas' findings and the failure is reported in output without crashing.
6. No secrets (API keys, `GITHUB_TOKEN`) appear in any log output, standard output, error output, or GitHub comment content under any execution path.
7. The `bop` CLI and `bop-mcp` server expose identical capabilities: run review, retrieve findings, post response to comment, dismiss review, re-request review — with no operation available in one binary that is absent or non-functional in the other.
8. All error messages for missing environment variables (absent `GITHUB_TOKEN` or LLM API key) identify the specific missing variable by name and include a corrective action in the message.
9. `bop --help` and per-command help output is complete, accurate, and consistent with actual behavior for all commands and flags.
10. The example GitHub Actions workflow in `.github/workflows/` executes successfully in a real repository, produces a review comment on a PR, and documents all required workflow permissions.

### Build Sequence

**Phase 1 — Triage and Prioritization**
- Audit all open issues from beta tester reports
- Label and prioritize: `critical`, `high`, `medium`, `low`
- Document any issues that are explicitly deferred to a later milestone with rationale

**Phase 2 — Critical Bug Fixes**
- Resolve all `critical`-labeled issues in priority order
- For each fix: reproduce the failure case, implement fix, verify the reproduction case no longer triggers
- Verify pipeline resilience: persona failure isolation, retry logic for transient GitHub and LLM API errors

**Phase 3 — CLI and MCP Parity Audit**
- Enumerate all capabilities exposed by `bop` CLI
- Verify each capability is present and functional in `bop-mcp`
- Implement any missing MCP tool handlers; wire up any missing CLI commands
- Document parity confirmation in a checklist committed to the repository

**Phase 4 — Security Hardening**
- Audit all log and output paths for accidental secret exposure
- Verify `bop.yaml` cannot be used to specify secrets (no secret-looking fields accepted or parsed)
- Review GitHub comment metadata content for any inadvertent credential inclusion
- Confirm `GITHUB_TOKEN` scope is not broader than required for all documented operations

**Phase 5 — Documentation and Example Verification**
- Run the example GitHub Actions workflow in the `delightfulhammers` org against a real PR
- Update workflow permissions documentation to match current requirements
- Review and update all `--help` output for accuracy
- Verify the example `bop.yaml` with security, architecture, and performance personas produces correct output end-to-end

---

## Milestone 2 — Hardening and Observability

**Status:** 📋 Planned

### Goal
Make Bop production-reliable by completing the test suite, improving operational visibility, and validating behavior at edge cases that real-world PRs will trigger.

### Acceptance Criteria

1. Unit test coverage exists for the deduplication pipeline (exact dedup, semantic dedup, and confidence scoring) with at least 10 representative test cases covering both true positive deduplication and false positive prevention.
2. Integration tests cover the full review pipeline end-to-end against a mocked GitHub API and mocked LLM responses, executing successfully in CI on every push to the main branch.
3. Bop handles a PR diff that exceeds a single LLM context window without crashing or silently truncating input; the behavior (chunking, summarization, or graceful degradation with a user-visible notice) is documented.
4. Bop handles a PR with no diff (empty changeset) gracefully — no review is posted, and a clear message is emitted explaining why.
5. Bop does not post duplicate review comments when a review round is re-run against the same diff without new commits; this is verified by running the pipeline twice consecutively on an unchanged PR.
6. Transient GitHub API errors (simulated 5xx responses) and transient LLM API errors trigger retry logic with exponential backoff and are retried at least twice before surfacing as terminal errors.
7. All retry attempts and terminal errors are logged with enough context (operation, error type, attempt count) to diagnose failures from log output alone.
8. Statically compiled binaries are produced in CI for all five target platforms: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` — with no CGo — and each binary executes without error on its target platform.
9. The `bop-mcp` MCP server registers and responds to all tool calls correctly when connected to at least one MCP-compliant client (e.g., Claude Desktop or an MCP test harness).
10. Local checkout review mode and remote PR review mode both complete successfully from a developer's local machine with `GITHUB_TOKEN` set in the environment.

### Rough Scope

- Implement and expand unit test coverage for the deduplication pipeline and persona engine
- Implement integration test harness with mocked GitHub and LLM APIs
- Add CI pipeline that builds all platform targets and runs all tests on every push
- Implement large-diff handling (context window management strategy)
- Implement and test duplicate-post prevention logic
- Implement retry logic with exponential backoff for GitHub and LLM API calls
- Improve structured logging throughout the review pipeline
- Verify MCP server behavior against a real MCP client

---

## Milestone 3 — v1.0 Release Candidate

**Status:** 📋 Planned

### Goal
Produce a release candidate that meets the quality, documentation, and usability bar required for a stable v1.0 public release — suitable for adoption by users who are not beta testers and who expect production-grade reliability.

### Acceptance Criteria

1. All acceptance criteria from Milestones 1 and 2 continue to pass with no regressions.
2. A `README.md` is complete and enables a developer with no prior Bop experience to add Bop to a GitHub Actions workflow and receive a working review on their first PR, using only the README and the example workflow — verified by a first-time user walkthrough.
3. The `bop.yaml` configuration reference is fully documented: every supported field, its type, its default value, and a usage example — sufficient that a developer can author a custom persona configuration without reading source code.
4. An upgrade guide is published documenting any breaking changes or behavioral differences between v0.12.x and v1.0, with specific callouts for `bop.yaml` schema changes if any exist.
5. Release binaries for all five platforms are published to GitHub Releases with SHA-256 checksums and a changelog derived from the issue tracker.
6. The `bop` default reviewer persona produces substantive, actionable review findings on at least three distinct real-world repositories (different languages and project types), confirmed by feedback from beta testers representing solo developer, small team, and at least one larger team context.
7. No known `critical` or `high` severity bugs exist at the time of RC tag.
8. The project's own `bop.yaml` (with security, architecture, and performance personas) has been actively used on the `delightfulhammers/bop` repository's own PRs for at least two weeks without triggering any `critical` or `high` severity issues.
9. Version strings, changelog, and release notes are consistent across the binary (`bop version`), GitHub Release, and repository tags.
10. Tracking issues for out-of-scope items (GitLab support, Bitbucket support, non-GitHub-Actions CI systems) are documented in `ROADMAP.md` or equivalent with enough context that future contributors can evaluate scope.

### Rough Scope

- Complete README, `bop.yaml` reference documentation, and upgrade guide
- Implement `bop version` command outputting version, commit, and build metadata
- Set up GitHub Releases pipeline with multi-platform binary publishing and checksum generation
- Finalize `bop.yaml` schema (no breaking changes after RC tag)
- Conduct structured beta tester review session and resolve any newly surfaced `high`+ issues
- Create `ROADMAP.md` documenting deferred platform and integration work
- Tag v1.0-rc.1 and announce to beta testers for final validation

---

## Milestone 4 — v1.0 Stable and Post-Launch

**Status:** 📋 Planned

### Goal
Ship v1.0 stable, establish a sustainable post-launch maintenance rhythm, and complete the highest-priority items surfaced during the RC period.

### Acceptance Criteria

1. v1.0 is tagged and published with release binaries for all five platforms, SHA-256 checksums, and a complete changelog.
2. The v1.0-rc.1 release candidate has been available for at least one week of final beta tester validation with no newly reported `critical` or `high` severity issues remaining open.
3. A `CONTRIBUTING.md` is published that documents how to build the project locally, run the test suite, contribute bug fixes, and propose new features — including the CLI/MCP parity requirement and the stateless execution model constraint.
4. The CI pipeline enforces: all tests pass, all platform binaries build successfully, and no secrets-handling regressions (static analysis or test-based) before any merge to the main branch.
5. A triage and response SLA is documented for incoming issues: `critical` issues receive an initial response within 48 hours of the v1.0 release; `high` issues within one week.
6. The project's own Bop review panel has been running continuously on `delightfulhammers/bop` PRs through the RC and stable release period with no unhandled panics or data loss events.
7. Bop handles the "AI coding assistant pace" scenario — PRs that arrive in rapid succession from an AI assistant — without race conditions, duplicate posts, or missed review rounds, verified by a simulated rapid-PR test sequence.

### Rough Scope

- Address all issues surfaced during RC validation period
- Publish v1.0 stable tag and release
- Publish `CONTRIBUTING.md`
- Establish issue triage labels and response time documentation
- Post v1.0 announcement in appropriate developer communities
- Begin backlog grooming for post-v1.0 work (GitLab tracking issue, Bitbucket tracking issue, CI system tracking issues) to inform the next planning cycle
