package config

import (
	"fmt"
)

// PlatformConfigResult holds the parsed config fetched from the platform API.
type PlatformConfigResult struct {
	// Config is the raw config map from GET /products/bop/config
	Config map[string]any

	// Tier is the user's effective tier (e.g., "pro", "free")
	Tier string

	// IsReadOnly indicates whether the config can be edited
	IsReadOnly bool
}

// MergePlatformConfig applies platform-sourced configuration over local config.
// Platform config overrides local for reviewer-related fields (reviewers, weights,
// model, custom prompts). Local config wins for non-platform fields (observability,
// git, output, budget, etc.).
//
// If local.Platform.Override is true, local reviewers take precedence over platform.
func MergePlatformConfig(local Config, result *PlatformConfigResult) Config {
	if result == nil || result.Config == nil {
		return local
	}

	// If local override is set, platform config is informational only —
	// local reviewers, weights, and models take precedence over platform.
	if local.Platform.Override {
		return local
	}

	merged := local

	// Apply platform reviewer list → DefaultReviewers
	if reviewers, ok := extractStringSlice(result.Config, "reviewers"); ok && len(reviewers) > 0 {
		merged.DefaultReviewers = reviewers
	}

	// Apply platform weights → individual ReviewerConfig.Weight
	if weights, ok := extractStringFloatMap(result.Config, "weights"); ok {
		if merged.Reviewers == nil {
			merged.Reviewers = make(map[string]ReviewerConfig)
		}
		for name, weight := range weights {
			rc := merged.Reviewers[name]
			rc.Weight = weight
			merged.Reviewers[name] = rc
		}
	}

	// Apply platform model to all reviewers that don't have an explicit model set.
	// This ensures platform model selection applies consistently across the panel.
	if model, ok := result.Config["model"].(string); ok && model != "" {
		if merged.Reviewers == nil {
			merged.Reviewers = make(map[string]ReviewerConfig)
		}
		for name, rc := range merged.Reviewers {
			if rc.Model == "" {
				rc.Model = model
				merged.Reviewers[name] = rc
			}
		}
	}

	// Apply platform custom_prompts → individual ReviewerConfig.Persona
	if prompts, ok := extractStringStringMap(result.Config, "custom_prompts"); ok {
		if merged.Reviewers == nil {
			merged.Reviewers = make(map[string]ReviewerConfig)
		}
		for name, persona := range prompts {
			rc := merged.Reviewers[name]
			rc.Persona = persona
			merged.Reviewers[name] = rc
		}
	}

	return merged
}

// extractStringSlice extracts a []string from a map[string]any value.
func extractStringSlice(m map[string]any, key string) ([]string, bool) {
	val, ok := m[key]
	if !ok {
		return nil, false
	}

	switch v := val.(type) {
	case []string:
		return v, true
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result, len(result) > 0
	default:
		return nil, false
	}
}

// extractStringFloatMap extracts a map[string]float64 from a map[string]any value.
func extractStringFloatMap(m map[string]any, key string) (map[string]float64, bool) {
	val, ok := m[key]
	if !ok {
		return nil, false
	}

	raw, ok := val.(map[string]any)
	if !ok {
		return nil, false
	}

	result := make(map[string]float64, len(raw))
	for k, v := range raw {
		switch f := v.(type) {
		case float64:
			result[k] = f
		case int:
			result[k] = float64(f)
		case int64:
			result[k] = float64(f)
		default:
			return nil, false
		}
	}
	return result, len(result) > 0
}

// extractStringStringMap extracts a map[string]string from a map[string]any value.
func extractStringStringMap(m map[string]any, key string) (map[string]string, bool) {
	val, ok := m[key]
	if !ok {
		return nil, false
	}

	raw, ok := val.(map[string]any)
	if !ok {
		return nil, false
	}

	result := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			result[k] = s
		} else {
			return nil, false
		}
	}
	return result, len(result) > 0
}

// FormatTierDisplay returns a human-readable tier label.
func FormatTierDisplay(tier string) string {
	switch tier {
	case "pro":
		return "Pro"
	case "", "free":
		return "Free"
	default:
		return fmt.Sprintf("Unknown (%s)", tier)
	}
}
