package auth

import (
	"testing"
)

func TestEntitlementChecker_HasEntitlement(t *testing.T) {
	tests := []struct {
		name        string
		auth        *StoredAuth
		entitlement string
		expected    bool
	}{
		{
			name:        "nil auth has no entitlements",
			auth:        nil,
			entitlement: EntitlementCodeReview,
			expected:    false,
		},
		{
			name:        "empty entitlements grants all (graceful fallback)",
			auth:        &StoredAuth{Entitlements: []string{}},
			entitlement: EntitlementCodeReview,
			expected:    true,
		},
		{
			name:        "has specific entitlement",
			auth:        &StoredAuth{Entitlements: []string{EntitlementCodeReview, EntitlementPrivateRepos}},
			entitlement: EntitlementCodeReview,
			expected:    true,
		},
		{
			name:        "missing specific entitlement",
			auth:        &StoredAuth{Entitlements: []string{EntitlementCodeReview}},
			entitlement: EntitlementPrivateRepos,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewEntitlementChecker(tt.auth)
			got := checker.HasEntitlement(tt.entitlement)
			if got != tt.expected {
				t.Errorf("HasEntitlement(%s) = %v, want %v", tt.entitlement, got, tt.expected)
			}
		})
	}
}

func TestEntitlementChecker_CanReviewCode(t *testing.T) {
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
			name:     "empty entitlements can review (graceful fallback)",
			auth:     &StoredAuth{Entitlements: []string{}},
			expected: true,
		},
		{
			name:     "has code-review entitlement",
			auth:     &StoredAuth{Entitlements: []string{EntitlementCodeReview}},
			expected: true,
		},
		{
			name:     "missing code-review entitlement",
			auth:     &StoredAuth{Entitlements: []string{EntitlementPrivateRepos}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewEntitlementChecker(tt.auth)
			got := checker.CanReviewCode()
			if got != tt.expected {
				t.Errorf("CanReviewCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEntitlementChecker_CanReviewPrivateRepos(t *testing.T) {
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
			auth:     &StoredAuth{Entitlements: []string{EntitlementCodeReview}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewEntitlementChecker(tt.auth)
			got := checker.CanReviewPrivateRepos()
			if got != tt.expected {
				t.Errorf("CanReviewPrivateRepos() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEntitlementChecker_Plan(t *testing.T) {
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
			auth:     &StoredAuth{Plan: "individual"},
			expected: "individual",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewEntitlementChecker(tt.auth)
			got := checker.Plan()
			if got != tt.expected {
				t.Errorf("Plan() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestEntitlementChecker_IsLoggedIn(t *testing.T) {
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
			checker := NewEntitlementChecker(tt.auth)
			got := checker.IsLoggedIn()
			if got != tt.expected {
				t.Errorf("IsLoggedIn() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEntitlementChecker_IsFreeUser(t *testing.T) {
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
			name:     "individual plan is not free",
			auth:     &StoredAuth{Plan: "individual"},
			expected: false,
		},
		{
			name:     "business plan is not free",
			auth:     &StoredAuth{Plan: "business"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewEntitlementChecker(tt.auth)
			got := checker.IsFreeUser()
			if got != tt.expected {
				t.Errorf("IsFreeUser() = %v, want %v", got, tt.expected)
			}
		})
	}
}
