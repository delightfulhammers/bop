package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewReviewer(t *testing.T) {
	t.Parallel()

	reviewer := NewReviewer("security")

	if reviewer.Name != "security" {
		t.Errorf("Name = %q, want %q", reviewer.Name, "security")
	}
	if reviewer.Weight != 1.0 {
		t.Errorf("Weight = %f, want 1.0", reviewer.Weight)
	}
	if !reviewer.Enabled {
		t.Error("Enabled = false, want true")
	}
	if reviewer.Focus == nil {
		t.Error("Focus is nil, want empty slice")
	}
	if reviewer.Ignore == nil {
		t.Error("Ignore is nil, want empty slice")
	}
}

func TestReviewer_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		reviewer Reviewer
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid_minimal",
			reviewer: Reviewer{
				Name:     "security",
				Provider: "anthropic",
				Model:    "claude-opus-4",
				Weight:   1.0,
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "valid_with_persona",
			reviewer: Reviewer{
				Name:     "security",
				Provider: "anthropic",
				Model:    "claude-opus-4",
				Weight:   1.5,
				Persona:  "You are a security expert...",
				Focus:    []string{"security", "authentication"},
				Ignore:   []string{"style"},
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "missing_name",
			reviewer: Reviewer{
				Provider: "anthropic",
				Model:    "claude-opus-4",
				Weight:   1.0,
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "missing_provider",
			reviewer: Reviewer{
				Name:   "security",
				Model:  "claude-opus-4",
				Weight: 1.0,
			},
			wantErr: true,
			errMsg:  "provider is required",
		},
		{
			name: "missing_model",
			reviewer: Reviewer{
				Name:     "security",
				Provider: "anthropic",
				Weight:   1.0,
			},
			wantErr: true,
			errMsg:  "model is required",
		},
		{
			name: "zero_weight",
			reviewer: Reviewer{
				Name:     "security",
				Provider: "anthropic",
				Model:    "claude-opus-4",
				Weight:   0,
			},
			wantErr: true,
			errMsg:  "weight must be positive",
		},
		{
			name: "negative_weight",
			reviewer: Reviewer{
				Name:     "security",
				Provider: "anthropic",
				Model:    "claude-opus-4",
				Weight:   -1.0,
			},
			wantErr: true,
			errMsg:  "weight must be positive",
		},
		{
			name: "focus_ignore_overlap",
			reviewer: Reviewer{
				Name:     "security",
				Provider: "anthropic",
				Model:    "claude-opus-4",
				Weight:   1.0,
				Focus:    []string{"security", "style"},
				Ignore:   []string{"style", "docs"},
			},
			wantErr: true,
			errMsg:  "cannot be both focused and ignored",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.reviewer.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() error = nil, want error containing %q", tt.errMsg)
					return
				}
				if !errors.Is(err, ErrInvalidReviewer) {
					t.Errorf("Validate() error = %v, want error wrapping ErrInvalidReviewer", err)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestReviewer_ShouldFocus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		reviewer Reviewer
		category string
		want     bool
	}{
		{
			name: "empty_focus_all_categories",
			reviewer: Reviewer{
				Focus:  []string{},
				Ignore: []string{},
			},
			category: "security",
			want:     true,
		},
		{
			name: "category_in_focus",
			reviewer: Reviewer{
				Focus:  []string{"security", "authentication"},
				Ignore: []string{},
			},
			category: "security",
			want:     true,
		},
		{
			name: "category_not_in_focus",
			reviewer: Reviewer{
				Focus:  []string{"security", "authentication"},
				Ignore: []string{},
			},
			category: "style",
			want:     false,
		},
		{
			name: "category_in_ignore",
			reviewer: Reviewer{
				Focus:  []string{},
				Ignore: []string{"style", "docs"},
			},
			category: "style",
			want:     false,
		},
		{
			name: "ignore_takes_precedence",
			reviewer: Reviewer{
				Focus:  []string{}, // Empty focus means "all"
				Ignore: []string{"security"},
			},
			category: "security",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.reviewer.ShouldFocus(tt.category)
			if got != tt.want {
				t.Errorf("ShouldFocus(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestReviewer_IsIgnored(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ignore   []string
		category string
		want     bool
	}{
		{
			name:     "empty_ignore",
			ignore:   []string{},
			category: "style",
			want:     false,
		},
		{
			name:     "category_in_ignore",
			ignore:   []string{"style", "docs"},
			category: "style",
			want:     true,
		},
		{
			name:     "category_not_in_ignore",
			ignore:   []string{"style", "docs"},
			category: "security",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reviewer := Reviewer{Ignore: tt.ignore}
			got := reviewer.IsIgnored(tt.category)
			if got != tt.want {
				t.Errorf("IsIgnored(%q) = %v, want %v", tt.category, got, tt.want)
			}
		})
	}
}

func TestReviewer_HasFocus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		focus []string
		want  bool
	}{
		{name: "nil_focus", focus: nil, want: false},
		{name: "empty_focus", focus: []string{}, want: false},
		{name: "has_focus", focus: []string{"security"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reviewer := Reviewer{Focus: tt.focus}
			if got := reviewer.HasFocus(); got != tt.want {
				t.Errorf("HasFocus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReviewer_HasIgnore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ignore []string
		want   bool
	}{
		{name: "nil_ignore", ignore: nil, want: false},
		{name: "empty_ignore", ignore: []string{}, want: false},
		{name: "has_ignore", ignore: []string{"style"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reviewer := Reviewer{Ignore: tt.ignore}
			if got := reviewer.HasIgnore(); got != tt.want {
				t.Errorf("HasIgnore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReviewer_IsActive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
		want    bool
	}{
		{name: "enabled", enabled: true, want: true},
		{name: "disabled", enabled: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reviewer := Reviewer{Enabled: tt.enabled}
			if got := reviewer.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReviewer_ModelIdentifier(t *testing.T) {
	t.Parallel()

	reviewer := Reviewer{
		Provider: "anthropic",
		Model:    "claude-opus-4",
	}

	want := "anthropic/claude-opus-4"
	if got := reviewer.ModelIdentifier(); got != want {
		t.Errorf("ModelIdentifier() = %q, want %q", got, want)
	}
}
