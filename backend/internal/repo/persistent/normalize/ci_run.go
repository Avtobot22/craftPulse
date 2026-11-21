package normalize

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *Repo) CurrentCIRunVersions(ctx context.Context, orgID entity.ID, objectIDs []entity.ID) ([]entity.CIRunVersion, error) {
	rows, err := r.q(ctx).CurrentCIRunVersions(ctx, CurrentCIRunVersionsParams{OrganizationID: uuid.UUID(orgID), ObjectIds: ids(objectIDs)})
	if err != nil {
		return nil, fmt.Errorf("normalize: current CI runs: %w", err)
	}
	return mapCIRunVersions(rows), nil
}

func (r *Repo) KnownCIRunVersions(ctx context.Context, orgID entity.ID, objectIDs []entity.ID) (map[entity.ID][]entity.Revision, error) {
	rows, err := r.q(ctx).KnownCIRunVersions(ctx, KnownCIRunVersionsParams{OrganizationID: uuid.UUID(orgID), ObjectIds: ids(objectIDs)})
	if err != nil {
		return nil, fmt.Errorf("normalize: known CI runs: %w", err)
	}
	out := make(map[entity.ID][]entity.Revision, len(rows))
	for _, row := range rows {
		key := entity.ID(row.RunID)
		out[key] = append(out[key], entity.Revision{Source: row.SourceRevision, Hash: row.ContentHash})
	}
	return out, nil
}

func (r *Repo) SupersedeCIRunVersions(ctx context.Context, orgID entity.ID, versions []entity.ID, seq int64, at time.Time) error {
	if len(versions) == 0 {
		return nil
	}
	err := r.q(ctx).SupersedeCIRunVersions(ctx, SupersedeCIRunVersionsParams{OrganizationID: uuid.UUID(orgID), Ids: ids(versions), KnownSeq: seq, At: at})
	if err != nil {
		return fmt.Errorf("normalize: supersede CI runs: %w", err)
	}
	return nil
}

func (r *Repo) InsertCIRunVersions(ctx context.Context, orgID entity.ID, versions []entity.CIRunVersion) error {
	params := make([]InsertCIRunVersionsParams, len(versions))
	for i := range versions {
		v := &versions[i]
		no, err := recordNo(v.Record.RecordNo)
		if err != nil {
			return err
		}
		params[i] = InsertCIRunVersionsParams{
			OrganizationID: uuid.UUID(orgID), RunID: uuid.UUID(v.RunID), AreaID: uuid.UUID(v.AreaID), MrID: nullID(v.MRID),
			SourceUpdatedAt: tstz(v.SourceUpdatedAt), RefName: v.RefName, Sha: v.SHA, ParentRunID: nullID(v.ParentRunID),
			DefinitionVersion: text(v.DefinitionVersion), NativeStatus: v.NativeStatus, Outcome: string(v.Outcome),
			StartedAt: tstz(v.StartedAt), FinishedAt: tstz(v.FinishedAt), AttemptHistory: string(v.AttemptHistory),
			KnownSeq: v.KnownSeq, ReceiptID: uuid.UUID(v.Record.ReceiptID), RecordNo: no, SourceRevision: v.Revision.Source,
			ContentHash: v.Revision.Hash, ObservedAt: v.ObservedAt,
		}
	}
	var batchErr error
	r.q(ctx).InsertCIRunVersions(ctx, params).Exec(func(_ int, err error) { batchErr = firstErr(batchErr, err) })
	if batchErr != nil {
		return fmt.Errorf("normalize: insert CI runs: %w", batchErr)
	}
	return nil
}

func mapCIRunVersions(rows []NormalizeCiRunVersion) []entity.CIRunVersion {
	out := make([]entity.CIRunVersion, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.CIRunVersion{
			ID: entity.ID(row.ID), RunID: entity.ID(row.RunID), AreaID: entity.ID(row.AreaID), MRID: idPtr(row.MrID),
			Knowledge: knowledge(row.KnownSeq, row.SupersededSeq, row.SupersededAt),
			Record:    entity.RecordRef{ReceiptID: entity.ID(row.ReceiptID), RecordNo: int(row.RecordNo)},
			Revision:  entity.Revision{Source: row.SourceRevision, Hash: row.ContentHash}, ObservedAt: row.ObservedAt,
			SourceUpdatedAt: timePtr(row.SourceUpdatedAt), RefName: row.RefName, SHA: row.Sha, ParentRunID: idPtr(row.ParentRunID),
			DefinitionVersion: row.DefinitionVersion.String, NativeStatus: row.NativeStatus, Outcome: entity.CIOutcome(row.Outcome),
			StartedAt: timePtr(row.StartedAt), FinishedAt: timePtr(row.FinishedAt), AttemptHistory: entity.AttemptHistory(row.AttemptHistory),
		}
	}
	return out
}

func (r *Repo) CurrentCIAttemptVersions(ctx context.Context, orgID entity.ID, objectIDs []entity.ID) ([]entity.CIAttemptVersion, error) {
	rows, err := r.q(ctx).CurrentCIAttemptVersions(ctx, CurrentCIAttemptVersionsParams{OrganizationID: uuid.UUID(orgID), ObjectIds: ids(objectIDs)})
	if err != nil {
		return nil, fmt.Errorf("normalize: current CI attempts: %w", err)
	}
	return mapCIAttemptVersions(rows), nil
}

func (r *Repo) KnownCIAttemptVersions(ctx context.Context, orgID entity.ID, objectIDs []entity.ID) (map[entity.ID][]entity.Revision, error) {
	rows, err := r.q(ctx).KnownCIAttemptVersions(ctx, KnownCIAttemptVersionsParams{OrganizationID: uuid.UUID(orgID), ObjectIds: ids(objectIDs)})
	if err != nil {
		return nil, fmt.Errorf("normalize: known CI attempts: %w", err)
	}
	out := make(map[entity.ID][]entity.Revision, len(rows))
	for _, row := range rows {
		key := entity.ID(row.AttemptID)
		out[key] = append(out[key], entity.Revision{Source: row.SourceRevision, Hash: row.ContentHash})
	}
	return out, nil
}

func (r *Repo) SupersedeCIAttemptVersions(ctx context.Context, orgID entity.ID, versions []entity.ID, seq int64, at time.Time) error {
	if len(versions) == 0 {
		return nil
	}
	err := r.q(ctx).SupersedeCIAttemptVersions(ctx, SupersedeCIAttemptVersionsParams{OrganizationID: uuid.UUID(orgID), Ids: ids(versions), KnownSeq: seq, At: at})
	if err != nil {
		return fmt.Errorf("normalize: supersede CI attempts: %w", err)
	}
	return nil
}

func (r *Repo) InsertCIAttemptVersions(ctx context.Context, orgID entity.ID, versions []entity.CIAttemptVersion) error {
	params := make([]InsertCIAttemptVersionsParams, len(versions))
	for i := range versions {
		v := &versions[i]
		no, err := recordNo(v.Record.RecordNo)
		if err != nil {
			return err
		}
		if v.AttemptNo < 1 || v.AttemptNo > math.MaxInt32 {
			return fmt.Errorf("normalize: attempt number %d is out of range", v.AttemptNo)
		}
		params[i] = InsertCIAttemptVersionsParams{
			OrganizationID: uuid.UUID(orgID), AttemptID: uuid.UUID(v.AttemptID), RunID: uuid.UUID(v.RunID), JobKey: v.JobKey,
			ConfigFingerprint: v.ConfigFingerprint, AttemptNo: int32(v.AttemptNo), NativeStatus: v.NativeStatus, Outcome: string(v.Outcome),
			AllowFailure: v.AllowFailure, FailureKind: text(v.FailureKind), RunnerClass: text(v.RunnerClass),
			StartedAt: tstz(v.StartedAt), FinishedAt: tstz(v.FinishedAt), DurationS: int8Of(v.DurationS),
			KnownSeq: v.KnownSeq, ReceiptID: uuid.UUID(v.Record.ReceiptID), RecordNo: no, SourceRevision: v.Revision.Source,
			ContentHash: v.Revision.Hash, ObservedAt: v.ObservedAt,
		}
	}
	var batchErr error
	r.q(ctx).InsertCIAttemptVersions(ctx, params).Exec(func(_ int, err error) { batchErr = firstErr(batchErr, err) })
	if batchErr != nil {
		return fmt.Errorf("normalize: insert CI attempts: %w", batchErr)
	}
	return nil
}

func mapCIAttemptVersions(rows []NormalizeCiAttemptVersion) []entity.CIAttemptVersion {
	out := make([]entity.CIAttemptVersion, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.CIAttemptVersion{
			ID: entity.ID(row.ID), AttemptID: entity.ID(row.AttemptID), RunID: entity.ID(row.RunID),
			Knowledge: knowledge(row.KnownSeq, row.SupersededSeq, row.SupersededAt),
			Record:    entity.RecordRef{ReceiptID: entity.ID(row.ReceiptID), RecordNo: int(row.RecordNo)},
			Revision:  entity.Revision{Source: row.SourceRevision, Hash: row.ContentHash}, ObservedAt: row.ObservedAt,
			JobKey: row.JobKey, ConfigFingerprint: row.ConfigFingerprint, AttemptNo: int(row.AttemptNo), NativeStatus: row.NativeStatus,
			Outcome: entity.CIOutcome(row.Outcome), AllowFailure: row.AllowFailure, FailureKind: row.FailureKind.String,
			RunnerClass: row.RunnerClass.String, StartedAt: timePtr(row.StartedAt), FinishedAt: timePtr(row.FinishedAt),
		}
		if row.DurationS.Valid {
			d := row.DurationS.Int64
			out[i].DurationS = &d
		}
	}
	return out
}

func (r *Repo) MatchRunMRs(ctx context.Context, orgID, instanceID entity.ID, matches []entity.RunMRMatch) (map[int]entity.ID, error) {
	if len(matches) == 0 {
		return nil, nil
	}
	p := MatchRunMRsParams{
		OrganizationID: uuid.UUID(orgID), InstanceID: uuid.UUID(instanceID),
		MrNativeIds: make([]string, len(matches)), AreaIds: make([]uuid.UUID, len(matches)), Shas: make([]string, len(matches)),
	}
	for i, m := range matches {
		p.MrNativeIds[i], p.AreaIds[i], p.Shas[i] = m.MRNativeID, uuid.UUID(m.AreaID), m.SHA
	}
	rows, err := r.q(ctx).MatchRunMRs(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("normalize: match run MRs: %w", err)
	}
	out := make(map[int]entity.ID, len(rows))
	for _, row := range rows {
		out[int(row.Ord)-1] = entity.ID(row.ID)
	}
	return out, nil
}

func (r *Repo) ReadCIRunVersions(ctx context.Context, orgID entity.ID, areas, allowed, runs []entity.ID, seq int64) ([]entity.CIRunVersion, error) {
	rows, err := r.q(ctx).ReadCIRunVersions(ctx, ReadCIRunVersionsParams{
		OrganizationID: uuid.UUID(orgID), Areas: ids(areas), Allowed: ids(allowed), RunIds: ids(runs), KnownSeq: seq,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: read CI runs: %w", err)
	}
	return mapCIRunVersions(rows), nil
}

func (r *Repo) FindCIRunIDs(ctx context.Context, orgID entity.ID, areas, allowed []entity.ID, q entity.RunsQuery, seq int64) ([]entity.ID, error) {
	p := FindCIRunIDsParams{
		OrganizationID: uuid.UUID(orgID), Areas: ids(areas), Allowed: ids(allowed), KnownSeq: seq,
		MrID: nullID(q.MRID), StartedFrom: tstz(q.From), StartedBefore: tstz(q.Before),
	}
	if q.Parents != nil {
		p.ParentRunIds = ids(q.Parents)
	}
	rows, err := r.q(ctx).FindCIRunIDs(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("normalize: find CI runs: %w", err)
	}
	return entityIDs(rows), nil
}

func (r *Repo) ReadCIAttemptVersions(ctx context.Context, orgID entity.ID, areas, allowed, runs []entity.ID, seq int64) ([]entity.CIAttemptVersion, error) {
	rows, err := r.q(ctx).ReadCIAttemptVersions(ctx, ReadCIAttemptVersionsParams{
		OrganizationID: uuid.UUID(orgID), Areas: ids(areas), Allowed: ids(allowed), RunIds: ids(runs), KnownSeq: seq,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: read CI attempts: %w", err)
	}
	return mapCIAttemptVersions(rows), nil
}

// RunsWithAttempts are the runs among runIDs with a current attempt of a job other than except.
func (r *Repo) RunsWithAttempts(ctx context.Context, orgID entity.ID, runIDs []entity.ID, except string) (map[entity.ID]bool, error) {
	if len(runIDs) == 0 {
		return nil, nil
	}
	rows, err := r.q(ctx).RunsWithAttempts(ctx, RunsWithAttemptsParams{OrganizationID: uuid.UUID(orgID), RunIds: ids(runIDs), ExceptJobKey: except})
	if err != nil {
		return nil, fmt.Errorf("normalize: runs with attempts: %w", err)
	}
	out := make(map[entity.ID]bool, len(rows))
	for _, id := range rows {
		out[entity.ID(id)] = true
	}
	return out, nil
}

func int8Of(p *int64) pgtype.Int8 {
	if p == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *p, Valid: true}
}
