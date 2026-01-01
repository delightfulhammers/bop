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
		Seed:         baseReq.Seed,
		MaxSize:      baseReq.MaxSize,
		ReviewerName: reviewer.Name,
	}, nil
}

// BuildWithSizeGuards creates a prompt with size guard enforcement for a specific reviewer.
//
// The persona overhead (persona text + focus/ignore instructions) is pre-calculated and
// reserved from the token budget before the base builder performs truncation. This ensures
// the final prompt stays within MaxTokens even after persona content is injected.
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

	// Build persona content once and reuse for both estimation and injection
	personaContent := b.buildPersonaContent(reviewer)
	personaOverhead := estimator.EstimateTokens(personaContent)

	// Reserve token budget for persona content using saturation arithmetic
	// (clamp to 0 rather than skipping adjustment if overhead >= budget)
	adjustedLimits := limits
	if personaOverhead > 0 {
		if adjustedLimits.MaxTokens > personaOverhead {
			adjustedLimits.MaxTokens -= personaOverhead
		} else {
			adjustedLimits.MaxTokens = 0
		}
		if adjustedLimits.WarnTokens > personaOverhead {
			adjustedLimits.WarnTokens -= personaOverhead
		} else {
			adjustedLimits.WarnTokens = 0
		}
	}

	// Build base prompt with size guards using adjusted limits
	baseReq, truncResult, err := b.base.BuildWithSizeGuards(
		filteredCtx, diff, req, reviewer.Provider, estimator, adjustedLimits,
	)
	if err != nil {
		return ProviderRequest{}, TruncationResult{}, fmt.Errorf("building base prompt with size guards: %w", err)
	}

	// Prepend cached persona content (reuse string built for estimation)
	prompt := personaContent + baseReq.Prompt

	// Re-estimate final token count on concatenated prompt for accuracy.
	// BPE tokenizers are non-additive across boundaries, so simple arithmetic
	// (FinalTokens += personaOverhead) can under/over-count.
	truncResult.FinalTokens = estimator.EstimateTokens(prompt)

	return ProviderRequest{
		Prompt:       prompt,
		Seed:         baseReq.Seed,
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

// buildPersonaContent builds the persona-specific content string for a reviewer.
// Returns an empty string if the reviewer has no persona or focus/ignore settings.
// Used for token budget estimation before size guards are applied.
func (b *PersonaPromptBuilder) buildPersonaContent(reviewer domain.Reviewer) string {
	var sections []string

	if reviewer.Persona != "" {
		sections = append(sections, b.formatPersonaSection(reviewer.Persona))
	}

	if reviewer.HasFocus() || reviewer.HasIgnore() {
		sections = append(sections, b.formatFocusSection(reviewer.Focus, reviewer.Ignore))
	}

	if len(sections) == 0 {
		return ""
	}

	// Include the separator that will be added when injecting
	return strings.Join(sections, "\n\n") + "\n\n"
}

// injectPersonaContent prepends persona-specific content to the prompt.
// This includes the persona description and focus/ignore instructions.
func (b *PersonaPromptBuilder) injectPersonaContent(prompt string, reviewer domain.Reviewer) string {
	personaContent := b.buildPersonaContent(reviewer)
	if personaContent == "" {
		return prompt
	}
	return personaContent + prompt
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
