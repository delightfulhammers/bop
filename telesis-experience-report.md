## Telesis End-to-End Experience Report

### Context
First-time `telesis init` on a mature Go project (bop, v0.12.3, ~280 Go files). The goal was to bootstrap project documentation, then run a full intake → plan → dispatch → review → ship pipeline on a real GitHub issue.

### What Worked Well
- **Note system** — Adding notes via CLI and having them flow into the regenerated CLAUDE.md was straightforward. The tag-based categorization is intuitive.
- **Plan generation** — `telesis intake approve --plan` produced a well-scoped, accurate task decomposition for issue #317. It correctly identified the file, the line range, the TDD approach, and even included the project's Definition of Done checklist.
- **Review convergence** — `telesis review` ran a multi-persona review (security, architecture, correctness) and converged in one round with zero findings. Fast (~1.7s) and cheap (~$0.04).
- **Drift detection** — Useful as a pre-commit sanity check. The substantive checks (CLAUDE.md freshness, stale references, milestone consistency) all worked correctly.

### Friction Points

**1. `telesis init` overwrites CLAUDE.md without preservation**
The existing CLAUDE.md (~300 lines of operational context) was replaced wholesale. There was no prompt to merge, diff, or preserve existing content. Recovery required manually diffing against `git show HEAD:CLAUDE.md` and re-adding ~9 notes. For a project that already has a well-maintained CLAUDE.md, init should at minimum warn or offer to merge.

**2. Generated docs contained hallucinated codebase structure**
ARCHITECTURE.md Section 4.9 showed a completely fabricated `internal/` layout (`pipeline/`, `persona/`, `dedup/`, `github/`, `llm/`) that didn't match the actual clean architecture structure (`adapter/`, `domain/`, `usecase/`, `config/`). The doc also claimed pure statelessness despite SQLite session storage existing. It appears the generation inferred structure from conceptual descriptions rather than inspecting the actual directory tree. Several sections across ARCHITECTURE.md and PRD.md needed manual correction.

**3. MCP server not auto-configured after init**
`telesis init` did not create a `.mcp.json` in the project. The MCP tools were unavailable until we manually copied the config from a sibling telesis project directory. For an MCP-first workflow, init should either create this file or prompt to do so.

**4. MCP tool coverage gaps — read-only for intake and dispatch**
The MCP server exposes `intake_list` and `intake_show` but not `intake github` (import) or `intake approve`. Similarly, `dispatch_list` and `dispatch_show` exist but not `dispatch run`. We had to fall back to the CLI for all write operations in the intake and dispatch phases. The MCP tools should have full parity with the CLI for the core pipeline actions.

**5. Large MCP responses exceed token limits**
Both `telesis_intake_list` (145K chars for 76 issues) and `telesis_dispatch_show` (82K chars for a 323-event session) exceeded MCP response size limits. We had to fall back to CLI with grep/jq to extract what we needed. These tools would benefit from pagination, filtering, or summary modes.

**6. Note tag duplication in generated CLAUDE.md**
Notes with multiple tags are rendered under *every* tag heading. A note tagged `[rules, pitfalls]` appears identically under both `### pitfalls` and `### rules`. With 9 notes averaging 2 tags each, the CLAUDE.md Development Notes section contains significant redundancy. Either deduplicate at render time or use a primary-tag model.

**7. Drift check false positive for Go projects**
The `expected-directories` check flagged 24 missing `src/` directories — these are telesis's own internal package conventions, not anything related to the target project. This check should either be scoped to telesis projects only or made configurable per-project.

**8. `GITHUB_TOKEN` not inherited from environment**
`telesis intake github` required `GITHUB_TOKEN` explicitly, even though `gh auth token` was available. Most GitHub-integrated CLI tools respect the `gh` auth chain or check the standard token locations. Having to wrap the command in `GITHUB_TOKEN=$(gh auth token) telesis intake github` adds friction.

### Summary
The core value proposition — structured documentation, multi-persona review, convergence detection — works well. The main gaps are around the **init experience for existing projects** (destructive CLAUDE.md overwrite, hallucinated structure), **MCP tool completeness** (read-only where read-write is needed), and **response sizing** (no pagination for large result sets). These are all tractable issues that would significantly smooth the workflow once addressed.
