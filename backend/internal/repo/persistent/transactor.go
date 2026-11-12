package persistent

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type Transactor struct {
	pool *pgxpool.Pool
}

func NewTransactor(pool *pgxpool.Pool) *Transactor {
	return &Transactor{pool: pool}
}

func (t *Transactor) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return postgres.InTx(ctx, t.pool, fn)
}

func (t *Transactor) InOrgTx(ctx context.Context, orgID entity.ID, fn func(ctx context.Context) error) error {
	return postgres.InOrgTx(ctx, t.pool, uuid.UUID(orgID), fn)
}
