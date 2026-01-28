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

	// Verify all 4 reviewers are present (security, architecture, performance, observability)
	expectedReviewers := []string{"security", "architecture", "performance", "observability"}
	for _, name := range expectedReviewers {
		if _, ok := cfg.Reviewers[name]; !ok {
			t.Errorf("expected reviewer %q to be configured", name)
		}
	}

	// Verify default reviewers
	if len(cfg.DefaultReviewers) == 0 {
		t.Error("expected defaultReviewers to be set")
	}

	// Verify security reviewer has expected properties
	if security, ok := cfg.Reviewers["security"]; ok {
		if security.Provider == "" {
			t.Error("expected security reviewer to have a provider")
		}
		if security.Weight == 0 {
			t.Error("expected security reviewer to have a weight")
		}
		if security.Persona == "" {
			t.Error("expected security reviewer to have a persona")
		}
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
	if len(cfg.Reviewers) < 4 {
		t.Errorf("expected at least 4 reviewers in CI, got %d", len(cfg.Reviewers))
	}
	if len(cfg.Providers) < 3 {
		t.Errorf("expected at least 3 providers in CI, got %d", len(cfg.Providers))
	}
}
