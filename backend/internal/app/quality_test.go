package app

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testadapter "github.com/Avtobot22/craftPulse/backend/internal/adapter/test"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
)

var (
	_weekOfTG1 = time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	_weekOfTG2 = time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC)
)

// pointOf is the stored point of a metric and an estimator of a revision starting at start.
func (e *targetEnv) pointOf(t *testing.T, revision entity.ID, metric, estimator string, start time.Time) entity.SeriesPoint {
	t.Helper()
	points := e.points(t, revision)
	i := indexPoint(points, metric, estimator, start)
	require.GreaterOrEqual(t, i, 0, "%s %s %s", metric, estimator, start)
	return points[i]
}

// lastPoint is the stored point of a metric and an estimator at the end of the window of a revision.
func (e *targetEnv) lastPoint(t *testing.T, revision entity.ID, metric, estimator string) entity.SeriesPoint {
	t.Helper()
	var out *entity.SeriesPoint
	points := e.points(t, revision)
	for i := range points {
		p := &points[i]
		if p.MetricID == metric && p.Estimator == estimator && (out == nil || p.Start.After(out.Start)) {
			out = p
		}
	}
	require.NotNil(t, out, metric)
	return *out
}

func (e *targetEnv) watermarks(t *testing.T, revision entity.ID) []entity.RevisionInput {
	t.Helper()
	inputs, err := e.calc.RevisionInputs(t.Context(), e.orgID, revision)
	require.NoError(t, err)
	var out []entity.RevisionInput
	for _, in := range inputs {
		if in.Kind == entity.InputKindWatermark {
			out = append(out, in)
		}
	}
	return out
}

func TestCalc_TargetDefinitionKeepsFollowUp(t *testing.T) {
	db := newDB(t)
	db.migrate(t)
	app := db.pool(t, "craftpulse_app")
	var raw []byte
	require.NoError(t, app.QueryRow(t.Context(), "SELECT parameters FROM calc.definition WHERE metric_id = 'T23' AND definition_version = 1").Scan(&raw))
	var params map[string]any
	require.NoError(t, json.Unmarshal(raw, &params))
	for _, def := range calc.Definitions() {
		if def.ID == "T23" {
			assert.InDelta(t, def.Parameters["follow_up_days"], params["follow_up_days"], 0, "the window H of the registry is the one of the row")
		}
	}
	assert.InDelta(t, 30.0, params["follow_up_days"], 0, "H = 30 calendar days in version 1 (BR-008)")
}

// TestQuality_TwoTargetVersionsAndStoppedCollection builds the series of the team with v1 and then v2 of the
// target of Story S, and again after its collection stopped on a failed page.
func TestQuality_TwoTargetVersionsAndStoppedCollection(t *testing.T) {
	e := newTargetEnv(t)
	orgs, _ := newOrgAccess(e.app, e.clock, nil, nil)
	c, err := newCalc(e.app, calcModules{structure: orgs, facts: e.runner.norm, settings: e.process, collection: e.runner.sync}, e.clock,
		prometheus.NewRegistry())
	require.NoError(t, err)
	e.calc = c
	r0 := e.build(t)
	none := e.pointOf(t, r0.ID, "T10", "", _weekOfTG1)
	assert.Equal(t, entity.StateUnknown, none.State, "without a target T10 gives no ratio")
	assert.Nil(t, none.Value)

	v1 := e.confirm(t, e.admin, _targetVersions, `"0"`, uuid.New(), manualTarget(2*_day))
	require.Equal(t, http.StatusCreated, v1.Code, v1.Body.String())
	e.recount(t)
	r1 := e.current(t)
	violated := e.pointOf(t, r1.ID, "T10", "", _weekOfTG1)
	assert.Equal(t, entity.StateAssessed, violated.State)
	assert.Equal(t, 0.0, *violated.Value, "TG-1 with cycle 3 days violates v1 = 2 days")
	assert.Equal(t, 1.0, *violated.Denominator)
	assert.Equal(t, 1.0, *e.pointOf(t, r1.ID, "T10", "", _weekOfTG2).Value, "TG-2 with 2 days is within v1")

	v2 := e.confirm(t, e.admin, _targetVersions, `"1"`, uuid.New(), manualTarget(4*_day))
	require.Equal(t, http.StatusCreated, v2.Code, v2.Body.String())
	e.recount(t)
	r2 := e.current(t)
	require.NotEqual(t, r1.ID, r2.ID)
	assert.Equal(t, 1.0, *e.pointOf(t, r2.ID, "T10", "", _weekOfTG1).Value, "the new revision uses v2 = 4 days")
	assert.Equal(t, 0.0, *e.pointOf(t, r1.ID, "T10", "", _weekOfTG1).Value, "the revision of v1 keeps its violation")
	require.Len(t, e.targetInputs(t, r2.ID), 1)
	assert.NotEqual(t, e.targetInputs(t, r1.ID), e.targetInputs(t, r2.ID))

	complete := e.lastPoint(t, r2.ID, "T39", "")
	assert.Equal(t, entity.StateAssessed, complete.State)
	assert.Equal(t, 1.0, *complete.Value, "the connection reconciled without differences")
	lag := e.lastPoint(t, r2.ID, "T40", calc.FreshnessLag)
	assert.Equal(t, entity.StateAssessed, lag.State)
	assert.GreaterOrEqual(t, *lag.Value, 0.0)
	page, err := e.calc.PointContributions(t.Context(), e.orgID, r2.ID, entity.ContributionQuery{
		MetricID: "T40", Estimator: calc.FreshnessLag, PointStart: lag.Start,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 1, "the connection is the object of T40")
	assert.Equal(t, entity.ObjectConnection, page.Items[0].ObjectKind)
	assert.Equal(t, entity.ID(e.conn), page.Items[0].ObjectID)
	marks := e.watermarks(t, r2.ID)
	require.Len(t, marks, 1, "one connection of the team")
	assert.Equal(t, e.conn.String(), marks[0].ID)
	assert.Equal(t, entity.CollectionComplete, marks[0].Completeness)
	require.NotNil(t, marks[0].Watermark)
	assert.Equal(t, entity.StateAssessed, e.lastPoint(t, r2.ID, "T42", calc.DifferenceMissing).State)

	require.NoError(t, e.adapter.src.SetConfig(testadapter.Config{Faults: []testadapter.Fault{
		{Operation: testadapter.OpFetch, SchemaID: "tasks", Kind: testadapter.FaultAuth},
	}}))
	require.Equal(t, http.StatusAccepted, e.syncRun(t, e.admin, e.conn, "resume").Code)
	e.settle(t)
	require.NoError(t, e.calc.Refresh(t.Context(), e.orgID, e.series.ID, _seriesWindow, e.clock.Now()))
	e.runJobs(t, 0)
	r3 := e.current(t)
	require.NotEqual(t, r2.ID, r3.ID)
	partial := e.lastPoint(t, r3.ID, "T39", "")
	assert.Equal(t, 0.0, *partial.Value, "a collection stopped on a failed page confirms no history")
	assert.Equal(t, "partial", e.watermarks(t, r3.ID)[0].Completeness)
	assert.Equal(t, entity.StateAssessed, e.lastPoint(t, r3.ID, "T40", calc.FreshnessSuccess).State,
		"the last success before the failure stays")
	assert.Equal(t, 1.0, *e.lastPoint(t, r2.ID, "T39", "").Value, "the earlier revision keeps its completeness")
}
