package app

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	outboundrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/outbound"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/outbound"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

func newPublisher(pool *pgxpool.Pool, changes outbound.Changes, adapters outbound.Adapter, site string,
	reg prometheus.Registerer,
) (*outbound.UseCase, error) {
	latency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "craftpulse_outbound_report_to_publish_seconds",
		Help:    "Time from the last required report to successful MR comment publication.",
		Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
	})
	outcomes := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "craftpulse_outbound_publication_outcomes_total",
		Help: "Persisted outcomes of external publication attempts.",
	}, []string{"kind", "status"})
	if err := reg.Register(latency); err != nil {
		return nil, err
	}
	if err := reg.Register(outcomes); err != nil {
		return nil, err
	}
	tx := persistent.NewTransactor(pool)
	c := clock.System{}
	return outbound.New(outbound.Deps{
		Changes: changes, Adapter: adapters, Store: outboundrepo.NewRepo(pool),
		Audit: audit.New(auditrepo.NewRepo(pool), tx, c), Tx: tx, Clock: c, SiteURL: site,
		Observe: func(kind, status string, d *time.Duration) {
			outcomes.WithLabelValues(kind, status).Inc()
			if d != nil {
				latency.Observe(d.Seconds())
			}
		},
	}), nil
}
