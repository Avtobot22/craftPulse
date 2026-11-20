package calc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/postgres"
)

var errOutOfRange = errors.New("calc: number does not fit its column")

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) q(ctx context.Context) *Queries { return New(postgres.Conn(ctx, r.pool)) }

func (r *Repo) EnsureSeries(ctx context.Context, orgID entity.ID, s entity.CalcSeries) (entity.CalcSeries, error) {
	q := r.q(ctx)
	err := q.EnsureSeries(ctx, EnsureSeriesParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(s.ID), ScopeHash: s.ScopeHash, Scope: s.Scope, MetricSet: s.MetricSet,
		Step: string(s.Step),
	})
	if err != nil {
		return entity.CalcSeries{}, fmt.Errorf("calc: ensure series: %w", err)
	}
	row, err := q.SeriesByScope(ctx, SeriesByScopeParams{
		OrganizationID: uuid.UUID(orgID), ScopeHash: s.ScopeHash, MetricSet: s.MetricSet, Step: string(s.Step),
	})
	if err != nil {
		return entity.CalcSeries{}, notFound("series by scope", err)
	}
	return series(&row), nil
}

func (r *Repo) Series(ctx context.Context, orgID, id entity.ID) (entity.CalcSeries, error) {
	row, err := r.q(ctx).SeriesByID(ctx, SeriesByIDParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if err != nil {
		return entity.CalcSeries{}, notFound("series", err)
	}
	return series(&row), nil
}

func (r *Repo) OrgSeries(ctx context.Context, orgID entity.ID, metricSet string) ([]entity.CalcSeries, error) {
	rows, err := r.q(ctx).OrgSeries(ctx, OrgSeriesParams{OrganizationID: uuid.UUID(orgID), MetricSet: metricSet})
	if err != nil {
		return nil, fmt.Errorf("calc: org series: %w", err)
	}
	out := make([]entity.CalcSeries, len(rows))
	for i := range rows {
		out[i] = series(&rows[i])
	}
	return out, nil
}

func (r *Repo) LastRevision(ctx context.Context, orgID, seriesID entity.ID) (entity.CalcRevision, bool, error) {
	id, err := r.q(ctx).LastRevisionID(ctx, LastRevisionIDParams{OrganizationID: uuid.UUID(orgID), SeriesID: uuid.UUID(seriesID)})
	return r.found(ctx, orgID, id, err, "last revision")
}

func (r *Repo) RevisionAfter(ctx context.Context, orgID, seriesID entity.ID, afterNo int) (entity.CalcRevision, bool, error) {
	if afterNo > math.MaxInt32 {
		return entity.CalcRevision{}, false, errOutOfRange
	}
	id, err := r.q(ctx).RevisionIDAfter(ctx, RevisionIDAfterParams{
		OrganizationID: uuid.UUID(orgID), SeriesID: uuid.UUID(seriesID), AfterNo: int32(afterNo), //nolint:gosec // checked above
	})
	return r.found(ctx, orgID, id, err, "revision after")
}

// found reads the revision a lookup returned; a lookup without a row is false.
func (r *Repo) found(ctx context.Context, orgID entity.ID, id uuid.UUID, err error, op string) (entity.CalcRevision, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.CalcRevision{}, false, nil
	}
	if err != nil {
		return entity.CalcRevision{}, false, fmt.Errorf("calc: %s: %w", op, err)
	}
	rev, err := r.Revision(ctx, orgID, entity.ID(id))
	return rev, err == nil, err
}

func (r *Repo) PartByHash(ctx context.Context, orgID entity.ID, inputHash []byte) (entity.ID, bool, error) {
	id, err := r.q(ctx).PartByHash(ctx, PartByHashParams{OrganizationID: uuid.UUID(orgID), InputHash: inputHash})
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ID{}, false, nil
	}
	if err != nil {
		return entity.ID{}, false, fmt.Errorf("calc: part by hash: %w", err)
	}
	return entity.ID(id), true, nil
}

func (r *Repo) FailStaleRevisions(ctx context.Context, orgID, seriesID entity.ID, before time.Time) error {
	err := r.q(ctx).FailStaleRevisions(ctx, FailStaleRevisionsParams{OrganizationID: uuid.UUID(orgID), SeriesID: uuid.UUID(seriesID), Before: before})
	if err != nil {
		return fmt.Errorf("calc: fail stale revisions: %w", err)
	}
	return nil
}

func (r *Repo) CreateRevision(ctx context.Context, orgID entity.ID, rev entity.CalcRevision) (bool, error) {
	_, err := r.q(ctx).CreateRevision(ctx, CreateRevisionParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(rev.ID), SeriesID: uuid.UUID(rev.SeriesID), Now: rev.StartedAt,
		WindowFrom: rev.Window.From, WindowTo: rev.Window.To, AsOf: rev.AsOf, KnowledgeSeq: rev.KnowledgeSeq, Timezone: rev.Timezone,
		InputHash: rev.InputHash, Loading: rev.Loading, PreviousRevisionID: nullID(rev.PreviousRevisionID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("calc: create revision: %w", err)
	}
	return true, nil
}

func (r *Repo) SetRevisionPrevious(ctx context.Context, orgID, revisionID entity.ID, prev *entity.ID) error {
	err := r.q(ctx).SetRevisionPrevious(ctx, SetRevisionPreviousParams{
		OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(revisionID), PreviousRevisionID: nullID(prev),
	})
	if err != nil {
		return fmt.Errorf("calc: set previous revision: %w", err)
	}
	return nil
}

func (r *Repo) InsertRevisionInputs(ctx context.Context, orgID, revisionID entity.ID, inputs []entity.RevisionInput) error {
	p := InsertRevisionInputsParams{OrganizationID: uuid.UUID(orgID), RevisionID: uuid.UUID(revisionID)}
	for _, in := range inputs {
		p.Kinds, p.InputIds, p.Versions = append(p.Kinds, in.Kind), append(p.InputIds, in.ID), append(p.Versions, in.Version)
		watermark := ""
		if in.Watermark != nil {
			watermark = in.Watermark.UTC().Format(time.RFC3339Nano)
		}
		p.Watermarks, p.Completeness = append(p.Watermarks, watermark), append(p.Completeness, in.Completeness)
	}
	if err := r.q(ctx).InsertRevisionInputs(ctx, p); err != nil {
		return fmt.Errorf("calc: insert revision inputs: %w", err)
	}
	return nil
}

func (r *Repo) Revision(ctx context.Context, orgID, id entity.ID) (entity.CalcRevision, error) {
	row, err := r.q(ctx).GetRevision(ctx, GetRevisionParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if err != nil {
		return entity.CalcRevision{}, notFound("revision", err)
	}
	out := entity.CalcRevision{
		ID: entity.ID(row.ID), SeriesID: entity.ID(row.SeriesID), RevisionNo: int(row.RevisionNo), Status: entity.RevisionStatus(row.Status),
		ProgressAt: row.ProgressAt.UTC(), Window: entity.Window{From: row.WindowFrom.UTC(), To: row.WindowTo.UTC()}, AsOf: row.AsOf.UTC(),
		KnowledgeSeq: row.KnowledgeSeq, Timezone: row.Timezone, InputHash: row.InputHash, Loading: row.Loading, StartedAt: row.StartedAt.UTC(),
		PublishedAt: timePtr(row.PublishedAt), SupersededAt: timePtr(row.SupersededAt), IrreproducibleReason: row.IrreproducibleReason.String,
	}
	if row.PreviousRevisionID.Valid {
		prev := entity.ID(row.PreviousRevisionID.UUID)
		out.PreviousRevisionID = &prev
	}
	return out, nil
}

func (r *Repo) TouchRevision(ctx context.Context, orgID, id entity.ID, at time.Time) (bool, error) {
	n, err := r.q(ctx).TouchRevision(ctx, TouchRevisionParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id), Now: at})
	if err != nil {
		return false, fmt.Errorf("calc: touch revision: %w", err)
	}
	return n == 1, nil
}

func (r *Repo) FailRevision(ctx context.Context, orgID, id entity.ID) (bool, error) {
	n, err := r.q(ctx).FailRevision(ctx, FailRevisionParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id)})
	if err != nil {
		return false, fmt.Errorf("calc: fail revision: %w", err)
	}
	return n == 1, nil
}

func (r *Repo) PublishRevision(ctx context.Context, orgID, id entity.ID, at time.Time) (bool, error) {
	n, err := r.q(ctx).PublishRevision(ctx, PublishRevisionParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id), Now: at})
	if err != nil {
		return false, fmt.Errorf("calc: publish revision: %w", err)
	}
	return n == 1, nil
}

func (r *Repo) SupersedeRevision(ctx context.Context, orgID, id entity.ID, at time.Time) error {
	if err := r.q(ctx).SupersedeRevision(ctx, SupersedeRevisionParams{OrganizationID: uuid.UUID(orgID), ID: uuid.UUID(id), Now: at}); err != nil {
		return fmt.Errorf("calc: supersede revision: %w", err)
	}
	return nil
}

func (r *Repo) MoveCurrentRevision(ctx context.Context, orgID, seriesID, revisionID entity.ID, prev *entity.ID, seq int64) (bool, error) {
	n, err := r.q(ctx).MoveCurrentRevision(ctx, MoveCurrentRevisionParams{
		RevisionID: uuid.NullUUID{UUID: uuid.UUID(revisionID), Valid: true}, KnowledgeSeq: seq, OrganizationID: uuid.UUID(orgID),
		SeriesID: uuid.UUID(seriesID), PreviousRevisionID: nullID(prev),
	})
	if err != nil {
		return false, fmt.Errorf("calc: move current revision: %w", err)
	}
	return n == 1, nil
}

// SavePart writes the points, contributions, intervals and attributes of a new part with one statement each; a
// part already stored under the same input hash is kept as it is (TZ 10.6).
func (r *Repo) SavePart(ctx context.Context, orgID, newID entity.ID, p *entity.StoredPart, at time.Time) (entity.ID, bool, error) {
	q := r.q(ctx)
	org := uuid.UUID(orgID)
	id, err := q.UpsertPart(ctx, UpsertPartParams{
		OrganizationID: org, ID: uuid.UUID(newID), InputHash: p.InputHash, MetricSet: p.MetricSet, PartFrom: p.From, PartTo: p.To, CreatedAt: at,
	})
	if err != nil {
		return entity.ID{}, false, fmt.Errorf("calc: upsert part: %w", err)
	}
	if id != uuid.UUID(newID) {
		return entity.ID(id), false, nil
	}
	points, contributions, err := pointParams(org, id, p.Points)
	if err != nil {
		return entity.ID{}, false, err
	}
	if err := q.InsertPoints(ctx, points); err != nil {
		return entity.ID{}, false, fmt.Errorf("calc: insert points: %w", err)
	}
	if err := q.InsertContributions(ctx, contributions); err != nil {
		return entity.ID{}, false, fmt.Errorf("calc: insert contributions: %w", err)
	}
	intervals, err := intervalParams(org, id, p.Intervals)
	if err != nil {
		return entity.ID{}, false, err
	}
	if err := q.InsertIntervals(ctx, intervals); err != nil {
		return entity.ID{}, false, fmt.Errorf("calc: insert intervals: %w", err)
	}
	if err := q.InsertTaskSlices(ctx, taskParams(org, id, p.Tasks)); err != nil {
		return entity.ID{}, false, fmt.Errorf("calc: insert task slices: %w", err)
	}
	return entity.ID(id), true, nil
}

func (r *Repo) LinkPart(ctx context.Context, orgID, revisionID entity.ID, partNo int, partID entity.ID) error {
	if partNo < 1 || partNo > math.MaxInt32 {
		return errOutOfRange
	}
	err := r.q(ctx).LinkPart(ctx, LinkPartParams{
		OrganizationID: uuid.UUID(orgID), RevisionID: uuid.UUID(revisionID), PartNo: int32(partNo), PartID: uuid.UUID(partID),
	})
	if err != nil {
		return fmt.Errorf("calc: link part: %w", err)
	}
	return nil
}

func (r *Repo) RevisionPoints(ctx context.Context, orgID, revisionID entity.ID) ([]entity.SeriesPoint, error) {
	rows, err := r.q(ctx).RevisionPoints(ctx, RevisionPointsParams{OrganizationID: uuid.UUID(orgID), RevisionID: uuid.UUID(revisionID)})
	if err != nil {
		return nil, fmt.Errorf("calc: revision points: %w", err)
	}
	out := make([]entity.SeriesPoint, len(rows))
	for i := range rows {
		row := &rows[i]
		p := entity.Point{
			Start: row.PointStart.UTC(), End: row.PointEnd.UTC(), State: entity.DataState(row.State), Value: optional(row.Value, row.HasValue),
			Numerator: optional(row.Numerator, row.HasNumerator), Denominator: optional(row.Denominator, row.HasDenominator),
			ExcludedCount: int(row.ExcludedCount), MissingCount: int(row.MissingCount), LowSample: row.LowSample, ReasonCode: row.ReasonCode.String,
		}
		if row.N.Valid {
			n := int(row.N.Int32)
			p.N = &n
		}
		if row.BoundKind.Valid {
			p.Bound = &entity.Bound{Kind: row.BoundKind.String, Value: row.BoundValue}
		}
		out[i] = entity.SeriesPoint{MetricID: row.MetricID, Estimator: row.Estimator, Unit: row.Unit, Point: p}
	}
	return out, nil
}

func (r *Repo) PointContributions(ctx context.Context, orgID, revisionID entity.ID, cq entity.ContributionQuery) ([]entity.Contribution, error) {
	p := PointContributionsParams{
		OrganizationID: uuid.UUID(orgID), RevisionID: uuid.UUID(revisionID), MetricID: cq.MetricID, Estimator: cq.Estimator,
		PointStart: cq.PointStart, RowLimit: int32(min(cq.Limit+1, math.MaxInt32)), //nolint:gosec // bounded by MaxInt32
	}
	if cq.After != nil {
		if cq.After.ItemNo < 0 || cq.After.ItemNo > math.MaxInt16 {
			return nil, errOutOfRange
		}
		p.AfterObjectID, p.AfterItemNo = uuid.UUID(cq.After.ObjectID), int16(cq.After.ItemNo)
	}
	rows, err := r.q(ctx).PointContributions(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("calc: point contributions: %w", err)
	}
	out := make([]entity.Contribution, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.Contribution{
			ObjectID: entity.ID(row.ObjectID), ItemNo: int(row.ItemNo), ObjectKind: entity.ObjectKind(row.ObjectKind),
			Role: entity.ContributionRole(row.Role), Value: optional(row.Value, row.HasValue), ReasonCode: row.ReasonCode.String, Evidence: row.Evidence,
		}
	}
	return out, nil
}

// TaskIntervals joins the pieces a task has in the parts of a revision back into its intervals.
func (r *Repo) TaskIntervals(ctx context.Context, orgID, revisionID, taskID entity.ID) ([]entity.WorkflowInterval, error) {
	return r.TasksIntervals(ctx, orgID, revisionID, []entity.ID{taskID})
}

// TasksIntervals joins the pieces the tasks have in the parts of a revision back into their intervals, by task.
func (r *Repo) TasksIntervals(ctx context.Context, orgID, revisionID entity.ID, taskIDs []entity.ID) ([]entity.WorkflowInterval, error) {
	tasks := make([]uuid.UUID, len(taskIDs))
	for i, id := range taskIDs {
		tasks[i] = uuid.UUID(id)
	}
	rows, err := r.q(ctx).TasksIntervals(ctx, TasksIntervalsParams{OrganizationID: uuid.UUID(orgID), RevisionID: uuid.UUID(revisionID), TaskIds: tasks})
	if err != nil {
		return nil, fmt.Errorf("calc: task intervals: %w", err)
	}
	var out []entity.WorkflowInterval
	for i := range rows {
		row := &rows[i]
		task := entity.ID(row.TaskID)
		if n := len(out); n > 0 && out[n-1].TaskID == task && out[n-1].EpisodeNo == int(row.EpisodeNo) && out[n-1].IntervalNo == int(row.IntervalNo) {
			if row.DuringTo.After(out[n-1].To) {
				out[n-1].To = row.DuringTo.UTC()
			}
			continue
		}
		iv := entity.WorkflowInterval{
			TaskID: task, EpisodeNo: int(row.EpisodeNo), IntervalNo: int(row.IntervalNo), Class: entity.IntervalClass(row.Class),
			From: row.DuringFrom.UTC(), To: row.DuringTo.UTC(), SourceEventIDs: make([]entity.ID, len(row.SourceEventIds)),
		}
		for j, id := range row.SourceEventIds {
			iv.SourceEventIDs[j] = entity.ID(id)
		}
		out = append(out, iv)
	}
	return out, nil
}

// RevisionCohort reads every contribution of a revision with the project, the type and the size of its task.
func (r *Repo) RevisionCohort(ctx context.Context, orgID, revisionID entity.ID) ([]entity.CohortRow, error) {
	rows, err := r.q(ctx).RevisionCohort(ctx, RevisionCohortParams{OrganizationID: uuid.UUID(orgID), RevisionID: uuid.UUID(revisionID)})
	if err != nil {
		return nil, fmt.Errorf("calc: revision cohort: %w", err)
	}
	out := make([]entity.CohortRow, len(rows))
	for i := range rows {
		row := &rows[i]
		c := entity.CohortRow{
			MetricID: row.MetricID, Estimator: row.Estimator, PointStart: row.PointStart.UTC(),
			Contribution: entity.Contribution{
				ObjectID: entity.ID(row.ObjectID), ItemNo: int(row.ItemNo), ObjectKind: entity.ObjectKind(row.ObjectKind),
				Role: entity.ContributionRole(row.Role), Value: optional(row.Value, row.HasValue), ReasonCode: row.ReasonCode.String, Evidence: row.Evidence,
			},
		}
		if row.ProjectID.Valid {
			project := entity.ID(row.ProjectID.UUID)
			c.ProjectID = &project
		}
		if row.TaskType.Valid {
			c.TaskType = &row.TaskType.String
		}
		if row.SizeLabel.Valid {
			c.SizeLabel = &row.SizeLabel.String
		}
		out[i] = c
	}
	return out, nil
}

func (r *Repo) RevisionInputs(ctx context.Context, orgID, revisionID entity.ID) ([]entity.RevisionInput, error) {
	rows, err := r.q(ctx).RevisionInputs(ctx, RevisionInputsParams{OrganizationID: uuid.UUID(orgID), RevisionID: uuid.UUID(revisionID)})
	if err != nil {
		return nil, fmt.Errorf("calc: revision inputs: %w", err)
	}
	out := make([]entity.RevisionInput, len(rows))
	for i, row := range rows {
		out[i] = entity.RevisionInput{
			Kind: row.InputKind, ID: row.InputID, Version: row.Version.String, Watermark: timePtr(row.Watermark),
			Completeness: row.Completeness.String,
		}
	}
	return out, nil
}

func (r *Repo) MarkBasisDeleted(ctx context.Context, orgID, objectID entity.ID) (int64, error) {
	n, err := r.q(ctx).MarkBasisDeleted(ctx, MarkBasisDeletedParams{OrganizationID: uuid.UUID(orgID), ObjectID: uuid.UUID(objectID)})
	if err != nil {
		return 0, fmt.Errorf("calc: mark basis deleted: %w", err)
	}
	return n, nil
}

func (r *Repo) MarkBasisExpired(ctx context.Context, orgID entity.ID, reproducibleFrom int64) (int64, error) {
	n, err := r.q(ctx).MarkBasisExpired(ctx, MarkBasisExpiredParams{OrganizationID: uuid.UUID(orgID), ReproducibleFrom: reproducibleFrom})
	if err != nil {
		return 0, fmt.Errorf("calc: mark basis expired: %w", err)
	}
	return n, nil
}

func (r *Repo) MarkDefinitionRetired(ctx context.Context, orgID entity.ID, current []string) (int64, error) {
	n, err := r.q(ctx).MarkDefinitionRetired(ctx, MarkDefinitionRetiredParams{OrganizationID: uuid.UUID(orgID), Current: current})
	if err != nil {
		return 0, fmt.Errorf("calc: mark definition retired: %w", err)
	}
	return n, nil
}

func (r *Repo) UnusedParts(ctx context.Context, orgID, after entity.ID, before time.Time, limit int) ([]entity.ID, error) {
	if limit < 1 || limit > math.MaxInt32 {
		return nil, errOutOfRange
	}
	ids, err := r.q(ctx).UnusedParts(ctx, UnusedPartsParams{OrganizationID: uuid.UUID(orgID), AfterID: uuid.UUID(after), Before: before, RowLimit: int32(limit)})
	if err != nil {
		return nil, fmt.Errorf("calc: unused parts: %w", err)
	}
	out := make([]entity.ID, len(ids))
	for i, id := range ids {
		out[i] = entity.ID(id)
	}
	return out, nil
}

// DeleteParts removes the parts with their links from failed revisions; points, contributions, intervals and
// attributes go with them.
func (r *Repo) DeleteParts(ctx context.Context, orgID entity.ID, ids []entity.ID) error {
	q := r.q(ctx)
	parts := make([]uuid.UUID, len(ids))
	for i, id := range ids {
		parts[i] = uuid.UUID(id)
	}
	if err := q.DeletePartLinks(ctx, DeletePartLinksParams{OrganizationID: uuid.UUID(orgID), PartIds: parts}); err != nil {
		return fmt.Errorf("calc: delete part links: %w", err)
	}
	if err := q.DeleteParts(ctx, DeletePartsParams{OrganizationID: uuid.UUID(orgID), PartIds: parts}); err != nil {
		return fmt.Errorf("calc: delete parts: %w", err)
	}
	return nil
}

func pointParams(org, part uuid.UUID, points []entity.SeriesPoint) (InsertPointsParams, InsertContributionsParams, error) {
	p := InsertPointsParams{OrganizationID: org, PartID: part}
	c := InsertContributionsParams{OrganizationID: org, PartID: part}
	for i := range points {
		sp := &points[i]
		v, hv := value(sp.Value)
		num, hnum := value(sp.Numerator)
		den, hden := value(sp.Denominator)
		var n int32
		if sp.N != nil {
			if *sp.N > math.MaxInt32 {
				return p, c, errOutOfRange
			}
			n = int32(*sp.N) //nolint:gosec // checked above
		}
		var bk string
		bv := "0"
		if sp.Bound != nil {
			bk, bv = sp.Bound.Kind, strconv.FormatFloat(sp.Bound.Value, 'g', -1, 64)
		}
		if sp.ExcludedCount > math.MaxInt32 || sp.MissingCount > math.MaxInt32 {
			return p, c, errOutOfRange
		}
		p.MetricIds, p.Estimators = append(p.MetricIds, sp.MetricID), append(p.Estimators, sp.Estimator)
		p.Starts, p.Ends = append(p.Starts, sp.Start), append(p.Ends, sp.End)
		p.PointValues, p.HasValues, p.Units = append(p.PointValues, v), append(p.HasValues, hv), append(p.Units, sp.Unit)
		p.Numerators, p.HasNumerators = append(p.Numerators, num), append(p.HasNumerators, hnum)
		p.Denominators, p.HasDenominators = append(p.Denominators, den), append(p.HasDenominators, hden)
		p.Ns, p.HasNs = append(p.Ns, n), append(p.HasNs, sp.N != nil)
		p.States, p.Reasons = append(p.States, string(sp.State)), append(p.Reasons, sp.ReasonCode)
		p.BoundKinds, p.BoundValues = append(p.BoundKinds, bk), append(p.BoundValues, bv)
		p.Excluded = append(p.Excluded, int32(sp.ExcludedCount)) //nolint:gosec // checked above
		p.Missing = append(p.Missing, int32(sp.MissingCount))    //nolint:gosec // checked above
		p.LowSamples = append(p.LowSamples, sp.LowSample)
		for j := range sp.Contributions {
			k := &sp.Contributions[j]
			if k.ItemNo < 1 || k.ItemNo > math.MaxInt16 {
				return p, c, errOutOfRange
			}
			cv, hcv := value(k.Value)
			c.MetricIds, c.Estimators, c.Starts = append(c.MetricIds, sp.MetricID), append(c.Estimators, sp.Estimator), append(c.Starts, sp.Start)
			c.ObjectIds, c.ItemNos = append(c.ObjectIds, uuid.UUID(k.ObjectID)), append(c.ItemNos, int16(k.ItemNo))
			c.Kinds, c.Roles = append(c.Kinds, string(k.ObjectKind)), append(c.Roles, string(k.Role))
			c.ContributionValues, c.HasValues, c.Reasons = append(c.ContributionValues, cv), append(c.HasValues, hcv), append(c.Reasons, k.ReasonCode)
			c.Evidence = append(c.Evidence, string(k.Evidence))
		}
	}
	return p, c, nil
}

func intervalParams(org, part uuid.UUID, intervals []entity.WorkflowInterval) (InsertIntervalsParams, error) {
	p := InsertIntervalsParams{OrganizationID: org, PartID: part}
	for i := range intervals {
		iv := &intervals[i]
		if iv.EpisodeNo > math.MaxInt32 || iv.IntervalNo > math.MaxInt32 {
			return p, errOutOfRange
		}
		ids := make([]string, len(iv.SourceEventIDs))
		for j, id := range iv.SourceEventIDs {
			ids[j] = id.String()
		}
		src, err := json.Marshal(ids)
		if err != nil {
			return p, fmt.Errorf("calc: encode source events: %w", err)
		}
		p.TaskIds, p.EpisodeNos = append(p.TaskIds, uuid.UUID(iv.TaskID)), append(p.EpisodeNos, int32(iv.EpisodeNo)) //nolint:gosec // checked above
		p.IntervalNos, p.Classes = append(p.IntervalNos, int32(iv.IntervalNo)), append(p.Classes, string(iv.Class))  //nolint:gosec // checked above
		p.Froms, p.Tos, p.Sources = append(p.Froms, iv.From), append(p.Tos, iv.To), append(p.Sources, string(src))
	}
	return p, nil
}

func taskParams(org, part uuid.UUID, tasks []entity.TaskSlice) InsertTaskSlicesParams {
	p := InsertTaskSlicesParams{OrganizationID: org, PartID: part}
	for i := range tasks {
		t := &tasks[i]
		p.TaskIds, p.ProjectIds = append(p.TaskIds, uuid.UUID(t.TaskID)), append(p.ProjectIds, uuid.UUID(t.ProjectID))
		p.TaskTypes, p.SizeLabels = append(p.TaskTypes, deref(t.TaskType)), append(p.SizeLabels, deref(t.SizeLabel))
	}
	return p
}

func series(row *CalcSeries) entity.CalcSeries {
	out := entity.CalcSeries{
		ID: entity.ID(row.ID), Scope: row.Scope, ScopeHash: row.ScopeHash, MetricSet: row.MetricSet, Step: entity.Step(row.Step),
		ConsumedSeq: row.ConsumedSeq,
	}
	if row.CurrentRevisionID.Valid {
		id := entity.ID(row.CurrentRevisionID.UUID)
		out.CurrentRevisionID = &id
	}
	return out
}

func notFound(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.ErrNotFound
	}
	return fmt.Errorf("calc: %s: %w", op, err)
}

func nullID(id *entity.ID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.UUID(*id), Valid: true}
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	utc := t.Time.UTC()
	return &utc
}

// value is a numeric as text in the shortest form that reads back to the same float64: a float8 cast to numeric
// keeps 15 digits only.
func value(v *float64) (string, bool) {
	if v == nil {
		return "0", false
	}
	return strconv.FormatFloat(*v, 'g', -1, 64), true
}

func optional(v float64, ok bool) *float64 {
	if !ok {
		return nil
	}
	return &v
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ScopeSeries are the series of a scope and a set of metrics, one per step.
func (r *Repo) ScopeSeries(ctx context.Context, orgID entity.ID, scopeHash []byte, metricSet string) ([]entity.CalcSeries, error) {
	rows, err := r.q(ctx).ScopeSeries(ctx, ScopeSeriesParams{OrganizationID: uuid.UUID(orgID), ScopeHash: scopeHash, MetricSet: metricSet})
	if err != nil {
		return nil, fmt.Errorf("calc: scope series: %w", err)
	}
	out := make([]entity.CalcSeries, len(rows))
	for i := range rows {
		out[i] = series(&rows[i])
	}
	return out, nil
}

// PointSamples reads the contributions of the points of a metric and an estimator of a revision that lie within
// the window, with the type and the size of the task in the part of the point.
func (r *Repo) PointSamples(ctx context.Context, orgID, revisionID entity.ID, metricID, estimator string, w entity.Window) ([]entity.PointSample, error) {
	rows, err := r.q(ctx).PointSamples(ctx, PointSamplesParams{
		OrganizationID: uuid.UUID(orgID), RevisionID: uuid.UUID(revisionID), MetricID: metricID, Estimator: estimator, FromAt: w.From, ToAt: w.To,
	})
	if err != nil {
		return nil, fmt.Errorf("calc: point samples: %w", err)
	}
	out := make([]entity.PointSample, len(rows))
	for i := range rows {
		row := &rows[i]
		out[i] = entity.PointSample{
			TaskID: entity.ID(row.ObjectID), Role: entity.ContributionRole(row.Role), Value: optional(row.Value, row.HasValue),
			ReasonCode: row.ReasonCode.String, TaskType: textPtr(row.TaskType), SizeLabel: textPtr(row.SizeLabel),
		}
	}
	return out, nil
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}
