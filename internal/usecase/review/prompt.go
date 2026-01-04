package review

import (
	"fmt"
	"strings"

	"github.com/delightfulhammers/bop/internal/domain"
)

// defaultMaxTokens sets the maximum output tokens for LLM responses.
//
// Set to 64000 as a safe default that works across all current providers:
// - Claude 4.5 models: 64K max output tokens
// - GPT-5.2: 128K max output tokens
// - Gemini 3 models: 64K max output tokens
//
// Note for thinking models (Gemini 2.5+/3 Pro, OpenAI o-series):
// These models use output tokens for internal reasoning BEFORE generating
// visible output. The max_output_tokens limit applies to BOTH thinking and
// output combined. With low limits (e.g., 8K), the model may exhaust the
// budget on reasoning alone, returning empty responses with MAX_TOKENS.
//
// The 64K default provides headroom for thinking models while staying within
// all current provider limits.
const defaultMaxTokens = 64000

// DefaultPromptBuilder renders a structured prompt for the provider.
// This is a simple implementation that doesn't use project context.
// For enhanced prompts with context, use EnhancedPromptBuilder.
func DefaultPromptBuilder(ctx ProjectContext, diff domain.Diff, req BranchRequest, providerName string) (ProviderRequest, error) {
	var builder strings.Builder
	builder.WriteString("You are an expert software engineer performing a code review.\n")
	builder.WriteString("Provide actionable findings in JSON matching the expected schema.\n\n")

	// Include custom instructions if provided
	if ctx.CustomInstructions != "" {
		builder.WriteString(fmt.Sprintf("Instructions: %s\n\n", ctx.CustomInstructions))
	}

	builder.WriteString(fmt.Sprintf("Base Ref: %s\n", req.BaseRef))
	builder.WriteString(fmt.Sprintf("Target Ref: %s\n\n", req.TargetRef))
	builder.WriteString("Unified Diff:\n")
	for _, file := range diff.Files {
		builder.WriteString(fmt.Sprintf("File: %s (%s)\n", file.Path, file.Status))
		builder.WriteString(file.Patch)
		builder.WriteString("\n")
	}

	return ProviderRequest{
		Prompt:  builder.String(),
		MaxSize: defaultMaxTokens,
	}, nil
}
