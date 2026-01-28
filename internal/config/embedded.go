package config

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/spf13/viper"
)

// embeddedConfigYAML contains the default bop.yaml configuration embedded at build time.
// This enables a zero-config user experience where bop works out of the box.
// The file is copied from the repository root by `mage prepareEmbed` before building.
// We use a subdirectory to avoid the file being picked up by config.Load() tests.
//
//go:embed embed/bop.yaml
var embeddedConfigYAML []byte

// LoadEmbedded returns the embedded default configuration.
// This is the lowest-priority configuration layer that everything else merges over.
// The embedded config is NOT gated by entitlements since it's controlled by bop maintainers.
//
// Load order (lowest to highest priority):
// 1. Embedded bop.yaml (this function) - always loaded, baked into binary
// 2. Platform API config - fetched from platform for authenticated users
// 3. Local files (~/.config/bop/bop.yaml, ./bop.yaml) - requires local-bop-config entitlement
// 4. CLI flags - highest priority
func LoadEmbedded() (Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	// Apply defaults first (same as Load does)
	setDefaults(v)

	// Read embedded config
	if err := v.ReadConfig(bytes.NewReader(embeddedConfigYAML)); err != nil {
		return Config{}, fmt.Errorf("read embedded config: %w", err)
	}

	// Unmarshal to Config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal embedded config: %w", err)
	}

	// Expand environment variables in config values (e.g., ${ANTHROPIC_API_KEY})
	cfg = expandEnvVars(cfg)

	// Process review config: expand threshold and apply defaults
	reviewConfig, err := processReviewConfig(cfg.Review)
	if err != nil {
		return Config{}, fmt.Errorf("process review config: %w", err)
	}
	cfg.Review = reviewConfig

	return cfg, nil
}

// HasEmbeddedConfig returns true if the embedded config is available.
// This is always true for properly built binaries.
func HasEmbeddedConfig() bool {
	return len(embeddedConfigYAML) > 0
}
