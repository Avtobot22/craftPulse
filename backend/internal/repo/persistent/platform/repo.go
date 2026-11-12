package platform

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// CreateTenant registers the organization for the scheduler and starts its knowledge counter (TZ 12.5.1).
func (r *Repo) CreateTenant(ctx context.Context, orgID entity.ID, at time.Time) error {
	q := New(postgres.Conn(ctx, r.pool))
	id := uuid.UUID(orgID)
	if err := q.CreateTenant(ctx, CreateTenantParams{OrganizationID: id, Status: "active", CreatedAt: at}); err != nil {
		return fmt.Errorf("platform: create tenant: %w", err)
	}
	if err := q.CreateOrgClock(ctx, id); err != nil {
		return fmt.Errorf("platform: create org clock: %w", err)
	}
	return nil
}

// NextSeq advances the knowledge counter of the organization; versions written in one change share the number.
func (r *Repo) NextSeq(ctx context.Context, orgID entity.ID) (int64, error) {
	seq, err := New(postgres.Conn(ctx, r.pool)).NextSeq(ctx, uuid.UUID(orgID))
	if err != nil {
		return 0, fmt.Errorf("platform: next seq: %w", err)
	}
	return seq, nil
}
