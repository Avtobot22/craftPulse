package calc

import (
	"crypto/sha256"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _day = 86400.0

// Statuses, resolutions and fields of the mapping m1 of TZ 4.4: intake does not start work, active and
// blocked do, done with an accepted resolution completes, cancelled and duplicate end without a result.
const (
	_intake    = "i"
	_active    = "a"
	_waiting   = "w"
	_blocked   = "b"
	_done      = "d"
	_cancelled = "c"
	_unknown   = "x"
	_accepted  = "acc"
	_sizeField = "size"
	_flagField = "flag"
)

var (
	_d0       = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	_instance = idOf("J1")
	_team     = idOf("team")
)

func d(n int) time.Time { return _d0.AddDate(0, 0, n) }

func idOf(name string) entity.ID {
	sum := sha256.Sum256([]byte(name))
	var id entity.ID
	copy(id[:], sum[:16])
	return id
}

func m1() *entity.MappingVersion {
	rule := func(kind entity.RuleKind, ref, value, meaning string) entity.MappingRule {
		return entity.MappingRule{Kind: kind, InstanceID: _instance, NativeRef: ref, NativeValue: value, Meaning: meaning}
	}
	return &entity.MappingVersion{VersionNo: 1, Timezone: "UTC", Rules: []entity.MappingRule{
		rule(entity.RuleStatus, _intake, "", entity.MeaningIntake),
		rule(entity.RuleStatus, _active, "", entity.MeaningActive),
		rule(entity.RuleStatus, _waiting, "", entity.MeaningWait),
		rule(entity.RuleStatus, _blocked, "", entity.MeaningBlocked),
		rule(entity.RuleStatus, _done, "", entity.MeaningDone),
		rule(entity.RuleStatus, _cancelled, "", entity.MeaningCancelled),
		rule(entity.RuleResolution, _accepted, "", entity.MeaningAccepted),
		rule(entity.RuleSizeField, _sizeField, "", ""),
		rule(entity.RuleSizeValue, _sizeField, "S", "S"),
		rule(entity.RuleSizeValue, _sizeField, "M", "M"),
		rule(entity.RuleBlockedFlag, _flagField, "yes", entity.MeaningBlocked),
	}}
}

type change struct {
	at        time.Time
	field, to string
}

// status is a change of the status at a moment.
func status(at time.Time, to string) change { return change{at: at, field: "status", to: to} }

// taskSpec is a task of a control set: its creation with the first values and the changes after it.
type taskSpec struct {
	name    string
	created time.Time
	initial map[string]string
	changes []change
	history *time.Time
}

// task builds the current version and the events of a task; a done status gets the accepted resolution at the
// same moment, as Jira writes them in one change.
func task(name string, created time.Time, initial string, changes ...change) taskSpec {
	init := map[string]string{"status": initial, _sizeField: "S"}
	if initial == _done {
		init["resolution"] = _accepted
	}
	var all []change
	for _, c := range changes {
		all = append(all, c)
		if c.field == "status" && c.to == _done {
			all = append(all, change{at: c.at, field: "resolution", to: _accepted})
		}
		if c.field == "status" && c.to != _done {
			all = append(all, change{at: c.at, field: "resolution", to: ""})
		}
	}
	return taskSpec{name: name, created: created, initial: init, changes: all}
}

func (s taskSpec) without(field string) taskSpec {
	delete(s.initial, field)
	return s
}

func (s taskSpec) knownFrom(t time.Time) taskSpec {
	s.history = &t
	return s
}

func (s taskSpec) input() taskInput {
	values := map[string]string{}
	for k, v := range s.initial {
		values[k] = v
	}
	id := idOf(s.name)
	var events []entity.TaskEvent
	for i, c := range s.changes {
		before, after := values[c.field], c.to
		if before == after {
			continue
		}
		values[c.field] = after
		events = append(events, entity.TaskEvent{
			ID: idOf(s.name + "/" + string(rune('a'+i))), TaskID: id, EventKey: s.name + "/" + string(rune('a'+i)),
			Kind: entity.TaskFieldChange, FieldNativeID: c.field, BeforeValue: optional(before), AfterValue: optional(after),
			EventTime: c.at,
		})
	}
	raw, _ := json.Marshal(map[string]string{_flagField: values[_flagField]})
	return taskInput{
		version: entity.TaskVersion{
			TaskID: id, CreatedSrcAt: s.created, StatusNativeID: values["status"], ResolutionNative: values["resolution"],
			SizeNative: values[_sizeField], RawFields: raw,
		},
		instance: _instance, project: idOf("P"), events: events, historyFrom: s.history,
	}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// run computes one point over [from, to) at as_of to: a month is longer than every window of the sets.
func run(t *testing.T, from, to time.Time, specs ...taskSpec) entity.CalcResult {
	t.Helper()
	return runAt(t, from, to, to, entity.GroupNone, specs...)
}

func runAt(t *testing.T, from, to, asOf time.Time, by entity.GroupBy, specs ...taskSpec) entity.CalcResult {
	t.Helper()
	in := input{mapping: m1(), team: _team, window: entity.Window{From: from, To: to, Step: entity.StepMonth}, asOf: asOf, location: time.UTC, groupBy: by}
	for _, s := range specs {
		in.tasks = append(in.tasks, s.input())
	}
	return compute(in)
}

// point is the only point of a metric and estimator of an ungrouped result.
func point(t *testing.T, r entity.CalcResult, metric, estimator string) entity.Point {
	t.Helper()
	i := slices.IndexFunc(r.Series, func(s entity.Series) bool { return s.MetricID == metric && s.Estimator == estimator })
	require.GreaterOrEqual(t, i, 0, metric+" "+estimator)
	require.Len(t, r.Series[i].Points, 1)
	return r.Series[i].Points[0]
}

func value(t *testing.T, p entity.Point) float64 {
	t.Helper()
	require.NotNil(t, p.Value, "state %s, reason %s", p.State, p.ReasonCode)
	return *p.Value
}

func slice(t *testing.T, r entity.CalcResult, name string) entity.TaskSlice {
	t.Helper()
	i := slices.IndexFunc(r.Tasks, func(s entity.TaskSlice) bool { return s.TaskID == idOf(name) })
	require.GreaterOrEqual(t, i, 0, name)
	return r.Tasks[i]
}

func contributions(p entity.Point, id entity.ID) []entity.Contribution {
	var out []entity.Contribution
	for _, c := range p.Contributions {
		if c.ObjectID == id {
			out = append(out, c)
		}
	}
	return out
}

// v03 is the set V-02/V-03: A goes through a blocking to an accepted result, B is cancelled.
func v03() []taskSpec {
	return []taskSpec{
		task("A", d(0), _intake, status(d(1), _active), status(d(2), _blocked), status(d(3), _active), status(d(4), _done)),
		task("B", d(0), _intake, status(d(1), _active), status(d(3), _cancelled)),
	}
}

func TestFlow_V03OrdinaryPathAndCancellation(t *testing.T) {
	r := run(t, d(0), d(10), v03()...)

	assert.Equal(t, 2.0, value(t, point(t, r, "T04", "")))
	t05 := point(t, r, "T05", "")
	assert.Equal(t, 1.0, value(t, t05))
	assert.Equal(t, entity.StateAssessed, t05.State)
	assert.Equal(t, []entity.Contribution{{ObjectID: idOf("B"), ItemNo: 1, ObjectKind: entity.ObjectTask, Role: entity.RoleExcluded, ReasonCode: entity.MeaningCancelled}},
		contributions(t05, idOf("B")), "B is shown as an exclusion of the completion cohort")
	t07 := point(t, r, "T07", "")
	assert.Equal(t, 1.0, value(t, t07))
	assert.Equal(t, entity.MeaningCancelled, t07.Contributions[0].ReasonCode, "T07 keeps the reason")

	for _, est := range []string{EstimatorP50, EstimatorP85} {
		t08 := point(t, r, "T08", est)
		assert.Equal(t, 3*_day, value(t, t08), est)
		assert.Equal(t, 1, *t08.N)
		assert.True(t, t08.LowSample, "n = 1 is a small sample")
		assert.Equal(t, entity.StateAssessed, t08.State)
		assert.Equal(t, 1, t08.ExcludedCount)
	}
	a := slice(t, r, "A")
	assert.Equal(t, "S", *a.SizeLabel, "the size at start")
	assert.Equal(t, []entity.WorkflowInterval{
		{TaskID: idOf("A"), EpisodeNo: 1, IntervalNo: 1, Class: entity.IntervalActive, From: d(1), To: d(2), SourceEventIDs: a.Intervals[0].SourceEventIDs},
		{TaskID: idOf("A"), EpisodeNo: 1, IntervalNo: 2, Class: entity.IntervalBlocked, From: d(2), To: d(3), SourceEventIDs: a.Intervals[1].SourceEventIDs},
		{TaskID: idOf("A"), EpisodeNo: 1, IntervalNo: 3, Class: entity.IntervalActive, From: d(3), To: d(4), SourceEventIDs: a.Intervals[2].SourceEventIDs},
	}, a.Intervals)
	t15 := point(t, r, "T15", "")
	assert.Equal(t, 1*_day, *t15.Numerator, "blocked duration of A is one day")
	assert.Equal(t, 5*_day, *t15.Denominator, "A works three days and B two")
}

func TestFlow_V03WIPSnapshots(t *testing.T) {
	tests := []struct {
		name        string
		at          time.Time
		wantWIP     float64
		wantBlocked float64
		wantShare   *float64
	}{
		{"d2", d(2), 2, 1, new(0.5)},
		{"d3", d(3), 1, 0, new(0.0)},
		{"d4, completion on the boundary", d(4), 0, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := run(t, d(0), tt.at, v03()...)
			assert.Equal(t, tt.wantWIP, value(t, point(t, r, "T12", "")))
			t13 := point(t, r, "T13", "")
			if tt.wantShare == nil {
				assert.Equal(t, entity.StateNotApplicable, t13.State, "no WIP, no denominator")
				return
			}
			assert.Equal(t, *tt.wantShare, value(t, t13))
			assert.Equal(t, tt.wantBlocked, *t13.Numerator, "blocked is a subset of WIP")
			assert.Equal(t, tt.wantWIP, *t13.Denominator)
		})
	}
}

func TestFlow_V03ReplayChangesNothing(t *testing.T) {
	once := run(t, d(0), d(10), v03()...)
	in := input{mapping: m1(), team: _team, window: entity.Window{From: d(0), To: d(10), Step: entity.StepMonth}, asOf: d(10), location: time.UTC}
	for _, s := range v03() {
		ti := s.input()
		ti.events = slices.Concat(ti.events, ti.events, ti.events)
		in.tasks = append(in.tasks, ti)
	}
	assert.Equal(t, once, compute(in), "three deliveries of the same events give the same numbers and intervals")
}

// v04 is the set V-04: A is completed, reopened and completed again.
func v04() taskSpec {
	return task("A", d(0), _intake, status(d(1), _active), status(d(3), _done), status(d(5), _active), status(d(8), _done))
}

func TestFlow_V04ReopenedTask(t *testing.T) {
	first := run(t, d(0), d(5), v04())
	assert.Equal(t, 1.0, value(t, point(t, first, "T05", "")))
	assert.Equal(t, 2*_day, value(t, point(t, first, "T08", EstimatorP50)))
	assert.Equal(t, 2*_day, value(t, point(t, first, "T08", EstimatorP85)))

	second := runAt(t, d(5), d(10), d(10), entity.GroupNone, v04())
	t05 := point(t, second, "T05", "")
	assert.Equal(t, entity.StateAssessed, t05.State)
	assert.Equal(t, 0.0, value(t, t05))
	assert.Equal(t, []entity.Contribution{{ObjectID: idOf("A"), ItemNo: 1, ObjectKind: entity.ObjectTask, Role: entity.RoleExcluded, ReasonCode: entity.ReasonRepeatCompletion}},
		t05.Contributions, "the repeated completion is shown apart")
	t08 := point(t, second, "T08", EstimatorP85)
	assert.Equal(t, entity.StateNotApplicable, t08.State)
	assert.Nil(t, t08.Value, "n = 0 has no percentile, not 0 days")
	assert.Equal(t, 0, *t08.N)

	a := slice(t, second, "A")
	assert.Equal(t, d(3), *a.FirstCompletion)
	assert.Equal(t, d(8), *a.FinalCompletion)
	assert.Equal(t, 2, a.Episodes)

	d6 := run(t, d(0), d(6), v04())
	assert.Equal(t, 1.0, value(t, point(t, d6, "T12", "")))
	age := point(t, d6, "T14", EstimatorP50)
	assert.Equal(t, 1*_day, value(t, age), "the current episode started on d5")
	full := contributions(age, idOf("A"))
	require.Len(t, full, 2)
	assert.Equal(t, entity.ReasonFullAge, full[1].ReasonCode)
	assert.Equal(t, 5*_day, *full[1].Value, "the age from the first start is shown apart")
	assert.Equal(t, 0.0, value(t, point(t, run(t, d(0), d(8), v04()), "T12", "")))
}

func TestFlow_V04UnknownHistoryIsNotExact(t *testing.T) {
	r := run(t, d(0), d(10), v04().knownFrom(d(2)))

	t05 := point(t, r, "T05", "")
	assert.Equal(t, entity.StateBounded, t05.State, "a known completion is not taken for the first one")
	assert.Equal(t, &entity.Bound{Kind: entity.BoundUpper, Value: 1}, t05.Bound)
	assert.Nil(t, t05.Value)
	assert.Equal(t, entity.ReasonHistoryIncomplete, t05.ReasonCode)
	t08 := point(t, r, "T08", EstimatorP85)
	assert.Equal(t, entity.StateUnknown, t08.State)
	assert.Nil(t, t08.Value)

	a := slice(t, r, "A")
	assert.Nil(t, a.FirstCompletion, "the first completion is not restored from what is known")
	assert.Equal(t, d(8), *a.FinalCompletion)
	assert.Equal(t, 1.0, value(t, point(t, run(t, d(0), d(6), v04().knownFrom(d(2))), "T12", "")),
		"the state after the beginning of the known history is known")
}

func TestFlow_V14Workflow(t *testing.T) {
	r := run(t, d(0), d(10),
		task("A", d(0), _active),
		task("B", d(0), _intake, status(d(3), _active)),
		task("C", d(0), _unknown, status(d(2), _done)),
	)
	assert.Equal(t, d(0), *slice(t, r, "A").FirstStart, "a task created at work starts at its creation")
	b := slice(t, r, "B")
	assert.Equal(t, d(3), *b.FirstStart, "intake gives no start")
	c := slice(t, r, "C")
	assert.Nil(t, c.FirstStart, "an unknown status gives no start")
	assert.Equal(t, []string{_unknown}, c.Unmapped)
	early := run(t, d(0), d(2), task("A", d(0), _active), task("B", d(0), _intake, status(d(3), _active)), task("C", d(0), _unknown))
	assert.Equal(t, 1.0, value(t, point(t, early, "T04", "")), "only A has started by d2")
}

func TestFlow_V41BoundariesAndUnits(t *testing.T) {
	specs := make([]taskSpec, 0, 5)
	for i, n := range []int{1, 2, 3, 4} {
		specs = append(specs, task(string(rune('P'+i)), d(0), _active, status(d(n), _done)))
	}
	specs = append(specs, task("Z", d(0), _active, status(d(10), _done)))
	r := runAt(t, d(0), d(10), d(11), entity.GroupNone, specs...)

	p50, p85 := point(t, r, "T08", EstimatorP50), point(t, r, "T08", EstimatorP85)
	assert.Equal(t, 2*_day, value(t, p50))
	assert.Equal(t, 4*_day, value(t, p85))
	assert.Equal(t, 4, *p85.N)
	assert.Equal(t, entity.StateAssessed, p85.State)
	assert.True(t, p85.LowSample, "n = 4 is marked apart from the state")
	assert.Equal(t, 4.0, value(t, point(t, r, "T05", "")), "a completion exactly at to is outside [from,to)")

	msk := time.FixedZone("MSK", 3*3600)
	offset := task("O", d(0), _intake, status(time.Date(2026, 10, 2, 3, 0, 0, 0, msk), _active))
	assert.True(t, slice(t, run(t, d(0), d(10), offset), "O").FirstStart.Equal(d(1)), "the offset is converted to UTC")

	flag := func(at time.Time, to string) change { return change{at: at, field: _flagField, to: to} }
	union := task("U", d(0), _active, status(d(1), _blocked), flag(d(2), "yes"), status(d(3), _active), flag(d(4), ""), status(d(6), _done))
	t15 := point(t, run(t, d(0), d(10), union), "T15", "")
	assert.Equal(t, 3*_day, *t15.Numerator, "blocked [d1,d3) by status and [d2,d4) by flag join to three days")
	assert.Equal(t, 6*_day, *t15.Denominator)
}

func TestFlow_V01States(t *testing.T) {
	conflicted := task("N", d(2), _intake, status(d(1), _active))
	tests := []struct {
		name      string
		give      []taskSpec
		mapping   bool
		metric    string
		estimator string
		wantState entity.DataState
		wantCode  string
	}{
		{"full scope", v03(), true, "T05", "", entity.StateAssessed, ""},
		{"no mapping", v03(), false, "T05", "", entity.StateNotEvaluated, entity.ReasonMappingMissing},
		{"no completion", []taskSpec{task("A", d(0), _active)}, true, "T08", EstimatorP85, entity.StateNotApplicable, ""},
		{"part of the cohort", []taskSpec{v03()[0], v04().knownFrom(d(2))}, true, "T08", EstimatorP85, entity.StateProvisional, entity.ReasonHistoryIncomplete},
		{"completion not known to be first", []taskSpec{v04().knownFrom(d(2))}, true, "T05", "", entity.StateBounded, entity.ReasonHistoryIncomplete},
		{"event before creation", []taskSpec{conflicted}, true, "T12", "", entity.StateBounded, entity.ReasonEventOrderConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := run(t, d(0), d(10), tt.give...)
			if !tt.mapping {
				in := input{team: _team, window: entity.Window{From: d(0), To: d(10), Step: entity.StepMonth}, asOf: d(10), location: time.UTC}
				r = compute(in)
			}
			p := point(t, r, tt.metric, tt.estimator)
			assert.Equal(t, tt.wantState, p.State)
			assert.Equal(t, tt.wantCode, p.ReasonCode)
			if tt.wantState != entity.StateAssessed && tt.wantState != entity.StateProvisional {
				assert.Nil(t, p.Value, "only ASSESSED and PROVISIONAL carry a value")
			}
		})
	}
	assert.True(t, slice(t, run(t, d(0), d(10), conflicted), "N").Conflict, "a change before the creation is a conflict of order")
}

func TestFlow_MissingMappingGivesNotEvaluated(t *testing.T) {
	in := input{team: _team, window: entity.Window{From: d(0), To: d(14), Step: entity.StepWeek}, asOf: d(14), location: time.UTC}
	for _, s := range v03() {
		in.tasks = append(in.tasks, s.input())
	}
	r := compute(in)
	assert.Len(t, r.Series, 25, "seventeen metrics, six of them with two or four estimators")
	for _, s := range r.Series {
		require.Len(t, s.Points, 2, s.MetricID)
		for _, p := range s.Points {
			assert.Equal(t, entity.StateNotEvaluated, p.State, s.MetricID)
			assert.Equal(t, entity.ReasonMappingMissing, p.ReasonCode)
			assert.Nil(t, p.Value)
		}
	}
	assert.Empty(t, r.Tasks)
}

func TestFlow_UnknownSizeIsItsOwnGroup(t *testing.T) {
	sized := task("S1", d(0), _active, status(d(3), _done))
	unsized := task("U1", d(0), _active, status(d(4), _done)).without(_sizeField)
	r := runAt(t, d(0), d(10), d(10), entity.GroupSizeLabel, sized, unsized)

	var groups []*string
	for _, s := range r.Series {
		if s.MetricID != "T05" {
			continue
		}
		groups = append(groups, s.Group.SizeLabel)
		p := s.Points[0]
		assert.Equal(t, 1.0, value(t, p), "every group counts its task")
		if s.Group.SizeLabel == nil {
			assert.Equal(t, idOf("U1"), p.Contributions[0].ObjectID, "the task without a size is in n and the contributions")
		}
	}
	require.Len(t, groups, 2)
	assert.Equal(t, "S", *groups[0])
	assert.Nil(t, groups[1], "the group without a size comes last")
	assert.Equal(t, entity.GroupingCurrentStructure, r.Grouping)
}

// TestFlow_ContributionsGivePoints recomputes every point of the sets from its contributions alone.
func TestFlow_ContributionsGivePoints(t *testing.T) {
	sets := map[string][]taskSpec{
		"V-03": v03(), "V-04": {v04()}, "weeks": slices.Concat(v03(), []taskSpec{
			task("C", d(2), _active, status(d(9), _blocked), status(d(12), _done)),
		}),
	}
	for name, set := range sets {
		r := runAt(t, d(0), d(21), d(21), entity.GroupNone, set...)
		for _, s := range r.Series {
			for _, p := range s.Points {
				assertRecomputed(t, name+" "+s.MetricID+" "+s.Estimator, s.MetricID, s.Estimator, p)
			}
		}
	}
}

func assertRecomputed(t *testing.T, label, metric, estimator string, p entity.Point) {
	t.Helper()
	var (
		count, excluded, missing int
		sum, denominator         float64
		sample                   []float64
	)
	for _, c := range p.Contributions {
		switch c.Role {
		case entity.RoleExcluded:
			excluded++
		case entity.RoleMissing:
			missing++
		case entity.RoleDenominator:
			denominator += *c.Value
		case entity.RoleNumerator:
			if c.ItemNo == 1 {
				count++
				sample = append(sample, *c.Value)
			}
			if c.ReasonCode != entity.ReasonFullAge {
				sum += *c.Value
			}
		}
	}
	assert.Equal(t, p.ExcludedCount, excluded, label)
	assert.Equal(t, p.MissingCount, missing, label)
	if p.State != entity.StateAssessed {
		return
	}
	switch metric {
	case "T04", "T05", "T07", "T12", "T18":
		assert.Equal(t, *p.Value, sum, label)
	case "T13", "T15":
		assert.Equal(t, *p.Numerator, sum, label)
		assert.Equal(t, *p.Denominator, denominator, label)
	case "T08", "T14":
		slices.Sort(sample)
		assert.Equal(t, *p.Value, percentile(sample, _percentiles[estimator]), label)
		assert.Equal(t, *p.N, count, label)
	}
}

func TestPercentile_NearestRank(t *testing.T) {
	twenty := make([]float64, 20)
	for i := range twenty {
		twenty[i] = float64(i + 1)
	}
	tests := []struct {
		name   string
		sample []float64
		p      int
		want   float64
	}{
		{"p50 of four", []float64{1, 2, 3, 4}, 50, 2},
		{"p85 of four", []float64{1, 2, 3, 4}, 85, 4},
		{"p85 of one", []float64{3}, 85, 3},
		{"p85 of twenty is the 17th", twenty, 85, 17},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, percentile(tt.sample, tt.p))
		})
	}
}
