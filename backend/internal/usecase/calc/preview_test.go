package calc

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// previewAt applies m1 as a draft to the tasks over the week of the report up to as_of.
func previewAt(asOf time.Time, specs ...taskSpec) entity.MappingPreview {
	in := input{mapping: m1(), team: _team, window: controlWindow(asOf, time.UTC), asOf: asOf, location: time.UTC}
	for _, s := range specs {
		in.tasks = append(in.tasks, s.input())
	}
	return preview(in)
}

func previewTask(t *testing.T, p entity.MappingPreview, name string) entity.PreviewTask {
	t.Helper()
	i := slices.IndexFunc(p.Tasks, func(v entity.PreviewTask) bool { return v.TaskID == idOf(name) })
	require.GreaterOrEqual(t, i, 0, name)
	return p.Tasks[i]
}

func control(t *testing.T, p entity.MappingPreview, metric string) entity.ControlValue {
	t.Helper()
	i := slices.IndexFunc(p.Controls, func(c entity.ControlValue) bool { return c.MetricID == metric })
	require.GreaterOrEqual(t, i, 0, metric)
	return p.Controls[i]
}

func TestPreview_V14Workflow(t *testing.T) {
	p := previewAt(d(1),
		task("A", d(0), _active),
		task("B", d(0), _intake),
		task("C", d(0), _unknown),
	)

	a := previewTask(t, p, "A")
	require.NotNil(t, a.StartedAt)
	assert.Equal(t, d(0), *a.StartedAt, "start(A) = d0: A is created at work")
	assert.Equal(t, entity.PreviewOpen, a.Outcome)
	b := previewTask(t, p, "B")
	assert.Nil(t, b.StartedAt, "intake gives no start")
	assert.Equal(t, entity.PreviewOpen, b.Outcome)
	c := previewTask(t, p, "C")
	assert.Nil(t, c.StartedAt)
	assert.Nil(t, c.CompletedAt)
	assert.Equal(t, entity.PreviewUnknown, c.Outcome, "a status without a rule leaves the outcome unknown")
	assert.Equal(t, []string{_unknown}, c.Unmapped)
	assert.Equal(t, []entity.NativeStatusRef{{InstanceID: _instance, NativeID: _unknown}}, p.Unmapped)
	assert.Equal(t, 1.0, value(t, control(t, p, "T12").Point), "only A is at work at as_of")
}

func TestPreview_V03TasksAndControls(t *testing.T) {
	asOf := d(10)
	p := previewAt(asOf, v03()...)

	a := previewTask(t, p, "A")
	assert.Equal(t, d(1), *a.StartedAt)
	assert.Equal(t, d(4), *a.CompletedAt)
	assert.Equal(t, entity.PreviewCompleted, a.Outcome)
	assert.Equal(t, int64(86400), *a.BlockedSeconds, "A is blocked one day")
	assert.Equal(t, "S", *a.SizeLabel)
	b := previewTask(t, p, "B")
	assert.Equal(t, entity.PreviewCancelled, b.Outcome)
	assert.Nil(t, b.CompletedAt, "a cancellation is not an accepted completion")
	assert.Zero(t, *b.BlockedSeconds)
	assert.Empty(t, p.Unmapped)

	assert.Equal(t, entity.Window{From: d(4), To: asOf, Step: entity.StepWeek}, p.Period, "October 11 2026 is a Sunday, its week starts on Monday the 5th")
	r := runAt(t, p.Period.From, p.Period.To, asOf, entity.GroupNone, v03()...)
	require.Len(t, p.Controls, 3)
	for _, c := range p.Controls {
		assert.Equal(t, point(t, r, c.MetricID, c.Estimator), c.Point, "the control %s is the point of the report", c.MetricID)
	}
	assert.Equal(t, []string{"T05", "T08", "T12"}, []string{p.Controls[0].MetricID, p.Controls[1].MetricID, p.Controls[2].MetricID})
	assert.Equal(t, EstimatorP50, p.Controls[1].Estimator)

	week := previewAt(d(5), v03()...)
	assert.Equal(t, 1.0, value(t, control(t, week, "T05").Point), "A is completed on October 5")
	assert.Equal(t, 3*_day, value(t, control(t, week, "T08").Point))
}

func TestPreview_SampleTakesEveryKindFirst(t *testing.T) {
	specs := make([]taskSpec, 0, 36)
	for i := range 30 {
		specs = append(specs, task("open"+strconv.Itoa(i), d(0), _active, status(d(1).Add(time.Duration(i)*time.Minute), _waiting)))
	}
	specs = append(specs,
		task("old-intake", d(-30), _intake),
		task("old-done", d(-30), _intake, status(d(-29), _active), status(d(-28), _done)),
		task("old-cancelled", d(-30), _intake, status(d(-29), _cancelled)),
		task("old-blocked", d(-30), _blocked),
		task("old-no-size", d(-30), _active).without(_sizeField),
		task("old-unknown", d(-30), _unknown),
	)

	p := previewAt(d(5), specs...)
	again := previewAt(d(5), specs...)

	require.Len(t, p.Tasks, _previewTasks)
	assert.Equal(t, p, again, "a repeat gives the same answer")
	for _, name := range []string{"old-intake", "old-done", "old-cancelled", "old-blocked", "old-no-size", "old-unknown"} {
		previewTask(t, p, name)
	}
	var open []string
	for i := range p.Tasks {
		if name := nameOf(p.Tasks, specs, i); !slices.Contains([]string{"old-intake", "old-done", "old-cancelled", "old-blocked", "old-no-size", "old-unknown"}, name) {
			open = append(open, name)
		}
	}
	slices.Sort(open)
	want := []string{"open16", "open17", "open18", "open19", "open20", "open21", "open22", "open23", "open24", "open25", "open26", "open27", "open28", "open29"}
	assert.Equal(t, want, open, "the rest are the most recently changed")
	assert.True(t, slices.IsSortedFunc(p.Tasks, func(a, b entity.PreviewTask) int { return a.Created.Compare(b.Created) }))
	assert.Equal(t, 32.0, value(t, control(t, p, "T12").Point), "the controls count every task, not the sample")
}

func TestPreview_ControlWindow(t *testing.T) {
	moscow, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	tests := []struct {
		name     string
		asOf     time.Time
		loc      *time.Location
		wantFrom time.Time
	}{
		{"inside a week", time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC), time.UTC, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)},
		{"Monday midnight takes the week before", time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), time.UTC, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)},
		{"time zone of the team", time.Date(2026, 11, 8, 22, 0, 0, 0, time.UTC), moscow, time.Date(2026, 11, 8, 21, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := controlWindow(tt.asOf, tt.loc)
			assert.Equal(t, entity.Window{From: tt.wantFrom, To: tt.asOf, Step: entity.StepWeek}, w)
			assert.Len(t, periods(w, tt.asOf, tt.loc), 1, "the window is one point of the report")
		})
	}
}

// nameOf is the name of the spec of the i-th task of a preview.
func nameOf(tasks []entity.PreviewTask, specs []taskSpec, i int) string {
	for _, s := range specs {
		if idOf(s.name) == tasks[i].TaskID {
			return s.name
		}
	}
	return ""
}
