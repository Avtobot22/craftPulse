package app

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workerctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/worker"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

// recountEnv is the series of seriesEnv with the collection of sync and the metrics of calc in their own registry.
type recountEnv struct {
	*seriesEnv
	reg *prometheus.Registry
}

func newRecountEnv(t *testing.T) *recountEnv {
	t.Helper()
	e := &recountEnv{seriesEnv: newSeriesEnv(t), reg: prometheus.NewRegistry()}
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	c, err := newCalc(e.app, calcModules{structure: orgs, facts: e.runner.norm, settings: e.process, collection: e.runner.sync}, e.clock, e.reg)
	require.NoError(t, err)
	e.calc = c
	return e
}

// counter is the value of a counter of calc.
func (e *recountEnv) counter(t *testing.T, name string) float64 {
	t.Helper()
	families, err := e.reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == name {
			return f.GetMetric()[0].GetCounter().GetValue()
		}
	}
	return 0
}

// scheduled are the times of the chains of the series waiting in the queue.
func (e *recountEnv) scheduled(t *testing.T) []time.Time {
	t.Helper()
	rows, err := e.owner.Query(t.Context(), "SELECT scheduled_at FROM river_job WHERE kind = $1 AND state IN ('available', 'scheduled') ORDER BY id",
		entity.KindCalcSlice)
	require.NoError(t, err)
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var at time.Time
		require.NoError(t, rows.Scan(&at))
		out = append(out, at)
	}
	require.NoError(t, rows.Err())
	return out
}

func (e *recountEnv) waiting(t *testing.T, kind string) int {
	t.Helper()
	return count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind = '"+kind+"' AND state IN ('available', 'scheduled', 'running')")
}

// partHolds tells whether the stored part covers the moment.
func (e *recountEnv) partHolds(t *testing.T, part uuid.UUID, at time.Time) bool {
	t.Helper()
	var holds bool
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT part_range @> $2::timestamptz FROM calc.part WHERE id = $1", part, at).Scan(&holds))
	return holds
}

func (e *recountEnv) mappingInput(t *testing.T, revision entity.ID) string {
	t.Helper()
	inputs, err := e.calc.RevisionInputs(t.Context(), e.orgID, revision)
	require.NoError(t, err)
	for _, in := range inputs {
		if in.Kind == entity.InputKindMapping {
			return in.Version
		}
	}
	return ""
}

func (e *recountEnv) refreshSeries(t *testing.T, b *browser, series entity.ID) (code int, op operationBody, location string) {
	t.Helper()
	rec := e.send(t, b, http.MethodPost, "/api/v1/series/"+series.String()+"/refreshes", http.Header{"Origin": {_site}}, http.NoBody)
	if rec.Code != http.StatusAccepted {
		return rec.Code, op, ""
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &op))
	return rec.Code, op, rec.Header().Get("Location")
}

// TestRecount_NewPackageGetsRevision is AC-14: a normalized package queues the recount of the area after the delay,
// the series gets a new revision without a request and the previous one reads its values.
func TestRecount_NewPackageGetsRevision(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	e.recount(t)
	first := e.current(t)
	stored := e.points(t, first.ID)
	assert.Zero(t, e.waiting(t, entity.KindCalcSlice), "the confirmation of the mapping and the collection of the setup are counted")

	e.flag(t, "20004", time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC), time.Date(2026, 2, 18, 10, 0, 0, 0, time.UTC))
	require.Equal(t, 1, e.waiting(t, entity.KindCalcRecount), "the last part of normalize.batch queues the recount of its area")
	queuedAt := e.clock.Now()
	e.runRecounts(t)
	at := e.scheduled(t)
	require.Len(t, at, 1, "one chain of the series of the team")
	assert.WithinDuration(t, queuedAt.Add(30*time.Second), at[0], time.Millisecond, "the facts of 30 s go into one chain")
	assert.Zero(t, e.runJobs(t, 0), "the chain waits for its delay")

	e.recount(t)
	second := e.current(t)
	assert.Greater(t, second.RevisionNo, first.RevisionNo)
	assert.Greater(t, second.KnowledgeSeq, first.KnowledgeSeq)
	assert.False(t, second.Loading, "the connection is incremental")
	old, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, &first.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionSuperseded, old.Status)
	assert.Equal(t, stored, e.points(t, first.ID), "the previous revision reads its values")
}

// TestRecount_LateEventRecountsItsWeek is AC-06 for a late event: of the parts of the window only the one with the
// week of the event and the last one, whose as_of moved, are computed; the others are taken from the previous
// revision, which stays readable.
func TestRecount_LateEventRecountsItsWeek(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	e.recount(t)
	first := e.current(t)
	before := e.parts(t, first.ID)
	require.Greater(t, len(before), 2)
	reused, computed := e.counter(t, "craftpulse_calc_parts_reused_total"), e.counter(t, "craftpulse_calc_parts_computed_total")

	late := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	e.flag(t, "20004", late, late.Add(24*time.Hour))
	e.recount(t)
	second := e.current(t)
	after := e.parts(t, second.ID)
	require.Len(t, after, len(before))
	last := len(after) - 1
	for i := range before {
		switch {
		case e.partHolds(t, before[i], late):
			assert.NotEqual(t, before[i], after[i], "the part with the week of the event is computed again")
		case i == last:
			assert.NotEqual(t, before[i], after[i], "the last part reads the new as_of")
		default:
			assert.Equal(t, before[i], after[i], "part %d is reused", i+1)
		}
	}
	assert.Equal(t, computed+2, e.counter(t, "craftpulse_calc_parts_computed_total"))
	assert.Equal(t, reused+float64(len(after)-2), e.counter(t, "craftpulse_calc_parts_reused_total"))
	old, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, &first.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionSuperseded, old.Status, "the previous revision stays")
}

// TestRecount_FactDuringChainQueuesItAgain: a fact normalized while the chain builds is not read by its parts, and
// the chain queues itself again before it ends instead of losing the fact (TZ 10.6).
func TestRecount_FactDuringChainQueuesItAgain(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	e.recount(t)
	e.flag(t, "20004", time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC), time.Date(2026, 2, 18, 10, 0, 0, 0, time.UTC))
	e.runRecounts(t)
	require.True(t, e.wake(t))
	e.runJobs(t, 2)
	building := e.building(t)
	require.Len(t, building, 1)

	e.flag(t, "20003", time.Date(2026, 3, 3, 10, 0, 0, 0, time.UTC), time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC))
	seq := e.lastSeq(t)
	e.recount(t)
	during, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, (*entity.ID)(&building[0]))
	require.NoError(t, err)
	assert.Contains(t, []entity.RevisionStatus{entity.RevisionPublished, entity.RevisionSuperseded}, during.Status)
	assert.Less(t, during.KnowledgeSeq, seq, "the chain keeps the cutoff of its first part")
	latest := e.current(t)
	assert.Greater(t, latest.RevisionNo, during.RevisionNo, "the chain queued itself again")
	assert.GreaterOrEqual(t, latest.KnowledgeSeq, seq, "the next revision reads the fact")
}

// TestRecount_NewMappingVersion is AC-03 for revisions: a new version of the mapping queues the recount of the team;
// the revision on the previous version keeps it among its inputs.
func TestRecount_NewMappingVersion(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	e.recount(t)
	first := e.current(t)
	require.Equal(t, "1", e.mappingInput(t, first.ID))

	rule := func(kind entity.RuleKind, ref, meaning string) entity.MappingRule {
		return entity.MappingRule{Kind: kind, InstanceID: entity.ID(e.instance), NativeRef: ref, Meaning: meaning}
	}
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	_, created, err := e.process.CreateMappingVersion(t.Context(), admin, entity.ID(e.team), 1, entity.ID(uuid.New()), entity.MappingDraft{
		Timezone: "Europe/Moscow", Rules: []entity.MappingRule{
			rule(entity.RuleStatus, "1", entity.MeaningIntake), rule(entity.RuleStatus, "3", entity.MeaningActive),
			rule(entity.RuleStatus, "5", entity.MeaningDone), rule(entity.RuleResolution, "10000", entity.MeaningAccepted),
		},
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, 1, e.waiting(t, entity.KindCalcRecount), "the version queues the recount of its team")

	e.recount(t)
	second := e.current(t)
	assert.Greater(t, second.RevisionNo, first.RevisionNo)
	assert.Equal(t, "2", e.mappingInput(t, second.ID))
	assert.Equal(t, "1", e.mappingInput(t, first.ID), "the previous revision keeps the version it was computed with")
}

// TestRecount_LoadingLimitsChains: while the history of an area loads, a series gets one chain in 5 minutes and its
// revisions are marked loading (FR-015).
func TestRecount_LoadingLimitsChains(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	_, err := e.owner.Exec(t.Context(), "UPDATE sync.state SET phase = 'backfill' WHERE connection_id = $1", e.conn)
	require.NoError(t, err)
	e.recount(t)
	first := e.current(t)
	assert.True(t, first.Loading, "the revision during the initial load is marked")

	e.flag(t, "20004", time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC), time.Date(2026, 2, 18, 10, 0, 0, 0, time.UTC))
	e.runRecounts(t)
	at := e.scheduled(t)
	require.Len(t, at, 1)
	assert.WithinDuration(t, first.StartedAt.Add(5*time.Minute), at[0], time.Millisecond, "not sooner than 5 minutes after the last chain")
	e.recount(t)
	assert.True(t, e.current(t).Loading)
}

// TestRecount_RefreshSeries: a manual refresh queues one chain with the definitions of a planned one; a repeat
// during the chain queues nothing more and returns the same descriptor, which leads to the new revision. A series
// of a team outside the area of the user is not found.
func TestRecount_RefreshSeries(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	e.recount(t)
	planned := e.current(t)

	code, op, location := e.refreshSeries(t, e.admin, e.series.ID)
	require.Equal(t, http.StatusAccepted, code)
	assert.Equal(t, "series_refresh", op.Kind)
	assert.Equal(t, "queued", op.Status)
	assert.Equal(t, "/api/v1/operations/"+op.ID.String(), location)
	assert.Equal(t, 1, e.waiting(t, entity.KindCalcSlice), "the refresh is queued at once")
	code, again, _ := e.refreshSeries(t, e.admin, e.series.ID)
	require.Equal(t, http.StatusAccepted, code)
	assert.Equal(t, op.ID, again.ID, "a repeat returns the active refresh")
	assert.Equal(t, 1, e.waiting(t, entity.KindCalcSlice))

	e.runJobs(t, 2)
	code, running, _ := e.refreshSeries(t, e.admin, e.series.ID)
	require.Equal(t, http.StatusAccepted, code)
	assert.Equal(t, op.ID, running.ID)
	assert.Equal(t, "running", running.Status)
	assert.Equal(t, 1, e.waiting(t, entity.KindCalcSlice), "a refresh during the chain does not start a second one")
	require.NoError(t, persistent.NewTransactor(e.app).InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
		_, err := e.calc.RequestRefresh(ctx, e.orgID, e.series.ID, entity.ID(uuid.New()))
		return err
	}))
	assert.Equal(t, 1, e.waiting(t, entity.KindCalcSlice), "the queue drops the refresh of another user while the chain builds")

	e.runJobs(t, 0)
	manual := e.current(t)
	require.Greater(t, manual.RevisionNo, planned.RevisionNo)
	code, done := e.operation(t, e.admin, location)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "succeeded", done.Status)
	require.NotNil(t, done.ResultURI)
	assert.Equal(t, "/api/v1/revisions/"+manual.ID.String(), *done.ResultURI)
	definitions := func(id entity.ID) []entity.RevisionInput {
		inputs, err := e.calc.RevisionInputs(t.Context(), e.orgID, id)
		require.NoError(t, err)
		var out []entity.RevisionInput
		for _, in := range inputs {
			if in.Kind == entity.InputKindDefinition {
				out = append(out, in)
			}
		}
		return out
	}
	assert.Equal(t, definitions(planned.ID), definitions(manual.ID), "the manual refresh uses the definitions of a planned one")
	assert.Equal(t, planned.Window.From, manual.Window.From, "and the window of the series")

	other, err := e.calc.TeamSeries(t.Context(), e.orgID, entity.ID(uuid.New()), entity.StepWeek)
	require.NoError(t, err)
	code, _, _ = e.refreshSeries(t, e.admin, other.ID)
	assert.Equal(t, http.StatusNotFound, code, "a series of a team outside the area")
	code, _, _ = e.refreshSeries(t, e.admin, entity.ID(uuid.New()))
	assert.Equal(t, http.StatusNotFound, code)
}

// TestRecount_DailyExtension: the hourly tick queues the extension of the window at the minute of the series in
// 01:00-05:00 of the time zone of the team by the managed clock, once a night.
func TestRecount_DailyExtension(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	e.recount(t)
	first := e.current(t)
	loc, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	local := e.clock.Now().In(loc)
	night := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 30, 0, 0, loc)
	e.clock.Advance(night.Sub(e.clock.Now()))

	extend := func() {
		require.NoError(t, e.calc.TickExtend(t.Context()))
		rows, err := e.owner.Query(t.Context(), "SELECT id, args FROM river_job WHERE kind = $1 AND state = 'available' ORDER BY id",
			entity.KindCalcExtend)
		require.NoError(t, err)
		jobs := map[int64]entity.CalcExtendJob{}
		for rows.Next() {
			var (
				id  int64
				raw []byte
				job entity.CalcExtendJob
			)
			require.NoError(t, rows.Scan(&id, &raw))
			require.NoError(t, json.Unmarshal(raw, &job))
			jobs[id] = job
		}
		require.NoError(t, rows.Err())
		ours := 0
		for id, job := range jobs {
			if job.OrganizationID == e.orgID {
				ours++
			}
			require.NoError(t, e.calc.RunExtend(t.Context(), job, completeJob(id)))
		}
		assert.Equal(t, 1, ours, "every organization gets its own job")
	}
	var slot time.Time
	for range 5 {
		extend()
		if at := e.scheduled(t); len(at) > 0 {
			require.Len(t, at, 1)
			slot = at[0]
			break
		}
		e.clock.Advance(time.Hour)
	}
	require.False(t, slot.IsZero(), "the extension is queued during the night")
	start, end := time.Date(night.Year(), night.Month(), night.Day(), 1, 0, 0, 0, loc), time.Date(night.Year(), night.Month(), night.Day(), 5, 0, 0, 0, loc)
	assert.False(t, slot.Before(start), "not before 01:00")
	assert.True(t, slot.Before(end), "before 05:00")
	assert.False(t, slot.Before(e.clock.Now()), "at the minute of the series")
	extend()
	assert.Len(t, e.scheduled(t), 1, "a repeated tick does not queue it twice")

	require.True(t, e.wake(t))
	e.runJobs(t, 0)
	extended := e.current(t)
	assert.Greater(t, extended.RevisionNo, first.RevisionNo)
	assert.True(t, extended.AsOf.Equal(slot), "the revision is as of the minute of the series")
	assert.False(t, extended.Window.To.Before(first.Window.To))
	assert.Equal(t, first.Window.From, extended.Window.From, "the window keeps its start")
	e.clock.Advance(time.Hour)
	extend()
	assert.Empty(t, e.scheduled(t), "the series is extended once a night")
}

// TestRecount_RefreshTakesOverStoppedChain: a refresh while the series holds a building revision without progress
// for 30 minutes is answered by the new chain that takes it over, not by the failed one.
func TestRecount_RefreshTakesOverStoppedChain(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	e.recount(t)
	e.refresh(t, _seriesAsOf)
	e.runJobs(t, 2)
	stopped := e.building(t)
	require.Len(t, stopped, 1)
	_, err := e.owner.Exec(t.Context(), "UPDATE calc.revision SET progress_at = $1 WHERE id = $2", e.clock.Now().Add(-31*time.Minute), stopped[0])
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), "UPDATE river_job SET state = 'discarded', finalized_at = now() WHERE kind = $1 AND state = 'available'",
		entity.KindCalcSlice)
	require.NoError(t, err)

	code, op, location := e.refreshSeries(t, e.admin, e.series.ID)
	require.Equal(t, http.StatusAccepted, code)
	e.runJobs(t, 0)
	taken, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, (*entity.ID)(&stopped[0]))
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionFailed, taken.Status, "the new chain takes the stopped revision over")
	code, done := e.operation(t, e.admin, location)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, op.ID, done.ID)
	assert.Equal(t, "succeeded", done.Status, "the refresh is answered by the revision of the new chain")
	require.NotNil(t, done.ResultURI)
	assert.Equal(t, "/api/v1/revisions/"+e.current(t).ID.String(), *done.ResultURI)
}

// TestRecount_SkipsSeriesOfAnotherScope: a series whose scope is not a team does not stop the recount of the others.
func TestRecount_SkipsSeriesOfAnotherScope(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	_, err := e.owner.Exec(t.Context(), `INSERT INTO calc.series (organization_id, scope_hash, scope, metric_set, step, consumed_seq)
		VALUES ($1, sha256('project'), '{"kind": "project"}', $2, 'week', 0)`, uuid.UUID(e.orgID), entity.MetricSetFlow)
	require.NoError(t, err)
	e.recount(t)
	assert.Equal(t, entity.RevisionPublished, e.current(t).Status, "the series of the team is counted")
}

// publishingSettings finishes the chain of another revision once, when a start reads the mapping: after the start
// read the series and before it writes its revision.
type publishingSettings struct {
	calc.Settings
	once    sync.Once
	publish func()
}

func (s *publishingSettings) MappingAt(ctx context.Context, orgID, teamID entity.ID, seq int64) (entity.MappingVersion, bool, error) {
	s.once.Do(s.publish)
	return s.Settings.MappingAt(ctx, orgID, teamID, seq)
}

// TestRecount_ChainStartedWhileAnotherPublishes: a chain whose start read the series before another chain published
// replaces the revision that chain published and publishes itself.
func TestRecount_ChainStartedWhileAnotherPublishes(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	e.recount(t)
	e.refresh(t, _seriesAsOf)
	e.runJobs(t, 4)
	require.Len(t, e.building(t), 1, "the first chain waits for its last part")

	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	settings := &publishingSettings{Settings: e.process, publish: func() { e.runJobs(t, 0) }}
	starter, err := newCalc(e.app, calcModules{structure: orgs, facts: e.runner.norm, settings: settings, collection: e.runner.sync}, e.clock,
		prometheus.NewRegistry())
	require.NoError(t, err)
	first := e.current(t)
	job := entity.CalcSliceJob{OrganizationID: e.orgID, SeriesID: e.series.ID, From: _seriesWindow.From, To: _seriesWindow.To, AsOf: _seriesAsOf}
	require.NoError(t, starter.RunSlice(t.Context(), job, func(context.Context) error { return nil }))
	published := e.current(t)
	require.NotEqual(t, first.ID, published.ID, "the other chain published during the start")

	e.runJobs(t, 0)
	second := e.current(t)
	require.NotEqual(t, published.ID, second.ID, "the chain of the start publishes after the other one")
	require.NotNil(t, second.PreviousRevisionID)
	assert.Equal(t, published.ID, *second.PreviousRevisionID)
	replaced, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, &published.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionSuperseded, replaced.Status)
}

// TestRecount_PartsReadCollectionOfTheStart: the parts of a chain read the state of collection the first part fixed,
// so a reconciliation that ends during the chain does not split the revision.
func TestRecount_PartsReadCollectionOfTheStart(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	first := e.build(t)

	e.refresh(t, _seriesAsOf)
	e.runJobs(t, 2)
	_, err := e.owner.Exec(t.Context(), `UPDATE sync.state SET completeness = CASE completeness WHEN 'complete' THEN 'partial' ELSE 'complete' END
		WHERE connection_id = $1`, e.conn)
	require.NoError(t, err)
	e.runJobs(t, 0)
	during := e.current(t)
	require.Greater(t, during.RevisionNo, first.RevisionNo)
	assert.Equal(t, e.parts(t, first.ID), e.parts(t, during.ID), "every part reads the collection of the start")

	e.recount(t)
	after := e.current(t)
	require.Greater(t, after.RevisionNo, during.RevisionNo, "the chain queued itself again")
	assert.NotEqual(t, e.parts(t, first.ID), e.parts(t, after.ID), "the next chain reads the changed collection")
}

// TestRecount_CollectionChangeDuringChainQueuesItAgain: a reconciliation that changes the collection while a chain
// builds asks for a recount the queue drops as a copy of that chain, so the chain queues itself again.
func TestRecount_CollectionChangeDuringChainQueuesItAgain(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	e.recount(t)
	e.refresh(t, _seriesAsOf)
	e.runJobs(t, 2)
	building := e.building(t)
	require.Len(t, building, 1)

	_, err := e.owner.Exec(t.Context(), `UPDATE sync.state SET completeness = CASE completeness WHEN 'complete' THEN 'partial' ELSE 'complete' END
		WHERE connection_id = $1`, e.conn)
	require.NoError(t, err)
	for {
		j, ok := e.nextJob(t)
		if !ok {
			break
		}
		require.NoError(t, e.calc.RunSlice(t.Context(), j.args, completeJob(j.id)))
	}
	during, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, (*entity.ID)(&building[0]))
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionPublished, during.Status)
	assert.Equal(t, 1, count(t, t.Context(), e.owner, `SELECT count(*) FROM river_job WHERE kind = '`+entity.KindCalcSlice+
		`' AND state = 'scheduled' AND (args->>'from')::timestamptz = '`+_seriesWindow.From.Format(time.RFC3339)+`'`),
		"the chain that read the previous collection queues itself again")
}

// TestRecount_StructureChange: a new structure queues the recount of every series of the organization, and a chain
// during which the structure changed queues itself again.
func TestRecount_StructureChange(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	e.recount(t)
	e.refresh(t, _seriesAsOf)
	e.runJobs(t, 2)
	building := e.building(t)
	require.Len(t, building, 1)

	st, tag := e.structure(t, e.admin)
	in := st.input()
	in.Projects = append(in.Projects, named{Name: "Platform", Status: "active"})
	rec := e.replace(t, e.admin, tag, in)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, count(t, t.Context(), e.owner, `SELECT count(*) FROM river_job WHERE kind = '`+entity.KindCalcRecount+
		`' AND state = 'available' AND NOT args ? 'area_id' AND NOT args ? 'team_id'`), "the recount of the whole organization")

	for {
		j, ok := e.nextJob(t)
		if !ok {
			break
		}
		require.NoError(t, e.calc.RunSlice(t.Context(), j.args, completeJob(j.id)))
	}
	during, err := e.calc.Revision(t.Context(), e.orgID, e.series.ID, (*entity.ID)(&building[0]))
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionPublished, during.Status)
	assert.Equal(t, 1, count(t, t.Context(), e.owner, `SELECT count(*) FROM river_job WHERE kind = '`+entity.KindCalcSlice+
		`' AND state = 'scheduled' AND (args->>'from')::timestamptz = '`+_seriesWindow.From.Format(time.RFC3339)+`'`),
		"the chain that read the previous structure queues itself again")

	e.recount(t)
	assert.Greater(t, e.current(t).RevisionNo, during.RevisionNo)
}

// TestRecount_RiverWorkerRunsTheChain runs the jobs of calc in a River client as the worker does: the recounts of the
// setup, a manual refresh and a new mapping each end in a published revision. The clock of calc is an hour behind,
// so the delayed chains are due at once.
func TestRecount_RiverWorkerRunsTheChain(t *testing.T) {
	t.Parallel()
	e := newRecountEnv(t)
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	behind := clock.NewManual(time.Now().Add(-time.Hour))
	worker, err := newCalc(e.app, calcModules{structure: orgs, facts: e.runner.norm, settings: e.process, collection: e.runner.sync}, behind,
		prometheus.NewRegistry())
	require.NoError(t, err)
	workers := river.NewWorkers()
	workerctrl.Register(workers, workerctrl.Deps{Collection: e.runner.sync, Normalizer: e.runner.norm, Calculator: worker})
	client, err := river.NewClient(riverpgxv5.New(e.app), &river.Config{
		Queues: map[string]river.QueueConfig{entity.QueueBulk: {MaxWorkers: 2}}, Workers: workers,
		JobTimeout: _jobTimeout, RescueStuckJobsAfter: _rescueAfter, FetchCooldown: 20 * time.Millisecond, FetchPollInterval: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NoError(t, client.Start(context.WithoutCancel(t.Context())))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Stop(ctx)
	})
	settled := func() bool {
		return count(t, t.Context(), e.owner, "SELECT count(*) FROM river_job WHERE kind LIKE 'calc.%' AND state NOT IN ('completed', 'discarded')") == 0
	}
	published := func() int {
		return count(t, t.Context(), e.owner, "SELECT count(*) FROM calc.revision WHERE series_id = '"+e.series.ID.String()+"' AND status = 'published'")
	}
	require.Eventually(t, func() bool { return settled() && published() == 1 }, 30*time.Second, 100*time.Millisecond,
		"the recounts of the setup publish the first revision")
	first := e.current(t)

	code, op, location := e.refreshSeries(t, e.admin, e.series.ID)
	require.Equal(t, http.StatusAccepted, code)
	require.Eventually(t, func() bool {
		_, got := e.operation(t, e.admin, location)
		return got.Status == "succeeded"
	}, 30*time.Second, 100*time.Millisecond, "the refresh ends in a revision")
	_, done := e.operation(t, e.admin, location)
	assert.Equal(t, op.ID, done.ID)
	manual := e.current(t)
	assert.Greater(t, manual.RevisionNo, first.RevisionNo)
	assert.Equal(t, "/api/v1/revisions/"+manual.ID.String(), *done.ResultURI)

	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	rule := func(kind entity.RuleKind, ref, meaning string) entity.MappingRule {
		return entity.MappingRule{Kind: kind, InstanceID: entity.ID(e.instance), NativeRef: ref, Meaning: meaning}
	}
	_, _, err = e.process.CreateMappingVersion(t.Context(), admin, entity.ID(e.team), 1, entity.ID(uuid.New()), entity.MappingDraft{
		Timezone: "Europe/Moscow", Rules: []entity.MappingRule{
			rule(entity.RuleStatus, "1", entity.MeaningIntake), rule(entity.RuleStatus, "3", entity.MeaningActive),
			rule(entity.RuleStatus, "5", entity.MeaningDone), rule(entity.RuleResolution, "10000", entity.MeaningAccepted),
		},
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return settled() && e.mappingInput(t, e.current(t).ID) == "2" }, 30*time.Second, 100*time.Millisecond,
		"the new mapping is counted by the worker")
	assert.Zero(t, count(t, t.Context(), e.owner, "SELECT count(*) FROM calc.revision WHERE series_id = '"+e.series.ID.String()+"' AND status = 'failed'"))
	var failures []string
	rows, err := e.owner.Query(t.Context(), "SELECT kind || ': ' || errors::text FROM river_job WHERE kind LIKE 'calc.%' AND errors IS NOT NULL")
	require.NoError(t, err)
	failures, err = pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	assert.Empty(t, failures, "no job of calc failed an attempt")
}
