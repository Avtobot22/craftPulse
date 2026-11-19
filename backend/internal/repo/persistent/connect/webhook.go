package connect

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

func (r *Repo) WebhookRoute(ctx context.Context, id entity.ID) (entity.WebhookRoute, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetWebhookRoute(ctx, uuid.UUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.WebhookRoute{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.WebhookRoute{}, fmt.Errorf("connect: webhook route: %w", err)
	}
	return entity.WebhookRoute{ID: entity.ID(row.ID), OrganizationID: entity.ID(row.OrganizationID), ConnectionID: entity.ID(row.ConnectionID)}, nil
}

func (r *Repo) WebhookRoutes(ctx context.Context, org entity.ID, connections []entity.ID) (map[entity.ID]entity.ID, error) {
	rows, err := New(postgres.Conn(ctx, r.pool)).WebhookRoutes(ctx, WebhookRoutesParams{OrganizationID: uuid.UUID(org), Column2: uuids(connections)})
	if err != nil {
		return nil, fmt.Errorf("connect: webhook routes: %w", err)
	}
	out := make(map[entity.ID]entity.ID, len(rows))
	for _, row := range rows {
		out[entity.ID(row.ConnectionID)] = entity.ID(row.ID)
	}
	return out, nil
}

func (r *Repo) EnsureWebhookRoute(ctx context.Context, org, conn entity.ID, at time.Time) error {
	if err := New(postgres.Conn(ctx, r.pool)).EnsureWebhookRoute(ctx, EnsureWebhookRouteParams{OrganizationID: uuid.UUID(org), ConnectionID: uuid.UUID(conn), CreatedAt: at}); err != nil {
		return fmt.Errorf("connect: create webhook route: %w", err)
	}
	return nil
}

func (r *Repo) RevokeWebhookRoutes(ctx context.Context, org, conn entity.ID, at time.Time) error {
	if err := New(postgres.Conn(ctx, r.pool)).RevokeWebhookRoutes(ctx, RevokeWebhookRoutesParams{OrganizationID: uuid.UUID(org), ConnectionID: uuid.UUID(conn), RevokedAt: tstz(&at)}); err != nil {
		return fmt.Errorf("connect: revoke webhook routes: %w", err)
	}
	return nil
}
