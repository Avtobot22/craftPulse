package view

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestWaitingForTaskHistory(t *testing.T) {
	t.Parallel()
	a, b := entity.ID{1}, entity.ID{2}
	stamp := time.Unix(1, 0)
	for _, tc := range []struct {
		name       string
		collection []entity.AreaCollection
		want       bool
	}{
		{name: "all areas await first sync", collection: []entity.AreaCollection{{AreaID: a}, {AreaID: b}}, want: true},
		{name: "mixed with an import-only area", collection: []entity.AreaCollection{{AreaID: a}}},
		{name: "initial sync finished", collection: []entity.AreaCollection{{AreaID: a, LastSuccessAt: &stamp}, {AreaID: b}}},
		{name: "history boundary exists", collection: []entity.AreaCollection{{AreaID: a}, {AreaID: b, HistoryFrom: &stamp}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, waitingForTaskHistory([]entity.ID{a, b}, tc.collection))
		})
	}
}
