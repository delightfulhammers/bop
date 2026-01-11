package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/delightfulhammers/bop/internal/config"
)

func TestMergePrioritizesLaterConfigs(t *testing.T) {
	base := config.Config{
		Output: config.OutputConfig{Directory: "default"},
	}
	file := config.Config{
		Output: config.OutputConfig{Directory: "file"},
	}
	final := config.Config{
		Output: config.OutputConfig{Directory: "env"},
	}

	merged, err := config.Merge(base, file, final)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	if merged.Output.Directory != "env" {
		t.Fatalf("expected env directory to win, got %s", merged.Output.Directory)
	}
}

func TestLoadReadsFromFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	if err := os.WriteFile(file, []byte("output:\n  directory: file\n"), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("BOP_OUTPUT_DIRECTORY", "env")

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "BOP",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	if cfg.Output.Directory != "env" {
		t.Fatalf("expected env override, got %s", cfg.Output.Directory)
	}
}

func TestObservabilityConfigDefaults(t *testing.T) {
	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{},
		FileName:    "nonexistent",
		EnvPrefix:   "BOP",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Verify default observability settings
	if !cfg.Observability.Logging.Enabled {
		t.Error("expected logging to be enabled by default")
	}
	if cfg.Observability.Logging.Level != "info" {
		t.Errorf("expected default log level 'info', got %s", cfg.Observability.Logging.Level)
	}
	if cfg.Observability.Logging.Format != "human" {
		t.Errorf("expected default log format 'human', got %s", cfg.Observability.Logging.Format)
	}
	if !cfg.Observability.Logging.RedactAPIKeys {
		t.Error("expected API key redaction to be enabled by default")
	}
	if !cfg.Observability.Metrics.Enabled {
		t.Error("expected metrics to be enabled by default")
	}
}

func TestObservabilityConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
observability:
  logging:
    enabled: false
    level: debug
    format: json
    redactAPIKeys: false
  metrics:
    enabled: false
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "BOP",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Verify file overrides defaults
	if cfg.Observability.Logging.Enabled {
		t.Error("expected logging to be disabled from file config")
	}
	if cfg.Observability.Logging.Level != "debug" {
		t.Errorf("expected log level 'debug', got %s", cfg.Observability.Logging.Level)
	}
	if cfg.Observability.Logging.Format != "json" {
		t.Errorf("expected log format 'json', got %s", cfg.Observability.Logging.Format)
	}
	if cfg.Observability.Logging.RedactAPIKeys {
		t.Error("expected API key redaction to be disabled from file config")
	}
	if cfg.Observability.Metrics.Enabled {
		t.Error("expected metrics to be disabled from file config")
	}
}

func TestReviewActionsDefaults(t *testing.T) {
	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{},
		FileName:    "nonexistent",
		EnvPrefix:   "BOP",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Verify default review actions (sensible defaults)
	if cfg.Review.Actions.OnCritical != "request_changes" {
		t.Errorf("expected OnCritical 'request_changes', got %s", cfg.Review.Actions.OnCritical)
	}
	if cfg.Review.Actions.OnHigh != "request_changes" {
		t.Errorf("expected OnHigh 'request_changes', got %s", cfg.Review.Actions.OnHigh)
	}
	if cfg.Review.Actions.OnMedium != "comment" {
		t.Errorf("expected OnMedium 'comment', got %s", cfg.Review.Actions.OnMedium)
	}
	if cfg.Review.Actions.OnLow != "comment" {
		t.Errorf("expected OnLow 'comment', got %s", cfg.Review.Actions.OnLow)
	}
	if cfg.Review.Actions.OnClean != "approve" {
		t.Errorf("expected OnClean 'approve', got %s", cfg.Review.Actions.OnClean)
	}
}

func TestReviewActionsFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  actions:
    onCritical: comment
    onHigh: approve
    onMedium: request_changes
    onLow: approve
    onClean: comment
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "BOP",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Verify file overrides defaults
	if cfg.Review.Actions.OnCritical != "comment" {
		t.Errorf("expected OnCritical 'comment', got %s", cfg.Review.Actions.OnCritical)
	}
	if cfg.Review.Actions.OnHigh != "approve" {
		t.Errorf("expected OnHigh 'approve', got %s", cfg.Review.Actions.OnHigh)
	}
	if cfg.Review.Actions.OnMedium != "request_changes" {
		t.Errorf("expected OnMedium 'request_changes', got %s", cfg.Review.Actions.OnMedium)
	}
	if cfg.Review.Actions.OnLow != "approve" {
		t.Errorf("expected OnLow 'approve', got %s", cfg.Review.Actions.OnLow)
	}
	if cfg.Review.Actions.OnClean != "comment" {
		t.Errorf("expected OnClean 'comment', got %s", cfg.Review.Actions.OnClean)
	}
}

func TestReviewActionsEnvOverride(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  actions:
    onCritical: comment
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Environment variable should override file
	t.Setenv("BOP_REVIEW_ACTIONS_ONCRITICAL", "approve")

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "BOP",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Verify env var overrides file
	if cfg.Review.Actions.OnCritical != "approve" {
		t.Errorf("expected OnCritical 'approve' from env var, got %s", cfg.Review.Actions.OnCritical)
	}
}

func TestReviewActionsMerge(t *testing.T) {
	base := config.Config{
		Review: config.ReviewConfig{
			Instructions: "base instructions",
			Actions: config.ReviewActions{
				OnCritical: "request_changes",
				OnHigh:     "request_changes",
			},
		},
	}
	overlay := config.Config{
		Review: config.ReviewConfig{
			Actions: config.ReviewActions{
				OnHigh:   "approve",
				OnMedium: "comment",
			},
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Overlay with non-empty actions should replace
	if merged.Review.Actions.OnHigh != "approve" {
		t.Errorf("expected OnHigh 'approve' from overlay, got %s", merged.Review.Actions.OnHigh)
	}
	if merged.Review.Actions.OnMedium != "comment" {
		t.Errorf("expected OnMedium 'comment' from overlay, got %s", merged.Review.Actions.OnMedium)
	}
	// Instructions should be preserved from base (overlay is empty)
	if merged.Review.Instructions != "base instructions" {
		t.Errorf("expected base instructions to be preserved, got %s", merged.Review.Instructions)
	}
}

func TestBotUsernameDefault(t *testing.T) {
	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{},
		FileName:    "nonexistent",
		EnvPrefix:   "CR_TEST_BOTUSER",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	if cfg.Review.BotUsername != "github-actions[bot]" {
		t.Errorf("expected default BotUsername 'github-actions[bot]', got %s", cfg.Review.BotUsername)
	}
}

func TestBotUsernameFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  botUsername: "custom-bot[bot]"
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_BOTUSER2",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	if cfg.Review.BotUsername != "custom-bot[bot]" {
		t.Errorf("expected BotUsername 'custom-bot[bot]' from file, got %s", cfg.Review.BotUsername)
	}
}

func TestBotUsernameMerge(t *testing.T) {
	base := config.Config{
		Review: config.ReviewConfig{
			BotUsername: "base-bot[bot]",
		},
	}
	overlay := config.Config{
		Review: config.ReviewConfig{
			BotUsername: "overlay-bot[bot]",
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	if merged.Review.BotUsername != "overlay-bot[bot]" {
		t.Errorf("expected BotUsername 'overlay-bot[bot]' from overlay, got %s", merged.Review.BotUsername)
	}
}

func TestBotUsernameMergePreservesBase(t *testing.T) {
	base := config.Config{
		Review: config.ReviewConfig{
			BotUsername: "base-bot[bot]",
		},
	}
	overlay := config.Config{
		Review: config.ReviewConfig{
			// Empty BotUsername should preserve base
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	if merged.Review.BotUsername != "base-bot[bot]" {
		t.Errorf("expected BotUsername 'base-bot[bot]' from base, got %s", merged.Review.BotUsername)
	}
}

func TestVerificationConfigDefaults(t *testing.T) {
	cfg, err := config.Load(config.LoaderOptions{
		EnvPrefix: "CR_TEST_VERIF_DEFAULTS",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Check defaults are applied
	// Note: Verification disabled by default to avoid unexpected LLM costs
	if cfg.Verification.Enabled {
		t.Error("expected Verification.Enabled to be false by default (opt-in for cost reasons)")
	}
	if cfg.Verification.Depth != "medium" {
		t.Errorf("expected Verification.Depth 'medium', got %s", cfg.Verification.Depth)
	}
	if cfg.Verification.CostCeiling != 0.50 {
		t.Errorf("expected Verification.CostCeiling 0.50, got %f", cfg.Verification.CostCeiling)
	}
	if cfg.Verification.Confidence.Default != 75 {
		t.Errorf("expected Verification.Confidence.Default 75, got %d", cfg.Verification.Confidence.Default)
	}
	if cfg.Verification.Confidence.Critical != 60 {
		t.Errorf("expected Verification.Confidence.Critical 60, got %d", cfg.Verification.Confidence.Critical)
	}
	if cfg.Verification.Confidence.High != 70 {
		t.Errorf("expected Verification.Confidence.High 70, got %d", cfg.Verification.Confidence.High)
	}
	if cfg.Verification.Confidence.Medium != 75 {
		t.Errorf("expected Verification.Confidence.Medium 75, got %d", cfg.Verification.Confidence.Medium)
	}
	if cfg.Verification.Confidence.Low != 85 {
		t.Errorf("expected Verification.Confidence.Low 85, got %d", cfg.Verification.Confidence.Low)
	}
}

func TestVerificationConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
verification:
  enabled: true
  depth: deep
  costCeiling: 1.25
  confidence:
    default: 80
    critical: 50
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_VERIF_FILE",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	if !cfg.Verification.Enabled {
		t.Error("expected Verification.Enabled to be true from file")
	}
	if cfg.Verification.Depth != "deep" {
		t.Errorf("expected Verification.Depth 'deep', got %s", cfg.Verification.Depth)
	}
	if cfg.Verification.CostCeiling != 1.25 {
		t.Errorf("expected Verification.CostCeiling 1.25, got %f", cfg.Verification.CostCeiling)
	}
	if cfg.Verification.Confidence.Default != 80 {
		t.Errorf("expected Verification.Confidence.Default 80, got %d", cfg.Verification.Confidence.Default)
	}
	if cfg.Verification.Confidence.Critical != 50 {
		t.Errorf("expected Verification.Confidence.Critical 50, got %d", cfg.Verification.Confidence.Critical)
	}
}

func TestVerificationConfigMerge(t *testing.T) {
	base := config.Config{
		Verification: config.VerificationConfig{
			Enabled:     false,
			Depth:       "quick",
			CostCeiling: 0.25,
			Confidence: config.ConfidenceThresholds{
				Default:  70,
				Critical: 55,
				High:     65,
				Medium:   70,
				Low:      80,
			},
		},
	}
	overlay := config.Config{
		Verification: config.VerificationConfig{
			Enabled: true,
			Depth:   "deep",
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Field-by-field merge: overlay fields override base, unset fields preserved from base
	if !merged.Verification.Enabled {
		t.Error("expected Verification.Enabled to be true from overlay")
	}
	if merged.Verification.Depth != "deep" {
		t.Errorf("expected Verification.Depth 'deep' from overlay, got %s", merged.Verification.Depth)
	}
	// CostCeiling not set in overlay, should be preserved from base
	if merged.Verification.CostCeiling != 0.25 {
		t.Errorf("expected Verification.CostCeiling 0.25 from base, got %f", merged.Verification.CostCeiling)
	}
	// Confidence thresholds not set in overlay, should be preserved from base
	if merged.Verification.Confidence.Default != 70 {
		t.Errorf("expected Verification.Confidence.Default 70 from base, got %d", merged.Verification.Confidence.Default)
	}
}

func TestVerificationConfigMergePreservesBase(t *testing.T) {
	base := config.Config{
		Verification: config.VerificationConfig{
			Enabled:     true,
			Depth:       "quick",
			CostCeiling: 0.25,
			Confidence: config.ConfidenceThresholds{
				Default: 70,
			},
		},
	}
	overlay := config.Config{
		// Empty verification config - should preserve base
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	if !merged.Verification.Enabled {
		t.Error("expected Verification.Enabled to be preserved from base")
	}
	if merged.Verification.Depth != "quick" {
		t.Errorf("expected Verification.Depth 'quick' from base, got %s", merged.Verification.Depth)
	}
	if merged.Verification.CostCeiling != 0.25 {
		t.Errorf("expected Verification.CostCeiling 0.25 from base, got %f", merged.Verification.CostCeiling)
	}
	if merged.Verification.Confidence.Default != 70 {
		t.Errorf("expected Verification.Confidence.Default 70 from base, got %d", merged.Verification.Confidence.Default)
	}
}

func TestVerificationConfigMergeCanDisable(t *testing.T) {
	base := config.Config{
		Verification: config.VerificationConfig{
			Enabled:     true,
			Depth:       "medium",
			CostCeiling: 0.50,
		},
	}
	overlay := config.Config{
		Verification: config.VerificationConfig{
			Enabled: false,      // Explicitly disable
			Depth:   "disabled", // Set another field to signal intentional config
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Overlay should be able to disable verification when other fields are set
	if merged.Verification.Enabled {
		t.Error("expected Verification.Enabled to be false (disabled by overlay)")
	}
	if merged.Verification.Depth != "disabled" {
		t.Errorf("expected Verification.Depth 'disabled' from overlay, got %s", merged.Verification.Depth)
	}
}

// SizeGuards config tests

func TestSizeGuardsConfigDefaults(t *testing.T) {
	// With no config set, GetLimitsForProvider should return hardcoded defaults
	cfg := config.SizeGuardsConfig{}

	warn, max := cfg.GetLimitsForProvider("openai")

	if warn != 150000 {
		t.Errorf("expected default warn tokens 150000, got %d", warn)
	}
	if max != 200000 {
		t.Errorf("expected default max tokens 200000, got %d", max)
	}
}

func TestSizeGuardsConfigGlobalOverrides(t *testing.T) {
	cfg := config.SizeGuardsConfig{
		WarnTokens: 100000,
		MaxTokens:  120000,
	}

	warn, max := cfg.GetLimitsForProvider("openai")

	if warn != 100000 {
		t.Errorf("expected warn tokens 100000, got %d", warn)
	}
	if max != 120000 {
		t.Errorf("expected max tokens 120000, got %d", max)
	}
}

func TestSizeGuardsConfigPerProviderOverrides(t *testing.T) {
	cfg := config.SizeGuardsConfig{
		WarnTokens: 150000,
		MaxTokens:  200000,
		Providers: map[string]config.ProviderSizeConfig{
			"gemini": {
				WarnTokens: 500000,
				MaxTokens:  900000,
			},
		},
	}

	// Default provider gets global limits
	warn, max := cfg.GetLimitsForProvider("openai")
	if warn != 150000 {
		t.Errorf("expected openai warn tokens 150000, got %d", warn)
	}
	if max != 200000 {
		t.Errorf("expected openai max tokens 200000, got %d", max)
	}

	// Gemini gets per-provider limits
	warn, max = cfg.GetLimitsForProvider("gemini")
	if warn != 500000 {
		t.Errorf("expected gemini warn tokens 500000, got %d", warn)
	}
	if max != 900000 {
		t.Errorf("expected gemini max tokens 900000, got %d", max)
	}
}

func TestSizeGuardsConfigPartialProviderOverride(t *testing.T) {
	cfg := config.SizeGuardsConfig{
		WarnTokens: 150000,
		MaxTokens:  200000,
		Providers: map[string]config.ProviderSizeConfig{
			"openai": {
				MaxTokens: 120000, // Only override max (creates warn > max situation)
			},
		},
	}

	warn, max := cfg.GetLimitsForProvider("openai")

	// When max < warn due to partial override, values are swapped
	// to maintain warn <= max invariant
	if warn != 120000 {
		t.Errorf("expected warn tokens 120000 (swapped from max), got %d", warn)
	}
	if max != 150000 {
		t.Errorf("expected max tokens 150000 (swapped from warn), got %d", max)
	}
}

func TestSizeGuardsConfigSwapsWarnMaxIfMisconfigured(t *testing.T) {
	// Test that warn > max is corrected by swapping
	cfg := config.SizeGuardsConfig{
		WarnTokens: 200000, // Warn is higher than max (misconfigured)
		MaxTokens:  100000,
	}

	warn, max := cfg.GetLimitsForProvider("openai")

	// Should swap to maintain warn <= max
	if warn != 100000 {
		t.Errorf("expected warn tokens 100000 after swap, got %d", warn)
	}
	if max != 200000 {
		t.Errorf("expected max tokens 200000 after swap, got %d", max)
	}
}

func TestSizeGuardsConfigIsEnabled(t *testing.T) {
	// Default (nil) should be enabled
	cfg := config.SizeGuardsConfig{}
	if !cfg.IsEnabled() {
		t.Error("expected SizeGuards to be enabled by default")
	}

	// Explicit true
	enabled := true
	cfg.Enabled = &enabled
	if !cfg.IsEnabled() {
		t.Error("expected SizeGuards to be enabled when Enabled=true")
	}

	// Explicit false
	disabled := false
	cfg.Enabled = &disabled
	if cfg.IsEnabled() {
		t.Error("expected SizeGuards to be disabled when Enabled=false")
	}
}

func TestSizeGuardsConfigMerge(t *testing.T) {
	base := config.Config{
		SizeGuards: config.SizeGuardsConfig{
			WarnTokens: 100000,
			MaxTokens:  150000,
		},
	}
	overlay := config.Config{
		SizeGuards: config.SizeGuardsConfig{
			MaxTokens: 200000, // Only override max
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Warn should be from base, max from overlay
	if merged.SizeGuards.WarnTokens != 100000 {
		t.Errorf("expected WarnTokens 100000 from base, got %d", merged.SizeGuards.WarnTokens)
	}
	if merged.SizeGuards.MaxTokens != 200000 {
		t.Errorf("expected MaxTokens 200000 from overlay, got %d", merged.SizeGuards.MaxTokens)
	}
}

func TestSizeGuardsConfigMergeProviders(t *testing.T) {
	base := config.Config{
		SizeGuards: config.SizeGuardsConfig{
			WarnTokens: 150000,
			MaxTokens:  200000,
			Providers: map[string]config.ProviderSizeConfig{
				"openai": {WarnTokens: 100000, MaxTokens: 120000},
			},
		},
	}
	overlay := config.Config{
		SizeGuards: config.SizeGuardsConfig{
			Providers: map[string]config.ProviderSizeConfig{
				"gemini": {WarnTokens: 500000, MaxTokens: 900000},
			},
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Both providers should exist in merged config
	if len(merged.SizeGuards.Providers) != 2 {
		t.Fatalf("expected 2 providers in merged config, got %d", len(merged.SizeGuards.Providers))
	}

	openai := merged.SizeGuards.Providers["openai"]
	if openai.WarnTokens != 100000 || openai.MaxTokens != 120000 {
		t.Errorf("expected openai from base, got warn=%d max=%d", openai.WarnTokens, openai.MaxTokens)
	}

	gemini := merged.SizeGuards.Providers["gemini"]
	if gemini.WarnTokens != 500000 || gemini.MaxTokens != 900000 {
		t.Errorf("expected gemini from overlay, got warn=%d max=%d", gemini.WarnTokens, gemini.MaxTokens)
	}
}

func TestSizeGuardsConfigMergeCanDisable(t *testing.T) {
	enabled := true
	disabled := false

	base := config.Config{
		SizeGuards: config.SizeGuardsConfig{
			Enabled:    &enabled,
			WarnTokens: 150000,
			MaxTokens:  200000,
		},
	}
	overlay := config.Config{
		SizeGuards: config.SizeGuardsConfig{
			Enabled: &disabled,
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	if merged.SizeGuards.IsEnabled() {
		t.Error("expected SizeGuards to be disabled by overlay")
	}
	// Other fields should be preserved from base
	if merged.SizeGuards.WarnTokens != 150000 {
		t.Errorf("expected WarnTokens 150000 from base, got %d", merged.SizeGuards.WarnTokens)
	}
}

func TestSizeGuardsConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
sizeGuards:
  warnTokens: 100000
  maxTokens: 150000
  enabled: false
  providers:
    gemini:
      warnTokens: 800000
      maxTokens: 1000000
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_SIZEGUARDS",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	if cfg.SizeGuards.IsEnabled() {
		t.Error("expected SizeGuards to be disabled from file")
	}
	if cfg.SizeGuards.WarnTokens != 100000 {
		t.Errorf("expected WarnTokens 100000 from file, got %d", cfg.SizeGuards.WarnTokens)
	}
	if cfg.SizeGuards.MaxTokens != 150000 {
		t.Errorf("expected MaxTokens 150000 from file, got %d", cfg.SizeGuards.MaxTokens)
	}

	gemini := cfg.SizeGuards.Providers["gemini"]
	if gemini.WarnTokens != 800000 {
		t.Errorf("expected gemini WarnTokens 800000, got %d", gemini.WarnTokens)
	}
	if gemini.MaxTokens != 1000000 {
		t.Errorf("expected gemini MaxTokens 1000000, got %d", gemini.MaxTokens)
	}
}

// BlockThreshold tests

func TestBlockThresholdExpansion_Critical(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  blockThreshold: critical
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_THRESHOLD_CRIT",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Only critical should block
	if cfg.Review.Actions.OnCritical != "request_changes" {
		t.Errorf("expected OnCritical 'request_changes', got %s", cfg.Review.Actions.OnCritical)
	}
	if cfg.Review.Actions.OnHigh != "comment" {
		t.Errorf("expected OnHigh 'comment', got %s", cfg.Review.Actions.OnHigh)
	}
	if cfg.Review.Actions.OnMedium != "comment" {
		t.Errorf("expected OnMedium 'comment', got %s", cfg.Review.Actions.OnMedium)
	}
	if cfg.Review.Actions.OnLow != "comment" {
		t.Errorf("expected OnLow 'comment', got %s", cfg.Review.Actions.OnLow)
	}
}

func TestBlockThresholdExpansion_High(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  blockThreshold: high
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_THRESHOLD_HIGH",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Critical and high should block
	if cfg.Review.Actions.OnCritical != "request_changes" {
		t.Errorf("expected OnCritical 'request_changes', got %s", cfg.Review.Actions.OnCritical)
	}
	if cfg.Review.Actions.OnHigh != "request_changes" {
		t.Errorf("expected OnHigh 'request_changes', got %s", cfg.Review.Actions.OnHigh)
	}
	if cfg.Review.Actions.OnMedium != "comment" {
		t.Errorf("expected OnMedium 'comment', got %s", cfg.Review.Actions.OnMedium)
	}
	if cfg.Review.Actions.OnLow != "comment" {
		t.Errorf("expected OnLow 'comment', got %s", cfg.Review.Actions.OnLow)
	}
}

func TestBlockThresholdExpansion_Medium(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  blockThreshold: medium
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_THRESHOLD_MED",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Critical, high, and medium should block
	if cfg.Review.Actions.OnCritical != "request_changes" {
		t.Errorf("expected OnCritical 'request_changes', got %s", cfg.Review.Actions.OnCritical)
	}
	if cfg.Review.Actions.OnHigh != "request_changes" {
		t.Errorf("expected OnHigh 'request_changes', got %s", cfg.Review.Actions.OnHigh)
	}
	if cfg.Review.Actions.OnMedium != "request_changes" {
		t.Errorf("expected OnMedium 'request_changes', got %s", cfg.Review.Actions.OnMedium)
	}
	if cfg.Review.Actions.OnLow != "comment" {
		t.Errorf("expected OnLow 'comment', got %s", cfg.Review.Actions.OnLow)
	}
}

func TestBlockThresholdExpansion_Low(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  blockThreshold: low
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_THRESHOLD_LOW",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// All severities should block
	if cfg.Review.Actions.OnCritical != "request_changes" {
		t.Errorf("expected OnCritical 'request_changes', got %s", cfg.Review.Actions.OnCritical)
	}
	if cfg.Review.Actions.OnHigh != "request_changes" {
		t.Errorf("expected OnHigh 'request_changes', got %s", cfg.Review.Actions.OnHigh)
	}
	if cfg.Review.Actions.OnMedium != "request_changes" {
		t.Errorf("expected OnMedium 'request_changes', got %s", cfg.Review.Actions.OnMedium)
	}
	if cfg.Review.Actions.OnLow != "request_changes" {
		t.Errorf("expected OnLow 'request_changes', got %s", cfg.Review.Actions.OnLow)
	}
}

func TestBlockThresholdExpansion_None(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  blockThreshold: none
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_THRESHOLD_NONE",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// No severities should block
	if cfg.Review.Actions.OnCritical != "comment" {
		t.Errorf("expected OnCritical 'comment', got %s", cfg.Review.Actions.OnCritical)
	}
	if cfg.Review.Actions.OnHigh != "comment" {
		t.Errorf("expected OnHigh 'comment', got %s", cfg.Review.Actions.OnHigh)
	}
	if cfg.Review.Actions.OnMedium != "comment" {
		t.Errorf("expected OnMedium 'comment', got %s", cfg.Review.Actions.OnMedium)
	}
	if cfg.Review.Actions.OnLow != "comment" {
		t.Errorf("expected OnLow 'comment', got %s", cfg.Review.Actions.OnLow)
	}
}

func TestBlockThresholdExpansion_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  blockThreshold: HIGH
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_THRESHOLD_CASE",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Should work same as "high"
	if cfg.Review.Actions.OnCritical != "request_changes" {
		t.Errorf("expected OnCritical 'request_changes', got %s", cfg.Review.Actions.OnCritical)
	}
	if cfg.Review.Actions.OnHigh != "request_changes" {
		t.Errorf("expected OnHigh 'request_changes', got %s", cfg.Review.Actions.OnHigh)
	}
}

func TestBlockThresholdExpansion_ExplicitActionsOverride(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  blockThreshold: high
  actions:
    onHigh: comment
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_THRESHOLD_OVERRIDE",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Threshold says high blocks, but explicit action overrides to comment
	if cfg.Review.Actions.OnCritical != "request_changes" {
		t.Errorf("expected OnCritical 'request_changes', got %s", cfg.Review.Actions.OnCritical)
	}
	if cfg.Review.Actions.OnHigh != "comment" {
		t.Errorf("expected OnHigh 'comment' (explicit override), got %s", cfg.Review.Actions.OnHigh)
	}
}

func TestBlockThresholdExpansion_InvalidThreshold(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  blockThreshold: invalid_value
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_THRESHOLD_INVALID",
	})

	// Invalid threshold should now return an error instead of silently falling back
	if err == nil {
		t.Fatal("expected error for invalid blockThreshold, got nil")
	}

	// Verify error message contains useful information
	errMsg := err.Error()
	if !strings.Contains(errMsg, "invalid blockThreshold") {
		t.Errorf("expected error to mention 'invalid blockThreshold', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "invalid_value") {
		t.Errorf("expected error to mention the invalid value 'invalid_value', got: %s", errMsg)
	}
}

// AlwaysBlockCategories tests

func TestAlwaysBlockCategories_FromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  alwaysBlockCategories:
    - security
    - bug
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_CATEGORIES_FILE",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	if len(cfg.Review.AlwaysBlockCategories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cfg.Review.AlwaysBlockCategories))
	}
	if cfg.Review.AlwaysBlockCategories[0] != "security" {
		t.Errorf("expected first category 'security', got %s", cfg.Review.AlwaysBlockCategories[0])
	}
	if cfg.Review.AlwaysBlockCategories[1] != "bug" {
		t.Errorf("expected second category 'bug', got %s", cfg.Review.AlwaysBlockCategories[1])
	}
}

func TestAlwaysBlockCategories_Merge(t *testing.T) {
	base := config.Config{
		Review: config.ReviewConfig{
			AlwaysBlockCategories: []string{"security"},
		},
	}
	overlay := config.Config{
		Review: config.ReviewConfig{
			AlwaysBlockCategories: []string{"bug", "data-loss"},
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Should have union of both
	if len(merged.Review.AlwaysBlockCategories) != 3 {
		t.Fatalf("expected 3 categories, got %d: %v", len(merged.Review.AlwaysBlockCategories), merged.Review.AlwaysBlockCategories)
	}
}

func TestAlwaysBlockCategories_MergeDeduplicates(t *testing.T) {
	base := config.Config{
		Review: config.ReviewConfig{
			AlwaysBlockCategories: []string{"security", "bug"},
		},
	}
	overlay := config.Config{
		Review: config.ReviewConfig{
			AlwaysBlockCategories: []string{"bug", "Security"}, // Duplicate with different case
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Should deduplicate (case-insensitive)
	if len(merged.Review.AlwaysBlockCategories) != 2 {
		t.Fatalf("expected 2 categories (deduplicated), got %d: %v", len(merged.Review.AlwaysBlockCategories), merged.Review.AlwaysBlockCategories)
	}
}

func TestBlockThresholdMerge_OverlayWins(t *testing.T) {
	base := config.Config{
		Review: config.ReviewConfig{
			BlockThreshold: "high",
		},
	}
	overlay := config.Config{
		Review: config.ReviewConfig{
			BlockThreshold: "medium",
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Medium threshold should win
	if merged.Review.Actions.OnMedium != "request_changes" {
		t.Errorf("expected OnMedium 'request_changes' from overlay threshold, got %s", merged.Review.Actions.OnMedium)
	}
}

// Reviewer config tests (Phase 3.2)

func TestMergeReviewers_OverlayWins(t *testing.T) {
	enabled := true
	base := config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"security": {
				Enabled:  &enabled,
				Provider: "anthropic",
				Model:    "claude-opus-4",
				Weight:   1.0,
			},
		},
	}
	overlay := config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"security": {
				Enabled:  &enabled,
				Provider: "openai",
				Model:    "gpt-4o",
				Weight:   2.0,
			},
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Overlay should completely replace reviewer
	if merged.Reviewers["security"].Provider != "openai" {
		t.Errorf("expected provider 'openai' from overlay, got %s", merged.Reviewers["security"].Provider)
	}
	if merged.Reviewers["security"].Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o' from overlay, got %s", merged.Reviewers["security"].Model)
	}
	if merged.Reviewers["security"].Weight != 2.0 {
		t.Errorf("expected weight 2.0 from overlay, got %f", merged.Reviewers["security"].Weight)
	}
}

func TestMergeReviewers_CombinesMaps(t *testing.T) {
	enabled := true
	base := config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"security": {
				Enabled:  &enabled,
				Provider: "anthropic",
				Model:    "claude-opus-4",
			},
		},
	}
	overlay := config.Config{
		Reviewers: map[string]config.ReviewerConfig{
			"performance": {
				Enabled:  &enabled,
				Provider: "openai",
				Model:    "gpt-4o",
			},
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Both reviewers should exist
	if len(merged.Reviewers) != 2 {
		t.Fatalf("expected 2 reviewers, got %d", len(merged.Reviewers))
	}
	if _, ok := merged.Reviewers["security"]; !ok {
		t.Error("expected 'security' reviewer to exist")
	}
	if _, ok := merged.Reviewers["performance"]; !ok {
		t.Error("expected 'performance' reviewer to exist")
	}
}

func TestMergeDefaultReviewers_OverlayWins(t *testing.T) {
	base := config.Config{
		DefaultReviewers: []string{"security", "performance"},
	}
	overlay := config.Config{
		DefaultReviewers: []string{"maintainability"},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Overlay completely replaces default reviewers
	if len(merged.DefaultReviewers) != 1 {
		t.Fatalf("expected 1 default reviewer, got %d", len(merged.DefaultReviewers))
	}
	if merged.DefaultReviewers[0] != "maintainability" {
		t.Errorf("expected 'maintainability', got %s", merged.DefaultReviewers[0])
	}
}

func TestMergeDefaultReviewers_PreservesBaseWhenOverlayEmpty(t *testing.T) {
	base := config.Config{
		DefaultReviewers: []string{"security", "performance"},
	}
	overlay := config.Config{
		// Empty DefaultReviewers
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Base should be preserved when overlay is empty
	if len(merged.DefaultReviewers) != 2 {
		t.Fatalf("expected 2 default reviewers, got %d", len(merged.DefaultReviewers))
	}
}

func TestReviewerConfigTriStateBool(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name     string
		enabled  *bool
		expected bool
	}{
		{name: "nil_defaults_to_enabled", enabled: nil, expected: true},
		{name: "explicit_true", enabled: &enabled, expected: true},
		{name: "explicit_false", enabled: &disabled, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ReviewerConfig{
				Enabled: tt.enabled,
			}
			got := cfg.IsEnabled()
			if got != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMergeReviewers_NilMaps(t *testing.T) {
	// Both nil maps
	base := config.Config{}
	overlay := config.Config{}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	if merged.Reviewers != nil {
		t.Error("expected nil Reviewers map when both are nil")
	}
}

func TestReviewerConfig_HasAllFields(t *testing.T) {
	enabled := true
	maxTokens := 64000

	cfg := config.ReviewerConfig{
		Enabled:         &enabled,
		Provider:        "anthropic",
		Model:           "claude-opus-4",
		APIKey:          "test-key",
		Weight:          1.5,
		Persona:         "You are a security expert",
		Focus:           []string{"security", "authentication"},
		Ignore:          []string{"style", "documentation"},
		MaxOutputTokens: &maxTokens,
	}

	// Verify all fields are accessible
	if !cfg.IsEnabled() {
		t.Error("expected enabled")
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %s", cfg.Provider)
	}
	if cfg.Model != "claude-opus-4" {
		t.Errorf("expected model 'claude-opus-4', got %s", cfg.Model)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("expected apiKey 'test-key', got %s", cfg.APIKey)
	}
	if cfg.Weight != 1.5 {
		t.Errorf("expected weight 1.5, got %f", cfg.Weight)
	}
	if cfg.Persona != "You are a security expert" {
		t.Errorf("unexpected persona")
	}
	if len(cfg.Focus) != 2 {
		t.Errorf("expected 2 focus categories, got %d", len(cfg.Focus))
	}
	if len(cfg.Ignore) != 2 {
		t.Errorf("expected 2 ignore categories, got %d", len(cfg.Ignore))
	}
	if *cfg.MaxOutputTokens != 64000 {
		t.Errorf("expected maxOutputTokens 64000, got %d", *cfg.MaxOutputTokens)
	}
}

func TestReviewerConfig_EnvExpansion(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
reviewers:
  security:
    enabled: true
    provider: anthropic
    model: claude-opus-4
    apiKey: "${TEST_REVIEWER_API_KEY}"
    weight: 1.5
    persona: "Security expert for ${TEST_ORG_NAME}"
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("TEST_REVIEWER_API_KEY", "sk-test-key-123")
	t.Setenv("TEST_ORG_NAME", "ACME Corp")

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_REVIEWER_ENV",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	reviewer, ok := cfg.Reviewers["security"]
	if !ok {
		t.Fatal("expected 'security' reviewer to exist")
	}

	if reviewer.APIKey != "sk-test-key-123" {
		t.Errorf("expected APIKey 'sk-test-key-123', got %s", reviewer.APIKey)
	}
	if !strings.Contains(reviewer.Persona, "ACME Corp") {
		t.Errorf("expected Persona to contain 'ACME Corp', got %s", reviewer.Persona)
	}
}

func TestReviewerConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
reviewers:
  security:
    enabled: true
    provider: anthropic
    model: claude-opus-4
    weight: 1.5
    persona: |
      You are a security expert specializing in OWASP vulnerabilities.
    focus:
      - security
      - authentication
    ignore:
      - style
      - documentation

  maintainability:
    enabled: true
    provider: openai
    model: gpt-4o
    weight: 1.0
    focus:
      - maintainability
      - complexity

defaultReviewers:
  - security
  - maintainability
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_REVIEWER_FILE",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	// Verify reviewers
	if len(cfg.Reviewers) != 2 {
		t.Fatalf("expected 2 reviewers, got %d", len(cfg.Reviewers))
	}

	security := cfg.Reviewers["security"]
	if security.Provider != "anthropic" {
		t.Errorf("expected security provider 'anthropic', got %s", security.Provider)
	}
	if security.Model != "claude-opus-4" {
		t.Errorf("expected security model 'claude-opus-4', got %s", security.Model)
	}
	if security.Weight != 1.5 {
		t.Errorf("expected security weight 1.5, got %f", security.Weight)
	}
	if len(security.Focus) != 2 {
		t.Errorf("expected 2 security focus categories, got %d", len(security.Focus))
	}
	if len(security.Ignore) != 2 {
		t.Errorf("expected 2 security ignore categories, got %d", len(security.Ignore))
	}

	// Verify default_reviewers
	if len(cfg.DefaultReviewers) != 2 {
		t.Fatalf("expected 2 default reviewers, got %d", len(cfg.DefaultReviewers))
	}
	if cfg.DefaultReviewers[0] != "security" {
		t.Errorf("expected first default reviewer 'security', got %s", cfg.DefaultReviewers[0])
	}
	if cfg.DefaultReviewers[1] != "maintainability" {
		t.Errorf("expected second default reviewer 'maintainability', got %s", cfg.DefaultReviewers[1])
	}
}

// PostOutOfDiffAsComments tests

func TestPostOutOfDiffAsComments_DefaultsToTrue(t *testing.T) {
	// ReviewConfig with nil PostOutOfDiffAsComments should default to true
	cfg := config.ReviewConfig{}

	if !cfg.ShouldPostOutOfDiff() {
		t.Error("expected ShouldPostOutOfDiff() to return true by default")
	}
}

func TestPostOutOfDiffAsComments_ExplicitTrue(t *testing.T) {
	enabled := true
	cfg := config.ReviewConfig{
		PostOutOfDiffAsComments: &enabled,
	}

	if !cfg.ShouldPostOutOfDiff() {
		t.Error("expected ShouldPostOutOfDiff() to return true when explicitly set")
	}
}

func TestPostOutOfDiffAsComments_ExplicitFalse(t *testing.T) {
	disabled := false
	cfg := config.ReviewConfig{
		PostOutOfDiffAsComments: &disabled,
	}

	if cfg.ShouldPostOutOfDiff() {
		t.Error("expected ShouldPostOutOfDiff() to return false when explicitly disabled")
	}
}

func TestPostOutOfDiffAsComments_FromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bop.yaml")
	content := `
review:
  postOutOfDiffAsComments: false
`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: []string{dir},
		FileName:    "bop",
		EnvPrefix:   "CR_TEST_OOD_FILE",
	})
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}

	if cfg.Review.ShouldPostOutOfDiff() {
		t.Error("expected ShouldPostOutOfDiff() to return false from file config")
	}
}

func TestPostOutOfDiffAsComments_MergeOverlayWins(t *testing.T) {
	enabled := true
	disabled := false

	base := config.Config{
		Review: config.ReviewConfig{
			PostOutOfDiffAsComments: &enabled,
		},
	}
	overlay := config.Config{
		Review: config.ReviewConfig{
			PostOutOfDiffAsComments: &disabled,
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	if merged.Review.ShouldPostOutOfDiff() {
		t.Error("expected ShouldPostOutOfDiff() to return false from overlay")
	}
}

func TestPostOutOfDiffAsComments_MergePreservesBase(t *testing.T) {
	disabled := false

	base := config.Config{
		Review: config.ReviewConfig{
			PostOutOfDiffAsComments: &disabled,
		},
	}
	overlay := config.Config{
		Review: config.ReviewConfig{
			// No PostOutOfDiffAsComments set - should preserve base
		},
	}

	merged, err := config.Merge(base, overlay)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	if merged.Review.ShouldPostOutOfDiff() {
		t.Error("expected ShouldPostOutOfDiff() to return false (preserved from base)")
	}
}
