package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

var (
	// _seriesWindow is ten months by weeks; as_of cuts it to 41 points, five parts of ten.
	_seriesWindow = entity.Window{
		From: time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC), Step: entity.StepWeek,
	}
	_seriesAsOf = time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
)

// seriesEnv is the team of processEnv with a confirmed mapping and the weekly series of its flow.
type seriesEnv struct {
	*processEnv
	calc   *calc.UseCase
	series entity.CalcSeries
}

func newSeriesEnv(t *testing.T) *seriesEnv {
	t.Helper()
	e := &seriesEnv{processEnv: newProcessEnv(t)}
	rule := func(kind entity.RuleKind, ref, value, meaning string) entity.MappingRule {
		return entity.MappingRule{Kind: kind, InstanceID: entity.ID(e.instance), NativeRef: ref, NativeValue: value, Meaning: meaning}
	}
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	_, _, err := e.process.CreateMappingVersion(t.Context(), admin, entity.ID(e.team), 0, entity.ID(uuid.New()), entity.MappingDraft{
		Timezone: "Europe/Moscow", Rules: []entity.MappingRule{
			rule(entity.RuleStatus, "1", "", entity.MeaningIntake), rule(entity.RuleStatus, "3", "", entity.MeaningActive),
			rule(entity.RuleStatus, "5", "", entity.MeaningDone), rule(entity.RuleResolution, "10000", "", entity.MeaningAccepted),
			rule(entity.RuleBlockedFlag, "customfield_10020", "Impediment", entity.MeaningBlocked),
		},
	})
	require.NoError(t, err)
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	e.calc, err = newCalc(e.app, calcModules{structure: orgs, facts: e.runner.norm, settings: e.process}, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	e.series, err = e.calc.TeamSeries(t.Context(), e.orgID, entity.ID(e.team), entity.StepWeek)
	require.NoError(t, err)
	again, err := e.calc.TeamSeries(t.Context(), e.orgID, entity.ID(e.team), entity.StepWeek)
	require.NoError(t, err)
	require.Equal(t, e.series.ID, again.ID, "one team finds one series")
	return e
}

type calcJob struct {
	id   int64
	args entity.CalcSliceJob
}

// _waitingJob is the oldest job of a kind that may run at the time of the test clock.
const _waitingJob = "SELECT id, args FROM river_job WHERE kind = $1 AND (state = 'available' OR state = 'scheduled' AND scheduled_at <= $2) " +
	"ORDER BY id LIMIT 1"

// nextJob is the oldest waiting job of calc.slice.
func (e *seriesEnv) nextJob(t *testing.T) (calcJob, bool) {
	t.Helper()
	var (
		j   calcJob
		raw []byte
	)
	err := e.owner.QueryRow(t.Context(), _waitingJob, entity.KindCalcSlice, e.clock.Now()).Scan(&j.id, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return j, false
	}
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &j.args))
	return j, true
}

// runJobs runs up to n waiting jobs of calc.slice, all of them when n is 0, in the order they were queued; the
// jobs of calc.recount queued before them run first and are not counted.
func (e *seriesEnv) runJobs(t *testing.T, n int) int {
	t.Helper()
	ran := 0
	for n == 0 || ran < n {
		e.runRecounts(t)
		j, ok := e.nextJob(t)
		if !ok {
			break
		}
		require.NoError(t, e.calc.RunSlice(t.Context(), j.args, completeJob(j.id)))
		ran++
	}
	return ran
}

// recount runs the chains the recounts queued with their delay, moving the test clock to each, until none is left.
func (e *seriesEnv) recount(t *testing.T) {
	t.Helper()
	for {
		e.runJobs(t, 0)
		if !e.wake(t) {
			return
		}
	}
}

// wake moves the test clock to the earliest scheduled job of calc; false when none is scheduled.
func (e *seriesEnv) wake(t *testing.T) bool {
	t.Helper()
	var at *time.Time
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT min(scheduled_at) FROM river_job WHERE kind IN ($1, $2) AND state = 'scheduled'",
		entity.KindCalcSlice, entity.KindCalcRecount).Scan(&at))
	if at == nil {
		return false
	}
	if d := at.Sub(e.clock.Now()); d > 0 {
		e.clock.Advance(d)
	}
	return true
}

// runRecounts runs the waiting jobs of calc.recount.
func (e *seriesEnv) runRecounts(t *testing.T) int {
	t.Helper()
	ran := 0
	for {
		var (
			id  int64
			raw []byte
			job entity.CalcRecountJob
		)
		err := e.owner.QueryRow(t.Context(), _waitingJob, entity.KindCalcRecount, e.clock.Now()).Scan(&id, &raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return ran
		}
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &job))
		require.NoError(t, e.calc.RunRecount(t.Context(), job, completeJob(id)))
		ran++
	}
}

func (e *seriesEnv) refresh(t *testing.T, asOf time.Time) {
	t.Helper()
	require.NoError(t, e.calc.Refresh(t.Context(), e.orgID, e.series.ID, _seriesWindow, asOf))
}

// build refreshes the series and runs its chain to the end; it returns the current revision.
func (e *seriesEnv) build(t *testing.T) entity.CalcRevision {
	t.Helper()
	e.refresh(t, _seriesAsOf)
	e.runJobs(t, 0)
	return e.current(t)
}

func (e *seriesEnv) current(t *testing.T) entity.CalcRevision {
	t.Helper()
	r, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, nil)
	require.NoError(t, err)
	return r
}

func (e *seriesEnv) parts(t *testing.T, revision entity.ID) []uuid.UUID {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), "SELECT part_id FROM calc.revision_part WHERE revision_id = $1 ORDER BY part_no", uuid.UUID(revision))
	require.NoError(t, err)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	require.NoError(t, err)
	return ids
}

func (e *seriesEnv) building(t *testing.T) []uuid.UUID {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), "SELECT id FROM calc.revision WHERE series_id = $1 AND status = 'building'", uuid.UUID(e.series.ID))
	require.NoError(t, err)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	require.NoError(t, err)
	return ids
}

func (e *seriesEnv) points(t *testing.T, revision entity.ID) []entity.SeriesPoint {
	t.Helper()
	p, err := e.calc.RevisionPoints(t.Context(), e.orgID, revision)
	require.NoError(t, err)
	return p
}

// flag writes the impediment flag of a task on and off; the value it had before stays.
func (e *seriesEnv) flag(t *testing.T, task string, on, off time.Time) {
	t.Helper()
	field, impediment := "customfield_10020", "Impediment"
	event := func(at time.Time, changelog string, before, after *string) *envelope.Record {
		raw, err := json.Marshal(envelope.TaskEventV1{
			TaskNativeId: task, EventKind: envelope.TaskEventV1EventKindFieldChange, FieldNativeId: &field, BeforeValue: before, AfterValue: after,
		})
		require.NoError(t, err)
		return &envelope.Record{
			RecordClass: envelope.RecordRecordClassEvent, RecordKind: "task_event", PayloadSchema: "task_event/1", NativeNs: task,
			NativeId: changelog + "/" + field, SourceRevision: &changelog, EventTime: &at, Payload: raw,
		}
	}
	e.collect(t, manifestOf(envelope.ManifestSchemaIdTaskEvents, envelope.SourceProviderJiraCloud, _jiraInstance, "10000"),
		event(on, "late-"+on.Format("0102"), nil, &impediment), event(off, "late-"+off.Format("0102"), &impediment, nil))
}

// formula is the result of the definitions over the window of a revision at its knowledge number.
func (e *seriesEnv) formula(t *testing.T, r entity.CalcRevision) entity.CalcResult {
	t.Helper()
	res, err := e.calc.Calculate(t.Context(), entity.CalcRequest{
		OrganizationID: e.orgID, TeamID: entity.ID(e.team), KnowledgeSeq: r.KnowledgeSeq,
		Window: entity.Window{From: r.Window.From, To: r.Window.To, Step: entity.StepWeek}, AsOf: r.AsOf,
	})
	require.NoError(t, err)
	return res
}

func TestCalcSeries_BuildsAndPublishesRevisionInParts(t *testing.T) {
	t.Parallel()
	e := newSeriesEnv(t)
	first := e.build(t)
	assert.Equal(t, entity.RevisionPublished, first.Status)
	assert.Equal(t, 1, first.RevisionNo)
	assert.Len(t, e.parts(t, first.ID), 5, "41 weekly points in parts of ten")

	stored := e.points(t, first.ID)
	res := e.formula(t, first)
	var want int
	for _, s := range res.Series {
		for _, p := range s.Points {
			want++
			i := indexPoint(stored, s.MetricID, s.Estimator, p.Start)
			require.GreaterOrEqual(t, i, 0, s.MetricID)
			got := stored[i]
			assert.Equal(t, p.State, got.State, s.MetricID)
			assert.Equal(t, p.Value, got.Value, s.MetricID)
			assert.Equal(t, p.N, got.N, s.MetricID)
			assert.Equal(t, p.End, got.End, s.MetricID)
		}
	}
	assert.Len(t, stored, want, "every point of the formula is stored once")

	wip := res.Series[indexSeries(res.Series, "T12", "")].Points[37]
	page, err := e.calc.PointContributions(t.Context(), e.orgID, first.ID, entity.ContributionQuery{MetricID: "T12", PointStart: wip.Start, Limit: 1})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.NotNil(t, page.Next, "the cohort pages by object")
	rest, err := e.calc.PointContributions(t.Context(), e.orgID, first.ID, entity.ContributionQuery{MetricID: "T12", PointStart: wip.Start, After: page.Next})
	require.NoError(t, err)
	assert.ElementsMatch(t, wip.Contributions, append(page.Items, rest.Items...), "the cohort of a point is the cohort of the formula")

	task := res.Tasks[slicesIndex(res.Tasks, "20003", e)]
	intervals, err := e.calc.TaskIntervals(t.Context(), e.orgID, first.ID, task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, task.Intervals, intervals, "the pieces of the parts join into the intervals of the task")
	inputs, err := e.calc.RevisionInputs(t.Context(), e.orgID, first.ID)
	require.NoError(t, err)
	assert.Len(t, inputs, len(calc.Definitions())+2, "definitions, the mapping and the structure")

	e.refresh(t, _seriesAsOf)
	e.runJobs(t, 3)
	assert.Equal(t, first.ID, e.current(t).ID, "until the last part the previous revision stays current")
	e.runJobs(t, 0)
	second := e.current(t)
	assert.Equal(t, 2, second.RevisionNo)
	assert.Equal(t, e.parts(t, first.ID), e.parts(t, second.ID), "without new facts every part is reused")
	old, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, &first.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionSuperseded, old.Status)
	assert.Equal(t, stored, e.points(t, first.ID), "the superseded revision reads the same values")
}

func TestCalcSeries_LateEventRecountsItsPartOnly(t *testing.T) {
	t.Parallel()
	e := newSeriesEnv(t)
	first := e.build(t)
	before := e.parts(t, first.ID)

	e.flag(t, "20004", time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC), time.Date(2026, 2, 18, 10, 0, 0, 0, time.UTC))
	second := e.build(t)
	after := e.parts(t, second.ID)
	require.Len(t, after, len(before))
	for i := range before {
		if i == 3 {
			assert.NotEqual(t, before[i], after[i], "the part with the week of the event is computed again")
			continue
		}
		assert.Equal(t, before[i], after[i], "part %d is reused", i+1)
	}

	e.refresh(t, _seriesAsOf)
	e.runJobs(t, 2)
	building := e.building(t)
	require.Len(t, building, 1)
	e.flag(t, "20003", time.Date(2025, 12, 2, 10, 0, 0, 0, time.UTC), time.Date(2025, 12, 3, 10, 0, 0, 0, time.UTC))
	e.recount(t)
	third, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, (*entity.ID)(&building[0]))
	require.NoError(t, err)
	assert.Contains(t, []entity.RevisionStatus{entity.RevisionPublished, entity.RevisionSuperseded}, third.Status)
	assert.Equal(t, after, e.parts(t, third.ID), "a fact that came during the chain is not read by its parts")
	assert.Less(t, third.KnowledgeSeq, e.lastSeq(t))
	latest := e.current(t)
	require.Greater(t, latest.RevisionNo, third.RevisionNo, "the fact that came during the chain queues it again")
	assert.NotEqual(t, after[2], e.parts(t, latest.ID)[2], "the next revision reads it")
}

func TestCalcSeries_ChainSurvivesCrashTakeoverAndRace(t *testing.T) {
	t.Parallel()
	e := newSeriesEnv(t)
	e.refresh(t, _seriesAsOf)
	e.runJobs(t, 1)
	j, ok := e.nextJob(t)
	require.True(t, ok)
	crash := errors.New("worker stopped before the commit")
	err := e.calc.RunSlice(t.Context(), j.args, func(context.Context) error { return crash })
	require.ErrorIs(t, err, crash)
	assert.Empty(t, e.parts(t, j.args.RevisionID), "the part of a stopped worker is not saved")
	e.runJobs(t, 0)
	first := e.current(t)
	parts := e.parts(t, first.ID)
	assert.Len(t, parts, 5, "the repeated job continues the chain without duplicates")

	e.refresh(t, _seriesAsOf)
	e.runJobs(t, 2)
	stale, ok := e.nextJob(t)
	require.True(t, ok)
	_, err = e.owner.Exec(t.Context(), "UPDATE calc.revision SET progress_at = $1 WHERE id = $2", e.clock.Now().Add(-31*time.Minute),
		uuid.UUID(stale.args.RevisionID))
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), "UPDATE river_job SET state = 'discarded', finalized_at = now() WHERE id = $1", stale.id)
	require.NoError(t, err)
	e.refresh(t, _seriesAsOf.Add(time.Hour))
	e.runJobs(t, 1)
	taken, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, &stale.args.RevisionID)
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionFailed, taken.Status, "a new chain takes over a revision whose progress stopped")
	require.NoError(t, e.calc.RunSlice(t.Context(), stale.args, completeJob(stale.id)))
	assert.Len(t, e.parts(t, stale.args.RevisionID), 1, "the job that woke after the takeover writes nothing")
	e.runJobs(t, 0)
	assert.Equal(t, 3, e.current(t).RevisionNo)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() {
			job := entity.CalcSliceJob{
				OrganizationID: e.orgID, SeriesID: e.series.ID, From: _seriesWindow.From, To: _seriesWindow.To,
				AsOf: _seriesAsOf.Add(time.Duration(i+2) * time.Hour),
			}
			errs[i] = e.calc.RunSlice(t.Context(), job, func(context.Context) error { return nil })
		})
	}
	wg.Wait()
	require.NoError(t, errors.Join(errs...))
	assert.Len(t, e.building(t), 1, "two chains of one series do not build at once")
}

func TestCalcSeries_DeletedBasisAndCollection(t *testing.T) {
	t.Parallel()
	e := newSeriesEnv(t)
	first := e.build(t)
	stored := e.points(t, first.ID)
	var task uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM normalize.source_object WHERE object_kind = 'task' AND native_id = '20003'").Scan(&task))
	n, err := e.calc.MarkBasisDeleted(t.Context(), e.orgID, entity.ID(task))
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	marked := e.current(t)
	assert.Equal(t, entity.IrreproducibleBasisDeleted, marked.IrreproducibleReason)
	assert.Equal(t, stored, e.points(t, first.ID), "the values of the revision stay")
	n, err = e.calc.MarkDefinitionRetired(t.Context(), e.orgID)
	require.NoError(t, err)
	assert.Zero(t, n, "every definition of the revision is in the registry")

	shifted := _seriesWindow
	shifted.From = shifted.From.AddDate(0, 0, 7)
	require.NoError(t, e.calc.Refresh(t.Context(), e.orgID, e.series.ID, shifted, _seriesAsOf))
	e.runJobs(t, 2)
	failed, ok := e.nextJob(t)
	require.True(t, ok)
	require.NoError(t, e.calc.FailSlice(t.Context(), failed.args))
	orphan := e.parts(t, failed.args.RevisionID)
	require.Len(t, orphan, 1)

	e.clock.Advance(25 * time.Hour)
	var gcJob int64
	require.NoError(t, e.calc.TickGC(t.Context()))
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM river_job WHERE kind = $1 AND state = 'available'", entity.KindCalcGC).Scan(&gcJob))
	require.NoError(t, e.calc.RunGC(t.Context(), entity.CalcGCJob{OrganizationID: e.orgID}, completeJob(gcJob)))
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM calc.part WHERE id = '"+orphan[0].String()+"'"),
		"a part of a failed revision only is collected")
	assert.Len(t, e.parts(t, first.ID), 5, "the parts of the published revision stay")
}

func indexPoint(points []entity.SeriesPoint, metric, estimator string, start time.Time) int {
	for i := range points {
		if points[i].MetricID == metric && points[i].Estimator == estimator && points[i].Start.Equal(start) {
			return i
		}
	}
	return -1
}

func indexSeries(series []entity.Series, metric, estimator string) int {
	for i := range series {
		if series[i].MetricID == metric && series[i].Estimator == estimator {
			return i
		}
	}
	return -1
}

func slicesIndex(tasks []entity.TaskSlice, native string, e *seriesEnv) int {
	for i := range tasks {
		var id string
		if e.owner.QueryRow(context.Background(), "SELECT native_id FROM normalize.source_object WHERE id = $1", uuid.UUID(tasks[i].TaskID)).Scan(&id) == nil && id == native {
			return i
		}
	}
	return -1
}
