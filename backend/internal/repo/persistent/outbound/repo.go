// Package outbound persists external targets and the outcome of every write attempt.
package outbound

import (
	"context"
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

func (r *Repo) Start(ctx context.Context, org, connection, revision entity.ID, target entity.PublicationTarget, now time.Time) (entity.ID, error) {
	conn := postgres.Conn(ctx, r.pool)
	_, err := conn.Exec(ctx, `INSERT INTO outbound.target (organization_id, target_kind, target_marker, connection_id, updated_at)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (organization_id, target_kind, target_marker) DO NOTHING`,
		uuid.UUID(org), target.Kind, target.Marker, uuid.UUID(connection), now)
	if err != nil {
		return entity.ID{}, err
	}
	// A worker can stop after sending the request and before saving its outcome.
	_, err = conn.Exec(ctx, `UPDATE outbound.action SET status = 'unknown', error_code = 'provider_error', completed_at = $4
		WHERE organization_id = $1 AND target_kind = $2 AND target_marker = $3 AND status = 'intent'`,
		uuid.UUID(org), target.Kind, target.Marker, now)
	if err != nil {
		return entity.ID{}, err
	}
	var id uuid.UUID
	err = conn.QueryRow(ctx, `INSERT INTO outbound.action
		(organization_id, target_kind, target_marker, summary_revision_id, status, attempt_no, created_at)
		SELECT $1,$2,$3,$4,'intent',COALESCE(max(attempt_no),0)+1,$5
		FROM outbound.action WHERE organization_id = $1 AND target_kind = $2 AND target_marker = $3
		RETURNING id`, uuid.UUID(org), target.Kind, target.Marker, uuid.UUID(revision), now).Scan(&id)
	return entity.ID(id), err
}

func (r *Repo) Finish(ctx context.Context, org, action, connection, revision entity.ID, target entity.PublicationTarget,
	outcome entity.Publication, now time.Time,
) error {
	conn := postgres.Conn(ctx, r.pool)
	_, err := conn.Exec(ctx, `UPDATE outbound.action SET status = $3, error_code = NULLIF($4,''), completed_at = $5
		WHERE organization_id = $1 AND id = $2`, uuid.UUID(org), uuid.UUID(action), outcome.Status, outcome.ErrorCode, now)
	if err != nil || outcome.Status != entity.PublicationSucceeded {
		return err
	}
	_, err = conn.Exec(ctx, `UPDATE outbound.target SET connection_id = $4, last_summary_revision_id = $5,
		external_id = $6, updated_at = $7 WHERE organization_id = $1 AND target_kind = $2 AND target_marker = $3`,
		uuid.UUID(org), target.Kind, target.Marker, uuid.UUID(connection), uuid.UUID(revision), outcome.ExternalID, now)
	return err
}
