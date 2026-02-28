package config

import (
	"fmt"
	"strings"
)

// Config represents the full application configuration.
type Config struct {
	Providers       map[string]ProviderConfig `yaml:"providers"`
	HTTP            HTTPConfig                `yaml:"http"`
	Merge           MergeConfig               `yaml:"merge"`
	Planning        PlanningConfig            `yaml:"planning"`
	Git             GitConfig                 `yaml:"git"`
	Output          OutputConfig              `yaml:"output"`
	Budget          BudgetConfig              `yaml:"budget"`
	Redaction       RedactionConfig           `yaml:"redaction"`
	Determinism     DeterminismConfig         `yaml:"determinism"`
	Store           StoreConfig               `yaml:"store"`
	Observability   ObservabilityConfig       `yaml:"observability"`
	Review          ReviewConfig              `yaml:"review"`
	Verification    VerificationConfig        `yaml:"verification"`
	Deduplication   DeduplicationConfig       `yaml:"deduplication"`
	ThemeExtraction ThemeExtractionConfig     `yaml:"themeExtraction"`
	SizeGuards      SizeGuardsConfig          `yaml:"sizeGuards"`
	Platform        PlatformConfig            `yaml:"platform"`

	// Phase 3.2: Reviewer Personas
	// Reviewers configures the reviewer personas for code review.
	// Each reviewer represents a specialized perspective (e.g., security expert, maintainability focused).
	// Map key is the reviewer name (e.g., "security", "performance").
	Reviewers map[string]ReviewerConfig `yaml:"reviewers"`

	// DefaultReviewers lists which reviewers to use by default when not overridden by CLI flags.
	// Each entry must correspond to a key in the Reviewers map.
	// Example: ["security", "maintainability", "performance"]
	DefaultReviewers []string `yaml:"defaultReviewers"`
}

// ProviderConfig configures a single LLM provider.
// Providers define connection credentials and optionally a default model.
// When reviewers are configured, they reference providers by name and can override the model.
type ProviderConfig struct {
	// Enabled controls whether this provider is used for reviews.
	// This is a tri-state field with the following semantics:
	//   - nil (not set in config): Provider is enabled if APIKey is non-empty.
	//     This preserves backward compatibility with configs that only set apiKey.
	//   - true: Provider is explicitly enabled, even without an APIKey.
	//     Use this for keyless providers like Ollama that don't require authentication.
	//   - false: Provider is explicitly disabled, even if APIKey is present.
	//     Use this to temporarily disable a provider without removing credentials.
	Enabled *bool `yaml:"enabled,omitempty"`

	// DefaultModel is the model to use when no reviewer-specific model is specified.
	// This is the recommended field for new configurations.
	// Example: "gpt-4o", "claude-sonnet-4-5", "gemini-2.5-pro"
	DefaultModel string `yaml:"defaultModel,omitempty"`

	// Model is deprecated but still supported for backward compatibility.
	// New configurations should use defaultModel instead.
	// If both are set, defaultModel takes precedence.
	Model string `yaml:"model,omitempty"`

	APIKey string `yaml:"apiKey"`

	// MaxOutputTokens overrides the default max output tokens for this provider.
	// Use this for models with different output limits (e.g., older models with 8K,
	// or newer models with 128K+). Default: 64000 (works for Claude 4.5, GPT-5.2, Gemini 3).
	MaxOutputTokens *int `yaml:"maxOutputTokens,omitempty"`

	// HTTP overrides (optional, use global HTTP config if not set)
	Timeout        *string `yaml:"timeout,omitempty"`
	MaxRetries     *int    `yaml:"maxRetries,omitempty"`
	InitialBackoff *string `yaml:"initialBackoff,omitempty"`
	MaxBackoff     *string `yaml:"maxBackoff,omitempty"`
}

// GetDefaultModel returns the effective default model for this provider.
// Prefers DefaultModel if set, falls back to Model for backward compatibility.
func (c ProviderConfig) GetDefaultModel() string {
	if c.DefaultModel != "" {
		return c.DefaultModel
	}
	return c.Model
}

// ReviewerConfig configures a single reviewer persona (Phase 3.2).
// Reviewers represent specialized code review perspectives (e.g., security expert,
// maintainability focused, performance analyst). Each reviewer uses a specific
// LLM provider and can have custom persona prompts, focus areas, and weighting.
type ReviewerConfig struct {
	// Enabled controls whether this reviewer participates in reviews.
	// This is a tri-state field with the following semantics:
	//   - nil (not set in config): Reviewer is enabled.
	//     This is the default - all configured reviewers are active unless explicitly disabled.
	//   - true: Reviewer is explicitly enabled.
	//   - false: Reviewer is explicitly disabled.
	//     Use this to temporarily disable a reviewer without removing its configuration.
	Enabled *bool `yaml:"enabled,omitempty"`

	// Provider is the LLM provider to use for this reviewer.
	// Must reference a provider name defined in the providers section.
	// Valid values: "openai", "anthropic", "gemini", "ollama"
	// Required field.
	Provider string `yaml:"provider"`

	// Model is the specific model to use from the provider.
	// Examples: "gpt-4o", "claude-opus-4", "gemini-2.5-pro"
	// Optional: if not set, uses the provider's defaultModel.
	Model string `yaml:"model,omitempty"`

	// APIKey is the API key for the provider.
	// Supports environment variable expansion: ${VAR} or $VAR
	// Optional - can also be set via CR_<PROVIDER>_API_KEY environment variable.
	APIKey string `yaml:"apiKey,omitempty"`

	// Weight controls how much this reviewer's findings influence the final merged review.
	// Higher weights give more influence. Default: 1.0
	// Example: A security expert might have weight 2.0 for security findings.
	Weight float64 `yaml:"weight,omitempty"`

	// Persona is a custom system prompt that defines this reviewer's expertise and perspective.
	// This is prepended to the standard review prompt to guide the LLM's behavior.
	// Example: "You are a security expert specializing in OWASP vulnerabilities..."
	// Optional - uses default neutral persona if not set.
	Persona string `yaml:"persona,omitempty"`

	// Focus lists the finding categories this reviewer should prioritize.
	// When set, the reviewer's persona is augmented to emphasize these areas.
	// Examples: ["security", "authentication"], ["performance", "scalability"]
	// Optional - reviews all categories equally if not set.
	Focus []string `yaml:"focus,omitempty"`

	// Ignore lists finding categories this reviewer should skip.
	// When set, the reviewer is instructed not to report findings in these categories.
	// Examples: ["style", "documentation"], ["test_coverage"]
	// Optional - reviews all categories if not set.
	Ignore []string `yaml:"ignore,omitempty"`

	// MaxOutputTokens overrides the default max output tokens for this reviewer.
	// Use this for models with different output limits.
	// Default: 64000 (works for Claude 4.5, GPT-4o, Gemini 2.5).
	MaxOutputTokens *int `yaml:"maxOutputTokens,omitempty"`

	// HTTP overrides (optional, use global HTTP config if not set)
	Timeout        *string `yaml:"timeout,omitempty"`
	MaxRetries     *int    `yaml:"maxRetries,omitempty"`
	InitialBackoff *string `yaml:"initialBackoff,omitempty"`
	MaxBackoff     *string `yaml:"maxBackoff,omitempty"`
}

// IsEnabled returns whether this reviewer is enabled.
// Defaults to true if not explicitly set.
func (c ReviewerConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true // Default enabled
	}
	return *c.Enabled
}

// HTTPConfig holds global HTTP client settings.
type HTTPConfig struct {
	Timeout           string  `yaml:"timeout"`
	MaxRetries        int     `yaml:"maxRetries"`
	InitialBackoff    string  `yaml:"initialBackoff"`
	MaxBackoff        string  `yaml:"maxBackoff"`
	BackoffMultiplier float64 `yaml:"backoffMultiplier"`
}

type MergeConfig struct {
	Enabled  bool               `yaml:"enabled"`
	Provider string             `yaml:"provider"`
	Model    string             `yaml:"model"`
	Strategy string             `yaml:"strategy"`
	Weights  map[string]float64 `yaml:"weights"`

	// Phase 3.2: Reviewer Personas
	// WeightByReviewer applies per-reviewer weights to finding scores.
	// When true, agreement score is weighted by reviewer weight instead of simple count.
	WeightByReviewer bool `yaml:"weightByReviewer"`

	// RespectFocus prevents penalizing low agreement for focused reviewers.
	// When true, findings from specialized reviewers (with non-empty ReviewerName)
	// are not penalized for lack of agreement from other reviewers.
	RespectFocus bool `yaml:"respectFocus"`
}

// PlanningConfig configures the interactive planning agent.
// The planning agent asks clarifying questions before starting the review
// to improve context and focus. Only runs in interactive (TTY) mode.
type PlanningConfig struct {
	Enabled      bool   `yaml:"enabled"`      // Enable interactive planning
	Provider     string `yaml:"provider"`     // LLM provider for planning (e.g., "openai", "anthropic")
	Model        string `yaml:"model"`        // Model for planning (e.g., "gpt-4o-mini", "claude-3-5-haiku")
	MaxQuestions int    `yaml:"maxQuestions"` // Maximum questions to ask (default: 5)
	Timeout      string `yaml:"timeout"`      // Timeout for planning phase (default: "30s")
}

type GitConfig struct {
	RepositoryDir string `yaml:"repositoryDir"`
}

type OutputConfig struct {
	Directory string `yaml:"directory"`
}

type BudgetConfig struct {
	HardCapUSD        float64  `yaml:"hardCapUSD"`
	DegradationPolicy []string `yaml:"degradationPolicy"`
}

type RedactionConfig struct {
	Enabled    bool     `yaml:"enabled"`
	DenyGlobs  []string `yaml:"denyGlobs"`
	AllowGlobs []string `yaml:"allowGlobs"`
}

type DeterminismConfig struct {
	Enabled     bool    `yaml:"enabled"`
	Temperature float64 `yaml:"temperature"`
	UseSeed     bool    `yaml:"useSeed"`
}

// StoreConfig configures the persistence layer.
type StoreConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// PlatformConfig configures bop Pro platform integration.
// When enabled (or when useCuratedPanel is true), bop connects to the platform
// to fetch curated reviewer panels, resolve entitlements, and sync team features.
type PlatformConfig struct {
	// Enabled toggles platform integration. When true, bop connects to
	// the bop Pro platform for curated reviewer panels, usage analytics,
	// and team features.
	Enabled bool `yaml:"enabled"`

	// Token is the platform authentication token.
	// Supports environment variable expansion: ${BOP_PLATFORM_TOKEN}
	// Optional when using `bop auth login` (device flow stores credentials separately).
	Token string `yaml:"token,omitempty"`

	// ManagedProxy routes LLM requests through the platform's managed proxy,
	// eliminating the need for individual API keys.
	ManagedProxy bool `yaml:"managedProxy"`

	// UseCuratedPanel uses the platform's provider-optimized reviewer panel
	// instead of the locally configured reviewers.
	UseCuratedPanel bool `yaml:"useCuratedPanel"`

	// URL is the platform API URL.
	// Default: "https://api.delightfulhammers.com"
	URL string `yaml:"url,omitempty"`

	// Override when true causes local reviewer configuration to take precedence
	// over platform-sourced reviewer config. By default, platform config wins.
	Override bool `yaml:"override"`
}

// ObservabilityConfig configures logging, metrics, and cost tracking.
type ObservabilityConfig struct {
	Logging LoggingConfig `yaml:"logging"`
	Metrics MetricsConfig `yaml:"metrics"`
}

// LoggingConfig configures request/response logging.
type LoggingConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Level         string `yaml:"level"`         // trace, debug, info, error
	Format        string `yaml:"format"`        // json, human
	RedactAPIKeys bool   `yaml:"redactAPIKeys"` // Redact API keys in logs

	// MaxContentBytes limits the size of logged prompt/response content at trace level.
	// This prevents log explosion with very large prompts or responses.
	// Default: 51200 (50KB). Set to 0 for unlimited (use with caution).
	MaxContentBytes int `yaml:"maxContentBytes"`
}

// MetricsConfig configures performance and cost metrics tracking.
type MetricsConfig struct {
	Enabled bool `yaml:"enabled"`
}

// ReviewConfig configures the code review behavior.
type ReviewConfig struct {
	// Instructions are custom instructions included in all review prompts.
	// These guide the LLM on what to look for during code review.
	Instructions string `yaml:"instructions"`

	// Actions configures the GitHub review action based on finding severity.
	Actions ReviewActions `yaml:"actions"`

	// BotUsername is the GitHub username of the bot for auto-dismissing stale reviews.
	// When set, previous reviews from this user are dismissed AFTER the new review
	// posts successfully. This ensures the PR always maintains review signal.
	// Set to "none" to explicitly disable auto-dismiss.
	// Default: "github-actions[bot]"
	BotUsername string `yaml:"botUsername"`

	// BlockThreshold is syntactic sugar for setting per-severity actions.
	// Valid values: "critical", "high", "medium", "low", "none"
	// - "critical": only critical findings block (request_changes)
	// - "high": critical and high block (default behavior)
	// - "medium": critical, high, and medium block
	// - "low": all severities block
	// - "none": nothing blocks (all findings are informational)
	// Explicit per-severity actions (Actions.OnCritical, etc.) override this threshold.
	BlockThreshold string `yaml:"blockThreshold"`

	// AlwaysBlockCategories lists finding categories that always trigger REQUEST_CHANGES
	// regardless of severity. This is additive - if a finding's category matches,
	// it blocks even if the severity threshold would not.
	// Example: ["security", "bug"] - security and bug findings always block
	AlwaysBlockCategories []string `yaml:"alwaysBlockCategories"`

	// MaxConcurrentReviewers limits the number of reviewers dispatched concurrently.
	// This prevents resource exhaustion and API rate limiting in enterprise deployments
	// with many configured reviewers.
	// Default: 0 (unlimited - all reviewers run in parallel)
	// Recommended: 3-5 for most deployments
	MaxConcurrentReviewers int `yaml:"maxConcurrentReviewers"`

	// PostOutOfDiffAsComments enables posting out-of-diff findings as individual
	// issue comments rather than only including them in the summary.
	// Out-of-diff findings are on lines not included in the PR diff (deleted lines,
	// unchanged context) and cannot be posted as inline review comments.
	// When enabled, these findings are posted as issue comments with fingerprint
	// markers, making them visible to MCP tools for triage.
	// Default: true (enabled by default)
	PostOutOfDiffAsComments *bool `yaml:"postOutOfDiffAsComments,omitempty"`
}

// ReviewActions maps finding severities to GitHub review actions.
// Valid action values (case-insensitive): approve, comment, request_changes.
type ReviewActions struct {
	// OnCritical is the action when any critical severity finding is present.
	OnCritical string `yaml:"onCritical"`

	// OnHigh is the action when any high severity finding is present (and no critical).
	OnHigh string `yaml:"onHigh"`

	// OnMedium is the action when any medium severity finding is present (and no higher).
	OnMedium string `yaml:"onMedium"`

	// OnLow is the action when any low severity finding is present (and no higher).
	OnLow string `yaml:"onLow"`

	// OnClean is the action when no findings are present in the diff.
	OnClean string `yaml:"onClean"`

	// OnNonBlocking is the action when findings exist but none trigger REQUEST_CHANGES.
	// This allows posting APPROVE with informational comments for low-severity issues.
	OnNonBlocking string `yaml:"onNonBlocking"`
}

// ShouldPostOutOfDiff returns whether out-of-diff findings should be posted as individual
// issue comments. Defaults to true if not explicitly set.
func (c ReviewConfig) ShouldPostOutOfDiff() bool {
	if c.PostOutOfDiffAsComments == nil {
		return true // Default enabled
	}
	return *c.PostOutOfDiffAsComments
}

// VerificationConfig configures the agent verification behavior.
// When enabled, candidate findings from discovery are verified by an agent
// before being reported.
type VerificationConfig struct {
	// Enabled toggles agent verification of findings.
	Enabled bool `yaml:"enabled"`

	// Provider is the LLM provider for verification (e.g., "gemini", "anthropic", "openai").
	// Default: "gemini"
	Provider string `yaml:"provider"`

	// Model is the model to use for verification.
	// Default: "gemini-3-flash-preview" (fast, large context, cost-effective)
	Model string `yaml:"model"`

	// MaxTokens is the maximum output tokens for batch verification responses.
	// Default: 64000 (large enough for many findings)
	MaxTokens int `yaml:"maxTokens"`

	// Depth controls how thoroughly the agent verifies findings.
	// Valid values: "quick" (read file only), "medium" (read + grep), "deep" (run build/tests).
	Depth string `yaml:"depth"`

	// CostCeiling is the maximum USD to spend on verification per review.
	// When reached, remaining candidates are reported as unverified with lower confidence.
	CostCeiling float64 `yaml:"costCeiling"`

	// Confidence contains per-severity confidence thresholds.
	Confidence ConfidenceThresholds `yaml:"confidence"`
}

// ConfidenceThresholds define minimum confidence levels (0-100) for reporting findings.
// Findings below the threshold for their severity level are discarded.
type ConfidenceThresholds struct {
	// Default is used when a severity-specific threshold is not set.
	Default int `yaml:"default"`

	// Critical is the threshold for critical severity findings.
	Critical int `yaml:"critical"`

	// High is the threshold for high severity findings.
	High int `yaml:"high"`

	// Medium is the threshold for medium severity findings.
	Medium int `yaml:"medium"`

	// Low is the threshold for low severity findings.
	Low int `yaml:"low"`
}

// Merge combines multiple configuration instances, prioritising the latter ones.
// After merging, threshold expansion and defaults are applied to the Review config.
// Returns an error if the merged configuration contains invalid values (e.g., invalid blockThreshold).
func Merge(configs ...Config) (Config, error) {
	result := Config{}
	for _, cfg := range configs {
		result = merge(result, cfg)
	}
	// Apply threshold expansion and defaults after all merging is complete
	reviewConfig, err := processReviewConfig(result.Review)
	if err != nil {
		return Config{}, err
	}
	result.Review = reviewConfig
	return result, nil
}

func merge(base, overlay Config) Config {
	result := base

	result.HTTP = chooseHTTP(base.HTTP, overlay.HTTP)
	result.Output = chooseOutput(base.Output, overlay.Output)
	result.Git = chooseGit(base.Git, overlay.Git)
	result.Budget = chooseBudget(base.Budget, overlay.Budget)
	result.Redaction = chooseRedaction(base.Redaction, overlay.Redaction)
	result.Determinism = chooseDeterminism(base.Determinism, overlay.Determinism)
	result.Merge = chooseMerge(base.Merge, overlay.Merge)
	result.Planning = choosePlanning(base.Planning, overlay.Planning)
	result.Store = chooseStore(base.Store, overlay.Store)
	result.Observability = chooseObservability(base.Observability, overlay.Observability)
	result.Review = chooseReview(base.Review, overlay.Review)
	result.Verification = chooseVerification(base.Verification, overlay.Verification)
	result.Deduplication = chooseDeduplication(base.Deduplication, overlay.Deduplication)
	result.ThemeExtraction = chooseThemeExtraction(base.ThemeExtraction, overlay.ThemeExtraction)
	result.SizeGuards = chooseSizeGuards(base.SizeGuards, overlay.SizeGuards)
	result.Platform = choosePlatform(base.Platform, overlay.Platform)
	result.Providers = mergeProviders(base.Providers, overlay.Providers)

	// Phase 3.2: Merge reviewer personas
	result.Reviewers = mergeReviewers(base.Reviewers, overlay.Reviewers)
	result.DefaultReviewers = mergeDefaultReviewers(base.DefaultReviewers, overlay.DefaultReviewers)

	return result
}

func mergeProviders(base, overlay map[string]ProviderConfig) map[string]ProviderConfig {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	result := make(map[string]ProviderConfig, len(base)+len(overlay))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range overlay {
		if existing, ok := result[key]; ok {
			result[key] = mergeProvider(existing, value)
		} else {
			result[key] = value
		}
	}
	return result
}

// mergeProvider performs field-level merge of two ProviderConfigs.
// Only non-zero fields from the overlay override the base.
func mergeProvider(base, overlay ProviderConfig) ProviderConfig {
	result := base
	if overlay.Enabled != nil {
		result.Enabled = overlay.Enabled
	}
	if overlay.DefaultModel != "" {
		result.DefaultModel = overlay.DefaultModel
	}
	if overlay.Model != "" {
		result.Model = overlay.Model
	}
	if overlay.APIKey != "" {
		result.APIKey = overlay.APIKey
	}
	if overlay.MaxOutputTokens != nil {
		result.MaxOutputTokens = overlay.MaxOutputTokens
	}
	if overlay.Timeout != nil {
		result.Timeout = overlay.Timeout
	}
	if overlay.MaxRetries != nil {
		result.MaxRetries = overlay.MaxRetries
	}
	if overlay.InitialBackoff != nil {
		result.InitialBackoff = overlay.InitialBackoff
	}
	if overlay.MaxBackoff != nil {
		result.MaxBackoff = overlay.MaxBackoff
	}
	return result
}

// mergeReviewers merges two reviewer maps, with overlay taking precedence.
// This follows the same pattern as mergeProviders.
func mergeReviewers(base, overlay map[string]ReviewerConfig) map[string]ReviewerConfig {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	result := make(map[string]ReviewerConfig, len(base)+len(overlay))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range overlay {
		result[key] = value
	}
	return result
}

// mergeDefaultReviewers merges default reviewer lists, with overlay taking precedence.
// Unlike categories (which union), default reviewers are replaced entirely by overlay.
func mergeDefaultReviewers(base, overlay []string) []string {
	if len(overlay) > 0 {
		return overlay
	}
	return base
}

func chooseOutput(base, overlay OutputConfig) OutputConfig {
	if overlay.Directory != "" {
		return overlay
	}
	return base
}

func chooseGit(base, overlay GitConfig) GitConfig {
	if overlay.RepositoryDir != "" {
		return overlay
	}
	return base
}

func chooseHTTP(base, overlay HTTPConfig) HTTPConfig {
	if overlay.Timeout != "" || overlay.MaxRetries != 0 || overlay.InitialBackoff != "" || overlay.MaxBackoff != "" || overlay.BackoffMultiplier != 0 {
		return overlay
	}
	return base
}

func chooseBudget(base, overlay BudgetConfig) BudgetConfig {
	if overlay.HardCapUSD != 0 || len(overlay.DegradationPolicy) > 0 {
		return overlay
	}
	return base
}

func chooseRedaction(base, overlay RedactionConfig) RedactionConfig {
	if overlay.Enabled || len(overlay.DenyGlobs) > 0 || len(overlay.AllowGlobs) > 0 {
		return overlay
	}
	return base
}

func chooseDeterminism(base, overlay DeterminismConfig) DeterminismConfig {
	if overlay.Enabled || overlay.Temperature != 0 || overlay.UseSeed {
		return overlay
	}
	return base
}

func chooseMerge(base, overlay MergeConfig) MergeConfig {
	if overlay.Enabled || overlay.Provider != "" || overlay.Model != "" || overlay.Strategy != "" || len(overlay.Weights) > 0 {
		return overlay
	}
	return base
}

func choosePlanning(base, overlay PlanningConfig) PlanningConfig {
	if overlay.Enabled || overlay.Provider != "" || overlay.Model != "" || overlay.MaxQuestions != 0 || overlay.Timeout != "" {
		return overlay
	}
	return base
}

func chooseStore(base, overlay StoreConfig) StoreConfig {
	if overlay.Enabled || overlay.Path != "" {
		return overlay
	}
	return base
}

func choosePlatform(base, overlay PlatformConfig) PlatformConfig {
	if overlay.Enabled || overlay.Token != "" || overlay.ManagedProxy || overlay.UseCuratedPanel || overlay.URL != "" || overlay.Override {
		return overlay
	}
	return base
}

func chooseObservability(base, overlay ObservabilityConfig) ObservabilityConfig {
	result := base

	// Merge logging config
	if overlay.Logging.Enabled || overlay.Logging.Level != "" || overlay.Logging.Format != "" {
		result.Logging = overlay.Logging
	}

	// Merge metrics config
	if overlay.Metrics.Enabled {
		result.Metrics = overlay.Metrics
	}

	return result
}

func chooseReview(base, overlay ReviewConfig) ReviewConfig {
	result := base

	// Instructions: overlay wins if non-empty
	if overlay.Instructions != "" {
		result.Instructions = overlay.Instructions
	}

	// BlockThreshold: overlay wins if non-empty
	if overlay.BlockThreshold != "" {
		result.BlockThreshold = overlay.BlockThreshold
	}

	// Actions: merge base and overlay (overlay wins for non-empty fields)
	if overlay.Actions.hasAny() {
		result.Actions = mergeReviewActions(base.Actions, overlay.Actions)
	}

	// NOTE: Threshold expansion and defaults are NOT applied here.
	// They are applied in processReviewConfig() which is called by Load()
	// after all config sources are merged. This prevents defaults from one
	// merge iteration from overriding threshold expansion in a later iteration.

	// BotUsername: overlay wins if non-empty
	if overlay.BotUsername != "" {
		result.BotUsername = overlay.BotUsername
	}

	// AlwaysBlockCategories: union of base and overlay (additive)
	result.AlwaysBlockCategories = mergeCategories(base.AlwaysBlockCategories, overlay.AlwaysBlockCategories)

	// MaxConcurrentReviewers: overlay wins if non-zero
	if overlay.MaxConcurrentReviewers != 0 {
		result.MaxConcurrentReviewers = overlay.MaxConcurrentReviewers
	}

	// PostOutOfDiffAsComments: overlay wins if set (not nil)
	if overlay.PostOutOfDiffAsComments != nil {
		result.PostOutOfDiffAsComments = overlay.PostOutOfDiffAsComments
	}

	return result
}

// hasAny returns true if any action field is non-empty.
func (a ReviewActions) hasAny() bool {
	return a.OnCritical != "" || a.OnHigh != "" || a.OnMedium != "" || a.OnLow != "" || a.OnClean != "" || a.OnNonBlocking != ""
}

// mergeReviewActions merges two ReviewActions, with overlay taking precedence for non-empty fields.
func mergeReviewActions(base, overlay ReviewActions) ReviewActions {
	result := base
	if overlay.OnCritical != "" {
		result.OnCritical = overlay.OnCritical
	}
	if overlay.OnHigh != "" {
		result.OnHigh = overlay.OnHigh
	}
	if overlay.OnMedium != "" {
		result.OnMedium = overlay.OnMedium
	}
	if overlay.OnLow != "" {
		result.OnLow = overlay.OnLow
	}
	if overlay.OnClean != "" {
		result.OnClean = overlay.OnClean
	}
	if overlay.OnNonBlocking != "" {
		result.OnNonBlocking = overlay.OnNonBlocking
	}
	return result
}

// applyActionDefaults fills in empty action slots with sensible defaults.
// Default behavior: critical/high block (request_changes), medium/low don't block (comment),
// clean reviews get approved, non-blocking findings get approved.
func applyActionDefaults(actions ReviewActions) ReviewActions {
	if actions.OnCritical == "" {
		actions.OnCritical = "request_changes"
	}
	if actions.OnHigh == "" {
		actions.OnHigh = "request_changes"
	}
	if actions.OnMedium == "" {
		actions.OnMedium = "comment"
	}
	if actions.OnLow == "" {
		actions.OnLow = "comment"
	}
	if actions.OnClean == "" {
		actions.OnClean = "approve"
	}
	if actions.OnNonBlocking == "" {
		actions.OnNonBlocking = "approve"
	}
	return actions
}

// ValidBlockThresholds lists the valid values for blockThreshold configuration.
var ValidBlockThresholds = []string{"critical", "high", "medium", "low", "none"}

// expandBlockThreshold converts a threshold string to explicit per-severity actions.
// Valid thresholds: "critical", "high", "medium", "low", "none"
// - "critical": only critical blocks
// - "high": critical and high block (matches default behavior)
// - "medium": critical, high, and medium block
// - "low": all severities block
// - "none": nothing blocks (all comment only)
// Returns zero-value ReviewActions and nil error if threshold is empty.
// Returns error if threshold is non-empty but invalid.
func expandBlockThreshold(threshold string) (ReviewActions, error) {
	if threshold == "" {
		return ReviewActions{}, nil
	}

	// Severity levels in order from highest to lowest
	// The threshold means "block at this level and above"
	// "none" is set to 5 (above critical=4) so no severity meets the threshold
	severityLevels := map[string]int{
		"critical": 4,
		"high":     3,
		"medium":   2,
		"low":      1,
		"none":     5, // Above all severities - nothing blocks
	}

	thresholdLevel, ok := severityLevels[strings.ToLower(threshold)]
	if !ok {
		return ReviewActions{}, fmt.Errorf("invalid blockThreshold %q: must be one of: critical, high, medium, low, none", threshold)
	}

	var actions ReviewActions

	// A severity blocks if its level >= threshold level
	// e.g., threshold "high" (level 3): critical (4) and high (3) block, medium (2) and low (1) don't
	const (
		criticalLevel = 4
		highLevel     = 3
		mediumLevel   = 2
		lowLevel      = 1
	)

	if criticalLevel >= thresholdLevel {
		actions.OnCritical = "request_changes"
	} else {
		actions.OnCritical = "comment"
	}

	if highLevel >= thresholdLevel {
		actions.OnHigh = "request_changes"
	} else {
		actions.OnHigh = "comment"
	}

	if mediumLevel >= thresholdLevel {
		actions.OnMedium = "request_changes"
	} else {
		actions.OnMedium = "comment"
	}

	if lowLevel >= thresholdLevel {
		actions.OnLow = "request_changes"
	} else {
		actions.OnLow = "comment"
	}

	return actions, nil
}

// mergeCategories returns the union of two category slices, preserving order and deduplicating.
func mergeCategories(base, overlay []string) []string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []string

	for _, cat := range base {
		normalized := strings.ToLower(strings.TrimSpace(cat))
		if normalized != "" && !seen[normalized] {
			seen[normalized] = true
			result = append(result, cat)
		}
	}

	for _, cat := range overlay {
		normalized := strings.ToLower(strings.TrimSpace(cat))
		if normalized != "" && !seen[normalized] {
			seen[normalized] = true
			result = append(result, cat)
		}
	}

	return result
}

func chooseVerification(base, overlay VerificationConfig) VerificationConfig {
	result := base

	// If overlay has any verification config set, use its Enabled value
	// This allows overlay to disable verification (Enabled=false) when other fields are set
	if hasAnyVerificationConfig(overlay) {
		result.Enabled = overlay.Enabled
	}

	// Provider: overlay wins if non-empty
	if overlay.Provider != "" {
		result.Provider = overlay.Provider
	}

	// Model: overlay wins if non-empty
	if overlay.Model != "" {
		result.Model = overlay.Model
	}

	// MaxTokens: overlay wins if non-zero
	if overlay.MaxTokens != 0 {
		result.MaxTokens = overlay.MaxTokens
	}

	// Depth: overlay wins if non-empty
	if overlay.Depth != "" {
		result.Depth = overlay.Depth
	}

	// CostCeiling: overlay wins if non-zero
	if overlay.CostCeiling != 0 {
		result.CostCeiling = overlay.CostCeiling
	}

	// Confidence: overlay wins if any field is set
	if hasConfidenceThresholds(overlay.Confidence) {
		result.Confidence = overlay.Confidence
	}

	return result
}

// hasAnyVerificationConfig returns true if any verification field is set in the config.
// This is used to determine if the Enabled field should be respected from the overlay.
func hasAnyVerificationConfig(vc VerificationConfig) bool {
	return vc.Enabled ||
		vc.Provider != "" ||
		vc.Model != "" ||
		vc.MaxTokens != 0 ||
		vc.Depth != "" ||
		vc.CostCeiling != 0 ||
		hasConfidenceThresholds(vc.Confidence)
}

func hasConfidenceThresholds(ct ConfidenceThresholds) bool {
	return ct.Default != 0 || ct.Critical != 0 || ct.High != 0 || ct.Medium != 0 || ct.Low != 0
}

// ThemeExtractionConfig configures LLM-based theme extraction from prior findings.
// When enabled, themes are extracted from previous review findings and included
// in the prompt to prevent thematic repetition across review rounds.
type ThemeExtractionConfig struct {
	// Enabled toggles theme extraction.
	// Default: true (when prior findings exist)
	Enabled *bool `yaml:"enabled,omitempty"`

	// Strategy determines the extraction approach:
	// - "abstract": High-level themes only (original behavior)
	// - "specific": Themes with specific conclusions and anti-patterns
	// - "comprehensive": Themes + conclusions + disputed patterns (default, most effective)
	// Default: "comprehensive"
	Strategy string `yaml:"strategy"`

	// Provider is the LLM provider for theme extraction (e.g., "anthropic", "openai", "gemini").
	// If not specified, uses the first available provider with an API key.
	Provider string `yaml:"provider"`

	// Model is the model to use for theme extraction.
	// Default: provider-specific (haiku for anthropic, gpt-4o-mini for openai, gemini-2.0-flash for gemini)
	Model string `yaml:"model"`

	// MaxTokens is the maximum output tokens for theme extraction.
	// Default: 4096
	MaxTokens int `yaml:"maxTokens"`

	// MinFindingsForTheme is the minimum number of prior findings required
	// before theme extraction is triggered.
	// Default: 3
	MinFindingsForTheme int `yaml:"minFindingsForTheme"`

	// MaxThemes is the maximum number of themes to extract.
	// Default: 10
	MaxThemes int `yaml:"maxThemes"`
}

// DeduplicationConfig configures semantic deduplication of findings.
// When enabled, findings that overlap spatially but have different fingerprints
// are compared using an LLM to detect semantic duplicates.
type DeduplicationConfig struct {
	// Semantic configures the LLM-based semantic deduplication (stage 2).
	// Stage 1 (fingerprint matching) is always enabled and has no configuration.
	Semantic SemanticDeduplicationConfig `yaml:"semantic"`
}

// SemanticDeduplicationConfig configures LLM-based semantic deduplication.
type SemanticDeduplicationConfig struct {
	// Enabled toggles semantic deduplication.
	// Default: true
	Enabled *bool `yaml:"enabled,omitempty"`

	// Provider is the LLM provider for semantic comparison (e.g., "anthropic", "openai").
	// Default: "anthropic"
	Provider string `yaml:"provider"`

	// Model is the model to use for semantic comparison.
	// Default: "claude-haiku-4-5-latest"
	Model string `yaml:"model"`

	// MaxTokens is the maximum output tokens for the deduplication response.
	// Default: 64000 (sufficient for all current Claude/GPT/Gemini models)
	MaxTokens int `yaml:"maxTokens"`

	// LineThreshold is the maximum line distance for findings to be considered
	// potentially duplicate. Findings further apart are not compared.
	// Default: 10
	LineThreshold int `yaml:"lineThreshold"`

	// MaxCandidates is the maximum number of candidate pairs to send for
	// semantic comparison per review. This acts as a cost guard.
	// Default: 200
	MaxCandidates int `yaml:"maxCandidates"`
}

// chooseDeduplication merges DeduplicationConfig with overlay taking precedence.
func chooseDeduplication(base, overlay DeduplicationConfig) DeduplicationConfig {
	result := base

	// Merge semantic config
	result.Semantic = chooseSemanticDeduplication(base.Semantic, overlay.Semantic)

	return result
}

// chooseSemanticDeduplication merges SemanticDeduplicationConfig.
func chooseSemanticDeduplication(base, overlay SemanticDeduplicationConfig) SemanticDeduplicationConfig {
	result := base

	// Enabled: overlay wins if set (not nil)
	if overlay.Enabled != nil {
		result.Enabled = overlay.Enabled
	}

	// Provider: overlay wins if non-empty
	if overlay.Provider != "" {
		result.Provider = overlay.Provider
	}

	// Model: overlay wins if non-empty
	if overlay.Model != "" {
		result.Model = overlay.Model
	}

	// MaxTokens: overlay wins if non-zero
	if overlay.MaxTokens != 0 {
		result.MaxTokens = overlay.MaxTokens
	}

	// LineThreshold: overlay wins if non-zero
	if overlay.LineThreshold != 0 {
		result.LineThreshold = overlay.LineThreshold
	}

	// MaxCandidates: overlay wins if non-zero
	if overlay.MaxCandidates != 0 {
		result.MaxCandidates = overlay.MaxCandidates
	}

	return result
}

// chooseThemeExtraction merges ThemeExtractionConfig with overlay taking precedence.
func chooseThemeExtraction(base, overlay ThemeExtractionConfig) ThemeExtractionConfig {
	result := base

	// Enabled: overlay wins if set (not nil)
	if overlay.Enabled != nil {
		result.Enabled = overlay.Enabled
	}

	// Strategy: overlay wins if non-empty
	if overlay.Strategy != "" {
		result.Strategy = overlay.Strategy
	}

	// Provider: overlay wins if non-empty
	if overlay.Provider != "" {
		result.Provider = overlay.Provider
	}

	// Model: overlay wins if non-empty
	if overlay.Model != "" {
		result.Model = overlay.Model
	}

	// MaxTokens: overlay wins if non-zero
	if overlay.MaxTokens != 0 {
		result.MaxTokens = overlay.MaxTokens
	}

	// MinFindingsForTheme: overlay wins if non-zero
	if overlay.MinFindingsForTheme != 0 {
		result.MinFindingsForTheme = overlay.MinFindingsForTheme
	}

	// MaxThemes: overlay wins if non-zero
	if overlay.MaxThemes != 0 {
		result.MaxThemes = overlay.MaxThemes
	}

	return result
}

// SizeGuardsConfig configures PR size limits and truncation behavior.
// This prevents context overflow when reviewing large PRs by warning at
// a threshold and truncating at a maximum.
type SizeGuardsConfig struct {
	// Enabled toggles size guard functionality.
	// Default: true
	Enabled *bool `yaml:"enabled,omitempty"`

	// WarnTokens is the token count at which to emit a warning.
	// The review continues but includes a note about size.
	// Default: 150000 (targets Claude 4.5's 200k context with margin)
	WarnTokens int `yaml:"warnTokens"`

	// MaxTokens is the maximum token count before truncation.
	// Files are removed by priority until under this limit.
	// Default: 200000 (Claude 4.5's context limit)
	MaxTokens int `yaml:"maxTokens"`

	// Providers allows per-provider override of size limits.
	// Use this when targeting providers with different context limits
	// (e.g., Gemini 1.5 Pro has 1M+ tokens, older GPT-4 has 128k).
	Providers map[string]ProviderSizeConfig `yaml:"providers,omitempty"`
}

// ProviderSizeConfig allows per-provider size limit overrides.
type ProviderSizeConfig struct {
	// WarnTokens overrides the global warn threshold for this provider.
	WarnTokens int `yaml:"warnTokens,omitempty"`

	// MaxTokens overrides the global max threshold for this provider.
	MaxTokens int `yaml:"maxTokens,omitempty"`
}

// GetLimitsForProvider returns the warn and max token limits for a specific provider.
// If the provider has overrides configured, those are used; otherwise global defaults apply.
// If warn > max (misconfiguration), the values are swapped to maintain the invariant.
func (c SizeGuardsConfig) GetLimitsForProvider(provider string) (warn, max int) {
	warn, max = c.WarnTokens, c.MaxTokens

	// Apply global defaults if not set
	if warn == 0 {
		warn = 150000
	}
	if max == 0 {
		max = 200000
	}

	// Apply provider-specific overrides
	if pc, ok := c.Providers[provider]; ok {
		if pc.WarnTokens > 0 {
			warn = pc.WarnTokens
		}
		if pc.MaxTokens > 0 {
			max = pc.MaxTokens
		}
	}

	// Ensure warn <= max invariant (swap if misconfigured)
	if warn > max {
		warn, max = max, warn
	}

	return warn, max
}

// IsEnabled returns whether size guards are enabled.
// Defaults to true if not explicitly set.
func (c SizeGuardsConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true // Default enabled
	}
	return *c.Enabled
}

// chooseSizeGuards merges SizeGuardsConfig with overlay taking precedence.
func chooseSizeGuards(base, overlay SizeGuardsConfig) SizeGuardsConfig {
	result := base

	// Enabled: overlay wins if set (not nil)
	if overlay.Enabled != nil {
		result.Enabled = overlay.Enabled
	}

	// WarnTokens: overlay wins if non-zero
	if overlay.WarnTokens != 0 {
		result.WarnTokens = overlay.WarnTokens
	}

	// MaxTokens: overlay wins if non-zero
	if overlay.MaxTokens != 0 {
		result.MaxTokens = overlay.MaxTokens
	}

	// Providers: merge maps
	result.Providers = mergeProviderSizeConfigs(base.Providers, overlay.Providers)

	return result
}

// mergeProviderSizeConfigs merges provider size config maps.
func mergeProviderSizeConfigs(base, overlay map[string]ProviderSizeConfig) map[string]ProviderSizeConfig {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	result := make(map[string]ProviderSizeConfig, len(base)+len(overlay))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range overlay {
		// Merge individual provider configs
		if existing, ok := result[key]; ok {
			if value.WarnTokens != 0 {
				existing.WarnTokens = value.WarnTokens
			}
			if value.MaxTokens != 0 {
				existing.MaxTokens = value.MaxTokens
			}
			result[key] = existing
		} else {
			result[key] = value
		}
	}
	return result
}
