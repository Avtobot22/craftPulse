package app

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent"
	linkrepo "github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/link"
	"github.com/Avtobot22/craftPulse/backend/internal/repo/persistent/platform"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
	linkuc "github.com/Avtobot22/craftPulse/backend/internal/usecase/link"
	"github.com/Avtobot22/craftPulse/backend/pkg/clock"
)

func activityCalculator(t *testing.T, pool *pgxpool.Pool, clk *clock.Manual, facts calc.Facts, collection calc.ActivityCollection, settings calc.Settings) *calc.UseCase {
	t.Helper()
	orgs, _, err := newOrgAccess(pool, clk, nil, nil)
	require.NoError(t, err)
	c, err := newCalc(pool, calcModules{structure: orgs, facts: facts, activity: collection, settings: settings}, clk, prometheus.NewRegistry())
	require.NoError(t, err)
	return c
}

func activityStoredPoint(t *testing.T, c *calc.UseCase, org, rev entity.ID, metric, estimator string, at time.Time) entity.SeriesPoint {
	t.Helper()
	points, err := c.RevisionPoints(t.Context(), org, rev)
	require.NoError(t, err)
	for i := range points {
		point := &points[i]
		if point.MetricID == metric && point.Estimator == estimator && !at.Before(point.Start) && at.Before(point.End) {
			return *point
		}
	}
	t.Fatalf("no point %s %s at %s", metric, estimator, at)
	return entity.SeriesPoint{}
}

func TestCalcActivity_ConnectionHistoryRecoveryAndSyncStatus(t *testing.T) {
	t.Parallel()
	e := newConnectionsEnv(t, testadapter.Config{})
	created, tag := e.create(t, jiraConnection("https://example.atlassian.net", "10000"))
	probe := e.probe(t, created.ID)
	require.Equal(t, "succeeded", probe.Status)
	rec := e.activate(t, created.ID, probe.ID, tag)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	conn := entity.ID(created.ID)

	limited := false
	for q := e.runner.next(t); q != nil; q = e.runner.next(t) {
		if q.kind == entity.KindSyncHistory && limited {
			break
		}
		if wait := q.scheduledAt.Sub(e.clock.Now()); wait > 0 {
			e.clock.Advance(wait)
		}
		if q.kind == entity.KindSyncHistory {
			limited = true
			require.NoError(t, e.adapter.src.SetConfig(testadapter.Config{Faults: []testadapter.Fault{
				{Operation: testadapter.OpFetch, SchemaID: "task_events", Kind: testadapter.FaultRateLimited, RetryAfterSeconds: 3600},
			}}))
			require.NoError(t, e.runner.run(t, q))
			require.NoError(t, e.adapter.src.SetConfig(testadapter.Config{}))
			continue
		}
		require.NoError(t, e.runner.run(t, q))
	}
	require.True(t, limited)

	c := activityCalculator(t, e.app, e.clock, e.runner.norm, e.runner.sync, nil)
	buildCalcJobs(t, e.owner, e.clock, c)
	scope := entity.ActivityScope{ConnectionID: conn}
	week, err := c.ActivitySeries(t.Context(), e.orgID, scope, entity.StepWeek)
	require.NoError(t, err)
	month, err := c.ActivitySeries(t.Context(), e.orgID, scope, entity.StepMonth)
	require.NoError(t, err)
	first, err := c.Revision(t.Context(), e.orgID, week.ID, nil)
	require.NoError(t, err)
	_, err = c.Revision(t.Context(), e.orgID, month.ID, nil)
	require.NoError(t, err)

	e.admin = e.loginEnv.admin(t, "demo")
	r := e.do(t, e.admin, http.MethodGet, "/api/v1/connections/"+conn.String()+"/sync-status", nil)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	status := decode[syncStatusBody](t, r)
	require.NotNil(t, status.LastReconcile)
	require.NotNil(t, status.LastReconcile.FinishedAt)
	taskIndex := slices.IndexFunc(status.DataKinds, func(k syncKindBody) bool { return k.DataKind == "tasks" })
	require.NotEqual(t, -1, taskIndex)
	data := status.DataKinds[taskIndex]
	for est, at := range map[string]*time.Time{
		"tasks:last_success_at": data.LastSuccessAt, "tasks:watermark": data.Watermark,
		"last_reconcile_at": status.LastReconcile.FinishedAt,
	} {
		require.NotNil(t, at)
		p := activityStoredPoint(t, c, e.orgID, first.ID, "A27", est, first.AsOf.Add(-time.Microsecond))
		require.NotNil(t, p.Value)
		assert.InDelta(t, float64(at.UnixMicro())/1e6, *p.Value, 1e-6, est)
	}

	tallyAt := *status.LastReconcile.FinishedAt
	history := activityStoredPoint(t, c, e.orgID, first.ID, "A24", "", tallyAt)
	require.NotNil(t, history.Value)
	assert.Less(t, *history.Value, 1.0)
	assert.Positive(t, *history.Numerator)
	assert.Positive(t, history.ExcludedCount)
	earliest := activityStoredPoint(t, c, e.orgID, first.ID, "A24", "history_from", tallyAt)
	require.NotNil(t, earliest.Value)
	lag := activityStoredPoint(t, c, e.orgID, first.ID, "A23", "backfill:p95", tallyAt)
	require.NotNil(t, lag.Value)
	assert.Positive(t, *lag.Value)

	var cutID uuid.UUID
	require.NoError(t, e.owner.QueryRow(t.Context(), "SELECT id FROM normalize.source_object WHERE object_kind='task' AND native_id='20003'").Scan(&cutID))
	checkCut := func(rev entity.ID, role entity.ContributionRole, reason string) {
		t.Helper()
		items, err := c.PointContributions(t.Context(), e.orgID, rev, entity.ContributionQuery{MetricID: "A24", PointStart: history.Start})
		require.NoError(t, err)
		p := contributionOf(entity.SeriesPoint{Point: entity.Point{Contributions: items.Items}}, entity.ID(cutID), role)
		require.NotNil(t, p)
		assert.Equal(t, reason, p.ReasonCode)
	}
	checkCut(first.ID, entity.RoleExcluded, "history_truncated")

	e.settle(t)
	buildCalcJobs(t, e.owner, e.clock, c)
	second, err := c.Revision(t.Context(), e.orgID, week.ID, nil)
	require.NoError(t, err)
	require.Greater(t, second.RevisionNo, first.RevisionNo)
	checkCut(second.ID, entity.RoleNumerator, "")
	checkCut(first.ID, entity.RoleExcluded, "history_truncated")
}

func TestCalcActivity_TeamActorAssignmentKeepsPreviousRevision(t *testing.T) {
	t.Parallel()
	e := newReviewEnv(t)
	coll, err := newSync(e.app, e.keyring, e.connect, e.uc, e.norm, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	e.calc = activityCalculator(t, e.app, e.clock, e.norm, coll, nil)
	org, team := uuid.UUID(e.orgID), uuid.New()
	_, err = e.owner.Exec(t.Context(), "INSERT INTO org.team (organization_id,id,name,status,created_at) VALUES ($1,$2,'Activity','active',$3)", org, team, e.day(1))
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), "INSERT INTO org.team_project (organization_id,team_id,project_id,valid,known_seq) VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)",
		org, team, uuid.UUID(e.project), e.day(1))
	require.NoError(t, err)
	_, err = e.owner.Exec(t.Context(), "INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq) VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1) ON CONFLICT DO NOTHING",
		org, uuid.UUID(e.project), uuid.UUID(e.area), e.day(1))
	require.NoError(t, err)

	s, err := e.calc.ActivitySeries(t.Context(), e.orgID, entity.ActivityScope{TeamID: entity.ID(team)}, entity.StepMonth)
	require.NoError(t, err)
	window := entity.Window{From: e.day(1).Add(-2 * time.Hour), To: e.clock.Now().Add(time.Hour), Step: entity.StepMonth}
	require.NoError(t, e.calc.Refresh(t.Context(), e.orgID, s.ID, window, e.clock.Now()))
	e.build(t)
	first, err := e.calc.Revision(t.Context(), e.orgID, s.ID, nil)
	require.NoError(t, err)
	assert.InDelta(t, 2.0/3, *e.point(t, first.ID, "A28", "", e.day(2)).Value, 1e-12)
	assert.InDelta(t, 2, *e.point(t, first.ID, "A28", calc.ActorHuman, e.day(2)).Value, 0)
	assert.InDelta(t, 1, *e.point(t, first.ID, "A28", calc.ActorUnknown, e.day(2)).Value, 0)

	e.confirm(t, "2", entity.PersonBot)
	e.build(t)
	second, err := e.calc.Revision(t.Context(), e.orgID, s.ID, nil)
	require.NoError(t, err)
	require.Greater(t, second.RevisionNo, first.RevisionNo)
	assert.InDelta(t, 1, *e.point(t, second.ID, "A28", calc.ActorBot, e.day(2)).Value, 0)
	assert.InDelta(t, 1, *e.point(t, second.ID, "A28", calc.ActorHuman, e.day(2)).Value, 0)
	assert.InDelta(t, 2, *e.point(t, first.ID, "A28", calc.ActorHuman, e.day(2)).Value, 0)

	accounts := make(map[entity.ID]bool)
	for _, estimator := range []string{calc.ActorBot, calc.ActorUnknown} {
		p := e.point(t, second.ID, "A28", estimator, e.day(2))
		items, err := e.calc.PointContributions(t.Context(), e.orgID, second.ID, entity.ContributionQuery{
			MetricID: "A28", Estimator: estimator, PointStart: p.Start,
		})
		require.NoError(t, err)
		require.Len(t, items.Items, 1)
		var evidence struct {
			AccountID entity.ID `json:"account_id"`
		}
		require.NoError(t, json.Unmarshal(items.Items[0].Evidence, &evidence))
		accounts[evidence.AccountID] = true
	}
	assert.Len(t, accounts, 2, "the imported account display names match but the source accounts are distinct")
}

func TestCalcActivity_TeamLinkAmbiguityIsStored(t *testing.T) {
	t.Parallel()
	e := newSeriesEnv(t)
	e.importMRs(t, entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)})
	pa3, mr := e.objectID(t, "task", "20003"), e.objectID(t, "merge_request", "9001")
	tx := persistent.NewTransactor(e.app)
	links := linkuc.New(linkuc.Deps{Repo: linkrepo.NewRepo(e.app), Tenants: platform.NewRepo(e.app), Tx: tx, Clock: e.clock, NewID: newID})
	require.NoError(t, tx.InOrgTx(t.Context(), e.orgID, func(ctx context.Context) error {
		_, err := links.Record(ctx, e.orgID, entity.LinkProposal{
			Type: entity.LinkTaskMR, From: pa3, To: mr, Method: entity.LinkTitleKey, Evidence: json.RawMessage(`{"title":"PA-3"}`),
		})
		return err
	}))

	c := activityCalculator(t, e.app, e.clock, e.runner.norm, e.runner.sync, e.process)
	series, err := c.ActivitySeries(t.Context(), e.orgID, entity.ActivityScope{TeamID: entity.ID(e.team)}, entity.StepWeek)
	require.NoError(t, err)
	window := entity.Window{From: _linkMerged.AddDate(0, 0, -7), To: _linkMerged.AddDate(0, 0, 7), Step: entity.StepWeek}
	require.NoError(t, c.Refresh(t.Context(), e.orgID, series.ID, window, window.To))
	buildCalcJobs(t, e.owner, e.clock, c)
	rev, err := c.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	point := activityStoredPoint(t, c, e.orgID, rev.ID, "A30", "ambiguous_share", _linkMerged.AddDate(0, 0, -3))
	require.NotNil(t, point.Value)
	assert.InDelta(t, 1, *point.Value, 0)
	assert.InDelta(t, 2, *point.Denominator, 0)
}
