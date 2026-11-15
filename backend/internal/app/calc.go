package app

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	calcrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/calc"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

type calcMetrics struct {
	failed prometheus.Counter
	reused prometheus.Counter
}

func newCalcMetrics(reg prometheus.Registerer) (calcMetrics, error) {
	m := calcMetrics{
		failed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "craftpulse_calc_revisions_failed_total",
			Help: "Revisions of series that failed; the current revision of the series stays (TZ 10.7).",
		}),
		reused: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "craftpulse_calc_parts_reused_total",
			Help: "Parts a revision took from an earlier one by the hash of their input.",
		}),
	}
	for _, c := range []prometheus.Collector{m.failed, m.reused} {
		if err := reg.Register(c); err != nil {
			return calcMetrics{}, fmt.Errorf("app: register calc metric: %w", err)
		}
	}
	return m, nil
}

func (m calcMetrics) RevisionFailed() { m.failed.Inc() }

func (m calcMetrics) PartReused() { m.reused.Inc() }

// calcModules are the modules calc reads (TZ 10.3): the projects of teams from org, the facts from normalize, the
// settings and the confirmation of a target from process, the state of collection from sync and the rights of a
// reader from access. None of them depends on calc.
type calcModules struct {
	structure  calc.Structure
	facts      calc.Facts
	settings   calc.Settings
	collection calc.Collection
	rights     calc.Rights
}

func newCalc(pool *pgxpool.Pool, m calcModules, c clock.Clock, reg prometheus.Registerer) (*calc.UseCase, error) {
	metrics, err := newCalcMetrics(reg)
	if err != nil {
		return nil, err
	}
	jobs, err := riverInsertClient(pool)
	if err != nil {
		return nil, err
	}
	return calc.New(calc.Deps{
		Structure: m.structure, Facts: m.facts, Settings: m.settings, Collection: m.collection, Rights: m.rights,
		Repo: calcrepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Queue: queue.New(jobs), Tx: persistent.NewTransactor(pool),
		Metrics: metrics, Clock: c, NewID: newID,
	}), nil
}
