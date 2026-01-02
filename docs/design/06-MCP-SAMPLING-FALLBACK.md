# Phase 3.5e: MCP Sampling Fallback

**Status:** Implemented
**Author:** Claude (with Brandon)
**Created:** 2026-01-01
**Implemented:** 2026-01-02
**Epic:** #195
**Sub-issues:** #196, #197, #198, #199, #200

---

## Executive Summary

Enable the `review_branch` and `review_pr` MCP tools to work without requiring LLM API keys by falling back to MCP sampling. When direct provider API keys (Anthropic, OpenAI) aren't configured, the MCP server will request the connected client (e.g., Claude Code) to perform LLM completions on its behalf.

This preserves the value of multi-provider diversity when API keys are available, while ensuring the tools always work out-of-the-box for users who only have access through their coding assistant.

---

## Problem Statement

### Current Limitation

The `review_branch` MCP tool requires `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` environment variables to be set. Without these, the tool returns "not implemented":

```
review_branch requires LLM providers - set ANTHROPIC_API_KEY or OPENAI_API_KEY
```

This creates friction for users who:
1. Don't have direct API access but use Claude Code (which has LLM access)
2. Want zero-configuration setup
3. Only need occasional code reviews (don't want to manage API keys)

### Opportunity

MCP defines a **sampling** capability that allows servers to request LLM completions from connected clients. Claude Code supports this. We can leverage sampling as a fallback when direct API keys aren't available.

---

## Design Goals

1. **Zero-configuration works** - Tools function without any API keys when used via Claude Code
2. **Preserve multi-provider value** - Direct API access still preferred for diversity
3. **Same output format** - Findings format identical regardless of provider source
4. **Graceful capability detection** - Handle clients that don't support sampling
5. **Persona support** - Reviewer personas work via sampling (system prompt)

---

## Architecture

### Provider Hierarchy

```
┌─────────────────────────────────────────────────────────────────────┐
│                      review.Provider interface                       │
│                                                                      │
│  func Review(ctx, ReviewRequest) (Review, error)                    │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────┐  │
│  │ anthropic       │  │ openai          │  │ sampling            │  │
│  │ .Provider       │  │ .Provider       │  │ .Provider           │  │
│  │                 │  │                 │  │                     │  │
│  │ Direct API      │  │ Direct API      │  │ Via MCP client      │  │
│  │ ANTHROPIC_KEY   │  │ OPENAI_KEY      │  │ req.Session         │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────────┘  │
│         ▲                    ▲                      ▲               │
│         │                    │                      │               │
│    [Priority 1]         [Priority 2]          [Fallback]           │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Request Flow

```
┌──────────────┐     ┌─────────────────┐     ┌──────────────────┐
│  Claude Code │────▶│  MCP Server     │────▶│  Direct Provider │
│  (Client)    │     │  (review_branch)│     │  (if API key)    │
└──────────────┘     └─────────────────┘     └──────────────────┘
       ▲                     │
       │                     │ [No API key?]
       │                     ▼
       │              ┌─────────────────┐
       └──────────────│  Sampling       │
         CreateMessage│  Provider       │
                      └─────────────────┘
```

### Data Flow with Sampling

```
1. User: "review my branch against main"
2. Claude Code → MCP Server: review_branch(base_ref: "main")
3. MCP Server: No ANTHROPIC_API_KEY, no OPENAI_API_KEY
4. MCP Server → Claude Code: CreateMessage(systemPrompt: persona, messages: [diff])
5. Claude Code: Performs LLM completion using its own context
6. Claude Code → MCP Server: CreateMessageResult(content: review_json)
7. MCP Server: Parses review, returns findings
8. MCP Server → Claude Code: ReviewBranchOutput(findings: [...])
```

---

## Implementation Plan

### Phase 1: Sampling Provider

Create a new provider that implements `review.Provider` using MCP sampling.

**New file:** `internal/adapter/llm/sampling/provider.go`

```go
package sampling

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/bkyoung/code-reviewer/internal/domain"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// SessionProvider is a function that returns the current MCP session.
// This allows the provider to be created once but get the session per-request.
type SessionProvider func() *mcp.ServerSession

// Provider implements review.Provider using MCP sampling.
// Instead of making direct LLM API calls, it requests the MCP client
// to perform completions on its behalf.
type Provider struct {
    getSession SessionProvider
    model      string // reported model name (e.g., "sampling-fallback")
}

// NewProvider creates a sampling-based review provider.
func NewProvider(getSession SessionProvider) *Provider {
    return &Provider{
        getSession: getSession,
        model:      "mcp-sampling",
    }
}

// Review performs a code review by requesting the MCP client to sample.
func (p *Provider) Review(ctx context.Context, req domain.ReviewRequest) (domain.Review, error) {
    session := p.getSession()
    if session == nil {
        return domain.Review{}, fmt.Errorf("no MCP session available for sampling")
    }

    // Build the sampling request
    samplingReq := &mcp.CreateMessageParams{
        SystemPrompt: req.SystemPrompt,
        Messages: []*mcp.SamplingMessage{
            {
                Role:    "user",
                Content: &mcp.TextContent{Text: req.UserPrompt},
            },
        },
        MaxTokens:   int64(req.MaxTokens),
        Temperature: req.Temperature,
    }

    // Request completion from MCP client
    result, err := session.CreateMessage(ctx, samplingReq)
    if err != nil {
        return domain.Review{}, fmt.Errorf("sampling request failed: %w", err)
    }

    // Extract text content from result
    textContent, ok := result.Content.(*mcp.TextContent)
    if !ok {
        return domain.Review{}, fmt.Errorf("unexpected content type: %T", result.Content)
    }

    // Parse the review response (same as direct providers)
    return p.parseResponse(textContent.Text, result.Model)
}

// Name returns the provider name.
func (p *Provider) Name() string {
    return "sampling"
}

// Model returns the model identifier.
func (p *Provider) Model() string {
    return p.model
}

func (p *Provider) parseResponse(content, model string) (domain.Review, error) {
    // Same JSON parsing logic as other providers
    var findings []domain.Finding
    if err := json.Unmarshal([]byte(content), &findings); err != nil {
        return domain.Review{}, fmt.Errorf("parse findings: %w", err)
    }

    return domain.Review{
        ProviderName: "sampling",
        ModelName:    model,
        Findings:     findings,
    }, nil
}
```

### Phase 2: Provider Factory with Fallback

Create a factory that builds the provider list with fallback support.

**New file:** `internal/adapter/llm/factory.go`

```go
package llm

import (
    "github.com/bkyoung/code-reviewer/internal/adapter/llm/anthropic"
    "github.com/bkyoung/code-reviewer/internal/adapter/llm/openai"
    "github.com/bkyoung/code-reviewer/internal/adapter/llm/sampling"
    "github.com/bkyoung/code-reviewer/internal/config"
    "github.com/bkyoung/code-reviewer/internal/usecase/review"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProviderFactoryOptions configures provider creation.
type ProviderFactoryOptions struct {
    Config         *config.Config
    SessionProvider sampling.SessionProvider // For MCP sampling fallback
}

// BuildProviders creates providers from configuration and environment.
// Returns both the direct providers map and a fallback provider if available.
func BuildProviders(opts ProviderFactoryOptions) (direct map[string]review.Provider, fallback review.Provider) {
    direct = make(map[string]review.Provider)

    // Build direct providers from API keys
    if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
        direct["anthropic"] = buildAnthropicProvider(key, opts.Config)
    }

    if key := os.Getenv("OPENAI_API_KEY"); key != "" {
        direct["openai"] = buildOpenAIProvider(key, opts.Config)
    }

    // Build sampling fallback if session provider available
    if opts.SessionProvider != nil {
        fallback = sampling.NewProvider(opts.SessionProvider)
    }

    return direct, fallback
}

// EffectiveProviders returns providers to use, including fallback if no direct providers.
func EffectiveProviders(direct map[string]review.Provider, fallback review.Provider) map[string]review.Provider {
    if len(direct) > 0 {
        return direct
    }

    if fallback != nil {
        return map[string]review.Provider{"sampling": fallback}
    }

    return nil
}
```

### Phase 3: Session Context Threading

The challenge is that `req.Session` is only available inside the tool handler, but providers are created at startup. We need to thread the session through.

**Option A: Per-request provider creation**

```go
func (s *Server) handleReviewBranch(ctx context.Context, req *mcp.CallToolRequest, input ReviewBranchInput) (...) {
    // Create sampling provider with this request's session
    sessionProvider := func() *mcp.ServerSession { return req.Session }
    samplingFallback := sampling.NewProvider(sessionProvider)

    // Get effective providers
    providers := llm.EffectiveProviders(s.deps.DirectProviders, samplingFallback)

    if len(providers) == 0 {
        return notImplementedResult("no providers available")
    }

    // Create per-request orchestrator
    orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
        Git:       s.deps.Git,
        Providers: providers,
        Merger:    s.deps.Merger,
        Registry:  s.deps.ReviewerRegistry,
    })

    return orchestrator.ReviewBranch(ctx, branchReq)
}
```

**Option B: Context-based session injection**

```go
// Store session in context
type sessionContextKey struct{}

func WithSession(ctx context.Context, session *mcp.ServerSession) context.Context {
    return context.WithValue(ctx, sessionContextKey{}, session)
}

func SessionFrom(ctx context.Context) *mcp.ServerSession {
    if s, ok := ctx.Value(sessionContextKey{}).(*mcp.ServerSession); ok {
        return s
    }
    return nil
}

// Sampling provider reads from context
func (p *Provider) Review(ctx context.Context, req ReviewRequest) (Review, error) {
    session := SessionFrom(ctx)
    if session == nil {
        return Review{}, ErrNoSession
    }
    // ...
}
```

**Recommendation:** Option A (per-request) is cleaner and more explicit.

### Phase 4: Capability Detection

Check if the client supports sampling before attempting to use it.

```go
func (s *Server) clientSupportsSampling(session *mcp.ServerSession) bool {
    params := session.InitializeParams()
    if params == nil || params.Capabilities == nil {
        return false
    }
    return params.Capabilities.Sampling != nil
}

func (s *Server) handleReviewBranch(...) {
    // Check capabilities
    if !s.clientSupportsSampling(req.Session) && len(s.deps.DirectProviders) == 0 {
        return notImplementedResult(
            "review_branch requires either: " +
            "(1) LLM API keys (ANTHROPIC_API_KEY, OPENAI_API_KEY), or " +
            "(2) an MCP client that supports sampling")
    }
    // ...
}
```

### Phase 5: Update MCP Server Wiring

**File:** `cmd/code-reviewer-mcp/main.go`

```go
func run() error {
    // ... existing setup ...

    // Build direct providers from environment
    directProviders := buildDirectProviders(&cfg)

    // Create server with direct providers only
    // (sampling fallback is created per-request in handlers)
    server := mcpadapter.NewServer(mcpadapter.ServerDeps{
        PRService:       prService,
        TriageService:   triageService,
        DirectProviders: directProviders,
        Git:             gitEngine,
        Merger:          merger,
        ReviewerRegistry: reviewerRegistry,
    })

    return server.Run(ctx)
}
```

---

## Key Decisions

### 1. Fallback vs Default

**Decision:** Sampling is the **fallback**, not the default.

**Rationale:**
- Multi-provider diversity provides value (different LLMs catch different issues)
- LLM-as-a-judge patterns benefit from independent models
- Direct API gives more control (model selection, parameters)
- Sampling adds latency (extra round-trip through client)

### 2. Per-Request vs Singleton Provider

**Decision:** Create sampling provider **per-request**.

**Rationale:**
- Session is only available in tool handler context
- Avoids complex session management
- Clean separation of concerns
- Minimal overhead (provider is lightweight)

### 3. Persona Support via System Prompt

**Decision:** Pass persona prompts through `SystemPrompt` field.

**Rationale:**
- MCP sampling supports system prompts
- Existing persona prompt builder works unchanged
- Client may modify/ignore, but that's acceptable for fallback

### 4. Error Handling Strategy

**Decision:** Graceful degradation with clear messaging.

```go
// Priority order:
// 1. Use direct providers if available
// 2. Use sampling if client supports it
// 3. Return helpful error explaining options
```

---

## Testing Strategy

### Unit Tests

1. **Sampling provider tests**
   - Mock session that returns expected responses
   - Test response parsing
   - Test error handling (no session, parse errors)

2. **Provider factory tests**
   - Test with various API key combinations
   - Test fallback provider creation
   - Test effective provider selection

3. **Capability detection tests**
   - Test with sampling capability present
   - Test with sampling capability absent
   - Test with nil capabilities

### Integration Tests

1. **End-to-end with mock client**
   - Set up in-memory MCP transport
   - Client implements CreateMessageHandler
   - Verify full flow works

2. **Fallback behavior**
   - No API keys, sampling works
   - API keys present, direct used
   - Mixed scenario

### Manual Testing

1. Connect code-reviewer-mcp to Claude Code
2. Remove all API keys from environment
3. Run `review_branch` tool
4. Verify it uses sampling and returns findings

---

## Migration & Compatibility

### Breaking Changes

None. This is purely additive:
- Existing behavior preserved when API keys present
- New fallback behavior when keys absent

### Configuration Changes

None required. Sampling is auto-detected based on:
1. Absence of API keys
2. Client capability advertisement

### Documentation Updates

1. Update tool descriptions to mention sampling fallback
2. Add troubleshooting for "client doesn't support sampling"
3. Document when to prefer direct API vs sampling

---

## Future Enhancements

### 1. Streaming Support

MCP supports streaming responses. Could improve UX for large reviews.

### 2. Model Preferences

Pass `ModelPreferences` to hint at desired model characteristics:

```go
ModelPreferences: &mcp.ModelPreferences{
    CostPriority:         0.3,  // Balance cost
    SpeedPriority:        0.3,  // Balance speed
    IntelligencePriority: 0.4,  // Slight preference for capability
}
```

### 3. Hybrid Mode

Use sampling for some reviewers, direct API for others:

```yaml
reviewers:
  security:
    provider: anthropic  # Direct API for critical reviews
  style:
    provider: sampling   # Fallback OK for style checks
```

### 4. Caching

Cache sampling responses for identical diffs to reduce client load.

---

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/adapter/llm/sampling/provider.go` | Create | Sampling-based provider |
| `internal/adapter/llm/sampling/provider_test.go` | Create | Unit tests |
| `internal/adapter/llm/factory.go` | Create | Provider factory with fallback |
| `internal/adapter/mcp/server.go` | Modify | Add DirectProviders to deps |
| `internal/adapter/mcp/review_handlers.go` | Modify | Per-request provider wiring |
| `cmd/code-reviewer-mcp/main.go` | Modify | Remove orchestrator from startup |
| `docs/design/06-MCP-SAMPLING-FALLBACK.md` | Create | This document |

---

## Implementation Notes

The implementation followed the design with the following actual file structure:

| File | Description |
|------|-------------|
| `internal/adapter/llm/sampling/provider.go` | Sampling-based provider using MCP CreateMessage |
| `internal/adapter/llm/sampling/provider_test.go` | Unit tests for sampling provider |
| `internal/adapter/llm/provider/factory.go` | Provider factory (in `provider` subpackage to avoid import cycles) |
| `internal/adapter/llm/provider/factory_test.go` | Factory unit tests |
| `internal/adapter/mcp/review_handlers.go` | Per-request orchestrator creation with `createPerRequestReviewer()` |
| `internal/adapter/mcp/sampling_integration_test.go` | Integration tests for sampling fallback |

**Key implementation decisions:**
- Factory placed in `internal/adapter/llm/provider/` subpackage to avoid import cycles
- Both `review_branch` and `review_pr` share the same fallback mechanism via `createPerRequestReviewer()`
- Tool descriptions updated to document sampling fallback behavior

---

## Acceptance Criteria

- [x] `review_branch` works without any API keys when client supports sampling
- [x] `review_branch` prefers direct providers when API keys are available
- [x] Reviewer personas work via sampling (system prompt passed)
- [x] Clear error message when neither API keys nor sampling available
- [x] Unit tests for sampling provider
- [x] Integration test with mock MCP client
- [x] Documentation updated

---

## References

- [MCP Sampling Specification](https://modelcontextprotocol.io/specification/2025-06-18/client/sampling)
- [go-sdk CreateMessage](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#ServerSession.CreateMessage)
- Issue #181: review_branch MCP tool (completed)
- `docs/design/05-PHASE-3.5-LOCAL-MODE.md`: Local mode design
