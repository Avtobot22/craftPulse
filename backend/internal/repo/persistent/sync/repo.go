package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

func (r *Repo) State(ctx context.Context, orgID, connectionID entity.ID, dataKind string) (entity.SyncState, bool, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetState(ctx, GetStateParams{OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), DataKind: dataKind})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.SyncState{}, false, nil
	}
	if err != nil {
		return entity.SyncState{}, false, fmt.Errorf("sync: get state: %w", err)
	}
	return syncState(&row), true, nil
}

func syncState(row *SyncState) entity.SyncState {
	return entity.SyncState{
		ConnectionID: entity.ID(row.ConnectionID), DataKind: row.DataKind, Phase: entity.SyncPhase(row.Phase), Cursor: row.Cursor.String,
		CheckpointAt: timePtr(row.CheckpointAt), FailedCursor: row.FailedCursor.String, LastAttemptAt: timePtr(row.LastAttemptAt),
		LastOutcome: row.LastOutcome.String, LastErrorCode: row.LastErrorCode.String, LastSuccessAt: timePtr(row.LastSuccessAt),
		Watermark: timePtr(row.Watermark), HistoryFrom: timePtr(row.HistoryFrom), Completeness: row.Completeness,
	}
}

func (r *Repo) SaveState(ctx context.Context, orgID entity.ID, s entity.SyncState) error {
	err := New(postgres.Conn(ctx, r.pool)).SaveState(ctx, SaveStateParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(s.ConnectionID), DataKind: s.DataKind, Phase: string(s.Phase),
		Cursor: text(s.Cursor), CheckpointAt: tstz(s.CheckpointAt), FailedCursor: text(s.FailedCursor), LastAttemptAt: tstz(s.LastAttemptAt),
		LastOutcome: text(s.LastOutcome), LastErrorCode: text(s.LastErrorCode), LastSuccessAt: tstz(s.LastSuccessAt),
		Watermark: tstz(s.Watermark), HistoryFrom: tstz(s.HistoryFrom), Completeness: s.Completeness,
	})
	if err != nil {
		return fmt.Errorf("sync: save state: %w", err)
	}
	return nil
}

func (r *Repo) AcquireLease(ctx context.Context, orgID, connectionID entity.ID, holder string, until, now time.Time) (bool, error) {
	_, err := New(postgres.Conn(ctx, r.pool)).AcquireLease(ctx, AcquireLeaseParams{
		OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), Holder: holder, LeaseUntil: until, Now: now,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sync: acquire lease: %w", err)
	}
	return true, nil
}

func (r *Repo) ReleaseLease(ctx context.Context, orgID, connectionID entity.ID, holder string) error {
	err := New(postgres.Conn(ctx, r.pool)).ReleaseLease(ctx, ReleaseLeaseParams{OrganizationID: uuid.UUID(orgID), ConnectionID: uuid.UUID(connectionID), Holder: holder})
	if err != nil {
		return fmt.Errorf("sync: release lease: %w", err)
	}
	return nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func tstz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
