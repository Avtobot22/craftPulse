package calc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func pointSample(task, taskType, size string, role entity.ContributionRole, value *float64) entity.PointSample {
	s := entity.PointSample{TaskID: idOf(task), Role: role, Value: value, TaskType: optional(taskType), SizeLabel: optional(size)}
	if role == entity.RoleMissing {
		s.ReasonCode = entity.ReasonHistoryIncomplete
	}
	return s
}

func TestBaselines_GroupsByTypeAndSize(t *testing.T) {
	w := entity.Window{From: d(0), To: d(28)}
	samples := []entity.PointSample{
		pointSample("A", "Story", "S", entity.RoleNumerator, num(3*_day)),
		pointSample("B", "Story", "S", entity.RoleNumerator, num(2*_day)),
		pointSample("C", "Story", "", entity.RoleNumerator, num(5*_day)),
		pointSample("D", "Bug", "S", entity.RoleExcluded, nil),
		pointSample("E", "", "S", entity.RoleNumerator, num(_day)),
	}

	got := baselines(samples, w)

	require.Len(t, got, 3, "a task without a type is in no group")
	assert.Equal(t, "Bug", got[0].TaskType)
	bug := got[0].P85
	assert.Equal(t, entity.StateNotApplicable, bug.State, "an exclusion gives no cycle")
	assert.Equal(t, 1, bug.ExcludedCount)
	assert.Equal(t, 0, *bug.N)

	s := got[1]
	assert.Equal(t, "Story", s.TaskType)
	assert.Equal(t, "S", *s.SizeLabel)
	assert.Equal(t, UnitSeconds, s.Unit)
	assert.Equal(t, entity.StateAssessed, s.P85.State)
	assert.Equal(t, 3*_day, *s.P85.Value, "p85 of two values is the second by nearest rank")
	assert.Equal(t, 2, *s.P85.N)
	assert.True(t, s.P85.LowSample)
	assert.Empty(t, s.P85.ReasonCode)
	assert.Nil(t, s.P85.Contributions)
	assert.Equal(t, w.From, s.P85.Start)
	assert.Equal(t, w.To, s.P85.End)

	unsized := got[2]
	assert.Nil(t, unsized.SizeLabel, "tasks without a size are their own group, last")
	assert.Equal(t, entity.ReasonSizeUnknown, unsized.P85.ReasonCode)
	assert.Equal(t, 5*_day, *unsized.P85.Value)
}

func TestBaselines_TaskOnceAndMissingHistory(t *testing.T) {
	samples := make([]entity.PointSample, 0, 23)
	samples = append(samples,
		pointSample("A", "Story", "S", entity.RoleMissing, nil),
		pointSample("A", "Story", "S", entity.RoleMissing, nil),
		pointSample("B", "Story", "S", entity.RoleNumerator, num(4*_day)),
	)
	for i := range 20 {
		samples = append(samples, pointSample("N"+string(rune('a'+i)), "Bug", "M", entity.RoleNumerator, num(float64(i+1)*3600)))
	}

	got := baselines(samples, entity.Window{From: d(0), To: d(7)})

	require.Len(t, got, 2)
	bug := got[0].P85
	assert.False(t, bug.LowSample, "n = 20 is not a small sample")
	assert.Equal(t, 17*3600.0, *bug.Value, "85% of 20 is the 17th value")
	story := got[1].P85
	assert.Equal(t, 1, story.MissingCount, "a task missing in two weeks counts once")
	assert.Equal(t, entity.StateProvisional, story.State)
	assert.Equal(t, entity.ReasonHistoryIncomplete, story.ReasonCode, "the reason of the missing task")
}
