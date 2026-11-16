package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	authrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/auth"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/auth"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/connect"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/view"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// retention runs the purges of the modules that own the tables of the retention table (TZ 12.9): auth outside
// row-level security, then connect and view under every active organization.
type retention struct {
	auth    *auth.Retention
	tenants repo.TenantRepo
	tx      repo.Transactor
	probes  *connect.UseCase
	views   *view.UseCase
}

func newRetention(pool *pgxpool.Pool, probes *connect.UseCase, views *view.UseCase, c clock.Clock) *retention {
	tx := persistent.NewTransactor(pool)
	return &retention{
		auth: auth.NewRetention(authrepo.NewRepo(pool), tx, c), tenants: platform.NewRepo(pool), tx: tx, probes: probes, views: views,
	}
}

func (r *retention) Purge(ctx context.Context) error {
	if err := r.auth.PurgeExpired(ctx); err != nil {
		return err
	}
	var orgs []entity.ID
	err := r.tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		orgs, err = r.tenants.ActiveTenants(ctx)
		return err
	})
	if err != nil {
		return err
	}
	for _, org := range orgs {
		if err := r.probes.PurgeProbes(ctx, org); err != nil {
			return err
		}
		if err := r.views.PurgeOperations(ctx, org); err != nil {
			return err
		}
	}
	return nil
}
