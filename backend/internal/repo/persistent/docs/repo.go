package docs

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
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var _ repo.DocumentationRepo = (*Repo)(nil)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo         { return &Repo{pool: pool} }
func (r *Repo) q(ctx context.Context) *Queries { return New(postgres.Conn(ctx, r.pool)) }

func (r *Repo) LockRootWrites(ctx context.Context, orgID entity.ID) error {
	if err := r.q(ctx).LockRootWrites(ctx, uuid.UUID(orgID)); err != nil {
		return fmt.Errorf("docs: lock root writes: %w", err)
	}
	return nil
}

func (r *Repo) LatestRootSet(ctx context.Context, orgID, serviceID entity.ID) (entity.DocumentationRootSet, error) {
	row, err := r.q(ctx).LatestRootSet(ctx, LatestRootSetParams{OrganizationID: uuid.UUID(orgID), ServiceID: uuid.UUID(serviceID)})
	if err != nil {
		return entity.DocumentationRootSet{}, readError(err)
	}
	return rootSet(row), nil
}

func (r *Repo) RootSetByConfirmation(ctx context.Context, orgID, key entity.ID) (entity.DocumentationRootSet, entity.ID, error) {
	row, err := r.q(ctx).RootSetByConfirmation(ctx, RootSetByConfirmationParams{OrganizationID: uuid.UUID(orgID), ConfirmationID: uuid.UUID(key)})
	if err != nil {
		return entity.DocumentationRootSet{}, entity.ID{}, readError(err)
	}
	return rootSet(row.DocsRootSet), entity.ID(row.ActorUserID), nil
}

func (r *Repo) RememberRootConfirmation(ctx context.Context, orgID, key, actor entity.ID, set entity.DocumentationRootSet, at time.Time) error {
	err := r.q(ctx).RememberRootConfirmation(ctx, RememberRootConfirmationParams{
		OrganizationID: uuid.UUID(orgID), ConfirmationID: uuid.UUID(key),
		ServiceID: uuid.UUID(set.ServiceID), VersionNo: set.Version, ActorUserID: uuid.UUID(actor), CreatedAt: at,
	})
	if err != nil {
		return fmt.Errorf("docs: remember root confirmation: %w", err)
	}
	return nil
}

func rootSet(row DocsRootSet) entity.DocumentationRootSet {
	return entity.DocumentationRootSet{
		ServiceID: entity.ID(row.ServiceID), Version: row.VersionNo, KnownSeq: row.KnownSeq, ContentHash: row.ContentHash,
		Confirmation: entity.Confirmation{ConfirmedBy: entity.ID(row.ConfirmedBy), ConfirmedAt: row.ConfirmedAt, ConfirmationID: entity.ID(row.ConfirmationID)},
	}
}

func readError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ErrNotFound
	}
	return fmt.Errorf("docs: read roots: %w", err)
}

func (r *Repo) RootsAt(ctx context.Context, orgID, serviceID entity.ID, seq int64) ([]entity.DocumentationRoot, error) {
	rows, err := r.q(ctx).RootsAt(ctx, RootsAtParams{OrganizationID: uuid.UUID(orgID), ServiceID: uuid.UUID(serviceID), KnownSeq: seq})
	if err != nil {
		return nil, readError(err)
	}
	return mapRoots(rows), nil
}

func (r *Repo) CurrentAreaRoots(ctx context.Context, orgID entity.ID, areaIDs []entity.ID) ([]entity.DocumentationRoot, error) {
	ids := make([]uuid.UUID, len(areaIDs))
	for i, id := range areaIDs {
		ids[i] = uuid.UUID(id)
	}
	rows, err := r.q(ctx).CurrentAreaRoots(ctx, CurrentAreaRootsParams{OrganizationID: uuid.UUID(orgID), AreaIds: ids})
	if err != nil {
		return nil, readError(err)
	}
	return mapRoots(rows), nil
}

func mapRoots(rows []DocsRoot) []entity.DocumentationRoot {
	roots := make([]entity.DocumentationRoot, len(rows))
	for i := range rows {
		row := &rows[i]
		roots[i] = entity.DocumentationRoot{
			ID: entity.ID(row.ID), ServiceID: entity.ID(row.ServiceID), AreaID: entity.ID(row.AreaID),
			PathPrefix: row.PathPrefix, ProfileID: row.ProfileID, Audience: row.Audience,
			Knowledge:    entity.Knowledge{KnownSeq: row.KnownSeq},
			Confirmation: entity.Confirmation{ConfirmedBy: entity.ID(row.ConfirmedBy), ConfirmedAt: row.ConfirmedAt, ConfirmationID: entity.ID(row.ConfirmationID)},
		}
		if row.OwnerPersonID.Valid {
			id := entity.ID(row.OwnerPersonID.UUID)
			roots[i].OwnerPersonID = &id
		}
		if row.SupersededSeq.Valid {
			seq := row.SupersededSeq.Int64
			roots[i].SupersededSeq = &seq
		}
		if row.SupersededAt.Valid {
			at := row.SupersededAt.Time
			roots[i].SupersededAt = &at
		}
	}
	return roots
}

func (r *Repo) SupersedeRoots(ctx context.Context, orgID, serviceID entity.ID, seq int64, at time.Time) error {
	if err := r.q(ctx).SupersedeRoots(ctx, SupersedeRootsParams{
		OrganizationID: uuid.UUID(orgID), ServiceID: uuid.UUID(serviceID),
		SupersededSeq: pgtype.Int8{Int64: seq, Valid: true}, SupersededAt: pgtype.Timestamptz{Time: at, Valid: true},
	}); err != nil {
		return fmt.Errorf("docs: supersede roots: %w", err)
	}
	return nil
}

func (r *Repo) InsertRootSet(ctx context.Context, orgID entity.ID, set entity.DocumentationRootSet) error {
	q := r.q(ctx)
	if err := q.InsertRootSet(ctx, InsertRootSetParams{
		OrganizationID: uuid.UUID(orgID), ServiceID: uuid.UUID(set.ServiceID), VersionNo: set.Version,
		KnownSeq: set.KnownSeq, ContentHash: set.ContentHash, ConfirmedBy: uuid.UUID(set.ConfirmedBy), ConfirmedAt: set.ConfirmedAt, ConfirmationID: uuid.UUID(set.ConfirmationID),
	}); err != nil {
		return fmt.Errorf("docs: insert root set: %w", err)
	}
	for i := range set.Roots {
		root := &set.Roots[i]
		in := InsertRootParams{
			OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(root.ID), ServiceID: uuid.UUID(set.ServiceID), AreaID: uuid.UUID(root.AreaID),
			PathPrefix: root.PathPrefix, ProfileID: root.ProfileID, Audience: root.Audience, KnownSeq: set.KnownSeq,
			ConfirmedBy: uuid.UUID(set.ConfirmedBy), ConfirmedAt: set.ConfirmedAt, ConfirmationID: uuid.UUID(set.ConfirmationID),
		}
		if root.OwnerPersonID != nil {
			in.OwnerPersonID = uuid.NullUUID{UUID: uuid.UUID(*root.OwnerPersonID), Valid: true}
		}
		if err := q.InsertRoot(ctx, in); err != nil {
			return fmt.Errorf("docs: insert root: %w", err)
		}
	}
	return nil
}
