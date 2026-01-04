# Development Skill

Load context for building, testing, and debugging the code reviewer.

## Instructions

When this skill is invoked, you have access to development workflows.

## Build Commands

```bash
# Build the CLI
go build -o bop ./cmd/bop

# Build with version injection
go build -ldflags "-X github.com/delightfulhammers/bop/internal/version.version=v0.2.3" -o bop ./cmd/bop
```

## Test Commands

```bash
# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Run specific package
go test ./internal/usecase/review/...

# Run with verbose output
go test -v ./...

# Run specific test
go test -run TestName ./path/to/package
```

## Code Quality

```bash
# Format code
gofmt -w .

# Lint (if installed)
golangci-lint run

# Check for race conditions
go test -race ./...
```

## Configuration

The tool uses `bop.yaml` for configuration. Key sections:

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
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `OPENAI_API_KEY` | OpenAI API access |
| `ANTHROPIC_API_KEY` | Anthropic API access |
| `GEMINI_API_KEY` | Google Gemini API access |

## Debugging Tips

1. **Empty reviews:** Check if diff is being generated correctly
2. **Provider errors:** Verify API keys are set and valid
3. **Token limits:** Large diffs may exceed context limits
4. **Secret redaction:** Check `redaction.enabled` in config

## After Loading Context

Assist with building, testing, or debugging issues in the codebase.
