// Package sampling provides an MCP sampling-based code review provider.
//
// This package implements the review.Provider interface using MCP's sampling
// capability (CreateMessage) instead of direct LLM API calls. This allows
// the code reviewer MCP tools to work without requiring users to have their
// own LLM API keys.
//
// # Architecture
//
// When a user runs review_branch or review_pr through Claude Code (or another
// MCP client that supports sampling), this provider routes the review request
// back to the client's LLM. The flow is:
//
//  1. User: "review my branch against main"
//  2. Claude Code → MCP Server: review_branch(base_ref: "main")
//  3. MCP Server creates sampling Provider with session
//  4. Provider → Claude Code: CreateMessage(prompt: review_prompt)
//  5. Claude Code performs completion using its own LLM access
//  6. Claude Code → Provider: CreateMessageResult(content: JSON review)
//  7. Provider parses JSON, returns domain.Review
//  8. MCP Server → Claude Code: review findings
//
// # Usage
//
// The sampling provider is used as a fallback when no direct API keys are
// available. It's created per-request since MCP sessions are request-scoped:
//
//	// In MCP tool handler:
//	func (s *Server) handleReviewBranch(ctx context.Context, req *mcp.CallToolRequest, input Input) (...) {
//	    // Create sampling provider with this request's session
//	    sessionProvider := func() sampling.Session { return req.Session }
//	    fallbackProvider := sampling.NewProvider(sessionProvider)
//
//	    // Use as fallback if no direct providers available
//	    providers := getEffectiveProviders(s.directProviders, fallbackProvider)
//	    // ...
//	}
//
// # Limitations
//
// Compared to direct API providers, the sampling provider has some limitations:
//
//   - No token usage or cost tracking (MCP sampling doesn't expose this)
//   - Model selection is up to the client (hints are advisory only)
//   - Additional round-trip latency through the MCP protocol
//   - Client may modify or truncate system prompts
//
// For these reasons, sampling is the fallback, not the default. When users
// have direct API keys configured, those are preferred.
//
// # Testing
//
// The Session interface abstracts the MCP session for testing:
//
//	type mockSession struct {
//	    createMessageFunc func(...) (*mcp.CreateMessageResult, error)
//	}
//
//	func (m *mockSession) CreateMessage(...) (*mcp.CreateMessageResult, error) {
//	    return m.createMessageFunc(...)
//	}
package sampling
