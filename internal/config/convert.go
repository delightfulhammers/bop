package config

import (
	"fmt"
)

// PlatformConfigFields are the fields returned by the platform config API.
// See: providers/config/bop/renderer.go in the platform repo.
const (
	FieldReviewers        = "reviewers"
	FieldWeights          = "weights"
	FieldModel            = "model"
	FieldSettings         = "settings"
	FieldCustomPrompts    = "custom_prompts"
	FieldAdvancedSettings = "advanced_settings"
)

// ConvertPlatformConfig converts a platform config response to bop's internal Config.
// It handles missing/partial config gracefully by returning defaults.
//
// Platform config structure:
//
//	{
//	  "reviewers": ["code-reviewer", "security-scanner"],
//	  "weights": {"code-reviewer": 1.0, "security-scanner": 0.8},
//	  "model": "claude-sonnet-4-20250514",
//	  "settings": {
//	    "auto_approve": false,
//	    "require_all_checks": true,
//	    "comment_style": "inline"
//	  }
//	}
func ConvertPlatformConfig(platformConfig map[string]any, tier string) Config {
	cfg := Config{
		// Initialize with defaults that the platform config will override
		Providers: make(map[string]ProviderConfig),
		Reviewers: make(map[string]ReviewerConfig),
		Merge: MergeConfig{
			Enabled:  true,
			Strategy: "consensus",
			Weights:  make(map[string]float64),
		},
	}

	// Convert reviewers list to DefaultReviewers
	if reviewers, ok := getStringSlice(platformConfig, FieldReviewers); ok {
		cfg.DefaultReviewers = reviewers
	}

	// Convert weights to Merge.Weights
	if weights, ok := getFloatMap(platformConfig, FieldWeights); ok {
		cfg.Merge.Weights = weights
		cfg.Merge.WeightByReviewer = true // Enable weighted merging when weights are set
	}

	// Convert model to default provider model
	if model, ok := getString(platformConfig, FieldModel); ok {
		// Set as the default model for anthropic provider (primary)
		cfg.Providers["anthropic"] = ProviderConfig{
			DefaultModel: model,
		}
	}

	// Convert settings to Review config
	if settings, ok := getMap(platformConfig, FieldSettings); ok {
		cfg.Review = convertSettings(settings)
	}

	// Convert custom_prompts (Enterprise only)
	if customPrompts, ok := getStringMap(platformConfig, FieldCustomPrompts); ok && len(customPrompts) > 0 {
		// Custom prompts map reviewer ID to custom system prompt
		// These will be used when creating reviewer configs
		for reviewerID, prompt := range customPrompts {
			reviewer := cfg.Reviewers[reviewerID]
			reviewer.Persona = prompt
			cfg.Reviewers[reviewerID] = reviewer
		}
	}

	// Convert advanced_settings (Enterprise only)
	if advSettings, ok := getMap(platformConfig, FieldAdvancedSettings); ok {
		cfg.Review = convertAdvancedSettings(cfg.Review, advSettings)
	}

	return cfg
}

// convertSettings converts platform settings to ReviewConfig.
func convertSettings(settings map[string]any) ReviewConfig {
	var review ReviewConfig

	// auto_approve maps to actions.onClean
	if autoApprove, ok := settings["auto_approve"].(bool); ok && autoApprove {
		review.Actions.OnClean = "approve"
	}

	// require_all_checks doesn't have a direct mapping yet - could be used for GH check config

	// comment_style could map to a future review.commentStyle field
	// For now, bop uses inline comments by default

	return review
}

// convertAdvancedSettings converts enterprise advanced settings.
func convertAdvancedSettings(review ReviewConfig, advSettings map[string]any) ReviewConfig {
	// parallel_reviews maps to MaxConcurrentReviewers
	// When true, unlimited parallel; when false, sequential
	if parallel, ok := advSettings["parallel_reviews"].(bool); ok {
		if !parallel {
			review.MaxConcurrentReviewers = 1 // Sequential
		}
	}

	// context_expansion could map to future context settings
	// Values: "minimal", "standard", "full"

	return review
}

// Helper functions for type-safe access to platform config map

func getString(m map[string]any, key string) (string, bool) {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

func getStringSlice(m map[string]any, key string) ([]string, bool) {
	if v, ok := m[key]; ok {
		// Handle []any (from JSON unmarshaling)
		if slice, ok := v.([]any); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result, len(result) > 0
		}
		// Handle []string (from typed sources)
		if slice, ok := v.([]string); ok {
			return slice, len(slice) > 0
		}
	}
	return nil, false
}

func getFloatMap(m map[string]any, key string) (map[string]float64, bool) {
	if v, ok := m[key]; ok {
		// Handle map[string]any (from JSON unmarshaling)
		if raw, ok := v.(map[string]any); ok {
			result := make(map[string]float64)
			for k, val := range raw {
				if f, ok := val.(float64); ok {
					result[k] = f
				}
			}
			return result, len(result) > 0
		}
		// Handle map[string]float64 (from typed sources)
		if typed, ok := v.(map[string]float64); ok {
			return typed, len(typed) > 0
		}
	}
	return nil, false
}

func getStringMap(m map[string]any, key string) (map[string]string, bool) {
	if v, ok := m[key]; ok {
		// Handle map[string]any (from JSON unmarshaling)
		if raw, ok := v.(map[string]any); ok {
			result := make(map[string]string)
			for k, val := range raw {
				if s, ok := val.(string); ok {
					result[k] = s
				}
			}
			return result, len(result) > 0
		}
		// Handle map[string]string (from typed sources)
		if typed, ok := v.(map[string]string); ok {
			return typed, len(typed) > 0
		}
	}
	return nil, false
}

func getMap(m map[string]any, key string) (map[string]any, bool) {
	if v, ok := m[key]; ok {
		if result, ok := v.(map[string]any); ok {
			return result, true
		}
	}
	return nil, false
}

// MergePlatformConfig merges platform config as base with local config as overlay.
// Local config fields override platform config when set.
// Returns the merged config.
func MergePlatformConfig(platform, local Config) (Config, error) {
	// Use the existing Merge function which handles overlay semantics
	return Merge(platform, local)
}

// ValidatePlatformConfig checks that platform config is usable.
// Returns an error if the config is missing required fields.
func ValidatePlatformConfig(platformConfig map[string]any) error {
	// At minimum, we need reviewers to be able to run a review
	if _, ok := getStringSlice(platformConfig, FieldReviewers); !ok {
		return fmt.Errorf("platform config missing required field: reviewers")
	}
	return nil
}
