package calc

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// regatherSet mixes the variants of the flow and quality tests: blocking, cancellation, a reopen, an unknown
// status, a task without a size and one whose history is partial.
func regatherSet() []taskSpec {
	return append(v16(),
		task("D", d(0), _intake, status(d(1), _active), status(d(3), _done), status(d(12), _active), status(d(20), _done)),
		task("E", d(2), _unknown, status(d(9), _done)),
		task("F", d(1), _intake, status(d(6), _active), status(d(8), _blocked)).knownFrom(d(5)),
		task("G", d(4), _active, status(d(30), _cancelled)),
	)
}

// regatherRun computes six weeks of typed tasks with a target for the group of A.
func regatherRun(t *testing.T, specs ...taskSpec) entity.CalcResult {
	t.Helper()
	in := input{
		mapping: typedMapping(), team: _team, location: time.UTC, followUp: 7 * 24 * time.Hour,
		window: entity.Window{From: d(0), To: d(42), Step: entity.StepWeek}, asOf: d(42),
		targets: map[entity.TargetGroup]int64{{TaskType: "Story", SizeLabel: "S"}: int64(3 * _day)},
	}
	for _, s := range specs {
		ti := s.input()
		ti.version.TypeNativeID = _story
		ti.area = idOf("area")
		in.tasks = append(in.tasks, ti)
	}
	return compute(in)
}

func assertSamePoint(t *testing.T, want, got entity.Point, msg string) {
	t.Helper()
	assert.Equal(t, want.State, got.State, msg)
	assert.Equal(t, want.Value, got.Value, msg)
	assert.Equal(t, want.Numerator, got.Numerator, msg)
	assert.Equal(t, want.Denominator, got.Denominator, msg)
	assert.Equal(t, want.N, got.N, msg)
	assert.Equal(t, want.Bound, got.Bound, msg)
	assert.Equal(t, want.ExcludedCount, got.ExcludedCount, msg)
	assert.Equal(t, want.MissingCount, got.MissingCount, msg)
	assert.Equal(t, want.LowSample, got.LowSample, msg)
	assert.Equal(t, want.ReasonCode, got.ReasonCode, msg)
}

func TestRegather_AllContributionsGiveTheStoredPoint(t *testing.T) {
	r := regatherRun(t, regatherSet()...)
	checked := 0
	for _, s := range r.Series {
		for _, p := range s.Points {
			sp := entity.SeriesPoint{MetricID: s.MetricID, Estimator: s.Estimator, Unit: s.Unit, Point: p}
			if !regathered(&sp) {
				continue
			}
			assertSamePoint(t, p, regather(&sp, p.Contributions), s.MetricID+" "+s.Estimator+" "+p.Start.Format(time.DateOnly))
			checked++
		}
	}
	assert.Greater(t, checked, 100)
}

func TestRegather_PartOfTheCohortIsTheFormulaOverThatPart(t *testing.T) {
	all := regatherSet()
	visible := []string{"A", "C", "D", "F"}
	part := slices.DeleteFunc(slices.Clone(all), func(s taskSpec) bool { return !slices.Contains(visible, s.name) })
	full, want := regatherRun(t, all...), regatherRun(t, part...)
	ids := make([]entity.ID, len(visible))
	for i, name := range visible {
		ids[i] = idOf(name)
	}
	for i, s := range full.Series {
		if _connectionMetrics[s.MetricID] {
			continue
		}
		for j, p := range s.Points {
			sp := entity.SeriesPoint{MetricID: s.MetricID, Estimator: s.Estimator, Unit: s.Unit, Point: p}
			seen := slices.DeleteFunc(slices.Clone(p.Contributions), func(c entity.Contribution) bool { return !slices.Contains(ids, c.ObjectID) })
			assertSamePoint(t, want.Series[i].Points[j], regather(&sp, seen), s.MetricID+" "+s.Estimator+" "+p.Start.Format(time.DateOnly))
		}
	}
}
