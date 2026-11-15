package calc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _story = "story"

var _storyS = entity.TargetGroup{TaskType: "Story", SizeLabel: "S"}

// typedMapping is m1 with the type story labelled Story.
func typedMapping() *entity.MappingVersion {
	m := m1()
	m.Rules = append(m.Rules, entity.MappingRule{Kind: entity.RuleIssueType, InstanceID: _instance, NativeRef: _story, Meaning: "Story"})
	return m
}

// runIn computes one point over [from, to) at as_of of typed tasks with the targets, the window H and the
// collection of the input it is given.
func runIn(t *testing.T, from, to, asOf time.Time, in input, specs ...taskSpec) entity.CalcResult {
	t.Helper()
	in.mapping, in.team, in.location = typedMapping(), _team, time.UTC
	in.window, in.asOf = entity.Window{From: from, To: to, Step: entity.StepMonth}, asOf
	for _, s := range specs {
		ti := s.input()
		ti.version.TypeNativeID = _story
		ti.area = idOf("area")
		in.tasks = append(in.tasks, ti)
	}
	return compute(in)
}

// v16 is V-03 with A of size S at its start and M after it, and C done without a size.
func v16() []taskSpec {
	a := v03()[0]
	a.changes = append(a.changes, change{at: d(2), field: _sizeField, to: "M"})
	return []taskSpec{a, v03()[1], task("C", d(0), _active, status(d(1), _done)).without(_sizeField)}
}

func TestQuality_V16TargetOfTypeAndSize(t *testing.T) {
	v1 := runIn(t, d(0), d(10), d(10), input{targets: map[entity.TargetGroup]int64{_storyS: 2 * 86400}}, v16()...)
	t10 := point(t, v1, "T10", "")
	assert.Equal(t, entity.StateProvisional, t10.State, "C without a size is missing, the rest is a usable part")
	assert.Equal(t, 0.0, value(t, t10), "A in S with cycle 3 days violates v1 = 2 days")
	assert.Equal(t, 0.0, *t10.Numerator)
	assert.Equal(t, 1.0, *t10.Denominator)
	assert.Equal(t, 1, t10.MissingCount)
	assert.Equal(t, "S", *slice(t, v1, "A").SizeLabel, "the size at start, not M")
	c := contributions(t10, idOf("C"))
	require.Len(t, c, 1)
	assert.Equal(t, entity.Contribution{ObjectID: idOf("C"), ItemNo: 1, ObjectKind: entity.ObjectTask, Role: entity.RoleMissing, ReasonCode: entity.ReasonSizeUnknown},
		c[0], "no target of another group is taken")
	a := contributions(t10, idOf("A"))
	require.Len(t, a, 1)
	assert.Equal(t, 3*_day, *a[0].Value, "the cycle of A is its contribution to the denominator")

	v2 := runIn(t, d(0), d(10), d(10), input{targets: map[entity.TargetGroup]int64{_storyS: 4 * 86400}}, v16()...)
	assert.Equal(t, 1.0, value(t, point(t, v2, "T10", "")), "the explicitly chosen v2 = 4 days gives its own result")

	none := runIn(t, d(0), d(10), d(10), input{}, v16()...)
	t10 = point(t, none, "T10", "")
	assert.Equal(t, entity.StateUnknown, t10.State, "without a target there is no eligible denominator")
	assert.Nil(t, t10.Value)
	assert.Equal(t, entity.ReasonTargetMissing, contributions(t10, idOf("A"))[0].ReasonCode)
}

func TestQuality_AgingOverTarget(t *testing.T) {
	targets := map[entity.TargetGroup]int64{_storyS: 2 * 86400}
	within := runIn(t, d(0), d(3), d(3), input{targets: targets}, v16()...)
	t11 := point(t, within, "T11", "")
	assert.Equal(t, entity.StateAssessed, t11.State)
	assert.Equal(t, 0.0, value(t, t11), "an age of exactly 2 days is not over the target")
	assert.Equal(t, 1.0, *contributions(t11, idOf("A"))[0].Value, "age / target")
	assert.Equal(t, 1.0, *t11.Denominator, "B was cancelled on d3")

	over := runIn(t, d(0), d(3).Add(12*time.Hour), d(4), input{targets: targets}, v16()...)
	t11 = point(t, over, "T11", "")
	assert.Equal(t, 1.0, value(t, t11))
	assert.Equal(t, 1.0, *t11.Numerator)
	assert.Equal(t, 1.25, *contributions(t11, idOf("A"))[0].Value)

	none := point(t, runIn(t, d(0), d(3), d(3), input{}, v16()...), "T11", "")
	assert.Equal(t, entity.StateUnknown, none.State, "without a target there is no excess")
	assert.Nil(t, none.Value)
	assert.Equal(t, entity.ReasonTargetMissing, none.ReasonCode)
}

func TestQuality_V04FollowUp(t *testing.T) {
	h := input{followUp: 4 * 24 * time.Hour}
	c := task("C", d(0), _intake, status(d(1), _active), status(d(9), _done))

	r := runIn(t, d(0), d(10), d(10), h, v04(), c)
	t23 := point(t, r, "T23", "")
	assert.Equal(t, entity.StateAssessed, t23.State)
	assert.Equal(t, 1.0, value(t, t23), "the full window [d3,d7) of A holds the reopen on d5")
	assert.Equal(t, 1.0, *t23.Denominator, "C is immature and not in the denominator")
	assert.Equal(t, []entity.Contribution{{ObjectID: idOf("C"), ItemNo: 1, ObjectKind: entity.ObjectTask, Role: entity.RoleExcluded, ReasonCode: entity.ReasonImmature}},
		contributions(t23, idOf("C")), "the immature task is listed apart")

	edge := task("A", d(0), _intake, status(d(1), _active), status(d(3), _done), status(d(7), _active), status(d(8), _done))
	r = runIn(t, d(0), d(10), d(10), h, edge, c)
	t23 = point(t, r, "T23", "")
	assert.Equal(t, 0.0, value(t, t23), "a reopen exactly at the end of [d3,d7) is outside")
	assert.Equal(t, 1.0, *t23.Denominator)
	assert.Equal(t, 0.0, value(t, point(t, runIn(t, d(0), d(7).Add(-time.Second), d(10), h, edge), "T12", "")))
	assert.Equal(t, 1.0, value(t, point(t, runIn(t, d(0), d(7), d(10), h, edge), "T12", "")),
		"the new WIP episode starts on d7: episode_started <= t (BR-005)")

	thirty := point(t, runIn(t, d(0), d(10), d(10), input{}, v04()), "T23", "")
	assert.Equal(t, entity.StateNotApplicable, thirty.State, "with H = 30 days of version 1 the window of A is still open")
	assert.Equal(t, entity.ReasonImmature, contributions(thirty, idOf("A"))[0].ReasonCode)
}

func TestQuality_V05HistoryCompleteness(t *testing.T) {
	r0 := runIn(t, d(0), d(5), d(5), input{}, task("H", d(0), _active, status(d(4), _done)).knownFrom(d(3)))
	t39 := point(t, r0, "T39", "")
	assert.Equal(t, 0.0, value(t, t39), "the history before d3 is unavailable")
	assert.Equal(t, entity.ReasonHistoryIncomplete, contributions(t39, idOf("H"))[0].ReasonCode)
	assert.Equal(t, entity.StateBounded, point(t, r0, "T05", "").State)
	assert.Equal(t, entity.StateUnknown, point(t, r0, "T08", EstimatorP85).State)
	assert.Equal(t, 0.0, value(t, point(t, r0, "T12", "")), "the current snapshot has no WIP on d5")

	r1 := runIn(t, d(0), d(5), d(5), input{}, task("H", d(0), _intake, status(d(1), _active), status(d(4), _done)))
	assert.Equal(t, 1.0, value(t, point(t, r1, "T39", "")), "every page of history d0-d5 is confirmed")
	assert.Equal(t, 1.0, value(t, point(t, r1, "T05", "")))
	t08 := point(t, r1, "T08", EstimatorP85)
	assert.Equal(t, entity.StateAssessed, t08.State)
	assert.Equal(t, 3*_day, value(t, t08))
	assert.True(t, t08.LowSample)

	partial := runIn(t, d(0), d(5), d(5), input{}, task("H", d(0), _active, status(d(1), _active), status(d(4), _done)).knownFrom(d(1)))
	assert.Equal(t, 0.0, value(t, point(t, partial, "T39", "")), "h1 without the confirmation of the pages does not raise completeness")
	assert.NotEqual(t, entity.StateAssessed, point(t, partial, "T05", "").State)

	collected := input{collection: []entity.AreaCollection{{AreaID: idOf("area"), ConnectionID: idOf("conn"), Completeness: "partial"}}}
	stopped := runIn(t, d(0), d(5), d(5), collected, task("H", d(0), _intake, status(d(1), _active), status(d(4), _done)))
	t39 = point(t, stopped, "T39", "")
	assert.Equal(t, 0.0, value(t, t39), "a collection stopped on an incomplete page confirms no history")
	assert.Equal(t, entity.ReasonCollectionPartial, contributions(t39, idOf("H"))[0].ReasonCode)
}

func TestQuality_FieldsAndMappingCoverage(t *testing.T) {
	r := runIn(t, d(0), d(10), d(10), input{},
		task("A", d(0), _active, status(d(2), _done)),
		task("B", d(0), _active).without(_sizeField),
		task("N", d(0), _intake),
		task("X", d(0), _intake, status(d(1), _unknown), status(d(2), _active)),
	)
	size := point(t, r, "T38", FieldSize)
	assert.Equal(t, 2.0/3, value(t, size), "the size counts the started tasks A, B and X; B has none")
	assert.Equal(t, entity.ReasonSizeUnknown, contributions(size, idOf("B"))[0].ReasonCode)
	assert.Equal(t, 1.0, value(t, point(t, r, "T38", FieldType)), "every task is of the type Story")
	assert.Equal(t, 1.0, value(t, point(t, r, "T38", FieldTeam)))
	start := point(t, r, "T38", FieldStart)
	assert.Equal(t, 1.0, value(t, start))
	assert.Equal(t, 3, *start.N, "N has not started and is not eligible for the start")

	tasks := point(t, r, "T41", CoverageTasks)
	assert.Equal(t, 0.75, value(t, tasks), "X entered a status without a rule")
	assert.Equal(t, entity.ReasonStatusUnmapped, contributions(tasks, idOf("X"))[0].ReasonCode)
	transitions := point(t, r, "T41", CoverageTransition)
	assert.Equal(t, 6.0/7, value(t, transitions), "seven statuses entered: A two, B and N one, X three with the unknown one")
	early := runIn(t, d(0), d(1), d(10), input{}, task("X", d(0), _intake, status(d(1), _unknown)))
	assert.Equal(t, 1.0, value(t, point(t, early, "T41", CoverageTasks)), "a status entered at the end of the point is not in it")
}

func TestQuality_FreshnessAndVariance(t *testing.T) {
	asOf := d(10)
	watermark, success := d(9), d(9).Add(12*time.Hour)
	expected, missing, changed := 40, 2, 1
	finished := d(8)
	in := input{collection: []entity.AreaCollection{
		{
			AreaID: idOf("area"), ConnectionID: idOf("c1"), Completeness: entity.CollectionComplete, Watermark: &watermark, LastSuccessAt: &success,
			Reconcile: &entity.ReconcileRun{ID: idOf("r1"), FinishedAt: &finished, ExpectedCount: &expected, MissingCount: &missing, ChangedCount: &changed},
		},
		{AreaID: idOf("other"), ConnectionID: idOf("c2"), Completeness: "unknown"},
		{AreaID: idOf("third"), ConnectionID: idOf("c1"), Completeness: entity.CollectionComplete, Watermark: &watermark, LastSuccessAt: &success},
	}}
	in.window = entity.Window{From: d(0), To: asOf, Step: entity.StepWeek}
	in.mapping, in.team, in.location, in.asOf = typedMapping(), _team, time.UTC, asOf
	r := compute(in)

	lag := lastPoint(t, r, "T40", FreshnessLag)
	assert.Equal(t, entity.StateProvisional, lag.State, "c2 has no watermark yet")
	assert.Equal(t, 86400.0, value(t, lag), "as_of - the watermark of c1")
	assert.Equal(t, 1, lag.MissingCount, "a connection of two areas counts once")
	require.Len(t, lag.Contributions, 2)
	assert.Equal(t, entity.ObjectConnection, lag.Contributions[0].ObjectKind)
	assert.Equal(t, 43200.0, value(t, lastPoint(t, r, "T40", FreshnessSuccess)))
	assert.Equal(t, entity.StateNotApplicable, firstPoint(t, r, "T40", FreshnessLag).State, "an earlier point does not describe the collection now")
	assert.Equal(t, entity.ReasonAsOfOnly, firstPoint(t, r, "T40", FreshnessLag).ReasonCode)

	t42 := lastPoint(t, r, "T42", DifferenceMissing)
	assert.Equal(t, entity.StateProvisional, t42.State, "c2 has no reconciliation")
	assert.Equal(t, 2.0, value(t, t42))
	assert.Equal(t, 40.0, *t42.Denominator)
	assert.Equal(t, 1.0, value(t, lastPoint(t, r, "T42", DifferenceChanged)))

	in.collection = nil
	none := compute(in)
	assert.Equal(t, entity.StateNotApplicable, lastPoint(t, none, "T40", FreshnessLag).State, "an imported area has no collection")
}

func firstPoint(t *testing.T, r entity.CalcResult, metric, estimator string) entity.Point {
	t.Helper()
	return seriesOf(t, r, metric, estimator)[0]
}

func lastPoint(t *testing.T, r entity.CalcResult, metric, estimator string) entity.Point {
	t.Helper()
	ps := seriesOf(t, r, metric, estimator)
	return ps[len(ps)-1]
}

func seriesOf(t *testing.T, r entity.CalcResult, metric, estimator string) []entity.Point {
	t.Helper()
	for _, s := range r.Series {
		if s.MetricID == metric && s.Estimator == estimator {
			require.NotEmpty(t, s.Points)
			return s.Points
		}
	}
	require.Failf(t, "no series", "%s %s", metric, estimator)
	return nil
}
