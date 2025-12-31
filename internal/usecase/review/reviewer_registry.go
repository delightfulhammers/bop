package review

import (
	"errors"
	"fmt"
	"sort"

	"github.com/bkyoung/code-reviewer/internal/config"
	"github.com/bkyoung/code-reviewer/internal/domain"
)

// Error definitions for ReviewerRegistry.
var (
	ErrReviewerNotFound      = errors.New("reviewer not found")
	ErrNoReviewersConfigured = errors.New("no reviewers configured")
	ErrNoEnabledReviewers    = errors.New("no enabled reviewers found")
)

// ReviewerRegistry provides access to configured code reviewers.
// It converts config.ReviewerConfig to domain.Reviewer and provides
// lookup, listing, and resolution operations.
type ReviewerRegistry interface {
	// Get returns a reviewer by name.
	Get(name string) (domain.Reviewer, error)

	// List returns all configured reviewers (enabled and disabled).
	List() []domain.Reviewer

	// ListEnabled returns only enabled reviewers.
	ListEnabled() []domain.Reviewer

	// Resolve resolves reviewer names to reviewer instances.
	// If names is empty, uses default reviewers from config.
	// Returns only enabled reviewers (disabled ones are skipped).
	Resolve(names []string) ([]domain.Reviewer, error)
}

// reviewerRegistry is the concrete implementation of ReviewerRegistry.
type reviewerRegistry struct {
	reviewers        map[string]domain.Reviewer
	defaultReviewers []string
}

// NewReviewerRegistry creates a new reviewer registry from configuration.
// Returns (nil, nil) if no reviewers are configured, indicating the caller
// should use provider-based dispatch instead.
// Returns an error if:
// - config is nil
// - any reviewer fails validation
// - any default reviewer name is not found
func NewReviewerRegistry(cfg *config.Config) (ReviewerRegistry, error) {
	if cfg == nil {
		return nil, errors.New("config cannot be nil")
	}

	// No reviewers configured = use provider-based dispatch
	if len(cfg.Reviewers) == 0 {
		return nil, nil
	}

	reviewers := make(map[string]domain.Reviewer, len(cfg.Reviewers))

	for name, reviewerCfg := range cfg.Reviewers {
		// Resolve model fallback: reviewer.Model -> provider.DefaultModel
		resolvedModel := reviewerCfg.Model
		if resolvedModel == "" {
			if providerCfg, ok := cfg.Providers[reviewerCfg.Provider]; ok {
				resolvedModel = providerCfg.GetDefaultModel()
			}
		}

		reviewer := configToReviewer(name, reviewerCfg, resolvedModel)

		if err := reviewer.Validate(); err != nil {
			return nil, fmt.Errorf("invalid reviewer %q: %w", name, err)
		}

		reviewers[name] = reviewer
	}

	// Validate default reviewers exist
	for _, name := range cfg.DefaultReviewers {
		if _, exists := reviewers[name]; !exists {
			return nil, fmt.Errorf("default reviewer %q not found in configured reviewers", name)
		}
	}

	return &reviewerRegistry{
		reviewers:        reviewers,
		defaultReviewers: cfg.DefaultReviewers,
	}, nil
}

// configToReviewer converts a config.ReviewerConfig to a domain.Reviewer.
// The resolvedModel parameter is the effective model to use, which may have
// been resolved from the provider's defaultModel if the reviewer's model was empty.
func configToReviewer(name string, cfg config.ReviewerConfig, resolvedModel string) domain.Reviewer {
	weight := cfg.Weight
	if weight == 0 {
		weight = 1.0 // Default weight
	}

	enabled := true
	if cfg.Enabled != nil {
		enabled = *cfg.Enabled
	}

	return domain.Reviewer{
		Name:     name,
		Provider: cfg.Provider,
		Model:    resolvedModel,
		Weight:   weight,
		Persona:  cfg.Persona,
		Focus:    cfg.Focus,
		Ignore:   cfg.Ignore,
		Enabled:  enabled,
	}
}

// Get returns a reviewer by name.
func (r *reviewerRegistry) Get(name string) (domain.Reviewer, error) {
	reviewer, exists := r.reviewers[name]
	if !exists {
		return domain.Reviewer{}, fmt.Errorf("%w: %q", ErrReviewerNotFound, name)
	}
	return reviewer, nil
}

// List returns all configured reviewers (enabled and disabled).
// Results are sorted by reviewer name for deterministic ordering.
func (r *reviewerRegistry) List() []domain.Reviewer {
	reviewers := make([]domain.Reviewer, 0, len(r.reviewers))
	for _, reviewer := range r.reviewers {
		reviewers = append(reviewers, reviewer)
	}
	sort.Slice(reviewers, func(i, j int) bool {
		return reviewers[i].Name < reviewers[j].Name
	})
	return reviewers
}

// ListEnabled returns only enabled reviewers.
// Results are sorted by reviewer name for deterministic ordering.
func (r *reviewerRegistry) ListEnabled() []domain.Reviewer {
	reviewers := make([]domain.Reviewer, 0)
	for _, reviewer := range r.reviewers {
		if reviewer.Enabled {
			reviewers = append(reviewers, reviewer)
		}
	}
	sort.Slice(reviewers, func(i, j int) bool {
		return reviewers[i].Name < reviewers[j].Name
	})
	return reviewers
}

// Resolve resolves reviewer names to reviewer instances.
// If names is empty, uses default reviewers from config.
// Returns only enabled reviewers (disabled ones are skipped).
func (r *reviewerRegistry) Resolve(names []string) ([]domain.Reviewer, error) {
	// Use defaults if no names provided
	if len(names) == 0 {
		names = r.defaultReviewers
	}

	// Handle case where both names and defaults are empty
	if len(names) == 0 {
		return nil, errors.New("no reviewers specified and no default reviewers configured")
	}

	reviewers := make([]domain.Reviewer, 0, len(names))

	for _, name := range names {
		reviewer, err := r.Get(name)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve reviewer %q: %w", name, err)
		}

		// Only include enabled reviewers
		if reviewer.Enabled {
			reviewers = append(reviewers, reviewer)
		}
	}

	if len(reviewers) == 0 {
		return nil, ErrNoEnabledReviewers
	}

	return reviewers, nil
}
