package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	processrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/process"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/process"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// newProcess reads the projects of teams from org and the dictionaries of sources from normalize; these and
// audit are all it depends on (TZ 10.3). The recount of calc it queues through the queue of jobs.
func newProcess(pool *pgxpool.Pool, teams process.Teams, dicts process.Dictionaries, c clock.Clock) (*process.UseCase, error) {
	jobs, err := riverInsertClient(pool)
	if err != nil {
		return nil, err
	}
	tx := persistent.NewTransactor(pool)
	return process.New(process.Deps{
		Repo: processrepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Teams: teams, Dictionaries: dicts,
		Auditor: audit.New(auditrepo.NewRepo(pool), tx, c), Queue: queue.New(jobs), Tx: tx, Clock: c, NewID: newID,
	}), nil
}
