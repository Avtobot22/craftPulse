package app

import (
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	linkrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/link"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	syncrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/sync"
	viewrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/view"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	linkuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/link"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/sync"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/view"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
	"github.com/Avtobot22/craftPulse/backend/pkg/crypto"
)

type syncMetrics struct {
	failed       *prometheus.CounterVec
	rejected     *prometheus.CounterVec
	sinceSuccess *prometheus.GaugeVec
	lag          *prometheus.GaugeVec
}

func newSyncMetrics(reg prometheus.Registerer) (syncMetrics, error) {
	m := syncMetrics{
		rejected: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "craftpulse_webhook_rejections_total", Help: "Rejected webhook deliveries by connection and reason."}, []string{"connection_id", "reason"}),
		failed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "craftpulse_sync_errors_total",
			Help: "Failed pages of collection by class of the failure.",
		}, []string{"class"}),
		sinceSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "craftpulse_sync_seconds_since_success",
			Help: "Seconds since the last reconciled collection of a data kind of a connection.",
		}, []string{"connection_id", "data_kind"}),
		lag: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "craftpulse_sync_lag_seconds",
			Help: "Seconds between now and the watermark of a data kind of a connection.",
		}, []string{"connection_id", "data_kind"}),
	}
	for _, c := range []prometheus.Collector{m.failed, m.rejected, m.sinceSuccess, m.lag} {
		if err := reg.Register(c); err != nil {
			return syncMetrics{}, err
		}
	}
	return m, nil
}

func (m syncMetrics) CollectionFailed(class string) { m.failed.WithLabelValues(class).Inc() }

func (m syncMetrics) Freshness(connectionID, dataKind string, sinceSuccess, lag *time.Duration) {
	set(m.sinceSuccess, sinceSuccess, connectionID, dataKind)
	set(m.lag, lag, connectionID, dataKind)
}

// set reports a duration, or removes the series while it is not known.
func set(g *prometheus.GaugeVec, d *time.Duration, labels ...string) {
	if d == nil {
		g.DeleteLabelValues(labels...)
		return
	}
	g.WithLabelValues(labels...).Set(d.Seconds())
}

// newSync collects with the client of connect and the intake of ingest and reconciles against normalize;
// its errors are counted by class: auth for 401/403, rate_limited, unavailable, timeout, adapter_unavailable
// (TZ 14.7).
func newSync(pool *pgxpool.Pool, keyring *crypto.Keyring, connect sync.Connect, intake sync.Ingest, normalize sync.Normalize, c clock.Clock,
	reg prometheus.Registerer,
) (*sync.UseCase, error) {
	metrics, err := newSyncMetrics(reg)
	if err != nil {
		return nil, err
	}
	jobs, err := riverInsertClient(pool)
	if err != nil {
		return nil, err
	}
	return sync.New(sync.Deps{
		Keyring: keyring, Repo: syncrepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Connect: connect, Ingest: intake, Normalize: normalize,
		Queue: queue.New(jobs), Tx: persistent.NewTransactor(pool), Metrics: metrics, Clock: c, Rand: rand.Float64,
	}), nil
}

// viewModules are the modules view reads: the runs of collection from sync, the series and the reports of calc,
// the source instances of connect and the rights of a reader from access.
type viewModules struct {
	collection view.Sync
	series     view.Series
	reports    view.Reports
	instances  view.Instances
	rights     interface {
		view.Authorizer
		view.SourceRights
	}
	code          view.CodeFacts
	actors        view.ActorAccounts
	runCalculator view.RunCalculator
}

// newView keeps the descriptors of the runs users ask for and reads the reports; the access module checks their
// action again.
func newView(pool *pgxpool.Pool, m viewModules, c clock.Clock) *view.UseCase {
	return view.New(view.Deps{
		Code: m.code, Actors: m.actors, RunCalculator: m.runCalculator, SourceRights: m.rights,
		CodeLinks: linkuc.New(linkuc.Deps{Repo: linkrepo.NewRepo(pool), Tx: persistent.NewTransactor(pool)}),
		Repo:      viewrepo.NewRepo(pool), Tx: persistent.NewTransactor(pool), Sync: m.collection, Series: m.series, Reports: m.reports,
		Instances: m.instances, Access: m.rights, Clock: c, NewID: newID,
	})
}

func (m syncMetrics) WebhookRejected(conn, reason string) {
	m.rejected.WithLabelValues(conn, reason).Inc()
}
