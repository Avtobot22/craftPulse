package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	auditrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/audit"
	decisionrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/decision"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/queue"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/audit"
	decisionuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/decision"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

func newDecision(pool *pgxpool.Pool, rights decisionuc.Access, c clock.Clock) (*decisionuc.UseCase, error) {
	jobs, err := riverInsertClient(pool)
	if err != nil {
		return nil, err
	}
	tx := persistent.NewTransactor(pool)
	return decisionuc.New(decisionuc.Deps{
		Repo: decisionrepo.NewRepo(pool), Tenants: platform.NewRepo(pool), Tx: tx, Clock: c, Queue: queue.New(jobs),
		Access: rights, Auditor: audit.New(auditrepo.NewRepo(pool), tx, c), NewID: newID,
	}), nil
}
