package app

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	calcrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/calc"
	changerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/change"
	docsrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/docs"
	linkrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/link"
	normalizerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/normalize"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/change"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/docs"
	linkuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/link"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

type calcMetrics struct {
	failed   prometheus.Counter
	reused   prometheus.Counter
	computed prometheus.Counter
	chain    prometheus.Histogram
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
		computed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "craftpulse_calc_parts_computed_total",
			Help: "Parts a revision computed because no stored part had the hash of their input.",
		}),
		chain: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "craftpulse_calc_chain_duration_seconds",
			Help:    "Time from the first part of a chain of calc.slice to the publication of its revision.",
			Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600, 1800},
		}),
	}
	for _, c := range []prometheus.Collector{m.failed, m.reused, m.computed, m.chain} {
		if err := reg.Register(c); err != nil {
			return calcMetrics{}, fmt.Errorf("app: register calc metric: %w", err)
		}
	}
	return m, nil
}

func (m calcMetrics) RevisionFailed() { m.failed.Inc() }

func (m calcMetrics) PartReused() { m.reused.Inc() }

func (m calcMetrics) PartComputed() { m.computed.Inc() }

func (m calcMetrics) ChainPublished(d time.Duration) { m.chain.Observe(d.Seconds()) }

// calcModules are the modules calc reads (TZ 10.3): the projects of teams from org, the facts from normalize, the
// settings and the confirmation of a target from process, the state of collection from sync and the rights of a
// reader from access. None of them depends on calc.
type calcModules struct {
	catalog      entity.CheckCatalog
	structure    calc.Structure
	facts        calc.Facts
	testCoverage calc.TestCoverageFacts
	security     calc.SecurityFacts
	settings     calc.Settings
	collection   calc.Collection
	activity     calc.ActivityCollection
	rights       calc.Rights
	decisions    calc.FindingDecisions
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
	tx := persistent.NewTransactor(pool)
	// calc only reads links; their decisions stay with normalize (TZ 10.3).
	links := linkuc.New(linkuc.Deps{Repo: linkrepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Tx: tx, Clock: c, NewID: newID})
	code, _ := m.facts.(calc.CodeReports)
	codeSources, _ := m.facts.(calc.CodeSources)
	docReports, _ := m.facts.(calc.DocumentationReports)
	analysis, _ := m.facts.(calc.AnalysisReports)
	documents, _ := m.facts.(docs.Documents)
	documentation := docs.New(docs.Deps{Repo: docsrepo.NewRepo(pool), Tx: tx, Documents: documents})
	var evidence calc.ReportEvidence
	components, _ := m.structure.(change.Components)
	decisions, _ := m.decisions.(change.Decisions)
	pipelines, _ := m.facts.(change.TechnicalPipelines)
	if components != nil && decisions != nil && pipelines != nil {
		summaryStore := changerepo.NewRepo(pool)
		reader := change.New(change.Deps{
			Components: components, Decisions: decisions, Tx: tx, TechnicalSummaries: summaryStore,
			Catalog: m.catalog, TechnicalPipelines: pipelines, MissingChecksAt: calc.MissingChecksAt,
			Summary: change.SummaryDeps{Store: summaryStore, Facts: normalizerepo.NewRepo(pool)},
		})
		evidence = reader.TechnicalSnapshot
	}
	return calc.New(calc.Deps{
		ReportEvidence: evidence,
		Structure:      m.structure, Facts: m.facts, TestCoverage: m.testCoverage, Security: m.security, Settings: m.settings,
		Collection: m.collection, Activity: m.activity, Code: code, CodeSources: codeSources, Rights: m.rights, Links: links,
		Docs: documentation, DocReports: docReports, Decisions: m.decisions, Analysis: analysis,
		Repo: calcrepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Queue: queue.New(jobs), Tx: tx,
		Metrics: metrics, Clock: c, NewID: newID,
	}), nil
}
