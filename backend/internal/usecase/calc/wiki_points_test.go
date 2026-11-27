package calc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestWikiD27(t *testing.T) {
	t.Parallel()
	first := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	last := first.AddDate(1, 0, 0)
	for _, tt := range []struct {
		name   string
		result entity.WikiCollection
		state  entity.DataState
		ratio  *float64
	}{
		{"complete", entity.WikiCollection{RunID: entity.ID{1}, Expected: 4, Read: 4, LastSuccess: &last}, entity.StateAssessed, new(1.0)},
		{"unknown ACL counts as unread", entity.WikiCollection{RunID: entity.ID{2}, Expected: 4, Read: 3, LastSuccess: &last}, entity.StateProvisional, new(0.75)},
		{"failed attempt preserves last success", entity.WikiCollection{RunID: entity.ID{4}, Failed: true, LastSuccess: &last}, entity.StateUnknown, nil},
		{"no reconciliation", entity.WikiCollection{}, entity.StateUnknown, nil},
		{"empty manifest", entity.WikiCollection{RunID: entity.ID{3}, LastSuccess: &last}, entity.StateNotApplicable, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := tt.result
			result.EarliestVersion = &first
			points := wikiPoints(period{start: last, end: last.AddDate(0, 0, 1)}, result)
			require.Len(t, points, 3)
			assert.Equal(t, tt.state, points[0].State)
			assert.Equal(t, tt.ratio, points[0].Value)
			if result.LastSuccess == nil {
				assert.Nil(t, points[1].Value)
			} else {
				assert.Equal(t, new(float64(last.Unix())), points[1].Value)
			}
			assert.Equal(t, new(float64(first.Unix())), points[2].Value)
		})
	}
}
