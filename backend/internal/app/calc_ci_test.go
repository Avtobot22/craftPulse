package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/envelope"
)

func newCalcCIEnv(t *testing.T, m *envelope.Manifest, records ...*envelope.Record) *reviewEnv {
	t.Helper()
	e := &reviewEnv{normalizeEnv: newNormalizeEnv(t)}
	for _, record := range records {
		if record.RecordKind != "ci_run" {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal(record.Payload, &payload))
		payload["finished_at"] = _t1.Add(130 * time.Second)
		record.Payload, _ = json.Marshal(payload)
	}
	rc := e.imported(t, e.admin, pack(t, m, records...))
	require.Equal(t, entity.ReceiptAccepted, rc.Status)
	e.drain(t)
	e.area = mrArea(t, e.normalizeEnv, rc, "42")
	orgs, _, err := newOrgAccess(e.app, e.clock, e.connect, nil)
	require.NoError(t, err)
	collection, err := newSync(e.app, e.keyring, e.connect, e.uc, e.norm, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	e.calc, err = newCalc(e.app, calcModules{structure: orgs, facts: e.norm, collection: collection}, e.clock, prometheus.NewRegistry())
	require.NoError(t, err)
	return e
}

func TestCalcCI_V28StoredContributionsAndLateAttempt(t *testing.T) {
	t.Parallel()
	data := v28(t)
	e := newCalcCIEnv(t, ciManifest("42"), data[0], data[1], data[3])
	series, err := e.calc.CISeries(t.Context(), e.orgID, entity.ReviewScope{RepositoryID: e.area}, entity.StepWeek)
	require.NoError(t, err)
	e.build(t)
	first, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	assert.InDelta(t, .5, *e.point(t, first.ID, "I01", "jobs", _t1).Value, 0)
	assert.InDelta(t, .5, *e.point(t, first.ID, "I02", "", _t1).Value, 0)
	assert.InDelta(t, 0, *e.point(t, first.ID, "I03", "", _t1).Value, 0)
	assert.InDelta(t, 100, *e.point(t, first.ID, "I04", "total_seconds", _t1).Value, 0)
	loaded := e.point(t, first.ID, "I10", "loaded", _t1)
	assert.Equal(t, entity.StateBounded, loaded.State)
	assert.Equal(t, &entity.Bound{Kind: entity.BoundLower, Value: 1}, loaded.Bound)

	e.imported(t, e.admin, pack(t, ciManifest("42"), data[2]))
	e.drain(t)
	e.build(t)
	second, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	require.Greater(t, second.RevisionNo, first.RevisionNo)
	assert.InDelta(t, 1, *e.point(t, second.ID, "I01", "jobs", _t1).Value, 0)
	assert.InDelta(t, .5, *e.point(t, second.ID, "I02", "", _t1).Value, 0)
	assert.InDelta(t, .5, *e.point(t, second.ID, "I03", "", _t1).Value, 0)
	assert.InDelta(t, 130, *e.point(t, second.ID, "I04", "total_seconds", _t1).Value, 0)
	pt := e.point(t, second.ID, "I04", "seconds", _t1)
	assert.InDelta(t, 30, *pt.Value, 0)
	page, err := e.calc.PointContributions(t.Context(), e.orgID, second.ID, entity.ContributionQuery{MetricID: "I04", Estimator: "seconds", PointStart: pt.Start})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, entity.ObjectCIAttempt, page.Items[0].ObjectKind)
	assert.Equal(t, entity.RoleNumerator, page.Items[0].Role)
	var evidence map[string]any
	require.NoError(t, json.Unmarshal(page.Items[0].Evidence, &evidence))
	assert.Equal(t, "success", evidence["native_status"])
	assert.Equal(t, 2.0, evidence["attempt_no"])
	assert.Equal(t, _linux, evidence["config_fingerprint"])
	assert.InDelta(t, 100, *e.point(t, first.ID, "I04", "total_seconds", _t1).Value, 0)
	old, err := e.calc.Revision(t.Context(), e.orgID, series.ID, &first.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.RevisionSuperseded, old.Status)

	e.imported(t, e.admin, pack(t, ciManifest("42"), data...))
	e.drain(t)
	e.build(t)
	assert.Equal(t, 3, e.n(t, "SELECT count(*) FROM normalize.ci_attempt_version"))

	_, err = e.calc.CISeries(t.Context(), e.orgID, entity.ReviewScope{RepositoryID: e.area}, entity.StepDay)
	require.Error(t, err)
	_, err = e.calc.Revision(t.Context(), e.otherID, series.ID, nil)
	require.ErrorIs(t, err, entity.ErrNotFound)
}

func TestCalcCI_TruncatedHistoryAndAllowedFailure(t *testing.T) {
	t.Parallel()
	data := v28(t)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(data[1].Payload, &payload))
	payload["allow_failure"] = true
	data[1].Payload, _ = json.Marshal(payload)
	e := newCalcCIEnv(t, ciManifest("42", "5001"), data[0], data[1])
	series, err := e.calc.CISeries(t.Context(), e.orgID, entity.ReviewScope{RepositoryID: e.area}, entity.StepMonth)
	require.NoError(t, err)
	e.build(t)
	rev, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
	require.NoError(t, err)
	for _, metric := range []string{"I02", "I03", "I05"} {
		assert.Nil(t, e.point(t, rev.ID, metric, "", _t1).Value)
	}
	assert.Equal(t, entity.StateBounded, e.point(t, rev.ID, "I04", "seconds", _t1).State)
	assert.Equal(t, entity.StateUnknown, e.point(t, rev.ID, "I10", "", _t1).State)
}

func TestCalcCI_TeamProjectsDeduplicateRepository(t *testing.T) {
	t.Parallel()
	e := newCalcCIEnv(t, ciManifest("42"), v28(t)...)
	orgs, _, err := newOrgAccess(e.app, e.clock, e.connect, nil)
	require.NoError(t, err)
	st, err := orgs.Structure(t.Context(), e.admin)
	require.NoError(t, err)
	in := entity.StructureInput{Teams: []entity.AreaInput{{Name: "Platform", Status: "active"}}}
	for _, p := range st.Projects {
		in.Projects = append(in.Projects, entity.AreaInput(p))
	}
	in.Projects = append(in.Projects, entity.AreaInput{Name: "Second", Status: "active"})
	st, err = orgs.ReplaceStructure(t.Context(), e.admin, st.Version, in)
	require.NoError(t, err)
	require.Len(t, st.Teams, 1)
	team := st.Teams[0].ID
	in = entity.StructureInput{Teams: []entity.AreaInput{{ID: team, Name: "Platform", Status: "active"}}}
	for _, p := range st.Projects {
		in.Projects = append(in.Projects, entity.AreaInput(p))
		in.TeamProjects = append(in.TeamProjects, entity.LinkInput{From: team, To: p.ID})
	}
	_, err = orgs.ReplaceStructure(t.Context(), e.admin, st.Version, in)
	require.NoError(t, err)
	for _, p := range st.Projects {
		_, err := e.owner.Exec(t.Context(), `INSERT INTO org.project_area (organization_id,project_id,area_id,valid,known_seq)
			VALUES ($1,$2,$3,tstzrange($4,NULL,'[)'),1)`, e.orgID, p.ID, e.area, e.clock.Now())
		require.NoError(t, err)
	}
	for _, step := range []entity.Step{entity.StepWeek, entity.StepMonth} {
		series, err := e.calc.CISeries(t.Context(), e.orgID, entity.ReviewScope{TeamID: team}, step)
		require.NoError(t, err)
		w := entity.Window{From: _t1.Add(-24 * time.Hour), To: _t1.AddDate(0, 1, 0), Step: step}
		require.NoError(t, e.calc.Refresh(t.Context(), e.orgID, series.ID, w, w.To))
		e.build(t)
		rev, err := e.calc.Revision(t.Context(), e.orgID, series.ID, nil)
		require.NoError(t, err)
		pt := e.point(t, rev.ID, "I04", "total_seconds", _t1)
		assert.InDelta(t, 130, *pt.Value, 0, "two projects referencing the repository do not double its runs")
		if step == entity.StepMonth {
			assert.True(t, pt.End.After(pt.Start.AddDate(0, 0, 20)), "monthly series must retain its calendar step")
		}
	}
}
