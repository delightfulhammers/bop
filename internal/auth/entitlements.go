package auth

import (
	"slices"
)

// Known entitlement names for bop.
const (
	// EntitlementCodeReview is the basic code review functionality.
	EntitlementCodeReview = "code-review"

	// EntitlementPrivateRepos allows reviewing private repositories.
	EntitlementPrivateRepos = "private-repos"

	// EntitlementUnlimitedReviews removes monthly review limits.
	EntitlementUnlimitedReviews = "unlimited-reviews"

	// EntitlementConfigAPI allows remote config sync.
	EntitlementConfigAPI = "config-api"
)

// EntitlementChecker checks if a user has specific entitlements.
// It implements graceful fallback: when entitlements are empty (platform not
// enforcing yet), all permissions are granted.
type EntitlementChecker struct {
	auth *StoredAuth
}

// NewEntitlementChecker creates an entitlement checker from stored auth.
// If auth is nil, all checks will return false (no permissions).
func NewEntitlementChecker(auth *StoredAuth) *EntitlementChecker {
	return &EntitlementChecker{auth: auth}
}

// HasEntitlement checks if the user has a specific entitlement.
// Graceful fallback: empty entitlements = all permissions granted.
func (e *EntitlementChecker) HasEntitlement(name string) bool {
	if e.auth == nil {
		return false
	}

	// Graceful fallback: empty entitlements means platform isn't enforcing yet
	if len(e.auth.Entitlements) == 0 {
		return true
	}

	return slices.Contains(e.auth.Entitlements, name)
}

// CanReviewCode checks if the user can perform code reviews.
func (e *EntitlementChecker) CanReviewCode() bool {
	return e.HasEntitlement(EntitlementCodeReview)
}

// CanReviewPrivateRepos checks if the user can review private repositories.
func (e *EntitlementChecker) CanReviewPrivateRepos() bool {
	return e.HasEntitlement(EntitlementPrivateRepos)
}

// CanUseConfigAPI checks if the user can use remote config sync.
func (e *EntitlementChecker) CanUseConfigAPI() bool {
	return e.HasEntitlement(EntitlementConfigAPI)
}

// HasUnlimitedReviews checks if the user has unlimited review quota.
func (e *EntitlementChecker) HasUnlimitedReviews() bool {
	return e.HasEntitlement(EntitlementUnlimitedReviews)
}

// Plan returns the user's subscription plan.
func (e *EntitlementChecker) Plan() string {
	if e.auth == nil {
		return ""
	}
	return e.auth.Plan
}

// IsLoggedIn returns true if there is valid auth state.
func (e *EntitlementChecker) IsLoggedIn() bool {
	return e.auth != nil
}

// IsFreeUser returns true if the user is on the free plan.
func (e *EntitlementChecker) IsFreeUser() bool {
	if e.auth == nil {
		return true
	}
	return e.auth.Plan == "" || e.auth.Plan == "free"
}
