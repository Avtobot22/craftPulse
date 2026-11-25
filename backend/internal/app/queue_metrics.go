package app

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_queueSampleInterval = 15 * time.Second
	_queueQueryTimeout   = 3 * time.Second
	_queueStatsSQL       = `
SELECT queues.name,
       count(*) FILTER (WHERE jobs.state = 'available'),
       coalesce(extract(epoch FROM statement_timestamp()
           - min(jobs.scheduled_at) FILTER (WHERE jobs.state = 'available')), 0)::double precision,
       count(*) FILTER (WHERE jobs.state = 'discarded')
FROM unnest($1::text[]) AS queues(name)
LEFT JOIN public.river_job AS jobs ON jobs.queue = queues.name
    AND (jobs.state = 'discarded' OR (jobs.state = 'available' AND jobs.scheduled_at <= statement_timestamp()))
GROUP BY queues.name
`
)

type queueStat struct {
	queue     string
	available int64
	oldest    float64
	discarded int64
}

type queueSnapshot struct {
	stats []queueStat
}

type queueMetrics struct {
	snapshot atomic.Pointer[queueSnapshot]
	up       *prometheus.Desc
	depth    *prometheus.Desc
	age      *prometheus.Desc
	discard  *prometheus.Desc
}

func newQueueMetrics(reg prometheus.Registerer) *queueMetrics {
	m := &queueMetrics{
		up: prometheus.NewDesc("craftpulse_river_query_up", "Whether the last River queue sample succeeded.", nil, nil),
		depth: prometheus.NewDesc("craftpulse_river_available_jobs", "Runnable jobs waiting in the River queue.",
			[]string{"queue"}, nil),
		age: prometheus.NewDesc("craftpulse_river_oldest_available_job_age_seconds",
			"Time since the oldest runnable job became due, or zero for an empty queue.", []string{"queue"}, nil),
		discard: prometheus.NewDesc("craftpulse_river_discarded_jobs", "Discarded jobs still stored by River.",
			[]string{"queue"}, nil),
	}
	reg.MustRegister(m)
	return m
}

func (m *queueMetrics) Describe(out chan<- *prometheus.Desc) {
	out <- m.up
	out <- m.depth
	out <- m.age
	out <- m.discard
}

func (m *queueMetrics) Collect(out chan<- prometheus.Metric) {
	snapshot := m.snapshot.Load()
	if snapshot == nil {
		out <- prometheus.MustNewConstMetric(m.up, prometheus.GaugeValue, 0)
		return
	}
	out <- prometheus.MustNewConstMetric(m.up, prometheus.GaugeValue, 1)
	for _, s := range snapshot.stats {
		out <- prometheus.MustNewConstMetric(m.depth, prometheus.GaugeValue, float64(s.available), s.queue)
		out <- prometheus.MustNewConstMetric(m.age, prometheus.GaugeValue, s.oldest, s.queue)
		out <- prometheus.MustNewConstMetric(m.discard, prometheus.GaugeValue, float64(s.discarded), s.queue)
	}
}

func (m *queueMetrics) run(ctx context.Context, pool *pgxpool.Pool, log zerolog.Logger) error {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		snapshot, err := readQueueStats(ctx, pool)
		if ctx.Err() != nil {
			return nil
		}
		m.snapshot.Store(snapshot)
		if err != nil {
			log.Warn().Err(err).Msg("sample River queue metrics")
		}
		timer.Reset(_queueSampleInterval)
	}
}

func readQueueStats(ctx context.Context, pool *pgxpool.Pool) (*queueSnapshot, error) {
	queryCtx, cancel := context.WithTimeout(ctx, _queueQueryTimeout)
	defer cancel()

	queues := []string{entity.QueueInteractive, entity.QueueBulk}
	rows, err := pool.Query(queryCtx, _queueStatsSQL, queues)
	if err != nil {
		return nil, err
	}
	stats, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (queueStat, error) {
		var s queueStat
		err := row.Scan(&s.queue, &s.available, &s.oldest, &s.discarded)
		return s, err
	})
	if err != nil {
		return nil, err
	}
	return &queueSnapshot{stats: stats}, nil
}
