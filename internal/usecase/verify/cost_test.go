package verify_test

import (
	"math"
	"sync"
	"testing"

	"github.com/delightfulhammers/bop/internal/usecase/verify"
)

// floatEquals checks if two floats are approximately equal.
func floatEquals(a, b float64) bool {
	const epsilon = 1e-9
	return math.Abs(a-b) < epsilon
}

// mockCostTracker implements verify.CostTracker for testing.
type mockCostTracker struct {
	total   float64
	ceiling float64
}

func (m *mockCostTracker) AddCost(amount float64) {
	m.total += amount
}

func (m *mockCostTracker) TotalCost() float64 {
	return m.total
}

func (m *mockCostTracker) ExceedsCeiling() bool {
	return m.total >= m.ceiling
}

func (m *mockCostTracker) RemainingBudget() float64 {
	if m.total >= m.ceiling {
		return 0
	}
	return m.ceiling - m.total
}

// Compile-time check that mockCostTracker implements CostTracker.
var _ verify.CostTracker = (*mockCostTracker)(nil)

func TestCostTracker_Interface(t *testing.T) {
	t.Run("tracks accumulated cost", func(t *testing.T) {
		tracker := &mockCostTracker{ceiling: 1.0}

		tracker.AddCost(0.25)
		if tracker.TotalCost() != 0.25 {
			t.Errorf("got total %f, want 0.25", tracker.TotalCost())
		}

		tracker.AddCost(0.15)
		if tracker.TotalCost() != 0.40 {
			t.Errorf("got total %f, want 0.40", tracker.TotalCost())
		}
	})

	t.Run("detects ceiling exceeded", func(t *testing.T) {
		tracker := &mockCostTracker{ceiling: 0.50}

		if tracker.ExceedsCeiling() {
			t.Error("should not exceed ceiling initially")
		}

		tracker.AddCost(0.50)
		if !tracker.ExceedsCeiling() {
			t.Error("should exceed ceiling at 0.50")
		}

		tracker.AddCost(0.01)
		if !tracker.ExceedsCeiling() {
			t.Error("should exceed ceiling at 0.51")
		}
	})

	t.Run("calculates remaining budget", func(t *testing.T) {
		tracker := &mockCostTracker{ceiling: 1.0}

		if tracker.RemainingBudget() != 1.0 {
			t.Errorf("got remaining %f, want 1.0", tracker.RemainingBudget())
		}

		tracker.AddCost(0.30)
		if tracker.RemainingBudget() != 0.70 {
			t.Errorf("got remaining %f, want 0.70", tracker.RemainingBudget())
		}

		tracker.AddCost(0.80) // Exceeds ceiling
		if tracker.RemainingBudget() != 0 {
			t.Errorf("got remaining %f, want 0 (exceeded ceiling)", tracker.RemainingBudget())
		}
	})
}

func TestNewCostTracker(t *testing.T) {
	t.Run("creates tracker with ceiling", func(t *testing.T) {
		tracker := verify.NewCostTracker(0.50)

		if tracker.TotalCost() != 0 {
			t.Errorf("got initial total %f, want 0", tracker.TotalCost())
		}
		if tracker.RemainingBudget() != 0.50 {
			t.Errorf("got initial remaining %f, want 0.50", tracker.RemainingBudget())
		}
	})

	t.Run("tracks cost accurately", func(t *testing.T) {
		tracker := verify.NewCostTracker(1.00)

		tracker.AddCost(0.10)
		tracker.AddCost(0.20)
		tracker.AddCost(0.30)

		if !floatEquals(tracker.TotalCost(), 0.60) {
			t.Errorf("got total %f, want 0.60", tracker.TotalCost())
		}
		if !floatEquals(tracker.RemainingBudget(), 0.40) {
			t.Errorf("got remaining %f, want 0.40", tracker.RemainingBudget())
		}
	})

	t.Run("exceeds ceiling correctly", func(t *testing.T) {
		tracker := verify.NewCostTracker(0.25)

		if tracker.ExceedsCeiling() {
			t.Error("should not exceed ceiling initially")
		}

		tracker.AddCost(0.20)
		if tracker.ExceedsCeiling() {
			t.Error("should not exceed ceiling at 0.20")
		}

		tracker.AddCost(0.05)
		if !tracker.ExceedsCeiling() {
			t.Error("should exceed ceiling at 0.25")
		}
	})

	t.Run("is safe for concurrent access", func(t *testing.T) {
		tracker := verify.NewCostTracker(100.0)

		var wg sync.WaitGroup
		numGoroutines := 100
		costPerGoroutine := 0.5

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tracker.AddCost(costPerGoroutine)
				_ = tracker.TotalCost()
				_ = tracker.ExceedsCeiling()
				_ = tracker.RemainingBudget()
			}()
		}

		wg.Wait()

		expectedTotal := float64(numGoroutines) * costPerGoroutine
		if !floatEquals(tracker.TotalCost(), expectedTotal) {
			t.Errorf("got total %f, want %f (race condition?)", tracker.TotalCost(), expectedTotal)
		}
	})
}
