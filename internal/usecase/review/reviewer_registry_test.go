package review_test

import (
	"testing"

	"github.com/delightfulhammers/bop/internal/config"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestNewReviewerRegistry(t *testing.T) {
	t.Parallel()

	t.Run("creates_registry_from_valid_config", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Reviewers: map[string]config.ReviewerConfig{
				"security": {
					Enabled:  boolPtr(true),
					Provider: "anthropic",
					Model:    "claude-opus-4",
					Weight:   1.5,
				},
			},
			DefaultReviewers: []string{"security"},
		}

		registry, err := review.NewReviewerRegistry(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if registry == nil {
			t.Fatal("expected non-nil registry")
		}
	})

	t.Run("errors_on_nil_config", func(t *testing.T) {
		t.Parallel()

		_, err := review.NewReviewerRegistry(nil)
		if err == nil {
			t.Fatal("expected error for nil config")
		}
	})

	t.Run("returns_nil_when_no_reviewers_configured", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Reviewers:        map[string]config.ReviewerConfig{},
			DefaultReviewers: []string{},
		}

		registry, err := review.NewReviewerRegistry(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if registry != nil {
			t.Fatal("expected nil registry when no reviewers configured")
		}
	})

	t.Run("errors_on_invalid_reviewer", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Reviewers: map[string]config.ReviewerConfig{
				"security": {
					Enabled: boolPtr(true),
					// Missing Provider and Model
				},
			},
			DefaultReviewers: []string{"security"},
		}

		_, err := review.NewReviewerRegistry(cfg)
		if err == nil {
			t.Fatal("expected error for invalid reviewer")
		}
	})

	t.Run("errors_on_invalid_default_reviewer", func(t *testing.T) {
		t.Parallel()

		cfg := &config.Config{
			Reviewers: map[string]config.ReviewerConfig{
				"security": {
					Enabled:  boolPtr(true),
					Provider: "anthropic",
					Model:    "claude-opus-4",
				},
			},
			DefaultReviewers: []string{"nonexistent"},
		}

		_, err := review.NewReviewerRegistry(cfg)
		if err == nil {
			t.Fatal("expected error for invalid default reviewer")
		}
	})
}

func TestReviewerRegistry_Get(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"security": {
				Enabled:  boolPtr(true),
				Provider: "anthropic",
				Model:    "claude-opus-4",
				Weight:   1.5,
				Persona:  "You are a security expert",
			},
		},
		DefaultReviewers: []string{"security"},
	}

	registry, err := review.NewReviewerRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("returns_existing_reviewer", func(t *testing.T) {
		t.Parallel()

		reviewer, err := registry.Get("security")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if reviewer.Name != "security" {
			t.Errorf("expected name 'security', got %s", reviewer.Name)
		}
		if reviewer.Provider != "anthropic" {
			t.Errorf("expected provider 'anthropic', got %s", reviewer.Provider)
		}
		if reviewer.Weight != 1.5 {
			t.Errorf("expected weight 1.5, got %f", reviewer.Weight)
		}
	})

	t.Run("errors_on_nonexistent_reviewer", func(t *testing.T) {
		t.Parallel()

		_, err := registry.Get("nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent reviewer")
		}
	})
}

func TestReviewerRegistry_List(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"security": {
				Enabled:  boolPtr(true),
				Provider: "anthropic",
				Model:    "claude-opus-4",
			},
			"performance": {
				Enabled:  boolPtr(false),
				Provider: "openai",
				Model:    "gpt-4o",
			},
		},
		DefaultReviewers: []string{"security"},
	}

	registry, err := review.NewReviewerRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reviewers := registry.List()
	if len(reviewers) != 2 {
		t.Fatalf("expected 2 reviewers, got %d", len(reviewers))
	}
}

func TestReviewerRegistry_ListEnabled(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"security": {
				Enabled:  boolPtr(true),
				Provider: "anthropic",
				Model:    "claude-opus-4",
			},
			"performance": {
				Enabled:  boolPtr(false),
				Provider: "openai",
				Model:    "gpt-4o",
			},
			"maintainability": {
				// nil enabled = defaults to true
				Provider: "gemini",
				Model:    "gemini-2.5-pro",
			},
		},
		DefaultReviewers: []string{"security"},
	}

	registry, err := review.NewReviewerRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reviewers := registry.ListEnabled()
	if len(reviewers) != 2 {
		t.Fatalf("expected 2 enabled reviewers, got %d", len(reviewers))
	}

	// Verify only enabled reviewers are returned
	for _, r := range reviewers {
		if !r.IsActive() {
			t.Errorf("expected only enabled reviewers, got disabled: %s", r.Name)
		}
	}
}

func TestReviewerRegistry_Resolve(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"security": {
				Enabled:  boolPtr(true),
				Provider: "anthropic",
				Model:    "claude-opus-4",
			},
			"performance": {
				Enabled:  boolPtr(true),
				Provider: "openai",
				Model:    "gpt-4o",
			},
			"disabled": {
				Enabled:  boolPtr(false),
				Provider: "gemini",
				Model:    "gemini-2.5-pro",
			},
		},
		DefaultReviewers: []string{"security", "performance"},
	}

	registry, err := review.NewReviewerRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("uses_defaults_when_names_empty", func(t *testing.T) {
		t.Parallel()

		reviewers, err := registry.Resolve(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reviewers) != 2 {
			t.Fatalf("expected 2 default reviewers, got %d", len(reviewers))
		}
	})

	t.Run("resolves_explicit_names", func(t *testing.T) {
		t.Parallel()

		reviewers, err := registry.Resolve([]string{"security"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reviewers) != 1 {
			t.Fatalf("expected 1 reviewer, got %d", len(reviewers))
		}
		if reviewers[0].Name != "security" {
			t.Errorf("expected 'security', got %s", reviewers[0].Name)
		}
	})

	t.Run("skips_disabled_reviewers", func(t *testing.T) {
		t.Parallel()

		reviewers, err := registry.Resolve([]string{"security", "disabled"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reviewers) != 1 {
			t.Fatalf("expected 1 enabled reviewer, got %d", len(reviewers))
		}
	})

	t.Run("errors_on_nonexistent_name", func(t *testing.T) {
		t.Parallel()

		_, err := registry.Resolve([]string{"nonexistent"})
		if err == nil {
			t.Fatal("expected error for nonexistent reviewer")
		}
	})

	t.Run("errors_when_all_disabled", func(t *testing.T) {
		t.Parallel()

		_, err := registry.Resolve([]string{"disabled"})
		if err == nil {
			t.Fatal("expected error when all resolved reviewers are disabled")
		}
	})
}

func TestReviewerRegistry_DefaultWeight(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"security": {
				Enabled:  boolPtr(true),
				Provider: "anthropic",
				Model:    "claude-opus-4",
				// Weight not set - should default to 1.0
			},
		},
		DefaultReviewers: []string{"security"},
	}

	registry, err := review.NewReviewerRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reviewer, err := registry.Get("security")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reviewer.Weight != 1.0 {
		t.Errorf("expected default weight 1.0, got %f", reviewer.Weight)
	}
}
