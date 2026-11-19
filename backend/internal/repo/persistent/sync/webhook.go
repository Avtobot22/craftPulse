package sync

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

func (r *Repo) ReceiveWebhook(ctx context.Context, org, conn entity.ID, at time.Time) (entity.ID, error) {
	id, err := New(postgres.Conn(ctx, r.pool)).ReceiveWebhook(ctx, ReceiveWebhookParams{OrganizationID: uuid.UUID(org), ConnectionID: uuid.UUID(conn), ReceivedAt: at})
	if err != nil {
		return entity.ID{}, fmt.Errorf("sync: receive webhook: %w", err)
	}
	return entity.ID(id), nil
}

func (r *Repo) RejectWebhook(ctx context.Context, org, id entity.ID, at time.Time, reason string) (bool, error) {
	n, err := New(postgres.Conn(ctx, r.pool)).RejectWebhook(ctx, RejectWebhookParams{OrganizationID: uuid.UUID(org), ID: uuid.UUID(id), RejectedAt: tstz(&at), RejectReason: text(reason)})
	if err != nil {
		return false, fmt.Errorf("sync: reject webhook: %w", err)
	}
	return n > 0, nil
}

func (r *Repo) WebhookStatus(ctx context.Context, org, conn entity.ID, since time.Time) (*entity.WebhookStatus, error) {
	q := New(postgres.Conn(ctx, r.pool))
	row, err := q.WebhookStatus(ctx, WebhookStatusParams{OrganizationID: uuid.UUID(org), ConnectionID: uuid.UUID(conn), Since: since})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sync: webhook status: %w", err)
	}
	out := &entity.WebhookStatus{LastReceivedAt: &row.LastReceivedAt, RejectedCount: int(row.RejectedCount)}
	last, err := q.LastWebhookRejection(ctx, LastWebhookRejectionParams{OrganizationID: uuid.UUID(org), ConnectionID: uuid.UUID(conn)})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sync: last webhook rejection: %w", err)
	}
	out.LastRejectedAt, out.LastRejectReason = timePtr(last.RejectedAt), last.RejectReason.String
	return out, nil
}

func (r *Repo) PendingRereads(ctx context.Context, org, conn entity.ID) (bool, error) {
	var found bool
	err := postgres.Conn(ctx, r.pool).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.river_job
        WHERE kind IN ('sync.mr', 'sync.dirty') AND state IN ('available', 'pending', 'running', 'retryable', 'scheduled')
        AND args->>'organization_id' = $1 AND args->>'connection_id' = $2)`, org.String(), conn.String()).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("sync: pending rereads: %w", err)
	}
	return found, nil
}
