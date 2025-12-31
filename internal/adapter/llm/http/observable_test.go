package http

import (
	"testing"
)

// mockObservable tracks which setter methods were called.
type mockObservable struct {
	loggerSet  bool
	metricsSet bool
	pricingSet bool
	logger     Logger
	metrics    Metrics
	pricing    Pricing
}

func (m *mockObservable) SetLogger(logger Logger) {
	m.loggerSet = true
	m.logger = logger
}

func (m *mockObservable) SetMetrics(metrics Metrics) {
	m.metricsSet = true
	m.metrics = metrics
}

func (m *mockObservable) SetPricing(pricing Pricing) {
	m.pricingSet = true
	m.pricing = pricing
}

func TestWireObservability(t *testing.T) {
	t.Run("wires all components when all provided", func(t *testing.T) {
		obs := &mockObservable{}
		logger := NewDefaultLogger(LogLevelInfo, LogFormatHuman, true)
		metrics := NewDefaultMetrics()
		pricing := NewDefaultPricing()

		WireObservability(obs, logger, metrics, pricing)

		if !obs.loggerSet {
			t.Error("expected logger to be set")
		}
		if !obs.metricsSet {
			t.Error("expected metrics to be set")
		}
		if !obs.pricingSet {
			t.Error("expected pricing to be set")
		}
		if obs.logger != logger {
			t.Error("expected logger to match provided instance")
		}
		if obs.metrics != metrics {
			t.Error("expected metrics to match provided instance")
		}
		if obs.pricing != pricing {
			t.Error("expected pricing to match provided instance")
		}
	})

	t.Run("skips nil logger", func(t *testing.T) {
		obs := &mockObservable{}
		metrics := NewDefaultMetrics()
		pricing := NewDefaultPricing()

		WireObservability(obs, nil, metrics, pricing)

		if obs.loggerSet {
			t.Error("expected logger NOT to be set when nil")
		}
		if !obs.metricsSet {
			t.Error("expected metrics to be set")
		}
		if !obs.pricingSet {
			t.Error("expected pricing to be set")
		}
	})

	t.Run("skips nil metrics", func(t *testing.T) {
		obs := &mockObservable{}
		logger := NewDefaultLogger(LogLevelInfo, LogFormatHuman, true)
		pricing := NewDefaultPricing()

		WireObservability(obs, logger, nil, pricing)

		if !obs.loggerSet {
			t.Error("expected logger to be set")
		}
		if obs.metricsSet {
			t.Error("expected metrics NOT to be set when nil")
		}
		if !obs.pricingSet {
			t.Error("expected pricing to be set")
		}
	})

	t.Run("skips nil pricing", func(t *testing.T) {
		obs := &mockObservable{}
		logger := NewDefaultLogger(LogLevelInfo, LogFormatHuman, true)
		metrics := NewDefaultMetrics()

		WireObservability(obs, logger, metrics, nil)

		if !obs.loggerSet {
			t.Error("expected logger to be set")
		}
		if !obs.metricsSet {
			t.Error("expected metrics to be set")
		}
		if obs.pricingSet {
			t.Error("expected pricing NOT to be set when nil")
		}
	})

	t.Run("handles all nil components", func(t *testing.T) {
		obs := &mockObservable{}

		WireObservability(obs, nil, nil, nil)

		if obs.loggerSet {
			t.Error("expected logger NOT to be set")
		}
		if obs.metricsSet {
			t.Error("expected metrics NOT to be set")
		}
		if obs.pricingSet {
			t.Error("expected pricing NOT to be set")
		}
	})

	t.Run("handles nil Observable without panic", func(t *testing.T) {
		logger := NewDefaultLogger(LogLevelInfo, LogFormatHuman, true)
		metrics := NewDefaultMetrics()
		pricing := NewDefaultPricing()

		// Should not panic when Observable is nil
		WireObservability(nil, logger, metrics, pricing)
	})
}
