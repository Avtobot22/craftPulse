package process

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
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var errOutOfRange = errors.New("process: number does not fit its column")

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) q(ctx context.Context) *Queries { return New(postgres.Conn(ctx, r.pool)) }

// TimezoneExists checks the name against pg_timezone_names, the zones the database computes local time by.
func (r *Repo) TimezoneExists(ctx context.Context, name string) (bool, error) {
	ok, err := r.q(ctx).TimezoneExists(ctx, name)
	if err != nil {
		return false, fmt.Errorf("process: timezone exists: %w", err)
	}
	return ok, nil
}

// LastSettingsSeq is the knowledge number of the last version of the mapping or the calendar of a team, 0 without one.
func (r *Repo) LastSettingsSeq(ctx context.Context, orgID, teamID entity.ID) (int64, error) {
	seq, err := r.q(ctx).LastSettingsSeq(ctx, LastSettingsSeqParams{OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID)})
	if err != nil {
		return 0, fmt.Errorf("process: last settings seq: %w", err)
	}
	return seq, nil
}

func (r *Repo) LatestMappingVersionNo(ctx context.Context, orgID, teamID entity.ID) (int, error) {
	no, err := r.q(ctx).LatestMappingVersionNo(ctx, LatestMappingVersionNoParams{OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID)})
	if err != nil {
		return 0, fmt.Errorf("process: latest mapping version: %w", err)
	}
	return int(no), nil
}

func (r *Repo) MappingVersionAt(ctx context.Context, orgID, teamID entity.ID, seq int64) (entity.MappingVersion, error) {
	row, err := r.q(ctx).MappingVersionAt(ctx, MappingVersionAtParams{OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID), KnownSeq: seq})
	if err != nil {
		return entity.MappingVersion{}, notFound("mapping version at", err)
	}
	return r.withRules(ctx, orgID, mappingVersion(&row.ProcessMappingVersion, row.CalendarVersionNo))
}

func (r *Repo) MappingVersionByConfirmation(ctx context.Context, orgID, confirmationID entity.ID) (entity.MappingVersion, error) {
	row, err := r.q(ctx).MappingVersionByConfirmation(ctx, MappingVersionByConfirmationParams{
		OrganizationID: uuid.UUID(orgID), ConfirmationID: uuid.UUID(confirmationID),
	})
	if err != nil {
		return entity.MappingVersion{}, notFound("mapping version by confirmation", err)
	}
	return r.withRules(ctx, orgID, mappingVersion(&row.ProcessMappingVersion, row.CalendarVersionNo))
}

// PageMappingVersions reads up to Limit+1 versions newest first, so the caller knows whether a page follows.
func (r *Repo) PageMappingVersions(ctx context.Context, orgID, teamID entity.ID, p entity.VersionPageRequest) ([]entity.MappingVersion, error) {
	params := PageMappingVersionsParams{OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID)}
	var err error
	if params.Before, params.RowLimit, err = pageParams(p); err != nil {
		return nil, err
	}
	rows, err := r.q(ctx).PageMappingVersions(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("process: page mapping versions: %w", err)
	}
	out := make([]entity.MappingVersion, len(rows))
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		out[i] = mappingVersion(&rows[i].ProcessMappingVersion, rows[i].CalendarVersionNo)
		ids[i] = rows[i].ProcessMappingVersion.ID
	}
	rules, err := r.rules(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Rules = rules[out[i].ID]
	}
	return out, nil
}

func (r *Repo) InsertMappingVersion(ctx context.Context, orgID entity.ID, v entity.MappingVersion) error {
	no, base, err := versionNumbers(v.VersionNo, v.BaseVersionNo)
	if err != nil {
		return err
	}
	q := r.q(ctx)
	err = q.InsertMappingVersion(ctx, InsertMappingVersionParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(v.ID), TeamID: uuid.UUID(v.TeamID), VersionNo: no, BaseVersionNo: base,
		ContentHash: v.ContentHash, Timezone: v.Timezone, CalendarVersionID: nullID(v.CalendarVersionID), KnownSeq: v.KnownSeq,
		ConfirmedBy: uuid.UUID(v.ConfirmedBy), ConfirmedAt: v.ConfirmedAt, ConfirmationID: uuid.UUID(v.ConfirmationID),
	})
	if err != nil {
		return fmt.Errorf("process: insert mapping version: %w", err)
	}
	if len(v.Rules) == 0 {
		return nil
	}
	params := make([]InsertMappingRuleParams, len(v.Rules))
	for i := range v.Rules {
		rule := &v.Rules[i]
		rank, err := smallint(rule.Rank)
		if err != nil {
			return err
		}
		params[i] = InsertMappingRuleParams{
			OrganizationID: uuid.UUID(orgID), MappingVersionID: uuid.UUID(v.ID), RuleKind: string(rule.Kind),
			InstanceID: uuid.UUID(rule.InstanceID), NativeRef: rule.NativeRef, NativeValue: rule.NativeValue, Meaning: rule.Meaning, Rank: rank,
		}
	}
	var batchErr error
	q.InsertMappingRule(ctx, params).Exec(func(_ int, err error) {
		if batchErr == nil {
			batchErr = err
		}
	})
	if batchErr != nil {
		return fmt.Errorf("process: insert mapping rules: %w", batchErr)
	}
	return nil
}

func (r *Repo) LatestCalendarVersionNo(ctx context.Context, orgID, teamID entity.ID) (int, error) {
	no, err := r.q(ctx).LatestCalendarVersionNo(ctx, LatestCalendarVersionNoParams{OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID)})
	if err != nil {
		return 0, fmt.Errorf("process: latest calendar version: %w", err)
	}
	return int(no), nil
}

func (r *Repo) CalendarVersionAt(ctx context.Context, orgID, teamID entity.ID, seq int64) (entity.CalendarVersion, error) {
	row, err := r.q(ctx).CalendarVersionAt(ctx, CalendarVersionAtParams{OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID), KnownSeq: seq})
	if err != nil {
		return entity.CalendarVersion{}, notFound("calendar version at", err)
	}
	return calendarVersion(&row), nil
}

func (r *Repo) CalendarVersionByNo(ctx context.Context, orgID, teamID entity.ID, no int) (entity.CalendarVersion, error) {
	if no < 1 || no > math.MaxInt32 {
		return entity.CalendarVersion{}, entity.ErrNotFound
	}
	row, err := r.q(ctx).CalendarVersionByNo(ctx, CalendarVersionByNoParams{
		OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID), VersionNo: int32(no),
	})
	if err != nil {
		return entity.CalendarVersion{}, notFound("calendar version by number", err)
	}
	return calendarVersion(&row), nil
}

func (r *Repo) CalendarVersionByConfirmation(ctx context.Context, orgID, confirmationID entity.ID) (entity.CalendarVersion, error) {
	row, err := r.q(ctx).CalendarVersionByConfirmation(ctx, CalendarVersionByConfirmationParams{
		OrganizationID: uuid.UUID(orgID), ConfirmationID: uuid.UUID(confirmationID),
	})
	if err != nil {
		return entity.CalendarVersion{}, notFound("calendar version by confirmation", err)
	}
	return calendarVersion(&row), nil
}

// PageCalendarVersions reads up to Limit+1 versions newest first, as PageMappingVersions does.
func (r *Repo) PageCalendarVersions(ctx context.Context, orgID, teamID entity.ID, p entity.VersionPageRequest) ([]entity.CalendarVersion, error) {
	params := PageCalendarVersionsParams{OrganizationID: uuid.UUID(orgID), TeamID: uuid.UUID(teamID)}
	var err error
	if params.Before, params.RowLimit, err = pageParams(p); err != nil {
		return nil, err
	}
	rows, err := r.q(ctx).PageCalendarVersions(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("process: page calendar versions: %w", err)
	}
	out := make([]entity.CalendarVersion, len(rows))
	for i := range rows {
		out[i] = calendarVersion(&rows[i])
	}
	return out, nil
}

func (r *Repo) InsertCalendarVersion(ctx context.Context, orgID entity.ID, v entity.CalendarVersion) error {
	no, base, err := versionNumbers(v.VersionNo, v.BaseVersionNo)
	if err != nil {
		return err
	}
	holidays := make([]pgtype.Date, len(v.Holidays))
	for i, d := range v.Holidays {
		holidays[i] = pgtype.Date{Time: d, Valid: true}
	}
	err = r.q(ctx).InsertCalendarVersion(ctx, InsertCalendarVersionParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(v.ID), TeamID: uuid.UUID(v.TeamID), VersionNo: no, BaseVersionNo: base,
		Timezone: v.Timezone, Workdays: int16(v.Workdays), WorkStart: timeOfDay(v.WorkStart), WorkEnd: timeOfDay(v.WorkEnd),
		Holidays: holidays, KnownSeq: v.KnownSeq, ConfirmedBy: uuid.UUID(v.ConfirmedBy), ConfirmedAt: v.ConfirmedAt,
		ConfirmationID: uuid.UUID(v.ConfirmationID),
	})
	if err != nil {
		return fmt.Errorf("process: insert calendar version: %w", err)
	}
	return nil
}

func (r *Repo) withRules(ctx context.Context, orgID entity.ID, v entity.MappingVersion) (entity.MappingVersion, error) {
	rules, err := r.rules(ctx, orgID, []uuid.UUID{uuid.UUID(v.ID)})
	if err != nil {
		return entity.MappingVersion{}, err
	}
	v.Rules = rules[v.ID]
	return v, nil
}

// rules reads the rules of versions by version, each in the order of its key.
func (r *Repo) rules(ctx context.Context, orgID entity.ID, versionIDs []uuid.UUID) (map[entity.ID][]entity.MappingRule, error) {
	out := make(map[entity.ID][]entity.MappingRule, len(versionIDs))
	if len(versionIDs) == 0 {
		return out, nil
	}
	rows, err := r.q(ctx).MappingRules(ctx, MappingRulesParams{OrganizationID: uuid.UUID(orgID), VersionIds: versionIDs})
	if err != nil {
		return nil, fmt.Errorf("process: mapping rules: %w", err)
	}
	for i := range rows {
		row := &rows[i]
		rule := entity.MappingRule{
			Kind: entity.RuleKind(row.RuleKind), InstanceID: entity.ID(row.InstanceID), NativeRef: row.NativeRef,
			NativeValue: row.NativeValue, Meaning: row.Meaning,
		}
		if row.Rank.Valid {
			rank := int(row.Rank.Int16)
			rule.Rank = &rank
		}
		id := entity.ID(row.MappingVersionID)
		out[id] = append(out[id], rule)
	}
	return out, nil
}

func mappingVersion(row *ProcessMappingVersion, calendarNo pgtype.Int4) entity.MappingVersion {
	v := entity.MappingVersion{
		ID: entity.ID(row.ID), TeamID: entity.ID(row.TeamID), VersionNo: int(row.VersionNo), BaseVersionNo: intPtr(row.BaseVersionNo),
		ContentHash: row.ContentHash, Timezone: row.Timezone, CalendarVersionNo: intPtr(calendarNo), KnownSeq: row.KnownSeq,
		Confirmation: entity.Confirmation{
			ConfirmedBy: entity.ID(row.ConfirmedBy), ConfirmedAt: row.ConfirmedAt, ConfirmationID: entity.ID(row.ConfirmationID),
		},
	}
	if row.CalendarVersionID.Valid {
		id := entity.ID(row.CalendarVersionID.UUID)
		v.CalendarVersionID = &id
	}
	return v
}

func calendarVersion(row *ProcessCalendarVersion) entity.CalendarVersion {
	v := entity.CalendarVersion{
		ID: entity.ID(row.ID), TeamID: entity.ID(row.TeamID), VersionNo: int(row.VersionNo), BaseVersionNo: intPtr(row.BaseVersionNo),
		CalendarDraft: entity.CalendarDraft{
			Timezone: row.Timezone, Workdays: workdays(row.Workdays), WorkStart: minutes(row.WorkStart), WorkEnd: minutes(row.WorkEnd),
			Holidays: make([]time.Time, len(row.Holidays)),
		},
		KnownSeq: row.KnownSeq,
		Confirmation: entity.Confirmation{
			ConfirmedBy: entity.ID(row.ConfirmedBy), ConfirmedAt: row.ConfirmedAt, ConfirmationID: entity.ID(row.ConfirmationID),
		},
	}
	for i, d := range row.Holidays {
		v.Holidays[i] = d.Time
	}
	return v
}

func notFound(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ErrNotFound
	}
	return fmt.Errorf("process: %s: %w", op, err)
}

func pageParams(p entity.VersionPageRequest) (pgtype.Int4, int32, error) {
	if p.Limit < 1 || p.Limit >= math.MaxInt32 {
		return pgtype.Int4{}, 0, errOutOfRange
	}
	var before pgtype.Int4
	if p.Before != nil {
		if *p.Before < 1 || *p.Before > math.MaxInt32 {
			return pgtype.Int4{}, 0, errOutOfRange
		}
		before = pgtype.Int4{Int32: int32(*p.Before), Valid: true}
	}
	return before, int32(p.Limit + 1), nil
}

func versionNumbers(no int, base *int) (int32, pgtype.Int4, error) {
	if no < 1 || no > math.MaxInt32 {
		return 0, pgtype.Int4{}, errOutOfRange
	}
	if base == nil {
		return int32(no), pgtype.Int4{}, nil
	}
	if *base < 1 || *base > math.MaxInt32 {
		return 0, pgtype.Int4{}, errOutOfRange
	}
	return int32(no), pgtype.Int4{Int32: int32(*base), Valid: true}, nil
}

func smallint(v *int) (pgtype.Int2, error) {
	if v == nil {
		return pgtype.Int2{}, nil
	}
	if *v < math.MinInt16 || *v > math.MaxInt16 {
		return pgtype.Int2{}, errOutOfRange
	}
	return pgtype.Int2{Int16: int16(*v), Valid: true}, nil
}

func intPtr(v pgtype.Int4) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

func nullID(id *entity.ID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*id), Valid: true}
}

// workdays reads the mask the column bounds to seven bits.
func workdays(v int16) entity.Workdays {
	return entity.Workdays(uint8(v & int16(entity.AllWorkdays)))
}

func timeOfDay(t *entity.TimeOfDay) pgtype.Time {
	if t == nil {
		return pgtype.Time{}
	}
	return pgtype.Time{Microseconds: int64(*t) * int64(time.Minute/time.Microsecond), Valid: true}
}

func minutes(t pgtype.Time) *entity.TimeOfDay {
	if !t.Valid {
		return nil
	}
	m := entity.TimeOfDay(t.Microseconds / int64(time.Minute/time.Microsecond))
	return &m
}
