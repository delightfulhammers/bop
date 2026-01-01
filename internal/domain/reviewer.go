package domain

import "fmt"

// Reviewer represents a specialized code reviewer persona.
// Each reviewer has a unique persona (system prompt), focus areas, and model configuration.
// Part of Phase 3.2 - Reviewer Personas.
type Reviewer struct {
	// Name is the unique identifier for this reviewer (e.g., "security", "performance").
	Name string `json:"name"`

	// Provider is the LLM provider to use (anthropic, openai, gemini, ollama).
	Provider string `json:"provider"`

	// Model is the specific model name (e.g., "claude-opus-4", "gpt-4").
	// In persona mode, this is resolved from config or provider defaults.
	// In legacy mode, this is empty because the Provider already has the model configured.
	Model string `json:"model"`

	// Weight controls the influence of this reviewer's findings in aggregation.
	// Default: 1.0. Must be positive (> 0).
	Weight float64 `json:"weight"`

	// Persona is the system prompt describing this reviewer's expertise and approach.
	// This shapes how the reviewer analyzes code.
	Persona string `json:"persona,omitempty"`

	// Focus lists categories this reviewer should emphasize.
	// Examples: ["security", "authentication"], ["performance", "scalability"].
	// Any strings allowed - not validated against a fixed enum.
	Focus []string `json:"focus,omitempty"`

	// Ignore lists categories this reviewer should skip.
	// Examples: ["style", "formatting"], ["documentation"].
	// Any strings allowed - not validated against a fixed enum.
	Ignore []string `json:"ignore,omitempty"`

	// Enabled indicates whether this reviewer is active.
	// Disabled reviewers are skipped during multi-reviewer orchestration.
	Enabled bool `json:"enabled"`

	// IsLegacy indicates this reviewer was synthesized for backward compatibility.
	// Legacy reviewers don't require Model because the Provider has it configured.
	// This makes the persona mode vs legacy mode contract explicit.
	IsLegacy bool `json:"isLegacy,omitempty"`
}

// NewReviewer creates a new Reviewer with sensible defaults.
// The reviewer is enabled by default with a weight of 1.0.
func NewReviewer(name string) Reviewer {
	return Reviewer{
		Name:    name,
		Weight:  1.0,
		Enabled: true,
		Focus:   make([]string, 0),
		Ignore:  make([]string, 0),
	}
}

// ErrInvalidReviewer is returned when a reviewer fails validation.
var ErrInvalidReviewer = fmt.Errorf("invalid reviewer")

// Validate checks that the reviewer has valid configuration.
// Returns ErrInvalidReviewer with details if validation fails.
//
// Note: For legacy mode reviewers (IsLegacy=true), Model is not required
// because the Provider already has the model configured internally.
func (r Reviewer) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidReviewer)
	}

	if r.Provider == "" {
		return fmt.Errorf("%w: provider is required for reviewer %q", ErrInvalidReviewer, r.Name)
	}

	// Model is required for persona mode, but not for legacy mode
	// (legacy reviewers use the Provider's internal model configuration)
	if r.Model == "" && !r.IsLegacy {
		return fmt.Errorf("%w: model is required for reviewer %q", ErrInvalidReviewer, r.Name)
	}

	if r.Weight <= 0 {
		return fmt.Errorf("%w: weight must be positive for reviewer %q (got %f)", ErrInvalidReviewer, r.Name, r.Weight)
	}

	if err := r.validateFocusIgnoreOverlap(); err != nil {
		return err
	}

	return nil
}

// validateFocusIgnoreOverlap checks that Focus and Ignore don't have common elements.
func (r Reviewer) validateFocusIgnoreOverlap() error {
	focusSet := make(map[string]struct{}, len(r.Focus))
	for _, category := range r.Focus {
		focusSet[category] = struct{}{}
	}

	for _, category := range r.Ignore {
		if _, exists := focusSet[category]; exists {
			return fmt.Errorf(
				"%w: category %q cannot be both focused and ignored for reviewer %q",
				ErrInvalidReviewer,
				category,
				r.Name,
			)
		}
	}

	return nil
}

// ShouldFocus returns true if the reviewer should emphasize the given category.
// Returns true if Focus is empty (no filtering) or if category is in Focus.
// Returns false if category is in Ignore.
func (r Reviewer) ShouldFocus(category string) bool {
	if r.IsIgnored(category) {
		return false
	}

	if len(r.Focus) == 0 {
		return true
	}

	for _, focused := range r.Focus {
		if focused == category {
			return true
		}
	}

	return false
}

// IsIgnored returns true if the reviewer should skip the given category.
func (r Reviewer) IsIgnored(category string) bool {
	for _, ignored := range r.Ignore {
		if ignored == category {
			return true
		}
	}
	return false
}

// HasFocus returns true if the reviewer has any explicit focus categories.
func (r Reviewer) HasFocus() bool {
	return len(r.Focus) > 0
}

// HasIgnore returns true if the reviewer has any explicit ignore categories.
func (r Reviewer) HasIgnore() bool {
	return len(r.Ignore) > 0
}

// IsActive returns true if the reviewer is enabled and can participate in reviews.
func (r Reviewer) IsActive() bool {
	return r.Enabled
}

// ModelIdentifier returns a combined provider/model identifier for logging and metrics.
func (r Reviewer) ModelIdentifier() string {
	return fmt.Sprintf("%s/%s", r.Provider, r.Model)
}
