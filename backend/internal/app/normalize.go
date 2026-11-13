package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	normalizerepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/normalize"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/normalize"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// newNormalize reads the journal and the packages of ingest and writes accounts through org (TZ 10.3).
func newNormalize(pool *pgxpool.Pool, store repo.ObjectStore, intake normalize.Ingest, accounts normalize.Accounts, c clock.Clock) (*normalize.UseCase, error) {
	jobs, err := riverInsertClient(pool)
	if err != nil {
		return nil, err
	}
	return normalize.New(normalize.Deps{
		Repo: normalizerepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Ingest: intake, Accounts: accounts, Objects: store,
		Queue: queue.New(jobs), Tx: persistent.NewTransactor(pool), Clock: c,
	}), nil
}
