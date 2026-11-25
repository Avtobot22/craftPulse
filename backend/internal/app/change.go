package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	changerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/change"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/change"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// newIdentifier is change for the worker: it identifies the findings of reports and decides baselines, so it reads
// the reports of normalize and the projects and components of org (TZ 10.3).
func newIdentifier(pool *pgxpool.Pool, reports change.Reports, components change.Components, c clock.Clock, decisions ...change.Decisions) *change.UseCase {
	d := changeDeps(pool, reports, components, c)
	if len(decisions) > 0 {
		d.Decisions = decisions[0]
	}
	return change.New(d)
}

func newSummarizer(pool *pgxpool.Pool, reports change.Reports, components change.Components, c clock.Clock,
	decisions change.Decisions, summary change.SummaryDeps, catalog entity.CheckCatalog,
) *change.UseCase {
	d := changeDeps(pool, reports, components, c)
	d.Decisions, d.Summary, d.Catalog = decisions, summary, catalog
	return change.New(d)
}

func changeDeps(pool *pgxpool.Pool, reports change.Reports, components change.Components, c clock.Clock) change.Deps {
	return change.Deps{
		Repo: changerepo.NewRepo(pool), Reports: reports, Tenants: platform.NewRepo(pool), Tx: persistent.NewTransactor(pool),
		Components: components, Clock: c, NewID: newID,
	}
}
