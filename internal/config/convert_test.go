package config

import (
	"reflect"
	"testing"
)

func TestConvertPlatformConfig(t *testing.T) {
	tests := []struct {
		name           string
		platformConfig map[string]any
		tier           string
		checkFn        func(t *testing.T, cfg Config)
	}{
		{
			name: "converts reviewers to DefaultReviewers",
			platformConfig: map[string]any{
				"reviewers": []any{"code-reviewer", "security-scanner"},
			},
			tier: "pro",
			checkFn: func(t *testing.T, cfg Config) {
				want := []string{"code-reviewer", "security-scanner"}
				if !reflect.DeepEqual(cfg.DefaultReviewers, want) {
					t.Errorf("DefaultReviewers = %v, want %v", cfg.DefaultReviewers, want)
				}
			},
		},
		{
			name: "converts weights to Merge.Weights",
			platformConfig: map[string]any{
				"reviewers": []any{"code-reviewer"},
				"weights": map[string]any{
					"code-reviewer":    1.0,
					"security-scanner": 0.8,
				},
			},
			tier: "pro",
			checkFn: func(t *testing.T, cfg Config) {
				if cfg.Merge.Weights["code-reviewer"] != 1.0 {
					t.Errorf("Merge.Weights[code-reviewer] = %v, want 1.0", cfg.Merge.Weights["code-reviewer"])
				}
				if cfg.Merge.Weights["security-scanner"] != 0.8 {
					t.Errorf("Merge.Weights[security-scanner] = %v, want 0.8", cfg.Merge.Weights["security-scanner"])
				}
				if !cfg.Merge.WeightByReviewer {
					t.Error("Merge.WeightByReviewer should be true when weights are set")
				}
			},
		},
		{
			name: "converts model to provider default",
			platformConfig: map[string]any{
				"reviewers": []any{"code-reviewer"},
				"model":     "claude-sonnet-4-20250514",
			},
			tier: "pro",
			checkFn: func(t *testing.T, cfg Config) {
				if cfg.Providers["anthropic"].DefaultModel != "claude-sonnet-4-20250514" {
					t.Errorf("Providers[anthropic].DefaultModel = %v, want claude-sonnet-4-20250514",
						cfg.Providers["anthropic"].DefaultModel)
				}
			},
		},
		{
			name: "converts settings",
			platformConfig: map[string]any{
				"reviewers": []any{"code-reviewer"},
				"settings": map[string]any{
					"auto_approve": true,
				},
			},
			tier: "pro",
			checkFn: func(t *testing.T, cfg Config) {
				if cfg.Review.Actions.OnClean != "approve" {
					t.Errorf("Review.Actions.OnClean = %v, want approve", cfg.Review.Actions.OnClean)
				}
			},
		},
		{
			name: "converts custom_prompts to reviewer personas",
			platformConfig: map[string]any{
				"reviewers": []any{"security"},
				"custom_prompts": map[string]any{
					"security": "You are a security expert...",
				},
			},
			tier: "enterprise",
			checkFn: func(t *testing.T, cfg Config) {
				if cfg.Reviewers["security"].Persona != "You are a security expert..." {
					t.Errorf("Reviewers[security].Persona = %v, want custom prompt",
						cfg.Reviewers["security"].Persona)
				}
			},
		},
		{
			name: "converts advanced_settings parallel_reviews false",
			platformConfig: map[string]any{
				"reviewers": []any{"code-reviewer"},
				"advanced_settings": map[string]any{
					"parallel_reviews": false,
				},
			},
			tier: "enterprise",
			checkFn: func(t *testing.T, cfg Config) {
				if cfg.Review.MaxConcurrentReviewers != 1 {
					t.Errorf("Review.MaxConcurrentReviewers = %v, want 1 (sequential)",
						cfg.Review.MaxConcurrentReviewers)
				}
			},
		},
		{
			name:           "empty config returns defaults",
			platformConfig: map[string]any{},
			tier:           "beta",
			checkFn: func(t *testing.T, cfg Config) {
				if cfg.Providers == nil {
					t.Error("Providers should be initialized")
				}
				if cfg.Reviewers == nil {
					t.Error("Reviewers should be initialized")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ConvertPlatformConfig(tt.platformConfig, tt.tier)
			tt.checkFn(t, cfg)
		})
	}
}

func TestValidatePlatformConfig(t *testing.T) {
	tests := []struct {
		name           string
		platformConfig map[string]any
		wantErr        bool
	}{
		{
			name: "valid config with reviewers",
			platformConfig: map[string]any{
				"reviewers": []any{"code-reviewer"},
			},
			wantErr: false,
		},
		{
			name:           "missing reviewers is invalid",
			platformConfig: map[string]any{},
			wantErr:        true,
		},
		{
			name: "empty reviewers is invalid",
			platformConfig: map[string]any{
				"reviewers": []any{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlatformConfig(tt.platformConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePlatformConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMergePlatformConfig(t *testing.T) {
	platform := Config{
		DefaultReviewers: []string{"code-reviewer"},
		Merge: MergeConfig{
			Strategy: "consensus",
			Weights: map[string]float64{
				"code-reviewer": 1.0,
			},
		},
	}

	local := Config{
		Review: ReviewConfig{
			Instructions: "Custom instructions from local config",
		},
	}

	merged, err := MergePlatformConfig(platform, local)
	if err != nil {
		t.Fatalf("MergePlatformConfig() error = %v", err)
	}

	// Platform values should be present
	if len(merged.DefaultReviewers) != 1 || merged.DefaultReviewers[0] != "code-reviewer" {
		t.Error("platform DefaultReviewers should be preserved")
	}

	// Local values should overlay
	if merged.Review.Instructions != "Custom instructions from local config" {
		t.Error("local Instructions should overlay")
	}
}

// Helper function tests
func TestGetStringSlice(t *testing.T) {
	tests := []struct {
		name   string
		m      map[string]any
		key    string
		want   []string
		wantOk bool
	}{
		{
			name:   "[]any converts to []string",
			m:      map[string]any{"items": []any{"a", "b"}},
			key:    "items",
			want:   []string{"a", "b"},
			wantOk: true,
		},
		{
			name:   "[]string passthrough",
			m:      map[string]any{"items": []string{"a", "b"}},
			key:    "items",
			want:   []string{"a", "b"},
			wantOk: true,
		},
		{
			name:   "missing key returns false",
			m:      map[string]any{},
			key:    "items",
			want:   nil,
			wantOk: false,
		},
		{
			name:   "empty slice returns false",
			m:      map[string]any{"items": []any{}},
			key:    "items",
			want:   []string{}, // Empty slice (not nil) but ok=false
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := getStringSlice(tt.m, tt.key)
			if ok != tt.wantOk {
				t.Errorf("getStringSlice() ok = %v, want %v", ok, tt.wantOk)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getStringSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetFloatMap(t *testing.T) {
	tests := []struct {
		name   string
		m      map[string]any
		key    string
		want   map[string]float64
		wantOk bool
	}{
		{
			name:   "map[string]any converts",
			m:      map[string]any{"weights": map[string]any{"a": 1.0, "b": 0.5}},
			key:    "weights",
			want:   map[string]float64{"a": 1.0, "b": 0.5},
			wantOk: true,
		},
		{
			name:   "map[string]float64 passthrough",
			m:      map[string]any{"weights": map[string]float64{"a": 1.0}},
			key:    "weights",
			want:   map[string]float64{"a": 1.0},
			wantOk: true,
		},
		{
			name:   "missing key returns false",
			m:      map[string]any{},
			key:    "weights",
			want:   nil,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := getFloatMap(tt.m, tt.key)
			if ok != tt.wantOk {
				t.Errorf("getFloatMap() ok = %v, want %v", ok, tt.wantOk)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getFloatMap() = %v, want %v", got, tt.want)
			}
		})
	}
}
