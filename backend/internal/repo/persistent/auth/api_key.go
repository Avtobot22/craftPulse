package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

func (r *Repo) ActiveAPIKey(ctx context.Context, hash []byte, at time.Time) (entity.Subject, error) {
	row, err := New(r.pool).ActiveAPIKeyByHash(ctx, ActiveAPIKeyByHashParams{TokenHash: hash, At: at})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Subject{}, entity.ErrAuthRequired
	}
	if err != nil {
		return entity.Subject{}, fmt.Errorf("auth: read API key: %w", err)
	}
	return entity.Subject{OrganizationID: entity.ID(row.OrganizationID), APIKeyID: entity.ID(row.ID), UserID: entity.ID(row.OwnerUserID)}, nil
}

func (r *Repo) APIKeyProjects(ctx context.Context, org, key entity.ID, at time.Time) ([]entity.ID, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).APIKeyProjects(ctx, APIKeyProjectsParams{
		OrganizationID: uuid.UUID(org), KeyID: uuid.UUID(key), At: at,
	})
	if err != nil {
		return nil, fmt.Errorf("auth: read API key projects: %w", err)
	}
	out := make([]entity.ID, len(rows))
	for i, id := range rows {
		out[i] = entity.ID(id)
	}
	return out, nil
}

func (r *Repo) RevokeExposedAPIKey(ctx context.Context, hash []byte, at time.Time) error {
	if err := New(r.pool).RevokeExposedAPIKey(ctx, RevokeExposedAPIKeyParams{TokenHash: hash, At: at}); err != nil {
		return fmt.Errorf("auth: revoke exposed API key: %w", err)
	}
	return nil
}
