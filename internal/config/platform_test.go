package config_test

import (
	"reflect"
	"testing"

	"github.com/delightfulhammers/bop/internal/config"
)

func TestMergePlatformConfig_NilResult(t *testing.T) {
	local := config.Config{
		DefaultReviewers: []string{"default"},
		Output:           config.OutputConfig{Directory: "out"},
		Git:              config.GitConfig{RepositoryDir: "/repo"},
	}
	merged := config.MergePlatformConfig(local, nil)
	if !reflect.DeepEqual(merged, local) {
		t.Errorf("expected local config unchanged\ngot:  %+v\nwant: %+v", merged, local)
	}
}

func TestMergePlatformConfig_EmptyConfig(t *testing.T) {
	local := config.Config{
		DefaultReviewers: []string{"default"},
	}
	result := &config.PlatformConfigResult{Config: map[string]any{}}
	merged := config.MergePlatformConfig(local, result)
	if len(merged.DefaultReviewers) != 1 || merged.DefaultReviewers[0] != "default" {
		t.Errorf("expected local config unchanged with empty platform config, got %v", merged.DefaultReviewers)
	}
}

func TestMergePlatformConfig_OverrideReviewers(t *testing.T) {
	local := config.Config{
		DefaultReviewers: []string{"default"},
	}
	result := &config.PlatformConfigResult{
		Config: map[string]any{
			"reviewers": []any{"security", "maintainability", "performance"},
		},
		Tier: "pro",
	}
	merged := config.MergePlatformConfig(local, result)
	if len(merged.DefaultReviewers) != 3 {
		t.Fatalf("expected 3 reviewers, got %d: %v", len(merged.DefaultReviewers), merged.DefaultReviewers)
	}
	if merged.DefaultReviewers[0] != "security" {
		t.Errorf("expected first reviewer 'security', got %q", merged.DefaultReviewers[0])
	}
}

func TestMergePlatformConfig_OverrideWeights(t *testing.T) {
	local := config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"security": {Provider: "anthropic", Weight: 1.0},
		},
	}
	result := &config.PlatformConfigResult{
		Config: map[string]any{
			"weights": map[string]any{
				"security": 2.5,
			},
		},
	}
	merged := config.MergePlatformConfig(local, result)
	if merged.Reviewers["security"].Weight != 2.5 {
		t.Errorf("expected weight 2.5, got %f", merged.Reviewers["security"].Weight)
	}
	// Provider should be preserved from local
	if merged.Reviewers["security"].Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic' preserved, got %q", merged.Reviewers["security"].Provider)
	}
}

func TestMergePlatformConfig_OverrideModel(t *testing.T) {
	local := config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"default":  {Provider: "anthropic"},                             // No explicit model → should get platform model
			"security": {Provider: "anthropic", Model: "claude-haiku-4-5"},  // Has explicit model → preserved
			"perf":     {Provider: "openai"},                                // No explicit model → should get platform model
		},
	}
	result := &config.PlatformConfigResult{
		Config: map[string]any{
			"model": "claude-sonnet-4-6",
		},
	}
	merged := config.MergePlatformConfig(local, result)

	// Reviewers without explicit model get the platform model
	if merged.Reviewers["default"].Model != "claude-sonnet-4-6" {
		t.Errorf("default model: got %q, want %q", merged.Reviewers["default"].Model, "claude-sonnet-4-6")
	}
	if merged.Reviewers["perf"].Model != "claude-sonnet-4-6" {
		t.Errorf("perf model: got %q, want %q", merged.Reviewers["perf"].Model, "claude-sonnet-4-6")
	}

	// Reviewer with explicit model is preserved
	if merged.Reviewers["security"].Model != "claude-haiku-4-5" {
		t.Errorf("security model: got %q, want %q (explicit model should be preserved)", merged.Reviewers["security"].Model, "claude-haiku-4-5")
	}
}

func TestMergePlatformConfig_OverrideCustomPrompts(t *testing.T) {
	local := config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"security": {Provider: "anthropic"},
		},
	}
	result := &config.PlatformConfigResult{
		Config: map[string]any{
			"custom_prompts": map[string]any{
				"security": "You are an OWASP security expert",
			},
		},
	}
	merged := config.MergePlatformConfig(local, result)
	if merged.Reviewers["security"].Persona != "You are an OWASP security expert" {
		t.Errorf("expected persona override, got %q", merged.Reviewers["security"].Persona)
	}
}

func TestMergePlatformConfig_LocalOverrideFlag(t *testing.T) {
	local := config.Config{
		DefaultReviewers: []string{"local-reviewer"},
		Platform: config.PlatformConfig{
			Override: true,
		},
	}
	result := &config.PlatformConfigResult{
		Config: map[string]any{
			"reviewers": []any{"platform-reviewer"},
		},
	}
	merged := config.MergePlatformConfig(local, result)
	// Local should win when Override is true
	if len(merged.DefaultReviewers) != 1 || merged.DefaultReviewers[0] != "local-reviewer" {
		t.Errorf("expected local reviewers to win with Override=true, got %v", merged.DefaultReviewers)
	}
}

func TestMergePlatformConfig_PreservesNonPlatformFields(t *testing.T) {
	local := config.Config{
		Output: config.OutputConfig{Directory: "my-output"},
		Git:    config.GitConfig{RepositoryDir: "/my/repo"},
		Budget: config.BudgetConfig{HardCapUSD: 5.0},
	}
	result := &config.PlatformConfigResult{
		Config: map[string]any{
			"reviewers": []any{"security"},
		},
	}
	merged := config.MergePlatformConfig(local, result)

	// Non-platform fields should be preserved
	if merged.Output.Directory != "my-output" {
		t.Errorf("Output.Directory should be preserved, got %q", merged.Output.Directory)
	}
	if merged.Git.RepositoryDir != "/my/repo" {
		t.Errorf("Git.RepositoryDir should be preserved, got %q", merged.Git.RepositoryDir)
	}
	if merged.Budget.HardCapUSD != 5.0 {
		t.Errorf("Budget.HardCapUSD should be preserved, got %f", merged.Budget.HardCapUSD)
	}
}

func TestMergePlatformConfig_CreatesReviewerMapIfNil(t *testing.T) {
	local := config.Config{
		Reviewers: nil, // No local reviewers
	}
	result := &config.PlatformConfigResult{
		Config: map[string]any{
			"weights": map[string]any{
				"security": 1.5,
			},
		},
	}
	merged := config.MergePlatformConfig(local, result)
	if merged.Reviewers == nil {
		t.Fatal("expected Reviewers map to be created")
	}
	if merged.Reviewers["security"].Weight != 1.5 {
		t.Errorf("expected weight 1.5, got %f", merged.Reviewers["security"].Weight)
	}
}

func TestFormatTierDisplay(t *testing.T) {
	tests := []struct {
		tier string
		want string
	}{
		{"pro", "Pro"},
		{"free", "Free"},
		{"", "Free"},
		{"enterprise", "Unknown (enterprise)"},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			got := config.FormatTierDisplay(tt.tier)
			if got != tt.want {
				t.Errorf("FormatTierDisplay(%q) = %q, want %q", tt.tier, got, tt.want)
			}
		})
	}
}

func TestPlatformConfig_URLAndOverrideInMerge(t *testing.T) {
	base := config.Config{
		Platform: config.PlatformConfig{
			URL: "https://default.example.com",
		},
	}
	overlay := config.Config{
		Platform: config.PlatformConfig{
			URL: "https://custom.example.com",
		},
	}
	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if merged.Platform.URL != "https://custom.example.com" {
		t.Errorf("expected URL 'https://custom.example.com', got %q", merged.Platform.URL)
	}
}

func TestPlatformConfig_OverrideFieldInMerge(t *testing.T) {
	base := config.Config{}
	overlay := config.Config{
		Platform: config.PlatformConfig{
			Override: true,
		},
	}
	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !merged.Platform.Override {
		t.Error("expected Override=true after merge")
	}
}
