package auth

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// Bop entitlement identifiers (12 entitlements per spec).
const (
	// Repository Access
	EntitlementPublicRepos  = "public-repos"
	EntitlementPrivateRepos = "private-repos"

	// Organization Scope
	EntitlementPersonalOrgOnly = "personal-org-only"
	EntitlementAnyOrg          = "any-org"

	// Model Selection
	EntitlementModelSelection = "model-selection"
	EntitlementOllamaModels   = "ollama-models"

	// Customization
	EntitlementCustomizeReviewerPanel  = "customize-reviewer-panel"
	EntitlementCustomizeReviewerPrompt = "customize-reviewer-prompt"

	// Provider Keys
	EntitlementBYOProviderKeys = "byo-provider-keys"

	// Configuration
	EntitlementLocalBopConfig = "local-bop-config"

	// Token Management (informational)
	EntitlementTokenPurchase = "token-purchase"
	EntitlementALaCarte      = "a-la-carte"
)

// DefaultModel is used when model-selection entitlement is not available.
const DefaultModel = "gpt-4o-mini"

// EntitlementError represents a hard-block entitlement failure.
type EntitlementError struct {
	Entitlement string
	Message     string
	UpgradeURL  string
}

func (e *EntitlementError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("feature %q not available on your plan", e.Entitlement)
}

// BopEntitlements checks entitlements and applies graceful fallbacks.
// Implements the bop entitlement spec with support for hard-block and graceful-fallback enforcement.
type BopEntitlements struct {
	auth             *StoredAuth
	output           io.Writer           // For user-visible fallback messages (nil = no output)
	fallbacksApplied map[string]struct{} // Deduplicated set of fallbacks
}

// NewBopEntitlements creates an entitlement checker from stored auth.
// If auth is nil, all checks will return false (no permissions).
// If output is non-nil, graceful fallback messages will be written there.
func NewBopEntitlements(auth *StoredAuth, output io.Writer) *BopEntitlements {
	return &BopEntitlements{
		auth:             auth,
		output:           output,
		fallbacksApplied: make(map[string]struct{}),
	}
}

// FallbacksApplied returns the list of entitlements that triggered graceful fallbacks.
// Use this for usage event reporting. Returns a copy to prevent mutation.
func (e *BopEntitlements) FallbacksApplied() []string {
	result := make([]string, 0, len(e.fallbacksApplied))
	for k := range e.fallbacksApplied {
		result = append(result, k)
	}
	return result
}

// notifyFallback records a fallback and optionally outputs a user-visible message.
// Only outputs the message once per entitlement (deduplicated).
func (e *BopEntitlements) notifyFallback(entitlement, message string) {
	// Only notify once per entitlement
	if _, exists := e.fallbacksApplied[entitlement]; exists {
		return
	}
	e.fallbacksApplied[entitlement] = struct{}{}
	if e.output != nil {
		_, _ = fmt.Fprintf(e.output, "ℹ️  %s\n", message)
	}
}

// HasEntitlement checks if the user has a specific entitlement.
// Graceful fallback: empty entitlements = all permissions granted (platform not enforcing yet).
func (e *BopEntitlements) HasEntitlement(name string) bool {
	if e.auth == nil {
		return false
	}

	// Graceful fallback: empty entitlements means platform isn't enforcing yet
	if len(e.auth.Entitlements) == 0 {
		return true
	}

	return slices.Contains(e.auth.Entitlements, name)
}

// Plan returns the user's subscription plan.
func (e *BopEntitlements) Plan() string {
	if e.auth == nil {
		return ""
	}
	return e.auth.Plan
}

// Tier returns the plan name (alias for Plan for spec compatibility).
func (e *BopEntitlements) Tier() string {
	return e.Plan()
}

// IsLoggedIn returns true if there is valid auth state.
func (e *BopEntitlements) IsLoggedIn() bool {
	return e.auth != nil
}

// IsBetaUser returns true if the user is on the beta plan.
func (e *BopEntitlements) IsBetaUser() bool {
	return e.Plan() == "beta"
}

// CanAccessRepo checks private-repos and org scope entitlements.
// Returns an EntitlementError for hard-blocks.
func (e *BopEntitlements) CanAccessRepo(isPrivate bool, ownerType string) error {
	// Public repos always allowed
	if !isPrivate {
		return nil
	}

	// Private repo requires entitlement
	if !e.HasEntitlement(EntitlementPrivateRepos) {
		return &EntitlementError{
			Entitlement: EntitlementPrivateRepos,
			Message:     "Private repository access requires Solo plan or higher",
			UpgradeURL:  "https://bop.dev/upgrade?feature=private-repos",
		}
	}

	// Check org scope: require any-org entitlement for organization repositories
	if ownerType == "Organization" && !e.HasEntitlement(EntitlementAnyOrg) {
		return &EntitlementError{
			Entitlement: EntitlementAnyOrg,
			Message:     "Using bop in organization repositories requires Pro plan",
			UpgradeURL:  "https://bop.dev/upgrade?feature=any-org",
		}
	}

	return nil
}

// ResolveModel applies model-selection entitlement with graceful fallback.
// Returns the model to use (may be different from requested if fallback applied).
func (e *BopEntitlements) ResolveModel(requested string) string {
	// Empty request uses default
	if requested == "" {
		return DefaultModel
	}

	// Ollama models always allowed
	if isOllamaModel(requested) {
		return requested
	}

	// Model selection entitlement required for non-default
	if !e.HasEntitlement(EntitlementModelSelection) && requested != DefaultModel {
		e.notifyFallback(EntitlementModelSelection,
			fmt.Sprintf("Model '%s' requires model-selection entitlement. Using '%s' instead. "+
				"Upgrade to Pro or add model-selection ($10/mo) to unlock all models.",
				requested, DefaultModel))
		return DefaultModel
	}

	return requested
}

// CanCustomizeReviewerPanel checks if reviewer panel customization is allowed.
// This is a graceful fallback - returns whether customization is allowed.
func (e *BopEntitlements) CanCustomizeReviewerPanel() bool {
	return e.HasEntitlement(EntitlementCustomizeReviewerPanel)
}

// ResolveReviewerPanel applies reviewer panel customization with graceful fallback.
// Returns the reviewer ID to use (may be default if fallback applied).
func (e *BopEntitlements) ResolveReviewerPanel(requestedReviewerID, defaultReviewerID string) string {
	if requestedReviewerID == "" || requestedReviewerID == defaultReviewerID {
		return defaultReviewerID
	}

	if !e.HasEntitlement(EntitlementCustomizeReviewerPanel) {
		e.notifyFallback(EntitlementCustomizeReviewerPanel,
			fmt.Sprintf("Reviewer '%s' requires customize-reviewer-panel entitlement. "+
				"Using default reviewer. Upgrade to Pro or add this feature ($10/mo).",
				requestedReviewerID))
		return defaultReviewerID
	}

	return requestedReviewerID
}

// CanCustomizeReviewerPrompt checks if custom reviewer prompts are allowed.
// This is Enterprise-only with graceful fallback.
func (e *BopEntitlements) CanCustomizeReviewerPrompt() bool {
	return e.HasEntitlement(EntitlementCustomizeReviewerPrompt)
}

// ResolveReviewerPrompt applies custom prompt entitlement with graceful fallback.
// Returns true if custom prompt can be used, false if predefined should be used.
func (e *BopEntitlements) ResolveReviewerPrompt(hasCustomPrompt bool) bool {
	if !hasCustomPrompt {
		return false
	}

	if !e.HasEntitlement(EntitlementCustomizeReviewerPrompt) {
		e.notifyFallback(EntitlementCustomizeReviewerPrompt,
			"Custom reviewer prompts require Enterprise plan. Using predefined prompt.")
		return false
	}

	return true
}

// CanUseBYOK checks if BYOK (bring your own keys) is allowed.
func (e *BopEntitlements) CanUseBYOK() bool {
	return e.HasEntitlement(EntitlementBYOProviderKeys)
}

// RequireBYOK checks if BYOK is required (beta tier) and configured.
// Returns an error if beta user hasn't configured API keys.
func (e *BopEntitlements) RequireBYOK(hasConfiguredKeys bool) error {
	// Beta users must have BYOK configured
	if e.IsBetaUser() && !hasConfiguredKeys {
		return &EntitlementError{
			Entitlement: EntitlementBYOProviderKeys,
			Message:     "Beta tier requires you to configure your own API keys. Visit bop.dev/settings to add your OpenAI, Anthropic, or other provider keys.",
			UpgradeURL:  "https://bop.dev/settings",
		}
	}
	return nil
}

// CanUseLocalConfig checks local-bop-config entitlement (hard block).
// Returns an error if local config is not allowed.
func (e *BopEntitlements) CanUseLocalConfig() error {
	if !e.HasEntitlement(EntitlementLocalBopConfig) {
		return &EntitlementError{
			Entitlement: EntitlementLocalBopConfig,
			Message:     "Local configuration requires Enterprise plan",
			UpgradeURL:  "https://bop.dev/contact-sales",
		}
	}
	return nil
}

// Helper functions

// isOllamaModel checks if a model string refers to an Ollama model.
func isOllamaModel(model string) bool {
	return strings.HasPrefix(model, "ollama/") ||
		strings.Contains(model, ":") // Ollama format: model:tag
}

// Legacy compatibility aliases (to be removed after migration)

// EntitlementChecker is an alias for BopEntitlements for backward compatibility.
type EntitlementChecker = BopEntitlements

// NewEntitlementChecker creates a BopEntitlements (legacy alias).
func NewEntitlementChecker(auth *StoredAuth) *EntitlementChecker {
	return NewBopEntitlements(auth, nil)
}

// CanReviewCode always returns true (basic access is assumed for authenticated users).
// This replaces the old code-review entitlement check.
func (e *BopEntitlements) CanReviewCode() bool {
	// Basic code review is always allowed for authenticated users
	// The old "code-review" entitlement is removed per spec
	return e.IsLoggedIn() || e.HasEntitlement(EntitlementPublicRepos)
}

// CanReviewPrivateRepos checks if the user can review private repositories.
func (e *BopEntitlements) CanReviewPrivateRepos() bool {
	return e.HasEntitlement(EntitlementPrivateRepos)
}

// HasUnlimitedReviews returns true (usage tracking replaces this in new spec).
func (e *BopEntitlements) HasUnlimitedReviews() bool {
	// Usage limits are now tracked via usage events, not entitlements
	return true
}

// IsFreeUser returns true if the user is on the free plan.
func (e *BopEntitlements) IsFreeUser() bool {
	if e.auth == nil {
		return true
	}
	plan := e.auth.Plan
	return plan == "" || plan == "free"
}
