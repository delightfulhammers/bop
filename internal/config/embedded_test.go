package config

import (
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	// Skip if embedded config is not available (e.g., during local development without mage prepareEmbed)
	if !HasEmbeddedConfig() {
		t.Skip("embedded config not available (run 'mage prepareEmbed' first)")
	}

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
	if !HasEmbeddedConfig() {
		t.Skip("embedded config not available (run 'mage prepareEmbed' first)")
	}

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
	if !HasEmbeddedConfig() {
		t.Skip("embedded config not available (run 'mage prepareEmbed' first)")
	}

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
	// HasEmbeddedConfig should return true if the embed worked
	// This test verifies the embed directive compiled successfully
	hasConfig := HasEmbeddedConfig()

	// Log the result for debugging
	t.Logf("HasEmbeddedConfig() = %v", hasConfig)

	// The test passes either way - we just want to verify the function works.
	// In CI after mage prepareEmbed, this should be true.
	// During local development without prepareEmbed, this may be false.
}
