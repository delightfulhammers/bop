# Bop

AI code reviews that actually matter. Multiple LLMs review your PR, agree on what's real, and post inline comments as a first-class GitHub reviewer.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/delightfulhammers/bop)](https://github.com/delightfulhammers/bop/releases)

## What You Get

- **Consensus reviews** — Multiple LLMs review in parallel and agree on findings, reducing false positives
- **Inline annotations** — Comments appear on the exact lines that need attention, not buried in PR comments
- **Triage with AI** — Use Claude Code to process feedback: accept, dispute, or fix findings interactively
- **Runs anywhere** — GitHub Actions, local CLI, or MCP server with Claude Code

## Quick Start

```bash
# Install (macOS)
brew install delightfulhammers/tap/bop

# Set your API key
export OPENAI_API_KEY="sk-..."

# Review current branch against main
bop review branch main
```

That's it. You'll get a detailed review in `./reviews/`.

## Installation

**Quick install (Linux/macOS):**
```bash
curl -sSfL https://raw.githubusercontent.com/delightfulhammers/bop/main/install.sh | sh
```

Options:
```bash
# Install specific version
curl -sSfL .../install.sh | sh -s -- --version v0.7.2

# Install to custom directory
curl -sSfL .../install.sh | sh -s -- --dir /usr/local/bin
```

**Homebrew (macOS):**
```bash
brew install delightfulhammers/tap/bop
```

**From source:**
```bash
go install github.com/delightfulhammers/bop/cmd/bop@latest
```

## GitHub Actions

Add automated reviews to every PR:

```yaml
name: Code Review
on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: delightfulhammers/bop/action@v0.7.2
        with:
          openai-api-key: ${{ secrets.OPENAI_API_KEY }}
          # Or use multiple providers:
          # anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}
          # gemini-api-key: ${{ secrets.GEMINI_API_KEY }}
```

See [GitHub Actions Setup](docs/GITHUB_ACTION_SETUP.md) for complete configuration.

## Claude Code Integration

Triage PR findings interactively with Claude Code using the MCP server:

```bash
# Install the MCP server
go build -o bop-mcp ./cmd/bop-mcp
sudo mv bop-mcp /usr/local/bin/
```

Add to your Claude Code settings (`.claude/settings.local.json`):

```json
{
  "mcpServers": {
    "bop": {
      "command": "bop-mcp"
    }
  }
}
```

Then triage findings naturally:

```
Triage the findings on PR #123 in owner/repo
```

The MCP server provides 12 tools for listing findings, viewing context, applying fixes, and responding to comments. See [MCP Server Documentation](#mcp-server) for details.

## Configuration

Create `~/.config/bop/bop.yaml`:

```yaml
providers:
  openai:
    enabled: true
    defaultModel: "gpt-5.2"
    apiKey: "${OPENAI_API_KEY}"

  anthropic:
    enabled: true
    defaultModel: "claude-sonnet-4-5"
    apiKey: "${ANTHROPIC_API_KEY}"

output:
  directory: "./reviews"
```

This minimal config gives you multi-provider consensus reviews. Each enabled provider reviews your code, and findings are merged by agreement.

### Multi-Provider Consensus

When multiple providers are enabled, findings are merged by agreement. A finding reported by 2 of 3 providers is weighted higher than one reported by only 1.

### Reviewer Personas (Optional)

For more control, define specialized reviewers with distinct personas and focuses:

```yaml
providers:
  anthropic:
    enabled: true
    defaultModel: "claude-sonnet-4-5"  # Default for reviewers using this provider
    apiKey: "${ANTHROPIC_API_KEY}"

  openai:
    enabled: true
    defaultModel: "gpt-5.2"
    apiKey: "${OPENAI_API_KEY}"

# Reviewers section is OPTIONAL - if omitted, provider-based dispatch is used
reviewers:
  security:
    provider: "anthropic"
    # model is optional - uses provider's defaultModel if not specified
    weight: 1.5  # Higher weight = more influence in consensus
    persona: |
      You are a senior security engineer focused on identifying
      vulnerabilities, authentication issues, and injection attacks.
    focus: [security, authentication, authorization]
    ignore: [style, documentation]

  architecture:
    provider: "openai"
    model: "gpt-5.2-pro"  # Override the provider's default
    weight: 1.0
    persona: |
      You are a software architect focused on maintainability,
      design patterns, and code organization.
    focus: [maintainability, architecture, complexity]

defaultReviewers:
  - security
  - architecture
```

Use `--reviewers` to run specific reviewers:

```bash
# Run only security review
bop review branch main --reviewers security

# Run multiple reviewers
bop review branch main --reviewers security,architecture
```

Built-in personas are available in `templates/reviewers/` for common review focuses.

### Supported Providers

| Provider | Models | Local Option |
|----------|--------|--------------|
| OpenAI | gpt-5.2, gpt-5.2-mini, gpt-5.2-pro | No |
| Anthropic | claude-haiku-4-5, claude-sonnet-4-5, claude-opus-4-5 | No |
| Google | gemini-3-pro-preview, gemini-3-flash-preview | No |
| Ollama | Any local model | Yes |

See [Configuration Guide](docs/CONFIGURATION.md) for all options.

## Features

### GitHub Integration
- **First-class reviewer** — Uses GitHub's Review API, not just comments
- **Request changes** — Block PRs on critical/high severity findings
- **Finding deduplication** — Won't re-flag the same issue on subsequent pushes
- **Skip triggers** — Bypass with `[skip code-review]` in commit message or PR title
- **Stale review dismissal** — Auto-dismiss old bot reviews when you push fixes

### Review Quality
- **Agent verification** — Secondary LLM validates findings to reduce false positives
- **Reviewer personas** — Specialized reviewers with distinct focuses (security, architecture, performance)
- **Confidence thresholds** — Configure minimum confidence per severity level
- **Secret redaction** — Automatically strips secrets before sending to LLMs
- **Finding attribution** — Each finding shows which reviewer identified it

### Output Formats
- **Markdown** — Human-readable review files
- **JSON** — Structured data for tooling
- **SARIF** — Standard format for security tools and GitHub Code Scanning

### LLM Observability

Debug LLM interactions with configurable logging:

```bash
# CLI flag
bop review branch main --log-level trace

# Or environment variable
export BOP_OBSERVABILITY_LOGGING_LEVEL=trace
```

**Log Levels:**
| Level | Output |
|-------|--------|
| `error` | API errors only |
| `info` | Response summaries (default) |
| `debug` | Request/response metadata |
| `trace` | Full prompts and responses (⚠️ may contain code) |

**Configuration:**
```yaml
observability:
  logging:
    enabled: true
    level: "info"           # trace, debug, info, error
    format: "human"         # human or json
    redactAPIKeys: true     # Redact API keys in logs
    maxContentBytes: 51200  # Truncate trace content at 50KB
```

> **Warning:** Trace logging may expose code sent to LLMs. Use only for debugging.

---

## MCP Server

The `bop-mcp` binary exposes 12 tools for PR triage:

**Read operations:**
| Tool | Description |
|------|-------------|
| `list_annotations` | SARIF findings for HEAD commit |
| `get_annotation` | Single annotation details |
| `list_findings` | PR comment findings |
| `get_finding` | Finding with thread context |
| `get_suggestion` | Extract structured code fix |
| `get_code_context` | File content at specific lines |
| `get_diff_context` | Diff hunk at location |

**Write operations:**
| Tool | Description |
|------|-------------|
| `get_thread` | Full comment thread history |
| `reply_to_finding` | Reply with status (fixed/disputed/acknowledged) |
| `post_comment` | New comment at file/line |
| `mark_resolved` | Mark thread resolved |
| `request_rereview` | Dismiss stale reviews, request fresh |

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_TOKEN` | Yes | Personal access token with `repo` scope |

> **Note:** Never commit tokens to config files. The MCP server inherits from your shell environment.

---

## Security

This tool sends code to third-party LLM APIs. Before using on private repositories:

- **Public repos** — Generally safe for open source
- **Private repos** — Use enterprise LLM tiers or local Ollama models
- **Secrets** — Automatic redaction helps, but review [Security Considerations](docs/SECURITY.md)

---

## Project Status

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1: Foundation | ✅ Complete | Multi-provider LLM, local CLI |
| Phase 2: GitHub Native | ✅ Complete | First-class reviewer with inline annotations |
| Phase 3.1: Triage | ✅ Complete | MCP server for AI-assisted triage |
| Phase 3.2: Personas | ✅ Complete | Specialized reviewer roles with personas |

**Current Version:** v0.7.2

---

## Documentation

- [Configuration Guide](docs/CONFIGURATION.md)
- [GitHub Actions Setup](docs/GITHUB_ACTION_SETUP.md)
- [Security Considerations](docs/SECURITY.md)
- [Cost Tracking](docs/COST_TRACKING.md)

## Contributing

```bash
# Build
go build -o bop ./cmd/bop

# Test
go test ./...

# Lint
golangci-lint run
```

See [Architecture](docs/design/02-ARCHITECTURE.md) for system design.

## License

MIT — see [LICENSE](LICENSE)
