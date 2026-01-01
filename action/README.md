# AI Code Reviewer Action

AI-powered code review for your pull requests using multiple LLM providers.

## Quick Start

```yaml
name: Code Review
on: [pull_request]

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
      - uses: bkyoung/code-reviewer/action@v0.6.3
        with:
          anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}
```

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `anthropic-api-key` | Anthropic API key | No* | |
| `openai-api-key` | OpenAI API key | No* | |
| `gemini-api-key` | Google Gemini API key | No* | |
| `base-branch` | Base branch to compare against | No | `main` |
| `reviewers` | Comma-separated reviewers (e.g., `security,architecture`) | No | |
| `block-threshold` | Severity threshold for blocking (`critical`, `high`, `medium`, `low`, `none`) | No | `none` |
| `post-review` | Post review comments to PR | No | `true` |
| `fail-on-findings` | Fail if findings exceed block-threshold | No | `false` |
| `config-file` | Path to cr.yaml config file | No | |
| `log-level` | Log level (`trace`, `debug`, `info`, `error`) | No | `info` |

\* At least one API key is required.

## Outputs

| Output | Description |
|--------|-------------|
| `findings-count` | Total number of findings |
| `critical-count` | Number of critical severity findings |
| `high-count` | Number of high severity findings |
| `medium-count` | Number of medium severity findings |
| `low-count` | Number of low severity findings |
| `summary` | Markdown summary of the review |

## Examples

### Multi-Provider Review

```yaml
- uses: bkyoung/code-reviewer/action@v0.6.3
  with:
    anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}
    openai-api-key: ${{ secrets.OPENAI_API_KEY }}
```

### Block on High Severity

```yaml
- uses: bkyoung/code-reviewer/action@v0.6.3
  with:
    anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}
    block-threshold: high
    fail-on-findings: true
```

### Custom Reviewers

```yaml
- uses: bkyoung/code-reviewer/action@v0.6.3
  with:
    anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}
    reviewers: security,performance
```

### With Config File

The action automatically detects config files in this order:
1. Explicit `config-file` input
2. `.github/cr.yaml`
3. `cr.yaml` (repo root)
4. Built-in defaults

```yaml
# Explicit config path
- uses: bkyoung/code-reviewer/action@v0.6.3
  with:
    anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}
    config-file: custom/path/cr.yaml

# Or just drop a cr.yaml in .github/ or repo root - it's auto-detected
- uses: bkyoung/code-reviewer/action@v0.6.3
  with:
    anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}
```

A well-commented example config is included in the [release tarballs](https://github.com/bkyoung/code-reviewer/releases) and in the [main repository](https://github.com/bkyoung/code-reviewer/blob/main/cr.yaml).

### Use Outputs in Subsequent Steps

```yaml
- uses: bkyoung/code-reviewer/action@v0.6.3
  id: review
  with:
    anthropic-api-key: ${{ secrets.ANTHROPIC_API_KEY }}

- name: Check findings
  if: steps.review.outputs.critical-count > 0
  run: |
    echo "Found ${{ steps.review.outputs.critical-count }} critical issues!"
    exit 1
```

## Permissions

The action requires the following permissions:

```yaml
permissions:
  contents: read        # Read repository content
  pull-requests: write  # Post review comments
```

## How It Works

This is a [composite action](https://docs.github.com/en/actions/creating-actions/creating-a-composite-action) that:

1. Downloads the `cr` binary from GitHub releases (matching the action version)
2. Runs the code review against your PR
3. Posts findings as PR comments (if enabled)
4. Sets outputs for use in subsequent steps

## Versioning

Reference specific versions for stability:

```yaml
# Pin to specific version (recommended)
- uses: bkyoung/code-reviewer/action@v0.6.3

# Use latest from main (not recommended for production)
- uses: bkyoung/code-reviewer/action@main
```

## License

MIT License - see [LICENSE](../LICENSE) for details.
