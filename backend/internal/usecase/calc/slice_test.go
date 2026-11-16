package calc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestSameCollection(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC)
	later := at.Add(time.Hour)
	moscow := time.FixedZone("MSK", 3*3600)
	base := func() entity.AreaCollection {
		finished := at
		return entity.AreaCollection{
			AreaID: idOf("area"), ConnectionID: idOf("conn"), Completeness: "complete", Phase: entity.SyncIncremental,
			Watermark: &at, LastSuccessAt: &at, Reconcile: &entity.ReconcileRun{ID: idOf("run"), Outcome: entity.ReconcileComplete, FinishedAt: &finished},
		}
	}
	tests := []struct {
		name   string
		change func(c *entity.AreaCollection)
		want   bool
	}{
		{name: "same", change: func(*entity.AreaCollection) {}, want: true},
		{name: "same moment in another zone", change: func(c *entity.AreaCollection) { w := at.In(moscow); c.Watermark = &w }, want: true},
		{name: "completeness", change: func(c *entity.AreaCollection) { c.Completeness = "partial" }},
		{name: "phase", change: func(c *entity.AreaCollection) { c.Phase = entity.SyncBackfill }},
		{name: "watermark", change: func(c *entity.AreaCollection) { c.Watermark = &later }},
		{name: "no last success", change: func(c *entity.AreaCollection) { c.LastSuccessAt = nil }},
		{name: "another reconciliation", change: func(c *entity.AreaCollection) { c.Reconcile = &entity.ReconcileRun{ID: idOf("next")} }},
		{name: "no reconciliation", change: func(c *entity.AreaCollection) { c.Reconcile = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			changed := base()
			tt.change(&changed)
			assert.Equal(t, tt.want, sameCollection([]entity.AreaCollection{base()}, []entity.AreaCollection{changed}))
		})
	}
	assert.False(t, sameCollection([]entity.AreaCollection{base()}, nil), "an area that stopped being collected")
}
