package calc

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func TestCrossPoints_UsesCardSignalsAndCountsDistinctTasks(t *testing.T) {
	t.Parallel()
	f := observationFixture()
	first := f.Reviews[0].MRs[0]
	second := first
	second.MRID = idOf("second MR")
	secondReady := d(3)
	second.ReadyAt = &secondReady
	unavailable := first
	unavailable.MRID, unavailable.ReviewerField = idOf("unavailable reviewers"), false
	f.Reviews[0].MRs = append(f.Reviews[0].MRs, second, unavailable)
	f.Links = append(f.Links, entity.LinkEdge{Type: entity.LinkTaskMR, From: f.Task.TaskID, To: second.MRID, Status: entity.LinkConfirmed})
	oldRun, run := idOf("old pipeline"), idOf("current pipeline")
	start, finish := d(4), d(5)
	f.CI.Runs = []entity.CIRunVersion{
		{RunID: oldRun, AreaID: first.AreaID, SHA: "old head", StartedAt: &start, FinishedAt: &finish, Outcome: entity.CIFailed},
		{
			RunID: run, AreaID: first.AreaID, SHA: first.HeadSHA, StartedAt: &start, FinishedAt: &finish, Outcome: entity.CISuccess,
			AttemptHistory: entity.AttemptHistoryComplete,
		},
	}
	f.CI.Attempts = []entity.CIAttemptVersion{
		{RunID: run, AttemptID: idOf("attempt 1"), AttemptNo: 1, JobKey: "test", Outcome: entity.CIFailed, FinishedAt: &finish},
		{RunID: run, AttemptID: idOf("attempt 2"), AttemptNo: 2, JobKey: "test", Outcome: entity.CISuccess, FinishedAt: &finish},
	}
	for _, id := range []entity.ID{oldRun, run} {
		f.Links = append(f.Links, entity.LinkEdge{Type: entity.LinkMRRun, From: first.MRID, To: id, Status: entity.LinkConfirmed})
	}
	gap := task("gap", d(0), _active).input()
	unknown := task("unknown assignment", d(0), _active).input()
	pr := prepare(input{
		mapping: f.Mapping, asOf: f.AsOf, tasks: []taskInput{{version: f.Task, instance: f.InstanceID, events: f.TaskEvents}, gap, unknown},
	})
	x := reportSignals{
		facts: f, review: reviewInput{mrs: observationReviews(&f)},
		taskMRs: map[entity.ID][]entity.ID{
			f.Task.TaskID: {first.MRID, second.MRID}, unknown.version.TaskID: {unavailable.MRID},
		},
		linked: map[entity.ID]bool{first.MRID: true, second.MRID: true, unavailable.MRID: true},
		runs:   map[entity.ID]entity.ID{oldRun: first.MRID, run: first.MRID},
	}
	p := period{start: d(2), end: f.AsOf}
	pts := x.crossPoints(pr, p)
	pointFor := func(est string) entity.Point {
		t.Helper()
		i := slices.IndexFunc(pts, func(pt entity.SeriesPoint) bool { return pt.Estimator == est })
		require.NotEqual(t, -1, i)
		return pts[i].Point
	}
	review := pointFor(_reviewNoReviewer)
	require.Len(t, review.Contributions, 3, "one row per WIP task")
	assert.Equal(t, num(1), review.Numerator)
	assert.Equal(t, num(1), review.Denominator)
	assert.Equal(t, 3, *review.N)
	assert.Equal(t, 1, review.MissingCount)
	assert.Equal(t, 1, review.ExcludedCount)
	assert.Equal(t, num(float64(seconds(*first.ReadyAt, f.AsOf))), review.Contributions[0].Value)
	assert.Equal(t, entity.RoleNumerator, review.Contributions[0].Role)
	assert.Equal(t, _reasonNoTaskMR, review.Contributions[1].ReasonCode)
	assert.Equal(t, entity.ReasonReviewerUnknown, review.Contributions[2].ReasonCode)
	assert.Equal(t, num(0), pointFor(_ciFailed).Numerator, "the run of a previous head does not count")
	assert.Equal(t, num(1), pointFor(_ciRetried).Numerator)
	assert.Equal(t, num(1), pointFor(_ciRetried).Denominator)
	missingReport := pointFor(_ciMissingReport)
	assert.Equal(t, entity.StateNotEvaluated, missingReport.State)
	assert.Equal(t, _reasonReportProfile, missingReport.ReasonCode)

	card := TaskObservations(&f)
	assert.Contains(t, cardKinds(card), entity.ObservationAwaitingReviewer)
	assert.Contains(t, cardKinds(card), entity.ObservationCIRetried)
	assert.NotContains(t, cardKinds(card), entity.ObservationCIFailed)
	current := x.observationsAt(f.AsOf)
	assert.Equal(t, entity.ObservationAwaitingReviewer, current[first.MRID].ReviewSignal)
	assert.True(t, current[first.MRID].Retry)

	for i := range x.review.mrs {
		x.review.mrs[i].version.HeadSHA = "new head"
	}
	pts = x.crossPoints(pr, p)
	assert.Nil(t, pointFor(_ciRetried).Numerator)
	assert.Equal(t, entity.StateUnknown, pointFor(_ciFailed).State)
}

func cardKinds(in []entity.TaskObservation) []entity.ObservationKind {
	out := make([]entity.ObservationKind, 0, len(in))
	for i := range in {
		out = append(out, in[i].Kind)
	}
	return out
}
