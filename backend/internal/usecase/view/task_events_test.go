package view

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestTaskEventsCursorOrdersTiesAndExcludesFutureEvents(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC)
	cut := taskCut{asOf: at, seq: 42}
	one, two, older := entity.ID{1}, entity.ID{2}, entity.ID{3}
	events := []entity.TaskHistoryEvent{
		{ID: two, EventTime: at},
		{ID: older, EventTime: at.Add(-time.Minute)},
		{ID: one, EventTime: at},
		{ID: entity.ID{4}, EventTime: at.Add(time.Minute)},
	}
	page := entity.TaskEventPage{Items: append([]entity.TaskHistoryEvent(nil), events...)}
	selectTaskEvents(&page, cut, entity.TaskEventQuery{Limit: 1})
	require.Len(t, page.Items, 1)
	require.Equal(t, one, page.Items[0].ID)
	require.NotNil(t, page.Next)
	require.Equal(t, cut.seq, page.Next.Knowledge)

	next := entity.TaskEventPage{Items: events}
	selectTaskEvents(&next, cut, entity.TaskEventQuery{Limit: 2, After: page.Next})
	require.Len(t, next.Items, 2)
	require.Equal(t, two, next.Items[0].ID)
	require.Equal(t, older, next.Items[1].ID)
	require.Nil(t, next.Next)
}
