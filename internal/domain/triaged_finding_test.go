package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bkyoung/code-reviewer/internal/domain"
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
