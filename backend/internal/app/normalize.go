package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	linkrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/link"
	normalizerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/normalize"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	linkuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/link"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/normalize"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// newNormalize reads the journal and the packages of ingest, writes accounts through org and decides link candidates
// through link, which only it uses (TZ 10.3); rights may be nil where decideLink is not served.
func newNormalize(pool *pgxpool.Pool, store repo.ObjectStore, intake normalize.Ingest, accounts normalize.Accounts, rights normalize.Rights,
	c clock.Clock,
) (*normalize.UseCase, error) {
	jobs, err := riverInsertClient(pool)
	if err != nil {
		return nil, err
	}
	tx := persistent.NewTransactor(pool)
	links := linkuc.New(linkuc.Deps{
		Repo: linkrepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Tx: tx, Auditor: audit.New(auditrepo.NewRepo(pool), tx, c), Clock: c, NewID: newID,
	})
	return normalize.New(normalize.Deps{
		Repo: normalizerepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Ingest: intake, Accounts: accounts, Links: links, Rights: rights,
		Objects: store, Queue: queue.New(jobs), Tx: tx, Clock: c,
	}), nil
}
