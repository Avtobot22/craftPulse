package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueueMetrics(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	pool := db.pool(t, "craftpulse_app")
	_, err := pool.Exec(t.Context(), `
INSERT INTO public.river_job (kind, queue, state, scheduled_at, finalized_at, args)
VALUES ('sample', 'interactive', 'available', now() - interval '3 minutes', NULL, '{}'),
       ('sample', 'interactive', 'available', now() - interval '1 minute', NULL, '{}'),
       ('sample', 'interactive', 'available', now() + interval '1 hour', NULL, '{}'),
       ('sample', 'interactive', 'scheduled', now() - interval '5 minutes', NULL, '{}'),
       ('sample', 'bulk', 'discarded', now() - interval '1 day', now(), '{}'),
       ('sample', 'other', 'available', now() - interval '1 day', NULL, '{}')
`)
	require.NoError(t, err)

	snapshot, err := readQueueStats(t.Context(), pool)
	require.NoError(t, err)
	require.Len(t, snapshot.stats, 2)
	for _, s := range snapshot.stats {
		if s.queue == "interactive" {
			assert.GreaterOrEqual(t, s.oldest, 180.0)
			assert.Less(t, s.oldest, 190.0)
		} else {
			assert.Zero(t, s.oldest)
		}
	}

	reg := prometheus.NewRegistry()
	metrics := newQueueMetrics(reg)
	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	metrics.snapshot.Store(snapshot)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", http.NoBody))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `craftpulse_river_available_jobs{queue="bulk"} 0`)
	assert.Contains(t, rec.Body.String(), `craftpulse_river_available_jobs{queue="interactive"} 2`)
	assert.Contains(t, rec.Body.String(), `craftpulse_river_discarded_jobs{queue="bulk"} 1`)
	assert.Contains(t, rec.Body.String(), `craftpulse_river_discarded_jobs{queue="interactive"} 0`)
	assert.Contains(t, rec.Body.String(), "craftpulse_river_query_up 1")

	metrics.snapshot.Store(nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", http.NoBody))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "craftpulse_river_query_up 0")
	assert.NotContains(t, rec.Body.String(), "craftpulse_river_available_jobs")
}
