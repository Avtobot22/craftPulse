package app

import (
	"math/rand/v2"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	syncrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/sync"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/sync"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

type syncMetrics struct {
	failed *prometheus.CounterVec
}

func (m syncMetrics) CollectionFailed(class string) { m.failed.WithLabelValues(class).Inc() }

// newSync collects with the client of connect and the intake of ingest; its errors are counted by class:
// auth for 401/403, rate_limited, unavailable, timeout, adapter_unavailable (TZ 14.7).
func newSync(pool *pgxpool.Pool, connect sync.Connect, intake sync.Ingest, c clock.Clock, reg prometheus.Registerer) (*sync.UseCase, error) {
	failed := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "craftpulse_sync_errors_total",
		Help: "Failed pages of collection by class of the failure.",
	}, []string{"class"})
	if err := reg.Register(failed); err != nil {
		return nil, err
	}
	jobs, err := riverInsertClient(pool)
	if err != nil {
		return nil, err
	}
	return sync.New(sync.Deps{
		Repo: syncrepo.NewRepo(pool), Connect: connect, Ingest: intake, Queue: queue.New(jobs), Tx: persistent.NewTransactor(pool),
		Metrics: syncMetrics{failed: failed}, Clock: c, Rand: rand.Float64,
	}), nil
}
