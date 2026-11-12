package auth

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (r *Repo) OrganizationBySlug(ctx context.Context, slug string) (entity.ID, bool, error) {
	id, err := New(postgres.Conn(ctx, r.pool)).GetLoginRoute(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ID{}, false, nil
	}
	if err != nil {
		return entity.ID{}, false, fmt.Errorf("auth: get login route: %w", err)
	}
	return entity.ID(id), true, nil
}

func (r *Repo) CreateLoginRoute(ctx context.Context, slug string, orgID entity.ID) error {
	err := New(postgres.Conn(ctx, r.pool)).CreateLoginRoute(ctx, CreateLoginRouteParams{Slug: slug, OrganizationID: uuid.UUID(orgID)})
	if err != nil {
		return fmt.Errorf("auth: create login route: %w", err)
	}
	return nil
}

func (r *Repo) CreateProviderRegistration(ctx context.Context, reg entity.ProviderRegistration, at time.Time) error {
	if reg.KeyVersion > math.MaxInt32 {
		return fmt.Errorf("auth: master key version %d does not fit the key_version column", reg.KeyVersion)
	}
	err := New(postgres.Conn(ctx, r.pool)).CreateProviderRegistration(ctx, CreateProviderRegistrationParams{
		OrganizationID:  uuid.UUID(reg.OrganizationID),
		Issuer:          reg.Issuer,
		ClientID:        reg.ClientID,
		ClientSecretEnc: reg.ClientSecretEnc,
		KeyVersion:      int32(reg.KeyVersion), //nolint:gosec // bounded by the check above
		CreatedAt:       at,
	})
	if err != nil {
		return fmt.Errorf("auth: create provider registration: %w", err)
	}
	return nil
}
