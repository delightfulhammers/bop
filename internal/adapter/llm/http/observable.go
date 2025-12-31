package http

// Observable defines the interface for types that accept observability components.
// All LLM HTTP clients implement this interface through their SetLogger, SetMetrics,
// and SetPricing methods.
//
// This interface enables centralized wiring of observability concerns via the
// WireObservability helper function, eliminating repetitive nil-check boilerplate.
type Observable interface {
	SetLogger(logger Logger)
	SetMetrics(metrics Metrics)
	SetPricing(pricing Pricing)
}

// WireObservability configures an Observable with logger, metrics, and pricing.
// Each component is only set if non-nil, allowing partial observability configuration.
// If obs is nil, this function is a no-op.
//
// Example usage:
//
//	client := anthropic.NewHTTPClient(apiKey, model, cfg, httpConfig)
//	llmhttp.WireObservability(client, logger, metrics, pricing)
func WireObservability(obs Observable, logger Logger, metrics Metrics, pricing Pricing) {
	if obs == nil {
		return
	}
	if logger != nil {
		obs.SetLogger(logger)
	}
	if metrics != nil {
		obs.SetMetrics(metrics)
	}
	if pricing != nil {
		obs.SetPricing(pricing)
	}
}
