package link

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var (
	_ repo.LinkRepo = (*Repo)(nil)

	errRecordNo = errors.New("link: record number does not fit its column")
)

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo         { return &Repo{pool: pool} }
func (r *Repo) q(ctx context.Context) *Queries { return New(postgres.Conn(ctx, r.pool)) }

func (r *Repo) LockEdgeWrites(ctx context.Context, orgID entity.ID) error {
	if err := r.q(ctx).LockEdgeWrites(ctx, uuid.UUID(orgID)); err != nil {
		return fmt.Errorf("link: lock edge writes: %w", err)
	}
	return nil
}

func (r *Repo) Edge(ctx context.Context, orgID, id entity.ID) (entity.LinkEdge, error) {
	row, err := r.q(ctx).Edge(ctx, EdgeParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	return one(row, err)
}

func (r *Repo) CurrentEdge(ctx context.Context, orgID entity.ID, t entity.LinkType, from, to entity.ID, m entity.LinkMethod) (entity.LinkEdge, error) {
	row, err := r.q(ctx).CurrentEdge(ctx, CurrentEdgeParams{
		OrganizationID: uuid.UUID(orgID), LinkType: string(t), FromObjectID: uuid.UUID(from), ToObjectID: uuid.UUID(to), Method: string(m),
	})
	return one(row, err)
}

func (r *Repo) CurrentConfirmedEdge(ctx context.Context, orgID entity.ID, t entity.LinkType, from, to entity.ID) (entity.LinkEdge, error) {
	row, err := r.q(ctx).CurrentConfirmedEdge(ctx, CurrentConfirmedEdgeParams{
		OrganizationID: uuid.UUID(orgID), LinkType: string(t), FromObjectID: uuid.UUID(from), ToObjectID: uuid.UUID(to),
	})
	return one(row, err)
}

func (r *Repo) EdgeByConfirmation(ctx context.Context, orgID, key entity.ID) (entity.LinkEdge, error) {
	row, err := r.q(ctx).EdgeByConfirmation(ctx, EdgeByConfirmationParams{
		OrganizationID: uuid.UUID(orgID), ConfirmationID: uuid.NullUUID{UUID: uuid.UUID(key), Valid: true},
	})
	return one(row, err)
}

func (r *Repo) EdgesAt(ctx context.Context, orgID, objectID entity.ID, dir entity.LinkDirection, seq int64) ([]entity.LinkEdge, error) {
	var (
		rows []LinkEdge
		err  error
	)
	if dir == entity.LinksTo {
		rows, err = r.q(ctx).EdgesToAt(ctx, EdgesToAtParams{OrganizationID: uuid.UUID(orgID), ObjectID: uuid.UUID(objectID), Seq: seq})
	} else {
		rows, err = r.q(ctx).EdgesFromAt(ctx, EdgesFromAtParams{OrganizationID: uuid.UUID(orgID), ObjectID: uuid.UUID(objectID), Seq: seq})
	}
	if err != nil {
		return nil, fmt.Errorf("link: read edges: %w", err)
	}
	out := make([]entity.LinkEdge, len(rows))
	for i := range rows {
		out[i] = toEdge(&rows[i])
	}
	return out, nil
}

// TaskMREdgesAt reads the links of tasks to MRs that hold for the slice seq with an end among the objects.
func (r *Repo) TaskMREdgesAt(ctx context.Context, orgID entity.ID, objectIDs []entity.ID, seq int64) ([]entity.LinkEdge, error) {
	ids := make([]uuid.UUID, len(objectIDs))
	for i, id := range objectIDs {
		ids[i] = uuid.UUID(id)
	}
	rows, err := r.q(ctx).TaskMREdgesAt(ctx, TaskMREdgesAtParams{OrganizationID: uuid.UUID(orgID), ObjectIds: ids, Seq: seq})
	if err != nil {
		return nil, fmt.Errorf("link: read task MR edges: %w", err)
	}
	out := make([]entity.LinkEdge, len(rows))
	for i := range rows {
		out[i] = toEdge(&rows[i])
	}
	return out, nil
}

func (r *Repo) MREdgesAt(ctx context.Context, orgID entity.ID, mrIDs []entity.ID, seq int64) ([]entity.LinkEdge, error) {
	ids := make([]uuid.UUID, len(mrIDs))
	for i, id := range mrIDs {
		ids[i] = uuid.UUID(id)
	}
	rows, err := r.q(ctx).MREdgesAt(ctx, MREdgesAtParams{OrganizationID: uuid.UUID(orgID), MrIds: ids, Seq: seq})
	if err != nil {
		return nil, fmt.Errorf("link: read MR edges: %w", err)
	}
	out := make([]entity.LinkEdge, len(rows))
	for i := range rows {
		out[i] = toEdge(&rows[i])
	}
	return out, nil
}

func (r *Repo) InsertEdge(ctx context.Context, orgID entity.ID, e *entity.LinkEdge) error {
	in := InsertEdgeParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(e.ID), LinkType: string(e.Type), FromObjectID: uuid.UUID(e.From),
		ToObjectID: uuid.UUID(e.To), Method: string(e.Method), Status: string(e.Status), KnownSeq: e.KnownSeq, Evidence: e.Evidence,
		ActorUserID: nullID(e.ActorUserID), ConfirmationID: nullID(e.ConfirmationID),
	}
	if e.Record != nil {
		if e.Record.RecordNo < 1 || e.Record.RecordNo > math.MaxInt32 {
			return errRecordNo
		}
		in.ReceiptID = uuid.NullUUID{UUID: uuid.UUID(e.Record.ReceiptID), Valid: true}
		in.RecordNo = pgtype.Int4{Int32: int32(e.Record.RecordNo), Valid: true}
	}
	if e.DecidedAt != nil {
		in.DecidedAt = pgtype.Timestamptz{Time: *e.DecidedAt, Valid: true}
	}
	if err := r.q(ctx).InsertEdge(ctx, in); err != nil {
		return fmt.Errorf("link: insert edge: %w", err)
	}
	return nil
}

func (r *Repo) SupersedeEdge(ctx context.Context, orgID, id entity.ID, seq int64, at time.Time) error {
	n, err := r.q(ctx).SupersedeEdge(ctx, SupersedeEdgeParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id),
		SupersededSeq: pgtype.Int8{Int64: seq, Valid: true}, SupersededAt: pgtype.Timestamptz{Time: at, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("link: supersede edge: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("link: supersede edge %s: %w", id, entity.ErrNotFound)
	}
	return nil
}

func one(row LinkEdge, err error) (entity.LinkEdge, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.LinkEdge{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.LinkEdge{}, fmt.Errorf("link: read edge: %w", err)
	}
	return toEdge(&row), nil
}

func toEdge(row *LinkEdge) entity.LinkEdge {
	out := entity.LinkEdge{
		ID: entity.ID(row.ID), Type: entity.LinkType(row.LinkType), From: entity.ID(row.FromObjectID), To: entity.ID(row.ToObjectID),
		Method: entity.LinkMethod(row.Method), Status: entity.LinkStatus(row.Status), Knowledge: entity.Knowledge{KnownSeq: row.KnownSeq},
		Evidence: row.Evidence, ActorUserID: id(row.ActorUserID), ConfirmationID: id(row.ConfirmationID),
	}
	if row.SupersededSeq.Valid {
		seq, at := row.SupersededSeq.Int64, row.SupersededAt.Time
		out.SupersededSeq, out.SupersededAt = &seq, &at
	}
	if row.ReceiptID.Valid {
		out.Record = &entity.RecordRef{ReceiptID: entity.ID(row.ReceiptID.UUID), RecordNo: int(row.RecordNo.Int32)}
	}
	if row.DecidedAt.Valid {
		at := row.DecidedAt.Time
		out.DecidedAt = &at
	}
	return out
}

func id(v uuid.NullUUID) *entity.ID {
	if !v.Valid {
		return nil
	}
	out := entity.ID(v.UUID)
	return &out
}

func nullID(v *entity.ID) uuid.NullUUID {
	if v == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*v), Valid: true}
}
