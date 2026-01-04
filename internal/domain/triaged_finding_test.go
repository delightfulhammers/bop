package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/delightfulhammers/bop/internal/domain"
)

func TestTriagedFindingContext_HasFindings(t *testing.T) {
	t.Run("returns false for nil findings", func(t *testing.T) {
		ctx := domain.TriagedFindingContext{}
		assert.False(t, ctx.HasFindings())
	})

	t.Run("returns false for empty findings", func(t *testing.T) {
		ctx := domain.TriagedFindingContext{
			PRNumber: 123,
			Findings: []domain.TriagedFinding{},
		}
		assert.False(t, ctx.HasFindings())
	})

	t.Run("returns true when findings exist", func(t *testing.T) {
		ctx := domain.TriagedFindingContext{
			PRNumber: 123,
			Findings: []domain.TriagedFinding{
				{File: "test.go", Status: domain.StatusAcknowledged},
			},
		}
		assert.True(t, ctx.HasFindings())
	})
}

func TestTriagedFindingContext_AcknowledgedFindings(t *testing.T) {
	ctx := domain.TriagedFindingContext{
		PRNumber: 123,
		Findings: []domain.TriagedFinding{
			{File: "a.go", Status: domain.StatusAcknowledged},
			{File: "b.go", Status: domain.StatusDisputed},
			{File: "c.go", Status: domain.StatusAcknowledged},
			{File: "d.go", Status: domain.StatusOpen},
		},
	}

	acknowledged := ctx.AcknowledgedFindings()
	assert.Len(t, acknowledged, 2)
	assert.Equal(t, "a.go", acknowledged[0].File)
	assert.Equal(t, "c.go", acknowledged[1].File)
}

func TestTriagedFindingContext_DisputedFindings(t *testing.T) {
	ctx := domain.TriagedFindingContext{
		PRNumber: 123,
		Findings: []domain.TriagedFinding{
			{File: "a.go", Status: domain.StatusAcknowledged},
			{File: "b.go", Status: domain.StatusDisputed},
			{File: "c.go", Status: domain.StatusDisputed},
			{File: "d.go", Status: domain.StatusOpen},
		},
	}

	disputed := ctx.DisputedFindings()
	assert.Len(t, disputed, 2)
	assert.Equal(t, "b.go", disputed[0].File)
	assert.Equal(t, "c.go", disputed[1].File)
}

func TestStatusReasons(t *testing.T) {
	t.Run("acknowledged reason is non-empty", func(t *testing.T) {
		reason := domain.StatusReasonForAcknowledged()
		assert.NotEmpty(t, reason)
		assert.Contains(t, reason, "acknowledged")
	})

	t.Run("disputed reason is non-empty", func(t *testing.T) {
		reason := domain.StatusReasonForDisputed()
		assert.NotEmpty(t, reason)
		assert.Contains(t, reason, "disputed")
	})
}

func TestTriagedFindingContext_FilterByReviewer(t *testing.T) {
	t.Run("returns nil for nil context", func(t *testing.T) {
		var ctx *domain.TriagedFindingContext
		result := ctx.FilterByReviewer("security")
		assert.Nil(t, result)
	})

	t.Run("returns nil for empty findings", func(t *testing.T) {
		ctx := &domain.TriagedFindingContext{
			PRNumber: 123,
			Findings: []domain.TriagedFinding{},
		}
		result := ctx.FilterByReviewer("security")
		assert.Nil(t, result)
	})

	t.Run("returns nil when no matching reviewer", func(t *testing.T) {
		ctx := &domain.TriagedFindingContext{
			PRNumber: 123,
			Findings: []domain.TriagedFinding{
				{File: "a.go", ReviewerName: "maintainability"},
				{File: "b.go", ReviewerName: "performance"},
			},
		}
		result := ctx.FilterByReviewer("security")
		assert.Nil(t, result)
	})

	t.Run("filters findings by reviewer name", func(t *testing.T) {
		ctx := &domain.TriagedFindingContext{
			PRNumber: 123,
			Findings: []domain.TriagedFinding{
				{File: "a.go", ReviewerName: "security", Category: "authentication"},
				{File: "b.go", ReviewerName: "maintainability", Category: "complexity"},
				{File: "c.go", ReviewerName: "security", Category: "sql_injection"},
				{File: "d.go", ReviewerName: "performance", Category: "n_plus_one"},
			},
		}

		result := ctx.FilterByReviewer("security")

		assert.NotNil(t, result)
		assert.Equal(t, 123, result.PRNumber)
		assert.Len(t, result.Findings, 2)
		assert.Equal(t, "a.go", result.Findings[0].File)
		assert.Equal(t, "authentication", result.Findings[0].Category)
		assert.Equal(t, "c.go", result.Findings[1].File)
		assert.Equal(t, "sql_injection", result.Findings[1].Category)
	})

	t.Run("preserves all finding fields", func(t *testing.T) {
		ctx := &domain.TriagedFindingContext{
			PRNumber: 456,
			Findings: []domain.TriagedFinding{
				{
					File:         "auth.go",
					LineStart:    10,
					LineEnd:      20,
					Category:     "security",
					Severity:     "high",
					Description:  "SQL injection vulnerability",
					Status:       domain.StatusAcknowledged,
					StatusReason: "Fixed in later commit",
					Fingerprint:  "abc123",
					ReviewerName: "security",
				},
			},
		}

		result := ctx.FilterByReviewer("security")

		assert.NotNil(t, result)
		assert.Len(t, result.Findings, 1)
		f := result.Findings[0]
		assert.Equal(t, "auth.go", f.File)
		assert.Equal(t, 10, f.LineStart)
		assert.Equal(t, 20, f.LineEnd)
		assert.Equal(t, "security", f.Category)
		assert.Equal(t, "high", f.Severity)
		assert.Equal(t, "SQL injection vulnerability", f.Description)
		assert.Equal(t, domain.StatusAcknowledged, f.Status)
		assert.Equal(t, "Fixed in later commit", f.StatusReason)
		assert.Equal(t, domain.FindingFingerprint("abc123"), f.Fingerprint)
		assert.Equal(t, "security", f.ReviewerName)
	})
}
