package auth

import (
	"bytes"
	"strings"
	"testing"
)

func TestBopEntitlements_HasEntitlement(t *testing.T) {
	tests := []struct {
		name        string
		auth        *StoredAuth
		entitlement string
		expected    bool
	}{
		{
			name:        "nil auth has no entitlements",
			auth:        nil,
			entitlement: EntitlementPrivateRepos,
			expected:    false,
		},
		{
			name:        "empty entitlements grants all (graceful fallback)",
			auth:        &StoredAuth{Entitlements: []string{}},
			entitlement: EntitlementPrivateRepos,
			expected:    true,
		},
		{
			name:        "has specific entitlement",
			auth:        &StoredAuth{Entitlements: []string{EntitlementPrivateRepos, EntitlementAnyOrg}},
			entitlement: EntitlementPrivateRepos,
			expected:    true,
		},
		{
			name:        "missing specific entitlement",
			auth:        &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			entitlement: EntitlementPrivateRepos,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewBopEntitlements(tt.auth, nil)
			got := checker.HasEntitlement(tt.entitlement)
			if got != tt.expected {
				t.Errorf("HasEntitlement(%s) = %v, want %v", tt.entitlement, got, tt.expected)
			}
		})
	}
}

func TestBopEntitlements_CanAccessRepo(t *testing.T) {
	tests := []struct {
		name      string
		auth      *StoredAuth
		isPrivate bool
		ownerType string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "public repo always allowed",
			auth:      &StoredAuth{Entitlements: []string{}},
			isPrivate: false,
			ownerType: "User",
			wantErr:   false,
		},
		{
			name:      "private repo with entitlement",
			auth:      &StoredAuth{Entitlements: []string{EntitlementPrivateRepos}},
			isPrivate: true,
			ownerType: "User",
			wantErr:   false,
		},
		{
			name:      "private repo without entitlement",
			auth:      &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			isPrivate: true,
			ownerType: "User",
			wantErr:   true,
			errMsg:    "Private repository access requires Solo plan",
		},
		{
			name:      "org repo with any-org entitlement",
			auth:      &StoredAuth{Entitlements: []string{EntitlementPrivateRepos, EntitlementAnyOrg}},
			isPrivate: true,
			ownerType: "Organization",
			wantErr:   false,
		},
		{
			name:      "org repo with personal-org-only (no any-org)",
			auth:      &StoredAuth{Entitlements: []string{EntitlementPrivateRepos, EntitlementPersonalOrgOnly}},
			isPrivate: true,
			ownerType: "Organization",
			wantErr:   true,
			errMsg:    "organization repositories requires Pro plan",
		},
		{
			name:      "graceful fallback with empty entitlements",
			auth:      &StoredAuth{Entitlements: []string{}},
			isPrivate: true,
			ownerType: "Organization",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewBopEntitlements(tt.auth, nil)
			err := checker.CanAccessRepo(tt.isPrivate, tt.ownerType)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBopEntitlements_ResolveModel(t *testing.T) {
	tests := []struct {
		name           string
		auth           *StoredAuth
		requested      string
		expected       string
		expectFallback bool
	}{
		{
			name:           "empty request uses default",
			auth:           &StoredAuth{Entitlements: []string{}},
			requested:      "",
			expected:       DefaultModel,
			expectFallback: false,
		},
		{
			name:           "ollama model always allowed",
			auth:           &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			requested:      "ollama/llama3",
			expected:       "ollama/llama3",
			expectFallback: false,
		},
		{
			name:           "ollama model with tag always allowed",
			auth:           &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			requested:      "llama3:latest",
			expected:       "llama3:latest",
			expectFallback: false,
		},
		{
			name:           "default model allowed without entitlement",
			auth:           &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			requested:      DefaultModel,
			expected:       DefaultModel,
			expectFallback: false,
		},
		{
			name:           "non-default model with entitlement",
			auth:           &StoredAuth{Entitlements: []string{EntitlementModelSelection}},
			requested:      "gpt-4",
			expected:       "gpt-4",
			expectFallback: false,
		},
		{
			name:           "non-default model without entitlement falls back",
			auth:           &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			requested:      "gpt-4",
			expected:       DefaultModel,
			expectFallback: true,
		},
		{
			name:           "graceful fallback with empty entitlements allows any model",
			auth:           &StoredAuth{Entitlements: []string{}},
			requested:      "gpt-4",
			expected:       "gpt-4",
			expectFallback: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			checker := NewBopEntitlements(tt.auth, &buf)
			got := checker.ResolveModel(tt.requested)
			if got != tt.expected {
				t.Errorf("ResolveModel(%s) = %s, want %s", tt.requested, got, tt.expected)
			}
			if tt.expectFallback {
				if len(checker.FallbacksApplied()) == 0 {
					t.Error("expected fallback to be recorded")
				}
				if !strings.Contains(buf.String(), "model-selection") {
					t.Errorf("expected fallback message, got: %s", buf.String())
				}
			}
		})
	}
}

func TestBopEntitlements_ResolveReviewerPanel(t *testing.T) {
	tests := []struct {
		name           string
		auth           *StoredAuth
		requested      string
		defaultID      string
		expected       string
		expectFallback bool
	}{
		{
			name:           "empty request uses default",
			auth:           &StoredAuth{Entitlements: []string{}},
			requested:      "",
			defaultID:      "default-reviewer",
			expected:       "default-reviewer",
			expectFallback: false,
		},
		{
			name:           "same as default passes through",
			auth:           &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			requested:      "default-reviewer",
			defaultID:      "default-reviewer",
			expected:       "default-reviewer",
			expectFallback: false,
		},
		{
			name:           "custom reviewer with entitlement",
			auth:           &StoredAuth{Entitlements: []string{EntitlementCustomizeReviewerPanel}},
			requested:      "security-expert",
			defaultID:      "default-reviewer",
			expected:       "security-expert",
			expectFallback: false,
		},
		{
			name:           "custom reviewer without entitlement falls back",
			auth:           &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			requested:      "security-expert",
			defaultID:      "default-reviewer",
			expected:       "default-reviewer",
			expectFallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			checker := NewBopEntitlements(tt.auth, &buf)
			got := checker.ResolveReviewerPanel(tt.requested, tt.defaultID)
			if got != tt.expected {
				t.Errorf("ResolveReviewerPanel(%s, %s) = %s, want %s", tt.requested, tt.defaultID, got, tt.expected)
			}
			if tt.expectFallback && len(checker.FallbacksApplied()) == 0 {
				t.Error("expected fallback to be recorded")
			}
		})
	}
}

func TestBopEntitlements_ResolveReviewerPrompt(t *testing.T) {
	tests := []struct {
		name           string
		auth           *StoredAuth
		hasCustom      bool
		expected       bool
		expectFallback bool
	}{
		{
			name:           "no custom prompt returns false",
			auth:           &StoredAuth{Entitlements: []string{}},
			hasCustom:      false,
			expected:       false,
			expectFallback: false,
		},
		{
			name:           "custom prompt with entitlement",
			auth:           &StoredAuth{Entitlements: []string{EntitlementCustomizeReviewerPrompt}},
			hasCustom:      true,
			expected:       true,
			expectFallback: false,
		},
		{
			name:           "custom prompt without entitlement falls back",
			auth:           &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			hasCustom:      true,
			expected:       false,
			expectFallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			checker := NewBopEntitlements(tt.auth, &buf)
			got := checker.ResolveReviewerPrompt(tt.hasCustom)
			if got != tt.expected {
				t.Errorf("ResolveReviewerPrompt(%v) = %v, want %v", tt.hasCustom, got, tt.expected)
			}
			if tt.expectFallback && len(checker.FallbacksApplied()) == 0 {
				t.Error("expected fallback to be recorded")
			}
		})
	}
}

func TestBopEntitlements_CanUseLocalConfig(t *testing.T) {
	tests := []struct {
		name    string
		auth    *StoredAuth
		wantErr bool
	}{
		{
			name:    "with entitlement",
			auth:    &StoredAuth{Entitlements: []string{EntitlementLocalBopConfig}},
			wantErr: false,
		},
		{
			name:    "without entitlement",
			auth:    &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			wantErr: true,
		},
		{
			name:    "graceful fallback with empty entitlements",
			auth:    &StoredAuth{Entitlements: []string{}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewBopEntitlements(tt.auth, nil)
			err := checker.CanUseLocalConfig()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			} else if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBopEntitlements_RequireBYOK(t *testing.T) {
	tests := []struct {
		name              string
		auth              *StoredAuth
		hasConfiguredKeys bool
		wantErr           bool
	}{
		{
			name:              "beta user with keys configured",
			auth:              &StoredAuth{Plan: "beta", Entitlements: []string{EntitlementBYOProviderKeys}},
			hasConfiguredKeys: true,
			wantErr:           false,
		},
		{
			name:              "beta user without keys configured",
			auth:              &StoredAuth{Plan: "beta", Entitlements: []string{EntitlementBYOProviderKeys}},
			hasConfiguredKeys: false,
			wantErr:           true,
		},
		{
			name:              "non-beta user without keys is ok",
			auth:              &StoredAuth{Plan: "solo", Entitlements: []string{EntitlementPublicRepos}},
			hasConfiguredKeys: false,
			wantErr:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewBopEntitlements(tt.auth, nil)
			err := checker.RequireBYOK(tt.hasConfiguredKeys)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			} else if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBopEntitlements_Plan(t *testing.T) {
	tests := []struct {
		name     string
		auth     *StoredAuth
		expected string
	}{
		{
			name:     "nil auth returns empty",
			auth:     nil,
			expected: "",
		},
		{
			name:     "returns plan from auth",
			auth:     &StoredAuth{Plan: "solo"},
			expected: "solo",
		},
		{
			name:     "returns beta plan",
			auth:     &StoredAuth{Plan: "beta"},
			expected: "beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewBopEntitlements(tt.auth, nil)
			got := checker.Plan()
			if got != tt.expected {
				t.Errorf("Plan() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestBopEntitlements_IsLoggedIn(t *testing.T) {
	tests := []struct {
		name     string
		auth     *StoredAuth
		expected bool
	}{
		{
			name:     "nil auth is not logged in",
			auth:     nil,
			expected: false,
		},
		{
			name:     "non-nil auth is logged in",
			auth:     &StoredAuth{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewBopEntitlements(tt.auth, nil)
			got := checker.IsLoggedIn()
			if got != tt.expected {
				t.Errorf("IsLoggedIn() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBopEntitlements_IsBetaUser(t *testing.T) {
	tests := []struct {
		name     string
		auth     *StoredAuth
		expected bool
	}{
		{
			name:     "nil auth is not beta",
			auth:     nil,
			expected: false,
		},
		{
			name:     "beta plan is beta",
			auth:     &StoredAuth{Plan: "beta"},
			expected: true,
		},
		{
			name:     "solo plan is not beta",
			auth:     &StoredAuth{Plan: "solo"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewBopEntitlements(tt.auth, nil)
			got := checker.IsBetaUser()
			if got != tt.expected {
				t.Errorf("IsBetaUser() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBopEntitlements_IsFreeUser(t *testing.T) {
	tests := []struct {
		name     string
		auth     *StoredAuth
		expected bool
	}{
		{
			name:     "nil auth is free",
			auth:     nil,
			expected: true,
		},
		{
			name:     "empty plan is free",
			auth:     &StoredAuth{Plan: ""},
			expected: true,
		},
		{
			name:     "free plan is free",
			auth:     &StoredAuth{Plan: "free"},
			expected: true,
		},
		{
			name:     "solo plan is not free",
			auth:     &StoredAuth{Plan: "solo"},
			expected: false,
		},
		{
			name:     "beta plan is not free",
			auth:     &StoredAuth{Plan: "beta"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewBopEntitlements(tt.auth, nil)
			got := checker.IsFreeUser()
			if got != tt.expected {
				t.Errorf("IsFreeUser() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBopEntitlements_FallbacksApplied(t *testing.T) {
	var buf bytes.Buffer
	checker := NewBopEntitlements(&StoredAuth{
		Entitlements: []string{EntitlementPublicRepos}, // Has public-repos only
	}, &buf)

	// Trigger a fallback
	_ = checker.ResolveModel("gpt-4") // Will fallback

	fallbacks := checker.FallbacksApplied()
	if len(fallbacks) != 1 {
		t.Errorf("expected 1 fallback, got %d", len(fallbacks))
	}
	if len(fallbacks) > 0 && fallbacks[0] != EntitlementModelSelection {
		t.Errorf("expected fallback for %s, got %s", EntitlementModelSelection, fallbacks[0])
	}
}

func TestEntitlementError(t *testing.T) {
	err := &EntitlementError{
		Entitlement: "private-repos",
		Message:     "Custom message",
		UpgradeURL:  "https://example.com",
	}

	if err.Error() != "Custom message" {
		t.Errorf("Error() = %s, want Custom message", err.Error())
	}

	errNoMsg := &EntitlementError{
		Entitlement: "private-repos",
	}
	if !strings.Contains(errNoMsg.Error(), "private-repos") {
		t.Errorf("Error() should contain entitlement name, got: %s", errNoMsg.Error())
	}
}

// Legacy compatibility tests

func TestNewEntitlementChecker_LegacyAlias(t *testing.T) {
	auth := &StoredAuth{Plan: "solo"}
	checker := NewEntitlementChecker(auth)

	if checker.Plan() != "solo" {
		t.Errorf("NewEntitlementChecker legacy alias failed, Plan() = %s", checker.Plan())
	}
}

func TestBopEntitlements_CanReviewCode(t *testing.T) {
	tests := []struct {
		name     string
		auth     *StoredAuth
		expected bool
	}{
		{
			name:     "nil auth cannot review",
			auth:     nil,
			expected: false,
		},
		{
			name:     "logged in can review",
			auth:     &StoredAuth{},
			expected: true,
		},
		{
			name:     "with public-repos can review",
			auth:     &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewBopEntitlements(tt.auth, nil)
			got := checker.CanReviewCode()
			if got != tt.expected {
				t.Errorf("CanReviewCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBopEntitlements_CanReviewPrivateRepos(t *testing.T) {
	tests := []struct {
		name     string
		auth     *StoredAuth
		expected bool
	}{
		{
			name:     "empty entitlements can review private (graceful fallback)",
			auth:     &StoredAuth{Entitlements: []string{}},
			expected: true,
		},
		{
			name:     "has private-repos entitlement",
			auth:     &StoredAuth{Entitlements: []string{EntitlementPrivateRepos}},
			expected: true,
		},
		{
			name:     "missing private-repos entitlement",
			auth:     &StoredAuth{Entitlements: []string{EntitlementPublicRepos}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewBopEntitlements(tt.auth, nil)
			got := checker.CanReviewPrivateRepos()
			if got != tt.expected {
				t.Errorf("CanReviewPrivateRepos() = %v, want %v", got, tt.expected)
			}
		})
	}
}
