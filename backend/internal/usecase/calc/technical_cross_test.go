package calc

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestTechnicalCrossDefinitions(t *testing.T) {
	t.Parallel()
	run := idOf("pipeline")
	clean := crossMR{
		ID: idOf("clean"), PipelineID: &run, CIClass: "success", FailToPass: new(false), AllowedFailure: new(false),
		Technical: &entity.TechnicalCross{Scopes: []entity.TechnicalScope{{Kind: entity.AnalysisCode, Expected: 1, Complete: 1, Comparable: 1}}},
	}
	warning := clean
	warning.ID = idOf("warning")
	warning.Technical = &entity.TechnicalCross{Scopes: []entity.TechnicalScope{{Kind: entity.AnalysisCode, Expected: 1, Complete: 1, Comparable: 1, New: 2}}}
	accepted := clean
	accepted.Technical = &entity.TechnicalCross{Scopes: []entity.TechnicalScope{{Kind: entity.AnalysisCode, Expected: 1, Complete: 1, Comparable: 1, Accepted: 1}}}
	retried := clean
	retried.FailToPass = new(true)
	allowed := clean
	allowed.AllowFailureSupported, allowed.AllowedFailure = true, new(true)
	missing := clean
	missing.PipelineID, missing.CIClass = nil, ""
	missing.Technical = &entity.TechnicalCross{Scopes: []entity.TechnicalScope{{Kind: entity.AnalysisCode, Expected: 1}}}
	for _, tt := range []struct {
		name, metric, estimator string
		mrs                     []crossMR
		role                    entity.ContributionRole
		value                   *float64
	}{
		{"two MRs one task", "X03", "warning", []crossMR{warning, clean}, entity.RoleNumerator, nil},
		{"exception is not a warning", "X03", "warning", []crossMR{accepted}, entity.RoleDenominator, nil},
		{"exception counted separately", "X03", "code_exception", []crossMR{accepted}, entity.RoleNumerator, nil},
		{"success after failure", "X04", "caveat", []crossMR{retried}, entity.RoleNumerator, nil},
		{"allowed failure", "X04", "allowed_failure", []crossMR{allowed}, entity.RoleNumerator, nil},
		{"missing matrix check", "X02", "missing_report", []crossMR{{Technical: &entity.TechnicalCross{MissingChecks: true}}}, entity.RoleNumerator, nil},
		{"no confirmed MR", "X05", "confirmed_mr", nil, entity.RoleDenominator, nil},
		{"no CI on new head", "X05", "complete_reports", []crossMR{missing}, entity.RoleDenominator, nil},
		{"missing reports on new head", "X02", "missing_report", []crossMR{missing}, entity.RoleNumerator, nil},
		{"unverified scope", "X06", "code:unverified", []crossMR{missing}, entity.RoleNumerator, num(1)},
		{"new findings counted", "X06", "code:new", []crossMR{warning}, entity.RoleNumerator, num(2)},
		{"known negative", "X03", "warning", []crossMR{clean}, entity.RoleDenominator, nil},
		{"unknown comparison", "X03", "warning", []crossMR{missing}, entity.RoleMissing, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var c entity.Contribution
			scoreTechnical(&c, tt.metric, tt.estimator, &crossEvidence{MRs: tt.mrs})
			assert.Equal(t, tt.role, c.Role)
			assert.Equal(t, tt.value, c.Value)
		})
	}
}

func TestTechnicalCrossCohortCountsOneTaskAndOnePipeline(t *testing.T) {
	t.Parallel()
	f := observationFixture()
	first := f.Reviews[0].MRs[0]
	second := first
	second.MRID = idOf("second")
	f.Reviews[0].MRs = append(f.Reviews[0].MRs, second)
	other := task("other", d(0), _active).input()
	pr := prepare(input{mapping: f.Mapping, asOf: f.AsOf, tasks: []taskInput{
		{version: f.Task, instance: f.InstanceID, events: f.TaskEvents}, other,
	}})
	p := period{start: d(2), end: f.AsOf}
	run, secondRun, staleRun := idOf("run"), idOf("second run"), idOf("stale run")
	completeCode := entity.TechnicalScope{Kind: entity.AnalysisCode, Expected: 1, Complete: 1, Comparable: 1}
	missingTests := entity.TechnicalScope{Kind: entity.AnalysisJUnit, Expected: 1}
	x := reportSignals{
		facts: f, review: reviewInput{mrs: observationReviews(&f)},
		linked: map[entity.ID]bool{first.MRID: true, second.MRID: true},
		edges: map[entity.ID][]entity.LinkEdge{
			f.Task.TaskID:        {{To: first.MRID}, {To: second.MRID}},
			other.version.TaskID: {{To: first.MRID}},
		},
		technical: map[int64]map[entity.ID]entity.TechnicalCrossEvidence{p.end.UnixNano(): {
			first.MRID: {
				Head: entity.TechnicalCross{Scopes: []entity.TechnicalScope{completeCode, missingTests}},
				PipelineReports: map[entity.ID]entity.TechnicalCross{
					run:       {Scopes: []entity.TechnicalScope{completeCode}},
					secondRun: {Scopes: []entity.TechnicalScope{completeCode, missingTests}},
				},
			},
			second.MRID: {Head: entity.TechnicalCross{Scopes: []entity.TechnicalScope{
				{Kind: entity.AnalysisCode, Expected: 1, Complete: 1, Comparable: 1, New: 1},
			}}},
		}},
	}
	start, finish, laterStart, laterFinish := d(3), d(4), d(5), d(6)
	x.facts.CI.Runs = []entity.CIRunVersion{
		{RunID: run, AreaID: first.AreaID, SHA: first.HeadSHA, StartedAt: &start, FinishedAt: &finish, Outcome: entity.CISuccess},
		{RunID: secondRun, AreaID: first.AreaID, SHA: first.HeadSHA, StartedAt: &laterStart, FinishedAt: &laterFinish, Outcome: entity.CISuccess},
		{RunID: staleRun, AreaID: first.AreaID, SHA: "previous head", StartedAt: &start, FinishedAt: &finish, Outcome: entity.CISuccess},
	}
	x.runs = map[entity.ID]entity.ID{run: first.MRID, secondRun: first.MRID, staleRun: first.MRID}
	_, heads, _ := x.crossObservations(p.end, true)
	i := slices.IndexFunc(heads, func(h entity.TechnicalHead) bool { return h.MRID == first.MRID })
	require.NotEqual(t, -1, i)
	assert.ElementsMatch(t, []entity.TechnicalPipeline{{ID: run, ProfileAt: start}, {ID: secondRun, ProfileAt: laterStart}}, heads[i].SuccessfulPipelines)
	points := x.technicalPoints(pr, p)
	point := func(metric, est string) entity.Point {
		t.Helper()
		i := slices.IndexFunc(points, func(pt entity.SeriesPoint) bool { return pt.MetricID == metric && pt.Estimator == est })
		require.NotEqual(t, -1, i)
		return points[i].Point
	}
	warnings := point("X03", "warning")
	assert.Equal(t, num(1), warnings.Numerator)
	assert.Equal(t, num(2), warnings.Denominator)
	require.Len(t, warnings.Contributions, 2)
	pipelines := point("X04", "new_code")
	assert.Equal(t, num(2), pipelines.Denominator)
	require.Len(t, pipelines.Contributions, 2)
	assert.ElementsMatch(t, []entity.ID{run, secondRun}, []entity.ID{pipelines.Contributions[0].ObjectID, pipelines.Contributions[1].ObjectID})

	missing := point("X04", "missing_report")
	assert.Equal(t, num(1), missing.Numerator)
	assert.Equal(t, num(2), missing.Denominator)
	require.Len(t, missing.Contributions, 2)
	for _, c := range missing.Contributions {
		want := entity.RoleDenominator
		if c.ObjectID == secondRun {
			want = entity.RoleNumerator
		}
		assert.Equal(t, want, c.Role)
	}
}
