# Code Reviewer

AI code reviews that actually matter. Multiple LLMs review your PR, agree on what's real, and post inline comments as a first-class GitHub reviewer.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/bkyoung/code-reviewer)](https://github.com/bkyoung/code-reviewer/releases)

## What You Get

- **Consensus reviews** — Multiple LLMs review in parallel and agree on findings, reducing false positives
- **Inline annotations** — Comments appear on the exact lines that need attention, not buried in PR comments
- **Triage with AI** — Use Claude Code to process feedback: accept, dispute, or fix findings interactively
- **Runs anywhere** — GitHub Actions, local CLI, or MCP server with Claude Code

## Quick Start

```bash
# Install
go install github.com/bkyoung/code-reviewer/cmd/cr@latest

# Set your API key
export OPENAI_API_KEY="sk-..."

# Review current branch against main
cr review branch main
```

That's it. You'll get a detailed review in `./reviews/`.

## Installation

**From releases (recommended):**
```bash
# macOS ARM64
curl -L https://github.com/bkyoung/code-reviewer/releases/latest/download/code-reviewer_darwin_arm64.tar.gz | tar xz
sudo mv cr /usr/local/bin/
```

**From source:**
```bash
git clone https://github.com/bkyoung/code-reviewer
cd code-reviewer
go build -o cr ./cmd/cr
```

## GitHub Actions

Add automated reviews to every PR:

```yaml
name: Code Review
on:
  pull_request:
    types: [opened, synchronize]

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

      - name: Run Code Review
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          cr review branch ${{ github.event.pull_request.base.ref }} \
            --post-github-review \
            --github-owner ${{ github.repository_owner }} \
            --github-repo ${{ github.event.repository.name }} \
            --pr-number ${{ github.event.pull_request.number }}
```

See [GitHub Actions Setup](docs/GITHUB_ACTION_SETUP.md) for complete configuration.

## Claude Code Integration

Triage PR findings interactively with Claude Code using the MCP server:

```bash
# Install the MCP server
go build -o code-reviewer-mcp ./cmd/code-reviewer-mcp
sudo mv code-reviewer-mcp /usr/local/bin/
```

Add to your Claude Code settings (`.claude/settings.local.json`):

```json
{
  "mcpServers": {
    "code-reviewer": {
      "command": "code-reviewer-mcp"
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

Create `~/.config/cr/cr.yaml`:

```yaml
providers:
  openai:
    enabled: true
    model: "gpt-4o-mini"
    apiKey: "${OPENAI_API_KEY}"

  anthropic:
    enabled: true
    model: "claude-sonnet-4-5"
    apiKey: "${ANTHROPIC_API_KEY}"

output:
  directory: "./reviews"
```

### Multi-Provider Consensus

When multiple providers are enabled, findings are merged by agreement. A finding reported by 2 of 3 providers is weighted higher than one reported by only 1.

### Supported Providers

| Provider | Models | Local Option |
|----------|--------|--------------|
| OpenAI | gpt-4o, gpt-4o-mini, o1 | No |
| Anthropic | claude-sonnet-4-5, claude-opus-4 | No |
| Google | gemini-2.5-pro, gemini-2.5-flash | No |
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
- **Confidence thresholds** — Configure minimum confidence per severity level
- **Secret redaction** — Automatically strips secrets before sending to LLMs

### Output Formats
- **Markdown** — Human-readable review files
- **JSON** — Structured data for tooling
- **SARIF** — Standard format for security tools and GitHub Code Scanning

---

## MCP Server

The `code-reviewer-mcp` binary exposes 12 tools for PR triage:

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
| Phase 3.2: Personas | 📋 Planned | Specialized reviewer roles |

**Current Version:** v0.5.0

---

## Documentation

- [Configuration Guide](docs/CONFIGURATION.md)
- [GitHub Actions Setup](docs/GITHUB_ACTION_SETUP.md)
- [Security Considerations](docs/SECURITY.md)
- [Cost Tracking](docs/COST_TRACKING.md)

## Contributing

```bash
# Build
go build -o cr ./cmd/cr

# Test
go test ./...

# Lint
golangci-lint run
```

See [Architecture](docs/design/02-ARCHITECTURE.md) for system design.

## License

MIT — see [LICENSE](LICENSE)
