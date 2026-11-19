package calc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

// CreateSavedView and its pin share the caller's organization transaction. The revision lock serializes
// pinning against retention; a building revision or one of another series cannot be pinned.
func (r *Repo) CreateSavedView(ctx context.Context, v entity.SavedView) error {
	q := New(postgres.Conn(ctx, r.pool))
	var pin uuid.NullUUID
	if v.PinnedRevisionID != nil {
		pin = uuid.NullUUID{UUID: uuid.UUID(*v.PinnedRevisionID), Valid: true}
		_, err := q.LockSavedRevision(ctx, LockSavedRevisionParams{
			OrganizationID: uuid.UUID(v.OrganizationID), ID: pin.UUID, SeriesID: uuid.UUID(v.SeriesID),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("calc: lock saved revision: %w", err)
		}
	}
	err := q.InsertSavedView(ctx, InsertSavedViewParams{
		OrganizationID: uuid.UUID(v.OrganizationID), ID: uuid.UUID(v.ID), OwnerUserID: uuid.UUID(v.OwnerUserID),
		Name: v.Name, SeriesID: uuid.UUID(v.SeriesID), Filters: v.Filters, PinnedRevisionID: pin, CreatedAt: v.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("calc: insert saved view: %w", err)
	}
	if pin.Valid {
		if err := q.PinSavedRevision(ctx, PinSavedRevisionParams{
			OrganizationID: uuid.UUID(v.OrganizationID), RevisionID: pin.UUID, HolderID: uuid.UUID(v.ID),
		}); err != nil {
			return fmt.Errorf("calc: pin saved revision: %w", err)
		}
	}
	return nil
}

func (r *Repo) SavedView(ctx context.Context, orgID, id entity.ID) (entity.SavedView, error) {
	row, err := New(postgres.Conn(ctx, r.pool)).GetSavedView(ctx, GetSavedViewParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.SavedView{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.SavedView{}, fmt.Errorf("calc: get saved view: %w", err)
	}
	return savedView(row), nil
}

func (r *Repo) PageSavedViews(ctx context.Context, orgID, ownerID entity.ID, f entity.SavedViewQuery) ([]entity.SavedView, error) {
	p := PageSavedViewsParams{OrganizationID: uuid.UUID(orgID), OwnerUserID: uuid.UUID(ownerID), PageLimit: int32(f.Limit + 1)} //nolint:gosec // bounded by the use case
	if f.After != nil {
		p.AfterAt = pgtype.Timestamptz{Time: f.After.CreatedAt, Valid: true}
		p.AfterID = uuid.UUID(f.After.ID)
	}
	rows, err := New(postgres.Conn(ctx, r.pool)).PageSavedViews(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("calc: page saved views: %w", err)
	}
	out := make([]entity.SavedView, len(rows))
	for i := range rows {
		out[i] = savedView(rows[i])
	}
	return out, nil
}

func savedView(row CalcSavedView) entity.SavedView {
	v := entity.SavedView{
		ID: entity.ID(row.ID), OrganizationID: entity.ID(row.OrganizationID), OwnerUserID: entity.ID(row.OwnerUserID),
		Name: row.Name, SeriesID: entity.ID(row.SeriesID), Filters: row.Filters, CreatedAt: row.CreatedAt,
	}
	if row.PinnedRevisionID.Valid {
		id := entity.ID(row.PinnedRevisionID.UUID)
		v.PinnedRevisionID = &id
	}
	return v
}

func (r *Repo) DeleteSavedView(ctx context.Context, orgID, ownerID, id entity.ID) error {
	q := New(postgres.Conn(ctx, r.pool))
	row, err := q.LockSavedView(ctx, LockSavedViewParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("calc: lock saved view: %w", err)
	}
	if row.OwnerUserID != uuid.UUID(ownerID) {
		return entity.ErrNotFound
	}
	if err := q.UnpinSavedRevision(ctx, UnpinSavedRevisionParams{OrganizationID: uuid.UUID(orgID), HolderID: uuid.UUID(id)}); err != nil {
		return fmt.Errorf("calc: unpin saved revision: %w", err)
	}
	if err := q.DeleteSavedView(ctx, DeleteSavedViewParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)}); err != nil {
		return fmt.Errorf("calc: delete saved view: %w", err)
	}
	return nil
}

// PurgeRevisions locks a bounded portion before checking holders in a fresh snapshot. A concurrent pin
// either owns its revision lock first or finds the revision already removed, without a dangling holder.
func (r *Repo) PurgeRevisions(ctx context.Context, orgID entity.ID, before, at time.Time, limit int) (int64, error) {
	q := New(postgres.Conn(ctx, r.pool))
	ids, err := q.ExpiredRevisions(ctx, ExpiredRevisionsParams{
		OrganizationID: uuid.UUID(orgID), BeforeAt: pgtype.Timestamptz{Time: before, Valid: true}, BatchLimit: int32(limit), //nolint:gosec // bounded by the use case
	})
	if err != nil {
		return 0, fmt.Errorf("calc: lock expired revisions: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	n, err := q.DeleteExpiredRevisions(ctx, DeleteExpiredRevisionsParams{OrganizationID: uuid.UUID(orgID), Ids: ids, AppliedAt: at, BeforeAt: before})
	if err != nil {
		return 0, fmt.Errorf("calc: purge expired revisions: %w", err)
	}
	return n, nil
}
