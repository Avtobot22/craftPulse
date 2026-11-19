package app

import (
	"encoding/json"
	"errors"
	"strings"
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

type reviewEnv struct {
	*normalizeEnv
	calc *calc.UseCase
	area entity.ID
	d1   time.Time
}

// day is the n-th day of the scenario at 09:00 UTC; the first is ten days before the clock of the test.
func (e *reviewEnv) day(n int) time.Time { return e.d1.AddDate(0, 0, n-1) }

func reviewEvent(t *testing.T, id, kind, actor string, at time.Time) *envelope.Record {
	t.Helper()
	r := mrRecord(t, "mr_event", id, "9001", id, map[string]any{"mr_native_id": "9001", "event_kind": kind, "actor_account_native_id": actor, "system_note": false})
	r.RecordClass, r.PayloadSchema, r.EventTime = "event", "mr_event/2", &at
	return r
}

// newReviewEnv imports the MR 9001 of V-15 ready on d1: a review of account 2 on d2, a comment of the author 1 on d3
// and a review of account 3 on d4; accounts 2 and 3 are confirmed humans.
func newReviewEnv(t *testing.T) *reviewEnv {
	t.Helper()
	e := &reviewEnv{normalizeEnv: newNormalizeEnv(t)}
	e.d1 = e.clock.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -10).Add(9 * time.Hour)
	reviewDay := e.day
	mr := mrRecord(t, "merge_request", "9001", "", "v1", map[string]any{
		"project_native_id": "42", "iid": 7, "title": "V-15", "state": "open", "draft": false, "author_account_native_id": "1",
		"source_branch": "feature/PA-7", "target_branch": "main", "head_sha": strings.Repeat("a", 40), "reviewer_field": true,
		"reviewer_account_native_ids": []string{"3"}, "opened_at": reviewDay(1).Add(-time.Hour), "ready_at": reviewDay(1), "updated_at": reviewDay(4),
	})
	rc := e.imported(t, e.admin, pack(t, mrManifest("42"), accountRecord(t, "1", "Same Name"), accountRecord(t, "2", "Same Name"),
		accountRecord(t, "3", "Reviewer"), mr, reviewEvent(t, "e2", "review", "2", reviewDay(2)), reviewEvent(t, "e3", "comment", "1", reviewDay(3)),
		reviewEvent(t, "e4", "review", "3", reviewDay(4))))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	e.area = mrArea(t, e.normalizeEnv, rc, "42")
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	e.calc, err = newCalc(e.app, calcModules{structure: orgs, facts: e.norm}, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	e.confirm(t, "2", entity.PersonHuman)
	e.confirm(t, "3", entity.PersonHuman)
	return e
}

func (e *reviewEnv) confirm(t *testing.T, native string, kind entity.PersonKind) {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM org.source_account WHERE native_account_id = $1", native).Scan(&id))
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	req := entity.AssignmentRequest{SourceAccountID: entity.ID(id), ActorKind: kind, CreatePerson: true}
	var base, person uuid.UUID
	err = e.owner.QueryRow(t.Context(), "SELECT id, person_id FROM org.account_assignment WHERE source_account_id = $1 AND superseded_seq IS NULL",
		id).Scan(&base, &person)
	if err == nil {
		b, p := entity.ID(base), entity.ID(person)
		req.BaseAssignmentID, req.PersonID, req.CreatePerson = &b, &p, false
	}
	out, err := orgs.AssignAccounts(t.Context(), e.admin, entity.ID(uuid.New()), []entity.AssignmentRequest{req})
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, entity.AssignmentSaved, out[0].Outcome)
}

// build runs the waiting jobs of calc until none is left, moving the clock to the scheduled ones.
func (e *reviewEnv) build(t *testing.T) {
	t.Helper()
	for range 200 {
		var (
			id   int64
			kind string
			raw  []byte
		)
		err := e.owner.QueryRow(t.Context(), "SELECT id, kind, args FROM river_job WHERE kind IN ($1, $2) AND state IN ('available', 'scheduled') ORDER BY scheduled_at, id LIMIT 1",
			entity.KindCalcSlice, entity.KindCalcRecount).Scan(&id, &kind, &raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return
		}
		require.NoError(t, err)
		var at time.Time
		require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT scheduled_at FROM river_job WHERE id = $1", id).Scan(&at))
		if d := at.Sub(e.clock.Now()); d > 0 {
			e.clock.Advance(d)
		}
		if kind == entity.KindCalcRecount {
			var job entity.CalcRecountJob
			require.NoError(t, json.Unmarshal(raw, &job))
			require.NoError(t, e.calc.RunRecount(t.Context(), job, completeJob(id)))
			continue
		}
		var job entity.CalcSliceJob
		require.NoError(t, json.Unmarshal(raw, &job))
		require.NoError(t, e.calc.RunSlice(t.Context(), job, completeJob(id)))
	}
	t.Fatal("calc jobs did not end")
}

// point is the point of a metric and an estimator of the revision whose period holds at.
func (e *reviewEnv) point(t *testing.T, revision entity.ID, metric, estimator string, at time.Time) entity.SeriesPoint {
	t.Helper()
	pts, err := e.calc.RevisionPoints(t.Context(), e.orgID, revision)
	require.NoError(t, err)
	for i := range pts {
		p := &pts[i]
		if p.MetricID == metric && p.Estimator == estimator && !at.Before(p.Start) && at.Before(p.End) {
			return *p
		}
	}
	t.Fatalf("no point %s %s at %s", metric, estimator, at)
	return entity.SeriesPoint{}
}

func TestCalcReview_RepositorySeriesAndAccountKindRevision(t *testing.T) {
	t.Parallel()
	e := newReviewEnv(t)
	scope := entity.ReviewScope{RepositoryID: e.area}
	_, err := e.calc.ReviewSeries(t.Context(), e.orgID, scope, entity.MetricSetReview, entity.StepDay)
	require.Error(t, err, "the review takes weeks or months")
	review, err := e.calc.ReviewSeries(t.Context(), e.orgID, scope, entity.MetricSetReview, entity.StepWeek)
	require.NoError(t, err)
	events, err := e.calc.ReviewSeries(t.Context(), e.orgID, scope, entity.MetricSetEvents, entity.StepDay)
	require.NoError(t, err)
	e.build(t)
	reviewDay := e.day

	first, err := e.calc.Revision(t.Context(), e.orgID, review.ID, nil)
	require.NoError(t, err)
	a07 := e.point(t, first.ID, "A07", calc.EstimatorP50, reviewDay(4))
	require.NotNil(t, a07.Value)
	assert.InDelta(t, 86400, *a07.Value, 0, "the confirmed human 2 responded on d2")
	a05 := e.point(t, first.ID, "A05", "", reviewDay(4))
	assert.InDelta(t, 1, *a05.Value, 0)
	firstEvents, err := e.calc.Revision(t.Context(), e.orgID, events.ID, nil)
	require.NoError(t, err)
	assert.InDelta(t, 1, *e.point(t, firstEvents.ID, "A22", calc.ActorHuman, reviewDay(2)).Value, 0)

	e.confirm(t, "2", entity.PersonBot)
	assert.Positive(t, e.n(t, "SELECT count(*) FROM river_job WHERE kind = $1", entity.KindCalcRecount), "a new kind of an account recounts the series")
	e.build(t)
	second, err := e.calc.Revision(t.Context(), e.orgID, review.ID, nil)
	require.NoError(t, err)
	require.Greater(t, second.RevisionNo, first.RevisionNo)
	a07 = e.point(t, second.ID, "A07", calc.EstimatorP50, reviewDay(4))
	assert.InDelta(t, 3*86400, *a07.Value, 0, "a bot does not end the wait: A07 is three days")
	old := e.point(t, first.ID, "A07", calc.EstimatorP50, reviewDay(4))
	assert.InDelta(t, 86400, *old.Value, 0, "the former revision keeps its values")
	secondEvents, err := e.calc.Revision(t.Context(), e.orgID, events.ID, nil)
	require.NoError(t, err)
	assert.InDelta(t, 1, *e.point(t, secondEvents.ID, "A22", calc.ActorBot, reviewDay(2)).Value, 0)
	assert.InDelta(t, 0, *e.point(t, secondEvents.ID, "A22", calc.ActorHuman, reviewDay(2)).Value, 0)

	waits, err := e.calc.ReviewWaits(t.Context(), e.orgID, e.area, math64(), reviewDay(3))
	require.NoError(t, err)
	require.Len(t, waits, 1)
	for _, w := range waits {
		assert.Equal(t, entity.ReviewAwaitingFirst, w.Kind, "the comment of the author and the review of a bot do not end the wait")
	}

	for range 3 {
		e.imported(t, e.admin, pack(t, mrManifest("42"), reviewEvent(t, "e2", "review", "2", reviewDay(2))))
		e.drain(t)
	}
	assert.Equal(t, 3, e.n(t, "SELECT count(*) FROM normalize.mr_event"), "repeated events add nothing to A22")
}

func math64() int64 { return 1<<63 - 1 }

func TestCalcReview_TeamEventsThroughProjectAreas(t *testing.T) {
	t.Parallel()
	e := newSeriesEnv(t)
	series, err := e.calc.ReviewSeries(t.Context(), e.orgID, entity.ReviewScope{TeamID: entity.ID(e.team)}, entity.MetricSetEvents, entity.StepDay)
	require.NoError(t, err)
	var first time.Time
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT min(event_time) FROM normalize.task_event").Scan(&first))
	from := first.UTC().Truncate(24 * time.Hour)
	w := entity.Window{From: from, To: from.AddDate(0, 0, 14), Step: entity.StepDay}
	require.NoError(t, e.calc.Refresh(t.Context(), e.orgID, series.ID, w, w.To))
	e.runJobs(t, 0)
	rev, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	require.Equal(t, entity.RevisionPublished, rev.Status)
	var want int
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT count(*) FROM normalize.task_event WHERE superseded_seq IS NULL AND event_time >= $1 AND event_time < $2",
		w.From, w.To).Scan(&want))
	require.Positive(t, want)
	pts := e.points(t, rev.ID)
	got := 0.0
	for _, p := range pts {
		require.Equal(t, "A22", p.MetricID)
		got += *p.Value
	}
	assert.InDelta(t, float64(want), got, 0, "every event of the tasks of the team is counted once, under the kind of its participant")
	assert.Len(t, pts, 14*4, "a point a day for each kind of participant")
}
