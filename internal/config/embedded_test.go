package config

import (
	"os"
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	cfg, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}

	// Verify basic structure is populated
	if cfg.HTTP.Timeout == "" {
		t.Error("expected HTTP.Timeout to be set")
	}

	// Verify providers are loaded
	if len(cfg.Providers) == 0 {
		t.Error("expected at least one provider to be configured")
	}

	// Verify OpenAI provider
	if openai, ok := cfg.Providers["openai"]; ok {
		if openai.GetDefaultModel() == "" {
			t.Error("expected openai.defaultModel to be set")
		}
	} else {
		t.Error("expected openai provider to be configured")
	}

	// Verify Anthropic provider
	if anthropic, ok := cfg.Providers["anthropic"]; ok {
		if anthropic.GetDefaultModel() == "" {
			t.Error("expected anthropic.defaultModel to be set")
		}
	} else {
		t.Error("expected anthropic provider to be configured")
	}
}

func TestLoadEmbeddedHasReviewers(t *testing.T) {
	cfg, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}

	// Verify the single default reviewer is present
	defaultReviewer, ok := cfg.Reviewers["default"]
	if !ok {
		t.Fatal("expected 'default' reviewer to be configured")
	}

	// Default reviewer should use anthropic (single API key experience)
	if defaultReviewer.Provider != "anthropic" {
		t.Errorf("expected default reviewer provider 'anthropic', got %q", defaultReviewer.Provider)
	}
	if defaultReviewer.Weight != 1.0 {
		t.Errorf("expected default reviewer weight 1.0, got %f", defaultReviewer.Weight)
	}

	// Default reviewer should have no persona (uses review.instructions directly)
	if defaultReviewer.Persona != "" {
		t.Error("expected default reviewer to have no persona (uses review.instructions)")
	}

	// Verify default reviewers list
	if len(cfg.DefaultReviewers) != 1 {
		t.Fatalf("expected 1 default reviewer, got %d", len(cfg.DefaultReviewers))
	}
	if cfg.DefaultReviewers[0] != "default" {
		t.Errorf("expected default reviewer 'default', got %q", cfg.DefaultReviewers[0])
	}
}

func TestLoadEmbeddedVerificationDisabled(t *testing.T) {
	cfg, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}

	// Verification should be disabled by default to avoid requiring a second API key.
	// It uses Gemini by default, which the default user (Anthropic-only) won't have.
	if cfg.Verification.Enabled {
		t.Error("expected Verification.Enabled to be false in embedded config (single API key experience)")
	}
}

func TestLoadEmbeddedMergePrecedence(t *testing.T) {
	// Load embedded as base
	embedded, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}

	// Create overlay config that overrides some values
	overlay := Config{
		HTTP: HTTPConfig{
			Timeout: "300s",
		},
		Review: ReviewConfig{
			BlockThreshold: "critical",
		},
	}

	// Merge: overlay should win
	merged, err := Merge(embedded, overlay)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	// Overlay values should take precedence
	if merged.HTTP.Timeout != "300s" {
		t.Errorf("expected merged HTTP.Timeout = %q, got %q", "300s", merged.HTTP.Timeout)
	}

	// Values not in overlay should come from embedded
	if len(merged.Providers) == 0 {
		t.Error("expected providers from embedded config to be preserved")
	}
	if len(merged.Reviewers) == 0 {
		t.Error("expected reviewers from embedded config to be preserved")
	}
}

func TestHasEmbeddedConfig(t *testing.T) {
	// The embedded config file is committed to the repo, so this should always pass.
	// This test validates the build pipeline hasn't broken the embed.
	if !HasEmbeddedConfig() {
		t.Fatal("HasEmbeddedConfig() = false; embedded config file is missing or empty")
	}
}

func TestEmbeddedConfigInCI(t *testing.T) {
	// In CI environments, we additionally verify the embedded config has real content
	// (not just a placeholder). This catches cases where the embed file gets corrupted
	// or the sync from root bop.yaml fails.
	if os.Getenv("CI") == "" {
		t.Skip("skipping CI-specific validation")
	}

	cfg, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}

	// Verify it's the real config, not a stub
	if len(cfg.Reviewers) < 1 {
		t.Errorf("expected at least 1 reviewer in CI, got %d", len(cfg.Reviewers))
	}
	if _, ok := cfg.Reviewers["default"]; !ok {
		t.Error("expected 'default' reviewer in CI")
	}
	if len(cfg.Providers) < 3 {
		t.Errorf("expected at least 3 providers in CI, got %d", len(cfg.Providers))
	}
}
