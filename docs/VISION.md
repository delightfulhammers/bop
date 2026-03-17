# Bop — A Multi-Persona Code Review Agent That Keeps Up With You

> Structured, senior-level code review for every pull request — automated, opinionated, and fast enough to match the pace of AI-assisted development.

---

## The Problem

Modern development workflows have a throughput problem. AI coding assistants can generate substantial code changes in minutes. Pull requests arrive faster, carry more surface area, and demand the same quality scrutiny as ever — but the humans responsible for reviewing them haven't scaled. For solo developers, there may be no reviewer at all. For small teams, the reviewer is also the one shipping. For larger organizations, review quality becomes inconsistent under pressure.

The result is code that gets merged without the structural, security, and performance questions it deserves — not because the team doesn't care, but because the review bandwidth simply isn't there.

---

## The Vision

Bop exists to make senior-level code review universally available — on every PR, every round, regardless of team size or review backlog.

When Bop succeeds, a solo developer working with an AI assistant gets the same quality feedback loop as an engineer at a well-staffed company with dedicated security, architecture, and performance reviewers. A small team shipping fast doesn't have to choose between review quality and velocity. An enterprise team gets consistent, configurable review standards applied uniformly across every contribution.

Bop doesn't replace human judgment. It ensures that by the time a human looks at a PR — or a developer merges their own work — the obvious and not-so-obvious problems have already been surfaced, deduplicated, and ranked by confidence. The bar is raised before anyone has to raise it manually.

---

## Principles

**1. Statelessness is a feature, not a constraint.**
Bop should never require an external database, a long-running service, or shared infrastructure to function correctly. GitHub's own comment metadata is the persistence layer. Any design decision that trades away stateless operation for convenience should be rejected — the operational simplicity this enables is worth protecting.

**2. Sensible defaults must be genuinely useful, not just present.**
The default reviewer persona that ships with Bop should be capable enough to deliver real value on a real project without any configuration. When choosing between a default that's safe-but-shallow and one that's substantive, choose substantive. New users should not need to write a `bop.yaml` to get something worth having.

**3. CLI and MCP server capabilities must stay at parity.**
Every capability exposed through the `bop` CLI must be equally accessible through `bop-mcp`. When adding a feature, it ships to both surfaces or it ships to neither. A developer's choice of interface — terminal, GitHub Actions, or MCP-connected coding assistant — should never determine what they can do.

**4. Deduplication and synthesis are not optional polish.**
The multi-layer deduplication pipeline, semantic deduplication, confidence scoring, and cross-round context injection are core to Bop's value — not enhancements to be added when time allows. A review that floods a PR with redundant or contradictory findings from multiple personas does more harm than good. When in doubt, do more synthesis work before posting, not less.

**5. Configuration should expand possibility, not require expertise.**
`bop.yaml` should allow a project to define a sophisticated, multi-persona review panel with fine-grained control over prompts, models, and weights. It should also be entirely optional. When a configuration option forces a user to understand internals before getting value, reconsider whether it's the right surface to expose.

**6. Deployment context should not limit capability.**
Whether Bop is running in a GitHub Actions workflow, invoked from a developer's local machine against a checkout, or operated through an MCP-compliant assistant reviewing a remote PR — the underlying review quality should be equivalent. The binary is the product; the deployment context is just wiring.

---

## What This Is

- A configurable, multi-persona code review agent that posts synthesized findings to GitHub pull requests
- A statically compiled Go binary (two binaries: `bop` and `bop-mcp`) designed to drop into GitHub Actions with minimal setup
- A local CLI tool for reviewing both local checkouts and remote PRs during active development
- An MCP server that exposes all review capabilities to any MCP-compliant coding assistant
- A system that supports Anthropic Claude (default), Google Gemini, OpenAI, and Ollama as LLM backends, with per-persona model configuration
- An open source project maintained under the `delightfulhammers` GitHub organization

## What This Isn't

- **An auto-fix tool.** Bop surfaces findings and posts them as review comments. It does not modify code, open fix PRs, or apply patches on behalf of any reviewer persona. That boundary is intentional.
- **A GitLab or Bitbucket integration.** Bop is deeply integrated with GitHub's APIs, comment model, and Actions runtime. Support for other platforms is tracked but not planned.
- **A CI-agnostic solution.** The Actions workflow integration is GitHub Actions-specific. Other CI systems are tracked but not currently targeted.
- **A stateful service.** Bop has no server to run, no database to provision, and no infrastructure to maintain. State lives in GitHub comments.
- **A replacement for human code review.** Bop is a force multiplier — it ensures every PR arrives at human review (or self-merge) with more problems already identified, not a substitute for the judgment of the people building the product.
