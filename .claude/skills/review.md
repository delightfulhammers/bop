# Review Skill

Load context for using the code reviewer tool itself.

## Instructions

When this skill is invoked, assist with running code reviews.

## Basic Usage

```bash
# Review current branch against main
./bop review branch main

# Review with output directory
./bop review branch main --output ./review-output

# Review specific target branch
./bop review branch feature/foo --base main
```

## CLI Flags

| Flag | Description |
|------|-------------|
| `--output` | Output directory for review files |
| `--base` | Base branch to compare against (default: main) |
| `--instructions` | Custom review instructions |
| `--context` | Additional context files to include |
| `--no-architecture` | Skip loading ARCHITECTURE.md |
| `--no-auto-context` | Skip automatic context discovery |
| `--interactive` | Enable planning agent for clarifying questions |

## Output Formats

The tool generates multiple formats:
- `*_merged_*.md` - Human-readable summary
- `*_<provider>_*.md` - Per-provider detailed findings
- `*.sarif` - SARIF for GitHub Code Scanning
- `*.json` - Structured JSON for programmatic use

## Configuration

Create `bop.yaml` in the repo root:

```yaml
providers:
  anthropic:
    enabled: true
    model: "claude-sonnet-4-5-20250929"
    apiKey: "${ANTHROPIC_API_KEY}"

output:
  directory: "./review-output"

redaction:
  enabled: true
  denyGlobs:
    - "**/*.env"
    - "**/*.pem"
    - "**/*.key"
```

## GitHub Actions

The tool integrates with GitHub Actions:

```yaml
- name: Run AI code review
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
  run: |
    ./bop review branch ${{ github.head_ref }} \
      --base ${{ github.event.pull_request.base.ref }} \
      --output ./review-output
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Empty review | Check if diff exists: `git diff main...HEAD` |
| Provider error | Verify API key is set and valid |
| Token limit | Large diffs may need `--no-auto-context` |
| Missing files | Ensure `--output` directory is writable |

## After Loading Context

Assist with running reviews, interpreting output, or troubleshooting issues.
