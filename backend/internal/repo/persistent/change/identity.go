package change

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// IdentitiesByFingerprint are the canonical findings of an area with any of the fingerprints, by fingerprint.
func (r *Repo) IdentitiesByFingerprint(ctx context.Context, orgID, areaID entity.ID, method int, fingerprints [][]byte) (map[string]entity.ID, error) {
	m, err := methodVersion(method)
	if err != nil {
		return nil, err
	}
	rows, err := r.q(ctx).IdentitiesByFingerprint(ctx, IdentitiesByFingerprintParams{
		OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(areaID), MethodVersion: m, Fingerprints: fingerprints,
	})
	if err != nil {
		return nil, fmt.Errorf("change: identities by fingerprint: %w", err)
	}
	out := make(map[string]entity.ID, len(rows))
	for _, row := range rows {
		out[string(row.Fingerprint)] = entity.ID(row.ID)
	}
	return out, nil
}

// IdentitiesByContent are the canonical findings of an area with any of the content fingerprints, the oldest first.
func (r *Repo) IdentitiesByContent(ctx context.Context, orgID, areaID entity.ID, method int, contents [][]byte) ([]entity.FindingIdentity, error) {
	m, err := methodVersion(method)
	if err != nil {
		return nil, err
	}
	rows, err := r.q(ctx).IdentitiesByContent(ctx, IdentitiesByContentParams{
		OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(areaID), MethodVersion: m, ContentFingerprints: contents,
	})
	if err != nil {
		return nil, fmt.Errorf("change: identities by content: %w", err)
	}
	out := make([]entity.FindingIdentity, len(rows))
	for i, row := range rows {
		out[i] = entity.FindingIdentity{ID: entity.ID(row.ID), AreaID: areaID, MethodVersion: method, RuleID: row.RuleID, ContentFingerprint: row.ContentFingerprint}
	}
	return out, nil
}

// InsertFindingIdentity writes a canonical finding and gives its ID, or the ID of the one of the same fingerprint.
func (r *Repo) InsertFindingIdentity(ctx context.Context, orgID entity.ID, f *entity.FindingIdentity) (entity.ID, error) {
	m, err := methodVersion(f.MethodVersion)
	if err != nil {
		return entity.ID{}, err
	}
	id, err := r.q(ctx).InsertFindingIdentity(ctx, InsertFindingIdentityParams{
		OrganizationID: uuid.UUID(orgID), AreaID: uuid.UUID(f.AreaID), MethodVersion: m, Fingerprint: f.Fingerprint,
		ContentFingerprint: f.ContentFingerprint, RuleID: f.RuleID, FindingKind: f.Kind, FirstReportID: uuid.UUID(f.FirstReportID),
		FirstSeenAt: f.FirstSeenAt,
	})
	if err != nil {
		return entity.ID{}, fmt.Errorf("change: insert finding identity: %w", err)
	}
	return entity.ID(id), nil
}

// AssignedOccurrences are the findings of reports among ids that already have their canonical finding.
func (r *Repo) AssignedOccurrences(ctx context.Context, orgID entity.ID, occurrences []entity.ID) ([]entity.ID, error) {
	rows, err := r.q(ctx).AssignedOccurrences(ctx, AssignedOccurrencesParams{OrganizationID: uuid.UUID(orgID), OccurrenceIds: uuids(occurrences)})
	if err != nil {
		return nil, fmt.Errorf("change: assigned occurrences: %w", err)
	}
	out := make([]entity.ID, len(rows))
	for i, id := range rows {
		out[i] = entity.ID(id)
	}
	return out, nil
}

// InsertOccurrenceIdentities writes the canonical findings of the findings of one report.
func (r *Repo) InsertOccurrenceIdentities(ctx context.Context, orgID entity.ID, list []entity.OccurrenceIdentity, at time.Time, seq int64) error {
	if len(list) == 0 {
		return nil
	}
	p := InsertOccurrenceIdentitiesParams{
		OrganizationID: uuid.UUID(orgID), ReportedAt: at, KnownSeq: seq, OccurrenceIds: make([]uuid.UUID, len(list)),
		FindingIdentityIds: make([]uuid.UUID, len(list)), MatchKinds: make([]string, len(list)),
	}
	for i := range list {
		p.OccurrenceIds[i], p.FindingIdentityIds[i], p.MatchKinds[i] = uuid.UUID(list[i].OccurrenceID), uuid.UUID(list[i].FindingIdentityID), list[i].MatchKind
	}
	if err := r.q(ctx).InsertOccurrenceIdentities(ctx, p); err != nil {
		return fmt.Errorf("change: insert occurrence identities: %w", err)
	}
	return nil
}

// FindingIdentity is a canonical finding by its ID; entity.ErrNotFound without one.
func (r *Repo) FindingIdentity(ctx context.Context, orgID, id entity.ID) (entity.FindingIdentity, error) {
	row, err := r.q(ctx).FindingIdentity(ctx, FindingIdentityParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.FindingIdentity{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.FindingIdentity{}, fmt.Errorf("change: finding identity: %w", err)
	}
	return entity.FindingIdentity{
		ID: entity.ID(row.ID), AreaID: entity.ID(row.AreaID), MethodVersion: int(row.MethodVersion), Fingerprint: row.Fingerprint,
		ContentFingerprint: row.ContentFingerprint, RuleID: row.RuleID, Kind: row.FindingKind, FirstReportID: entity.ID(row.FirstReportID),
		FirstSeenAt: row.FirstSeenAt,
	}, nil
}

// OccurrenceIdentities are the canonical findings of the findings of reports among ids.
func (r *Repo) OccurrenceIdentities(ctx context.Context, orgID entity.ID, occurrences []entity.ID) ([]entity.OccurrenceIdentity, error) {
	rows, err := r.q(ctx).ReportIdentities(ctx, ReportIdentitiesParams{OrganizationID: uuid.UUID(orgID), OccurrenceIds: uuids(occurrences)})
	if err != nil {
		return nil, fmt.Errorf("change: occurrence identities: %w", err)
	}
	out := make([]entity.OccurrenceIdentity, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.OccurrenceIdentity{
			OccurrenceID: entity.ID(row.OccurrenceID), FindingIdentityID: entity.ID(row.FindingIdentityID), MatchKind: row.MatchKind,
			ReportedAt: row.ReportedAt, KnownSeq: row.KnownSeq,
		}
	}
	return out, nil
}

// LockCurrentBaseline is the baseline in force for a component, language and kind, locked to its transaction;
// entity.ErrNotFound without one.
func (r *Repo) LockCurrentBaseline(ctx context.Context, orgID, componentID entity.ID, language, kind string) (entity.Baseline, error) {
	row, err := r.q(ctx).LockCurrentBaseline(ctx, LockCurrentBaselineParams{
		OrganizationID: uuid.UUID(orgID), ComponentID: uuid.UUID(componentID), Language: language, AnalysisKind: kind,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Baseline{}, entity.ErrNotFound
	}
	if err != nil {
		return entity.Baseline{}, fmt.Errorf("change: current baseline: %w", err)
	}
	return baselineOf(row), nil
}

// ReplaceBaseline supersedes the baseline in force, when there is one, and writes the new one.
func (r *Repo) ReplaceBaseline(ctx context.Context, orgID entity.ID, old *entity.Baseline, b *entity.Baseline) error {
	q := r.q(ctx)
	if old != nil {
		if err := q.SupersedeBaseline(ctx, SupersedeBaselineParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(old.ID), SupersededAt: pgtype.Timestamptz{Time: b.EstablishedAt, Valid: true}}); err != nil {
			return fmt.Errorf("change: supersede baseline: %w", err)
		}
	}
	err := q.InsertBaseline(ctx, InsertBaselineParams{
		OrganizationID: uuid.UUID(orgID), ComponentID: uuid.UUID(b.ComponentID), Language: b.Language, AnalysisKind: b.AnalysisKind,
		ProfileVersionID: uuid.UUID(b.ProfileVersionID), ReportID: uuid.UUID(b.ReportID), EstablishedAt: b.EstablishedAt,
	})
	if err != nil {
		return fmt.Errorf("change: insert baseline: %w", err)
	}
	return nil
}

// CurrentBaselines are the baselines in force of the components.
func (r *Repo) CurrentBaselines(ctx context.Context, orgID entity.ID, components []entity.ID) ([]entity.Baseline, error) {
	rows, err := r.q(ctx).CurrentBaselines(ctx, CurrentBaselinesParams{OrganizationID: uuid.UUID(orgID), ComponentIds: uuids(components)})
	if err != nil {
		return nil, fmt.Errorf("change: current baselines: %w", err)
	}
	out := make([]entity.Baseline, len(rows))
	for i := range rows {
		out[i] = baselineOf(rows[i])
	}
	return out, nil
}

func baselineOf(row ChangeBaseline) entity.Baseline {
	b := entity.Baseline{
		ID: entity.ID(row.ID), ComponentID: entity.ID(row.ComponentID), Language: row.Language, AnalysisKind: row.AnalysisKind,
		ProfileVersionID: entity.ID(row.ProfileVersionID), ReportID: entity.ID(row.ReportID), EstablishedAt: row.EstablishedAt,
	}
	if row.SupersededAt.Valid {
		t := row.SupersededAt.Time
		b.SupersededAt = &t
	}
	return b
}

func methodVersion(v int) (int32, error) {
	if v < 1 || v > math.MaxInt32 {
		return 0, fmt.Errorf("change: method version %d is out of range", v)
	}
	return int32(v), nil
}

func uuids(in []entity.ID) []uuid.UUID {
	out := make([]uuid.UUID, len(in))
	for i, id := range in {
		out[i] = uuid.UUID(id)
	}
	return out
}
