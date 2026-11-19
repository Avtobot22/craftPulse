package normalize

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) CurrentRepositoryVersions(ctx context.Context, orgID entity.ID, objectIDs []entity.ID) ([]entity.RepositoryVersion, error) {
	rows, err := r.q(ctx).CurrentRepositoryVersions(ctx, CurrentRepositoryVersionsParams{
		OrganizationID: uuid.UUID(orgID),
		ObjectIds:      ids(objectIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: current RepositoryVersions: %w", err)
	}
	return mapRepositoryVersions(rows), nil
}

func (r *Repo) KnownRepositoryVersions(ctx context.Context, orgID entity.ID, objectIDs []entity.ID) (map[entity.ID][]entity.Revision, error) {
	rows, err := r.q(ctx).KnownRepositoryVersions(ctx, KnownRepositoryVersionsParams{
		OrganizationID: uuid.UUID(orgID),
		ObjectIds:      ids(objectIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: known RepositoryVersions: %w", err)
	}
	out := make(map[entity.ID][]entity.Revision, len(rows))
	for _, row := range rows {
		key := entity.ID(row.RepositoryID)
		out[key] = append(out[key], entity.Revision{
			Source: row.SourceRevision,
			Hash:   row.ContentHash,
		})
	}
	return out, nil
}

func (r *Repo) SupersedeRepositoryVersions(ctx context.Context, orgID entity.ID, versions []entity.ID, seq int64, at time.Time) error {
	if len(versions) == 0 {
		return nil
	}
	if err := r.q(ctx).SupersedeRepositoryVersions(ctx, SupersedeRepositoryVersionsParams{
		OrganizationID: uuid.UUID(orgID),
		Ids:            ids(versions),
		KnownSeq:       seq,
		At:             at,
	}); err != nil {
		return fmt.Errorf("normalize: supersede RepositoryVersions: %w", err)
	}
	return nil
}

func (r *Repo) InsertRepositoryVersions(ctx context.Context, orgID entity.ID, versions []entity.RepositoryVersion) error {
	params := make([]InsertRepositoryVersionsParams, len(versions))
	for i := range versions {
		v := &versions[i]
		no, err := recordNo(v.Record.RecordNo)
		if err != nil {
			return err
		}
		params[i] = InsertRepositoryVersionsParams{
			OrganizationID:  uuid.UUID(orgID),
			RepositoryID:    uuid.UUID(v.RepositoryID),
			SourceUpdatedAt: tstz(v.SourceUpdatedAt),
			Path:            v.Path,
			Name:            v.Name,
			DefaultBranch:   text(v.DefaultBranch),
			Visibility:      v.Visibility,
			WebUrl:          v.WebURL,
			KnownSeq:        v.KnownSeq,
			ReceiptID:       uuid.UUID(v.Record.ReceiptID),
			RecordNo:        no,
			SourceRevision:  v.Revision.Source,
			ContentHash:     v.Revision.Hash,
			ObservedAt:      v.ObservedAt,
		}
	}
	var batchErr error
	r.q(ctx).InsertRepositoryVersions(ctx, params).Exec(func(_ int, err error) { batchErr = firstErr(batchErr, err) })
	if batchErr != nil {
		return fmt.Errorf("normalize: insert RepositoryVersions: %w", batchErr)
	}
	return nil
}

func mapRepositoryVersions(rows []NormalizeRepositoryVersion) []entity.RepositoryVersion {
	out := make([]entity.RepositoryVersion, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.RepositoryVersion{
			ID:        entity.ID(row.ID),
			Knowledge: knowledge(row.KnownSeq, row.SupersededSeq, row.SupersededAt),
			Record: entity.RecordRef{
				ReceiptID: entity.ID(row.ReceiptID),
				RecordNo:  int(row.RecordNo),
			},
			Revision: entity.Revision{
				Source: row.SourceRevision,
				Hash:   row.ContentHash,
			},
			ObservedAt:      row.ObservedAt,
			RepositoryID:    entity.ID(row.RepositoryID),
			SourceUpdatedAt: timePtr(row.SourceUpdatedAt),
			Path:            row.Path,
			Name:            row.Name,
			DefaultBranch:   row.DefaultBranch.String,
			Visibility:      row.Visibility,
			WebURL:          row.WebUrl,
		}
	}
	return out
}

func (r *Repo) CurrentMRVersions(ctx context.Context, orgID entity.ID, objectIDs []entity.ID) ([]entity.MRVersion, error) {
	rows, err := r.q(ctx).CurrentMRVersions(ctx, CurrentMRVersionsParams{
		OrganizationID: uuid.UUID(orgID),
		ObjectIds:      ids(objectIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: current MRVersions: %w", err)
	}
	return mapMRVersions(rows), nil
}

func (r *Repo) KnownMRVersions(ctx context.Context, orgID entity.ID, objectIDs []entity.ID) (map[entity.ID][]entity.Revision, error) {
	rows, err := r.q(ctx).KnownMRVersions(ctx, KnownMRVersionsParams{
		OrganizationID: uuid.UUID(orgID),
		ObjectIds:      ids(objectIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: known MRVersions: %w", err)
	}
	out := make(map[entity.ID][]entity.Revision, len(rows))
	for _, row := range rows {
		key := entity.ID(row.MrID)
		out[key] = append(out[key], entity.Revision{
			Source: row.SourceRevision,
			Hash:   row.ContentHash,
		})
	}
	return out, nil
}

func (r *Repo) SupersedeMRVersions(ctx context.Context, orgID entity.ID, versions []entity.ID, seq int64, at time.Time) error {
	if len(versions) == 0 {
		return nil
	}
	if err := r.q(ctx).SupersedeMRVersions(ctx, SupersedeMRVersionsParams{
		OrganizationID: uuid.UUID(orgID),
		Ids:            ids(versions),
		KnownSeq:       seq,
		At:             at,
	}); err != nil {
		return fmt.Errorf("normalize: supersede MRVersions: %w", err)
	}
	return nil
}

func (r *Repo) InsertMRVersions(ctx context.Context, orgID entity.ID, versions []entity.MRVersion) error {
	params := make([]InsertMRVersionsParams, len(versions))
	for i := range versions {
		v := &versions[i]
		no, err := recordNo(v.Record.RecordNo)
		if err != nil {
			return err
		}
		params[i] = InsertMRVersionsParams{
			OrganizationID:     uuid.UUID(orgID),
			MrID:               uuid.UUID(v.MRID),
			AreaID:             uuid.UUID(v.AreaID),
			SourceUpdatedAt:    tstz(v.SourceUpdatedAt),
			Iid:                intValue(v.IID),
			Title:              text(v.Title),
			State:              v.State,
			Draft:              v.Draft,
			AuthorAccountID:    nullID(v.AuthorAccountID),
			SourceBranch:       v.SourceBranch,
			TargetBranch:       v.TargetBranch,
			HeadSha:            v.HeadSHA,
			BaseSha:            text(v.BaseSHA),
			MergeBaseSha:       text(v.MergeBaseSHA),
			ReviewerField:      v.ReviewerField,
			ReviewerAccountIds: ids(v.ReviewerAccountIDs),
			OpenedAt:           v.OpenedAt,
			ReadyAt:            tstz(v.ReadyAt),
			MergedAt:           tstz(v.MergedAt),
			ClosedAt:           tstz(v.ClosedAt),
			KnownSeq:           v.KnownSeq,
			ReceiptID:          uuid.UUID(v.Record.ReceiptID),
			RecordNo:           no,
			SourceRevision:     v.Revision.Source,
			ContentHash:        v.Revision.Hash,
			ObservedAt:         v.ObservedAt,
		}
	}
	var batchErr error
	r.q(ctx).InsertMRVersions(ctx, params).Exec(func(_ int, err error) { batchErr = firstErr(batchErr, err) })
	if batchErr != nil {
		return fmt.Errorf("normalize: insert MRVersions: %w", batchErr)
	}
	return nil
}

func mapMRVersions(rows []NormalizeMrVersion) []entity.MRVersion {
	out := make([]entity.MRVersion, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.MRVersion{
			ID:        entity.ID(row.ID),
			Knowledge: knowledge(row.KnownSeq, row.SupersededSeq, row.SupersededAt),
			Record: entity.RecordRef{
				ReceiptID: entity.ID(row.ReceiptID),
				RecordNo:  int(row.RecordNo),
			},
			Revision: entity.Revision{
				Source: row.SourceRevision,
				Hash:   row.ContentHash,
			},
			ObservedAt:         row.ObservedAt,
			MRID:               entity.ID(row.MrID),
			AreaID:             entity.ID(row.AreaID),
			SourceUpdatedAt:    timePtr(row.SourceUpdatedAt),
			IID:                intPtr(row.Iid),
			Title:              row.Title.String,
			State:              row.State,
			Draft:              row.Draft,
			AuthorAccountID:    idPtr(row.AuthorAccountID),
			SourceBranch:       row.SourceBranch,
			TargetBranch:       row.TargetBranch,
			HeadSHA:            row.HeadSha,
			BaseSHA:            row.BaseSha.String,
			MergeBaseSHA:       row.MergeBaseSha.String,
			ReviewerField:      row.ReviewerField,
			ReviewerAccountIDs: entityIDs(row.ReviewerAccountIds),
			OpenedAt:           row.OpenedAt,
			ReadyAt:            timePtr(row.ReadyAt),
			MergedAt:           timePtr(row.MergedAt),
			ClosedAt:           timePtr(row.ClosedAt),
		}
	}
	return out
}

func (r *Repo) CurrentMREvents(ctx context.Context, orgID entity.ID, keys []entity.MRFactKey) ([]entity.MREvent, error) {
	mrIDs, factKeys := mrFactParams(keys)
	rows, err := r.q(ctx).CurrentMREvents(ctx, CurrentMREventsParams{
		OrganizationID: uuid.UUID(orgID),
		MrIds:          mrIDs,
		FactKeys:       factKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: current MREvents: %w", err)
	}
	return mapMREvents(rows), nil
}

func (r *Repo) KnownMREvents(ctx context.Context, orgID entity.ID, keys []entity.MRFactKey) (map[entity.MRFactKey][]entity.Revision, error) {
	mrIDs, factKeys := mrFactParams(keys)
	rows, err := r.q(ctx).KnownMREvents(ctx, KnownMREventsParams{
		OrganizationID: uuid.UUID(orgID),
		MrIds:          mrIDs,
		FactKeys:       factKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: known MREvents: %w", err)
	}
	out := make(map[entity.MRFactKey][]entity.Revision, len(rows))
	for _, row := range rows {
		key := entity.MRFactKey{
			MRID: entity.ID(row.MrID),
			Key:  row.EventKey,
		}
		out[key] = append(out[key], entity.Revision{
			Source: row.SourceRevision,
			Hash:   row.ContentHash,
		})
	}
	return out, nil
}

func (r *Repo) SupersedeMREvents(ctx context.Context, orgID entity.ID, versions []entity.ID, seq int64, at time.Time) error {
	if len(versions) == 0 {
		return nil
	}
	if err := r.q(ctx).SupersedeMREvents(ctx, SupersedeMREventsParams{
		OrganizationID: uuid.UUID(orgID),
		Ids:            ids(versions),
		KnownSeq:       seq,
		At:             at,
	}); err != nil {
		return fmt.Errorf("normalize: supersede MREvents: %w", err)
	}
	return nil
}

func (r *Repo) InsertMREvents(ctx context.Context, orgID entity.ID, versions []entity.MREvent) error {
	params := make([]InsertMREventsParams, len(versions))
	for i := range versions {
		v := &versions[i]
		no, err := recordNo(v.Record.RecordNo)
		if err != nil {
			return err
		}
		params[i] = InsertMREventsParams{
			OrganizationID:  uuid.UUID(orgID),
			MrID:            uuid.UUID(v.MRID),
			EventKey:        v.EventKey,
			EventKind:       v.Kind,
			ActorAccountID:  nullID(v.ActorAccountID),
			TargetAccountID: nullID(v.TargetAccountID),
			HeadSha:         text(v.HeadSHA),
			ReviewState:     text(v.ReviewState),
			EventTime:       v.EventTime,
			EventOffsetS:    int32(v.EventOffsetS), //nolint:gosec // validated envelope offsets are bounded to 18 hours
			ObservedVia:     string(v.ObservedVia),
			KnownSeq:        v.KnownSeq,
			ReceiptID:       uuid.UUID(v.Record.ReceiptID),
			RecordNo:        no,
			SourceRevision:  v.Revision.Source,
			ContentHash:     v.Revision.Hash,
			ObservedAt:      v.ObservedAt,
		}
	}
	var batchErr error
	r.q(ctx).InsertMREvents(ctx, params).Exec(func(_ int, err error) { batchErr = firstErr(batchErr, err) })
	if batchErr != nil {
		return fmt.Errorf("normalize: insert MREvents: %w", batchErr)
	}
	return nil
}

func mapMREvents(rows []NormalizeMrEvent) []entity.MREvent {
	out := make([]entity.MREvent, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.MREvent{
			ID:        entity.ID(row.ID),
			Knowledge: knowledge(row.KnownSeq, row.SupersededSeq, row.SupersededAt),
			Record: entity.RecordRef{
				ReceiptID: entity.ID(row.ReceiptID),
				RecordNo:  int(row.RecordNo),
			},
			Revision: entity.Revision{
				Source: row.SourceRevision,
				Hash:   row.ContentHash,
			},
			ObservedAt:      row.ObservedAt,
			MRID:            entity.ID(row.MrID),
			EventKey:        row.EventKey,
			Kind:            row.EventKind,
			ActorAccountID:  idPtr(row.ActorAccountID),
			TargetAccountID: idPtr(row.TargetAccountID),
			HeadSHA:         row.HeadSha.String,
			ReviewState:     row.ReviewState.String,
			EventTime:       row.EventTime,
			EventOffsetS:    int(row.EventOffsetS),
			ObservedVia:     entity.ObservedVia(row.ObservedVia),
			ActorKind:       entity.PersonUnknown,
		}
	}
	return out
}

func (r *Repo) CurrentMRRevisions(ctx context.Context, orgID entity.ID, keys []entity.MRFactKey) ([]entity.MRRevision, error) {
	mrIDs, factKeys := mrFactParams(keys)
	rows, err := r.q(ctx).CurrentMRRevisions(ctx, CurrentMRRevisionsParams{
		OrganizationID: uuid.UUID(orgID),
		MrIds:          mrIDs,
		FactKeys:       factKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: current MRRevisions: %w", err)
	}
	return mapMRRevisions(rows), nil
}

func (r *Repo) KnownMRRevisions(ctx context.Context, orgID entity.ID, keys []entity.MRFactKey) (map[entity.MRFactKey][]entity.Revision, error) {
	mrIDs, factKeys := mrFactParams(keys)
	rows, err := r.q(ctx).KnownMRRevisions(ctx, KnownMRRevisionsParams{
		OrganizationID: uuid.UUID(orgID),
		MrIds:          mrIDs,
		FactKeys:       factKeys,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: known MRRevisions: %w", err)
	}
	out := make(map[entity.MRFactKey][]entity.Revision, len(rows))
	for _, row := range rows {
		key := entity.MRFactKey{
			MRID: entity.ID(row.MrID),
			Key:  row.RevisionKey,
		}
		out[key] = append(out[key], entity.Revision{
			Source: row.SourceRevision,
			Hash:   row.ContentHash,
		})
	}
	return out, nil
}

func (r *Repo) SupersedeMRRevisions(ctx context.Context, orgID entity.ID, versions []entity.ID, seq int64, at time.Time) error {
	if len(versions) == 0 {
		return nil
	}
	if err := r.q(ctx).SupersedeMRRevisions(ctx, SupersedeMRRevisionsParams{
		OrganizationID: uuid.UUID(orgID),
		Ids:            ids(versions),
		KnownSeq:       seq,
		At:             at,
	}); err != nil {
		return fmt.Errorf("normalize: supersede MRRevisions: %w", err)
	}
	return nil
}

func (r *Repo) InsertMRRevisions(ctx context.Context, orgID entity.ID, versions []entity.MRRevision) error {
	params := make([]InsertMRRevisionsParams, len(versions))
	for i := range versions {
		v := &versions[i]
		no, err := recordNo(v.Record.RecordNo)
		if err != nil {
			return err
		}
		params[i] = InsertMRRevisionsParams{
			OrganizationID: uuid.UUID(orgID),
			MrID:           uuid.UUID(v.MRID),
			RevisionKey:    v.RevisionKey,
			HeadSha:        v.HeadSHA,
			BaseSha:        text(v.BaseSHA),
			StartSha:       text(v.StartSHA),
			CreatedAt:      v.CreatedAt,
			KnownSeq:       v.KnownSeq,
			ReceiptID:      uuid.UUID(v.Record.ReceiptID),
			RecordNo:       no,
			SourceRevision: v.Revision.Source,
			ContentHash:    v.Revision.Hash,
			ObservedAt:     v.ObservedAt,
		}
	}
	var batchErr error
	r.q(ctx).InsertMRRevisions(ctx, params).Exec(func(_ int, err error) { batchErr = firstErr(batchErr, err) })
	if batchErr != nil {
		return fmt.Errorf("normalize: insert MRRevisions: %w", batchErr)
	}
	return nil
}

func mapMRRevisions(rows []NormalizeMrRevision) []entity.MRRevision {
	out := make([]entity.MRRevision, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.MRRevision{
			ID:        entity.ID(row.ID),
			Knowledge: knowledge(row.KnownSeq, row.SupersededSeq, row.SupersededAt),
			Record: entity.RecordRef{
				ReceiptID: entity.ID(row.ReceiptID),
				RecordNo:  int(row.RecordNo),
			},
			Revision: entity.Revision{
				Source: row.SourceRevision,
				Hash:   row.ContentHash,
			},
			ObservedAt:  row.ObservedAt,
			MRID:        entity.ID(row.MrID),
			RevisionKey: row.RevisionKey,
			HeadSHA:     row.HeadSha,
			BaseSHA:     row.BaseSha.String,
			StartSHA:    row.StartSha.String,
			CreatedAt:   row.CreatedAt,
		}
	}
	return out
}

func (r *Repo) ReadMRVersions(ctx context.Context, orgID entity.ID, areas, allowed, mrs []entity.ID, seq int64) ([]entity.MRVersion, error) {
	rows, err := r.q(ctx).ReadMRVersions(ctx, ReadMRVersionsParams{
		OrganizationID: uuid.UUID(orgID),
		Areas:          ids(areas),
		Allowed:        ids(allowed),
		MrIds:          ids(mrs),
		KnownSeq:       seq,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: read MRVersions: %w", err)
	}
	return mapMRVersions(rows), nil
}

func (r *Repo) ReadMRRevisions(ctx context.Context, orgID entity.ID, areas, allowed, mrs []entity.ID, seq int64) ([]entity.MRRevision, error) {
	rows, err := r.q(ctx).ReadMRRevisions(ctx, ReadMRRevisionsParams{
		OrganizationID: uuid.UUID(orgID),
		Areas:          ids(areas),
		Allowed:        ids(allowed),
		MrIds:          ids(mrs),
		KnownSeq:       seq,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: read MRRevisions: %w", err)
	}
	return mapMRRevisions(rows), nil
}

func (r *Repo) ReadMREvents(ctx context.Context, orgID entity.ID, areas, allowed, mrs []entity.ID, seq int64) ([]entity.MREvent, error) {
	rows, err := r.q(ctx).ReadMREvents(ctx, ReadMREventsParams{
		OrganizationID: uuid.UUID(orgID),
		Areas:          ids(areas),
		Allowed:        ids(allowed),
		MrIds:          ids(mrs),
		KnownSeq:       seq,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: read MREvents: %w", err)
	}
	return mapMREvents(rows), nil
}

func (r *Repo) FindMRIDs(ctx context.Context, orgID entity.ID, areas, allowed []entity.ID, key entity.SourceKey, sha string, seq int64) ([]entity.ID, error) {
	rows, err := r.q(ctx).FindMRIDs(ctx, FindMRIDsParams{
		OrganizationID: uuid.UUID(orgID),
		Areas:          ids(areas),
		Allowed:        ids(allowed),
		InstanceID:     uuid.UUID(key.InstanceID),
		NativeNs:       key.NativeNS,
		NativeID:       key.NativeID,
		HeadSha:        sha,
		KnownSeq:       seq,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: find MR IDs: %w", err)
	}
	return entityIDs(rows), nil
}

func (r *Repo) ReadRepositoryVersion(ctx context.Context, orgID, repositoryID entity.ID) (entity.RepositoryVersion, error) {
	row, err := r.q(ctx).ReadRepositoryVersion(ctx, ReadRepositoryVersionParams{
		OrganizationID: uuid.UUID(orgID),
		RepositoryID:   uuid.UUID(repositoryID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.RepositoryVersion{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.RepositoryVersion{}, fmt.Errorf("normalize: read repository: %w", err)
	}
	return mapRepositoryVersions([]NormalizeRepositoryVersion{row})[0], nil
}

func entityIDs(in []uuid.UUID) []entity.ID {
	out := make([]entity.ID, len(in))
	for i, id := range in {
		out[i] = entity.ID(id)
	}
	return out
}

func intValue(p *int) pgtype.Int8 {
	if p == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{
		Int64: int64(*p),
		Valid: true,
	}
}

func intPtr(p pgtype.Int8) *int {
	if !p.Valid {
		return nil
	}
	n := int(p.Int64)
	return &n
}

func mrFactParams(keys []entity.MRFactKey) (mrIDs []uuid.UUID, factKeys []string) {
	mrIDs, factKeys = make([]uuid.UUID, len(keys)), make([]string, len(keys))
	for i, key := range keys {
		mrIDs[i], factKeys[i] = uuid.UUID(key.MRID), key.Key
	}
	return mrIDs, factKeys
}
