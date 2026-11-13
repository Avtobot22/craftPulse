package normalize

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var errRecordNo = errors.New("normalize: record number does not fit its column")

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) q(ctx context.Context) *Queries { return New(postgres.Conn(ctx, r.pool)) }

func (r *Repo) EnsureObjects(ctx context.Context, orgID entity.ID, keys []entity.SourceKey, quality entity.IdentityQuality,
	seq int64,
) (map[entity.SourceKey]entity.SourceObject, error) {
	out := make(map[entity.SourceKey]entity.SourceObject, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	in := InsertObjectsParams{OrganizationID: uuid.UUID(orgID), KnownSeq: seq}
	for _, k := range keys {
		in.InstanceIds = append(in.InstanceIds, uuid.UUID(k.InstanceID))
		in.Kinds = append(in.Kinds, string(k.Kind))
		in.NativeNss = append(in.NativeNss, k.NativeNS)
		in.NativeIds = append(in.NativeIds, k.NativeID)
		in.Qualities = append(in.Qualities, string(quality))
	}
	if err := r.q(ctx).InsertObjects(ctx, in); err != nil {
		return nil, fmt.Errorf("normalize: insert objects: %w", err)
	}
	rows, err := r.q(ctx).GetObjects(ctx, GetObjectsParams{
		OrganizationID: in.OrganizationID, InstanceIds: in.InstanceIds, Kinds: in.Kinds, NativeNss: in.NativeNss, NativeIds: in.NativeIds,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: get objects: %w", err)
	}
	for i := range rows {
		row := &rows[i]
		o := entity.SourceObject{
			ID:      entity.ID(row.ID),
			Key:     entity.SourceKey{InstanceID: entity.ID(row.InstanceID), Kind: entity.ObjectKind(row.ObjectKind), NativeNS: row.NativeNs, NativeID: row.NativeID},
			Quality: entity.IdentityQuality(row.IdentityQuality), FirstKnownSeq: row.FirstKnownSeq,
			CurrentAreaID: idPtr(row.CurrentAreaID), Restricted: row.Restricted, DeletedAt: timePtr(row.DeletedAt),
		}
		out[o.Key] = o
	}
	return out, nil
}

func (r *Repo) EnsureAreas(ctx context.Context, orgID entity.ID, areas map[entity.ID]entity.ObjectKind) error {
	if len(areas) == 0 {
		return nil
	}
	in := InsertAreasParams{OrganizationID: uuid.UUID(orgID)}
	for id, kind := range areas {
		in.AreaIds = append(in.AreaIds, uuid.UUID(id))
		in.Kinds = append(in.Kinds, string(kind))
	}
	if err := r.q(ctx).InsertAreas(ctx, in); err != nil {
		return fmt.Errorf("normalize: insert areas: %w", err)
	}
	return nil
}

func (r *Repo) PlaceObjects(ctx context.Context, orgID entity.ID, placements []entity.ObjectPlacement) error {
	if len(placements) == 0 {
		return nil
	}
	in := PlaceObjectsParams{OrganizationID: uuid.UUID(orgID)}
	for _, p := range placements {
		in.ObjectIds = append(in.ObjectIds, uuid.UUID(p.ObjectID))
		in.AreaIds = append(in.AreaIds, uuid.UUID(p.AreaID))
		in.Restricted = append(in.Restricted, p.Restricted)
	}
	if err := r.q(ctx).PlaceObjects(ctx, in); err != nil {
		return fmt.Errorf("normalize: place objects: %w", err)
	}
	return nil
}

func (r *Repo) RecordAreaChanges(ctx context.Context, orgID entity.ID, seq int64, changes []entity.AreaChange, at time.Time) error {
	if len(changes) == 0 {
		return nil
	}
	params := make([]RecordAreaChangeParams, len(changes))
	ids := make([]uuid.UUID, len(changes))
	for i, c := range changes {
		params[i] = RecordAreaChangeParams{
			OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(c.AreaID), KnownSeq: seq, EarliestEventTime: tstz(c.EarliestEventTime),
			FactKinds: c.FactKinds, RecordedAt: at,
		}
		ids[i] = uuid.UUID(c.AreaID)
	}
	var batchErr error
	r.q(ctx).RecordAreaChange(ctx, params).Exec(func(_ int, err error) { batchErr = firstErr(batchErr, err) })
	if batchErr != nil {
		return fmt.Errorf("normalize: record area changes: %w", batchErr)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	if err := r.q(ctx).SetAreaChangeSeq(ctx, SetAreaChangeSeqParams{KnownSeq: seq, OrganizationID: uuid.UUID(orgID), AreaIds: ids}); err != nil {
		return fmt.Errorf("normalize: set area change: %w", err)
	}
	return nil
}

func (r *Repo) AreaChanges(ctx context.Context, orgID, areaID entity.ID, afterSeq int64) ([]entity.AreaChange, error) {
	rows, err := r.q(ctx).AreaChanges(ctx, AreaChangesParams{OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(areaID), AfterSeq: afterSeq})
	if err != nil {
		return nil, fmt.Errorf("normalize: area changes: %w", err)
	}
	out := make([]entity.AreaChange, len(rows))
	for i, row := range rows {
		out[i] = entity.AreaChange{AreaID: entity.ID(row.AreaID), KnownSeq: row.KnownSeq, EarliestEventTime: timePtr(row.EarliestEventTime), FactKinds: row.FactKinds}
	}
	return out, nil
}

func (r *Repo) AreaOf(ctx context.Context, orgID, instanceID entity.ID, kind entity.ObjectKind, nativeID string) (entity.ID, error) {
	id, err := r.q(ctx).AreaOf(ctx, AreaOfParams{OrganizationID: uuid.UUID(orgID), InstanceID: uuid.UUID(instanceID), ObjectKind: string(kind), NativeID: nativeID})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ID{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.ID{}, fmt.Errorf("normalize: area of: %w", err)
	}
	return entity.ID(id), nil
}

// firstErr keeps the first error of a batch.
func firstErr(first, err error) error {
	if first != nil {
		return first
	}
	return err
}

func ids(in []entity.ID) []uuid.UUID {
	out := make([]uuid.UUID, len(in))
	for i, id := range in {
		out[i] = uuid.UUID(id)
	}
	return out
}

func recordNo(no int) (int32, error) {
	if no < 1 || no > math.MaxInt32 {
		return 0, errRecordNo
	}
	return int32(no), nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func textPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func strPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

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

func nullID(id *entity.ID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*id), Valid: true}
}

func idPtr(id uuid.NullUUID) *entity.ID {
	if !id.Valid {
		return nil
	}
	out := entity.ID(id.UUID)
	return &out
}

func knowledge(known int64, seq pgtype.Int8, at pgtype.Timestamptz) entity.Knowledge {
	k := entity.Knowledge{KnownSeq: known, SupersededAt: timePtr(at)}
	if seq.Valid {
		k.SupersededSeq = &seq.Int64
	}
	return k
}
