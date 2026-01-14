package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/viper"
)

// LoaderOptions describes how configuration should be discovered.
type LoaderOptions struct {
	ConfigPaths []string
	FileName    string
	EnvPrefix   string
}

// Load returns the merged configuration from files and environment variables.
// Config files are loaded in order: user config (~/.config/bop/) first as base,
// then project config (current directory) overlays it. Environment variables
// override both.
func Load(opts LoaderOptions) (Config, error) {
	name := opts.FileName
	if name == "" {
		name = "bop"
	}

	prefix := opts.EnvPrefix
	if prefix == "" {
		prefix = "BOP"
	}

	// Create master viper instance with defaults and env var handling
	v := viper.New()
	v.SetEnvPrefix(prefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AllowEmptyEnv(true)
	setDefaults(v)

	// Find all config files in priority order (base → overlay)
	configFiles := locateConfigFiles(name, opts.ConfigPaths)

	// Read and merge each config file (later files override earlier)
	for _, configFile := range configFiles {
		fileViper := viper.New()
		fileViper.SetConfigFile(configFile)
		if err := fileViper.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config %s: %w", configFile, err)
		}
		// Merge this file's settings into master viper
		if err := v.MergeConfigMap(fileViper.AllSettings()); err != nil {
			return Config{}, fmt.Errorf("merge config %s: %w", configFile, err)
		}
	}

	// Unmarshal merged config
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	// Expand environment variables in config values
	cfg = expandEnvVars(cfg)

	// Process review config: expand threshold and apply defaults
	reviewConfig, err := processReviewConfig(cfg.Review)
	if err != nil {
		return Config{}, fmt.Errorf("process review config: %w", err)
	}
	cfg.Review = reviewConfig

	return cfg, nil
}

// processReviewConfig applies threshold expansion and defaults to the review configuration.
// This is called after loading to ensure threshold-based configuration works correctly.
// Returns an error if the blockThreshold value is invalid.
func processReviewConfig(review ReviewConfig) (ReviewConfig, error) {
	// Expand threshold to per-severity actions
	expandedActions, err := expandBlockThreshold(review.BlockThreshold)
	if err != nil {
		return ReviewConfig{}, err
	}

	// Merge expanded threshold with explicit actions (explicit wins)
	review.Actions = mergeReviewActions(expandedActions, review.Actions)

	// Apply defaults for any remaining empty action slots
	review.Actions = applyActionDefaults(review.Actions)

	return review, nil
}

// expandEnvVars expands ${VAR} and $VAR syntax in configuration strings.
func expandEnvVars(cfg Config) Config {
	// Expand provider API keys and models
	for name, provider := range cfg.Providers {
		provider.APIKey = expandEnvString(provider.APIKey)
		provider.Model = expandEnvString(provider.Model)

		// Expand provider-specific HTTP overrides
		if provider.Timeout != nil {
			timeout := expandEnvString(*provider.Timeout)
			provider.Timeout = &timeout
		}
		if provider.InitialBackoff != nil {
			backoff := expandEnvString(*provider.InitialBackoff)
			provider.InitialBackoff = &backoff
		}
		if provider.MaxBackoff != nil {
			backoff := expandEnvString(*provider.MaxBackoff)
			provider.MaxBackoff = &backoff
		}

		cfg.Providers[name] = provider
	}

	// Phase 3.2: Expand reviewer config fields
	for name, reviewer := range cfg.Reviewers {
		reviewer.APIKey = expandEnvString(reviewer.APIKey)
		reviewer.Provider = expandEnvString(reviewer.Provider)
		reviewer.Model = expandEnvString(reviewer.Model)
		reviewer.Persona = expandEnvString(reviewer.Persona)
		reviewer.Focus = expandEnvStringSlice(reviewer.Focus)
		reviewer.Ignore = expandEnvStringSlice(reviewer.Ignore)

		// Expand reviewer-specific HTTP overrides
		if reviewer.Timeout != nil {
			timeout := expandEnvString(*reviewer.Timeout)
			reviewer.Timeout = &timeout
		}
		if reviewer.InitialBackoff != nil {
			backoff := expandEnvString(*reviewer.InitialBackoff)
			reviewer.InitialBackoff = &backoff
		}
		if reviewer.MaxBackoff != nil {
			backoff := expandEnvString(*reviewer.MaxBackoff)
			reviewer.MaxBackoff = &backoff
		}

		cfg.Reviewers[name] = reviewer
	}

	// Expand HTTP config
	cfg.HTTP.Timeout = expandEnvString(cfg.HTTP.Timeout)
	cfg.HTTP.InitialBackoff = expandEnvString(cfg.HTTP.InitialBackoff)
	cfg.HTTP.MaxBackoff = expandEnvString(cfg.HTTP.MaxBackoff)

	// Expand merge config
	cfg.Merge.Provider = expandEnvString(cfg.Merge.Provider)
	cfg.Merge.Model = expandEnvString(cfg.Merge.Model)
	cfg.Merge.Strategy = expandEnvString(cfg.Merge.Strategy)

	// Expand verification config
	cfg.Verification.Provider = expandEnvString(cfg.Verification.Provider)
	cfg.Verification.Model = expandEnvString(cfg.Verification.Model)
	cfg.Verification.Depth = expandEnvString(cfg.Verification.Depth)

	// Expand git config
	cfg.Git.RepositoryDir = expandEnvString(cfg.Git.RepositoryDir)

	// Expand output config
	cfg.Output.Directory = expandEnvString(cfg.Output.Directory)

	// Expand budget config
	cfg.Budget.DegradationPolicy = expandEnvStringSlice(cfg.Budget.DegradationPolicy)

	// Expand redaction config
	cfg.Redaction.DenyGlobs = expandEnvStringSlice(cfg.Redaction.DenyGlobs)
	cfg.Redaction.AllowGlobs = expandEnvStringSlice(cfg.Redaction.AllowGlobs)

	// Expand store config
	cfg.Store.Path = expandEnvString(cfg.Store.Path)

	// Expand auth config
	cfg.Auth.Mode = expandEnvString(cfg.Auth.Mode)
	cfg.Auth.ServiceURL = expandEnvString(cfg.Auth.ServiceURL)
	cfg.Auth.ProductID = expandEnvString(cfg.Auth.ProductID)

	// Expand observability config
	cfg.Observability.Logging.Level = expandEnvString(cfg.Observability.Logging.Level)
	cfg.Observability.Logging.Format = expandEnvString(cfg.Observability.Logging.Format)

	return cfg
}

// expandEnvString replaces ${VAR} or $VAR with environment variable values
// and expands ~ to the user's home directory when it appears at the start.
func expandEnvString(s string) string {
	if s == "" {
		return s
	}

	// Expand tilde at start of path (shell convention)
	// Only expand if tilde is at the beginning and not escaped
	if len(s) > 0 && s[0] == '~' && (len(s) == 1 || s[1] == '/') {
		if home, err := os.UserHomeDir(); err == nil {
			if len(s) == 1 {
				s = home
			} else if len(s) == 2 && s[1] == '/' {
				// Special case: "~/" becomes "$HOME/"
				s = home + "/"
			} else {
				s = filepath.Join(home, s[2:])
			}
		}
	}

	// Replace ${VAR} syntax
	re := regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)
	s = re.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1] // Remove ${ and }
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return match // Keep original if not found
	})

	// Replace $VAR syntax (without braces)
	re = regexp.MustCompile(`\$([A-Z_][A-Z0-9_]*)`)
	s = re.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[1:] // Remove $
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return match // Keep original if not found
	})

	return s
}

// expandEnvStringSlice expands environment variables in a slice of strings.
func expandEnvStringSlice(slice []string) []string {
	if len(slice) == 0 {
		return slice
	}
	result := make([]string, len(slice))
	for i, s := range slice {
		result[i] = expandEnvString(s)
	}
	return result
}

// locateConfigFiles returns all matching config files in load order (base → overlay).
// User config (~/.config/bop/) is loaded first as base, then paths in order,
// finally current directory. Later files override earlier ones during merge.
func locateConfigFiles(name string, paths []string) []string {
	var files []string
	seen := make(map[string]bool)

	// Build search paths in priority order (lowest first, loaded first)
	searchPaths := make([]string, 0, len(paths)+2)

	// 1. User config directory (lowest priority, loaded first as base)
	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(home, ".config", "bop"))
	}

	// 2. Explicit paths from caller
	searchPaths = append(searchPaths, paths...)

	// 3. Current directory (highest priority, loaded last as overlay)
	searchPaths = append(searchPaths, ".")

	for _, dir := range searchPaths {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name+".yaml")

		// Normalize path to avoid duplicates from "." resolution
		absCandidate, err := filepath.Abs(candidate)
		if err != nil {
			absCandidate = candidate
		}

		if seen[absCandidate] {
			continue
		}

		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			files = append(files, candidate)
			seen[absCandidate] = true
		}
	}
	return files
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("output.directory", "out")

	// HTTP defaults
	v.SetDefault("http.timeout", "60s")
	v.SetDefault("http.maxRetries", 5)
	v.SetDefault("http.initialBackoff", "2s")
	v.SetDefault("http.maxBackoff", "32s")
	v.SetDefault("http.backoffMultiplier", 2.0)

	// Determinism defaults (Phase 2)
	v.SetDefault("determinism.enabled", true)
	v.SetDefault("determinism.temperature", 0.0)
	v.SetDefault("determinism.useSeed", true)

	// Redaction defaults (Phase 2)
	v.SetDefault("redaction.enabled", true)

	// Merge defaults (Phase 2)
	v.SetDefault("merge.enabled", true)
	v.SetDefault("merge.strategy", "consensus")

	// Store defaults (Phase 3)
	v.SetDefault("store.enabled", true)
	v.SetDefault("store.path", defaultStorePath())

	// Auth defaults (Week 14: Platform Authentication)
	// Default to legacy mode for backward compatibility
	v.SetDefault("auth.mode", "legacy")
	v.SetDefault("auth.productId", "bop")
	// Note: auth.serviceUrl has no default - must be set when mode is "platform"

	// Observability defaults (Phase 3)
	v.SetDefault("observability.logging.enabled", true)
	v.SetDefault("observability.logging.level", "info")
	v.SetDefault("observability.logging.format", "human")
	v.SetDefault("observability.logging.redactAPIKeys", true)
	v.SetDefault("observability.logging.maxContentBytes", 51200) // 50KB default for trace logging
	v.SetDefault("observability.metrics.enabled", true)

	// Provider defaults (Phase 1 + Phase 2)
	// Note: providers.*.enabled is intentionally not defaulted.
	// When nil (unset), isProviderEnabled uses API key presence to determine if enabled.
	// This maintains backward compatibility while allowing explicit enabled: false to work.
	v.SetDefault("providers.openai.defaultModel", "gpt-5.2")
	v.SetDefault("providers.anthropic.defaultModel", "claude-sonnet-4-5")
	v.SetDefault("providers.gemini.defaultModel", "gemini-3-pro-preview")
	// NOTE: Ollama default intentionally omitted. Setting a default for keyless providers
	// causes Viper to create a config entry, which triggers "enabled by presence" logic
	// and activates the provider even when commented out in config. Users must explicitly
	// configure Ollama with their desired model.
	v.SetDefault("providers.static.defaultModel", "static-v1")

	// Review action defaults are NOT set here.
	// They are applied in config.go after blockThreshold expansion to allow
	// threshold-based configuration to work correctly. See chooseReview().

	// Bot username for auto-dismissing stale reviews (Phase 2)
	v.SetDefault("review.botUsername", "github-actions[bot]")

	// Verification defaults (Epic #92 - agent verification)
	// Disabled by default to avoid unexpected LLM costs; users must opt-in
	v.SetDefault("verification.enabled", false)
	v.SetDefault("verification.provider", "gemini")
	v.SetDefault("verification.model", "gemini-3-flash-preview")
	v.SetDefault("verification.maxTokens", 64000)
	v.SetDefault("verification.depth", "medium")
	v.SetDefault("verification.costCeiling", 0.50)
	v.SetDefault("verification.confidence.default", 75)
	v.SetDefault("verification.confidence.critical", 60)
	v.SetDefault("verification.confidence.high", 70)
	v.SetDefault("verification.confidence.medium", 75)
	v.SetDefault("verification.confidence.low", 85)

	// Merge defaults (synthesis provider)
	// Uses alias without date suffix to always get latest model version
	v.SetDefault("merge.provider", "anthropic")
	v.SetDefault("merge.model", "claude-haiku-4-5")
}

func defaultStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./reviews.db"
	}
	return filepath.Join(home, ".config", "bop", "reviews.db")
}
