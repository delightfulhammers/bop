package review

import (
	"fmt"
	"strings"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// PersonaPromptBuilder generates prompts tailored to reviewer personas.
// It wraps the EnhancedPromptBuilder and adds persona injection, focus/ignore
// instructions, and prior findings filtering.
//
// Part of Phase 3.2 - Reviewer Personas.
type PersonaPromptBuilder struct {
	base *EnhancedPromptBuilder
}

// NewPersonaPromptBuilder creates a new PersonaPromptBuilder wrapping the given base builder.
// Panics if base is nil.
func NewPersonaPromptBuilder(base *EnhancedPromptBuilder) *PersonaPromptBuilder {
	if base == nil {
		panic("base prompt builder cannot be nil")
	}
	return &PersonaPromptBuilder{
		base: base,
	}
}

// Build creates a prompt for a specific reviewer, injecting persona and focus/ignore
// instructions at the prompt start.
func (b *PersonaPromptBuilder) Build(
	ctx ProjectContext,
	diff domain.Diff,
	req BranchRequest,
	reviewer domain.Reviewer,
) (ProviderRequest, error) {
	// Filter prior findings for this reviewer only
	filteredCtx := b.filterContext(ctx, reviewer)

	// Build base prompt using enhanced builder with filtered context
	baseReq, err := b.base.Build(filteredCtx, diff, req, reviewer.Provider)
	if err != nil {
		return ProviderRequest{}, fmt.Errorf("building base prompt: %w", err)
	}

	// Inject persona-specific content at prompt start
	prompt := b.injectPersonaContent(baseReq.Prompt, reviewer)

	return ProviderRequest{
		Prompt:       prompt,
		MaxSize:      baseReq.MaxSize,
		ReviewerName: reviewer.Name,
	}, nil
}

// BuildWithSizeGuards creates a prompt with size guard enforcement for a specific reviewer.
//
// Note: Persona content (persona text + focus/ignore instructions) is added AFTER the base
// builder performs truncation. This means the final token count may exceed MaxTokens by the
// persona overhead. In practice, persona content is typically small (a few hundred tokens)
// compared to token limits (tens of thousands), so this is acceptable. If persona overhead
// becomes a concern, consider pre-calculating and reserving tokens for persona content.
func (b *PersonaPromptBuilder) BuildWithSizeGuards(
	ctx ProjectContext,
	diff domain.Diff,
	req BranchRequest,
	reviewer domain.Reviewer,
	estimator TokenEstimator,
	limits SizeGuardLimits,
) (ProviderRequest, TruncationResult, error) {
	if estimator == nil {
		return ProviderRequest{}, TruncationResult{}, fmt.Errorf("estimator cannot be nil")
	}

	// Filter prior findings for this reviewer only
	filteredCtx := b.filterContext(ctx, reviewer)

	// Build base prompt with size guards using filtered context
	baseReq, truncResult, err := b.base.BuildWithSizeGuards(
		filteredCtx, diff, req, reviewer.Provider, estimator, limits,
	)
	if err != nil {
		return ProviderRequest{}, TruncationResult{}, fmt.Errorf("building base prompt with size guards: %w", err)
	}

	// Inject persona-specific content at prompt start
	prompt := b.injectPersonaContent(baseReq.Prompt, reviewer)

	// Update token count to include persona overhead
	truncResult.FinalTokens = estimator.EstimateTokens(prompt)

	return ProviderRequest{
		Prompt:       prompt,
		MaxSize:      baseReq.MaxSize,
		ReviewerName: reviewer.Name,
	}, truncResult, nil
}

// filterContext returns a copy of the context with prior findings filtered
// to only include findings from the specified reviewer.
func (b *PersonaPromptBuilder) filterContext(ctx ProjectContext, reviewer domain.Reviewer) ProjectContext {
	filtered := ctx

	if ctx.TriagedFindings != nil {
		filtered.TriagedFindings = ctx.TriagedFindings.FilterByReviewer(reviewer.Name)
	}

	return filtered
}

// injectPersonaContent prepends persona-specific content to the prompt.
// This includes the persona description and focus/ignore instructions.
func (b *PersonaPromptBuilder) injectPersonaContent(prompt string, reviewer domain.Reviewer) string {
	var sections []string

	// Add persona section if defined
	if reviewer.Persona != "" {
		sections = append(sections, b.formatPersonaSection(reviewer.Persona))
	}

	// Add focus/ignore section if defined
	if reviewer.HasFocus() || reviewer.HasIgnore() {
		sections = append(sections, b.formatFocusSection(reviewer.Focus, reviewer.Ignore))
	}

	// No persona content to inject
	if len(sections) == 0 {
		return prompt
	}

	// Prepend persona content to prompt
	personaContent := strings.Join(sections, "\n\n")
	return personaContent + "\n\n" + prompt
}

// formatPersonaSection creates the reviewer persona section.
func (b *PersonaPromptBuilder) formatPersonaSection(persona string) string {
	return fmt.Sprintf(`## Reviewer Persona

%s`, persona)
}

// formatFocusSection creates the category focus/ignore section.
func (b *PersonaPromptBuilder) formatFocusSection(focus, ignore []string) string {
	var sb strings.Builder
	sb.WriteString("## Category Focus\n\n")

	if len(focus) > 0 {
		sb.WriteString("**FOCUS** on these categories - prioritize findings in these areas:\n")
		for _, cat := range focus {
			sb.WriteString(fmt.Sprintf("- %s\n", cat))
		}
	}

	if len(ignore) > 0 {
		if len(focus) > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("**IGNORE** these categories - do not raise findings in these areas:\n")
		for _, cat := range ignore {
			sb.WriteString(fmt.Sprintf("- %s\n", cat))
		}
	}

	return sb.String()
}
