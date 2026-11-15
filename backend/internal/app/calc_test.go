package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase/calc"
)

func TestCalc_DefinitionRowsFollowRegistry(t *testing.T) {
	t.Parallel()
	db := newMigratedDB(t)
	app := db.pool(t, "craftpulse_app")
	rows, err := app.Query(t.Context(), "SELECT metric_id, definition_version, unit, catalog_ref FROM calc.definition ORDER BY metric_id")
	require.NoError(t, err)
	defer rows.Close()
	var got []calc.Definition
	for rows.Next() {
		var def calc.Definition
		require.NoError(t, rows.Scan(&def.ID, &def.Version, &def.Unit, &def.CatalogRef))
		got = append(got, def)
	}
	require.NoError(t, rows.Err())

	defs := calc.Definitions()
	want := make([]calc.Definition, 0, len(defs))
	for _, def := range defs {
		want = append(want, calc.Definition{ID: def.ID, Version: def.Version, Unit: def.Unit, CatalogRef: def.CatalogRef})
	}
	slices.SortFunc(want, func(a, b calc.Definition) int { return strings.Compare(a.ID, b.ID) })
	assert.Equal(t, want, got, "the release migration adds the rows of the registry")
	_, err = app.Exec(t.Context(), "UPDATE calc.definition SET unit = 'count' WHERE metric_id = 'T08'")
	assert.Error(t, err, "the application does not change definitions")
}

// TestCalc_FlowOfCollectedTasks computes the flow of the team from the tasks the test adapter collected: before
// the mapping is confirmed every point is NOT_EVALUATED, after it the starts, the completion, the cycle time
// and the blocked WIP follow the recorded history.
func TestCalc_FlowOfCollectedTasks(t *testing.T) {
	t.Parallel()
	e := newProcessEnv(t)
	orgs, _, err := newOrgAccess(e.app, e.clock, nil, nil)
	require.NoError(t, err)
	uc := calc.New(calc.Deps{Structure: orgs, Facts: e.runner.norm, Settings: e.process, Clock: e.clock})
	req := entity.CalcRequest{
		OrganizationID: e.orgID, TeamID: entity.ID(e.team), KnowledgeSeq: e.lastSeq(t),
		Window: entity.Window{From: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Step: entity.StepMonth},
		AsOf:   time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
	}
	before, err := uc.Calculate(t.Context(), req)
	require.NoError(t, err)
	require.NotEmpty(t, before.Series)
	for _, s := range before.Series {
		for _, p := range s.Points {
			assert.Equal(t, entity.StateNotEvaluated, p.State, s.MetricID)
			assert.Equal(t, entity.ReasonMappingMissing, p.ReasonCode)
		}
	}

	rule := func(kind entity.RuleKind, ref, value, meaning string) entity.MappingRule {
		return entity.MappingRule{Kind: kind, InstanceID: entity.ID(e.instance), NativeRef: ref, NativeValue: value, Meaning: meaning}
	}
	admin := entity.Subject{OrganizationID: e.orgID, UserID: entity.ID(e.adminID)}
	_, _, err = e.process.CreateMappingVersion(t.Context(), admin, entity.ID(e.team), 0, entity.ID(uuid.New()), entity.MappingDraft{
		Timezone: "Europe/Moscow", Rules: []entity.MappingRule{
			rule(entity.RuleStatus, "1", "", entity.MeaningIntake), rule(entity.RuleStatus, "3", "", entity.MeaningActive),
			rule(entity.RuleStatus, "5", "", entity.MeaningDone), rule(entity.RuleResolution, "10000", "", entity.MeaningAccepted),
			rule(entity.RuleBlockedFlag, "customfield_10020", "Impediment", entity.MeaningBlocked),
		},
	})
	require.NoError(t, err)
	stale, err := uc.Calculate(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, entity.StateNotEvaluated, stale.Series[0].Points[0].State, "the slice of the request does not see a later mapping")

	req.KnowledgeSeq = e.lastSeq(t)
	r, err := uc.Calculate(t.Context(), req)
	require.NoError(t, err)
	require.NotNil(t, r.MappingVersion)
	assert.Equal(t, 1, *r.MappingVersion)
	assert.Len(t, r.Tasks, 7)

	series := func(metric, estimator string) []entity.Point {
		i := slices.IndexFunc(r.Series, func(s entity.Series) bool { return s.MetricID == metric && s.Estimator == estimator })
		require.GreaterOrEqual(t, i, 0, metric)
		require.Len(t, r.Series[i].Points, 2, "February and March up to as_of")
		return r.Series[i].Points
	}
	values := func(points []entity.Point) []float64 {
		out := make([]float64, len(points))
		for i, p := range points {
			require.Equal(t, entity.StateAssessed, p.State, p.ReasonCode)
			out[i] = *p.Value
		}
		return out
	}
	assert.Equal(t, []float64{2, 2}, values(series("T04", "")), "20004 created at work and 20003 in February, 20005 and 20002 in March")
	assert.Equal(t, []float64{0, 1}, values(series("T05", "")))
	assert.Equal(t, 25*3600.0, *series("T08", calc.EstimatorP85)[1].Value, "20005 started on March 2 at 09:00 and was done the next day at 10:00")
	assert.Equal(t, []float64{2, 3}, values(series("T12", "")))
	t13 := series("T13", "")[1]
	assert.Equal(t, 1.0, *t13.Numerator, "20003 carries the impediment flag")
	assert.Equal(t, 3.0, *t13.Denominator)
}
