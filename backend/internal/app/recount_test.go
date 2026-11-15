package app

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
)

// recountEnv is the series of seriesEnv with the collection of sync and the metrics of calc in their own registry.
type recountEnv struct {
	*seriesEnv
	reg *prometheus.Registry
}

func newRecountEnv(t *testing.T) *recountEnv {
	t.Helper()
	e := &recountEnv{seriesEnv: newSeriesEnv(t), reg: prometheus.NewRegistry()}
	orgs, _ := newOrgAccess(e.app, e.clock, nil, nil)
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
