package calc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func ciFixture() (entity.CIFacts, period) {
	start, end := day(1), day(8)
	finished := start.Add(130 * time.Second)
	run, area := newID(), newID()
	f := entity.CIFacts{AreaID: area, Provider: entity.ProviderGitLab, Runs: []entity.CIRunVersion{{
		ID: newID(), RunID: run, AreaID: area, NativeStatus: "success", Outcome: entity.CISuccess,
		StartedAt: &start, FinishedAt: &finished, AttemptHistory: entity.AttemptHistoryComplete, AllowFailureSemantics: true,
	}}}
	for i, s := range []struct {
		config, native string
		no             int
		seconds        int64
	}{
		{"linux", "failed", 1, 60}, {"linux", "success", 2, 30}, {"windows", "success", 1, 40},
	} {
		f.Attempts = append(f.Attempts, entity.CIAttemptVersion{
			ID: newID(), AttemptID: newID(), RunID: run,
			JobKey: "test", ConfigFingerprint: []byte(s.config), AttemptNo: s.no, NativeStatus: s.native, DurationS: &s.seconds,
			StartedAt: &start, FinishedAt: &finished,
		})
		if i == 0 {
			f.Attempts[i].FailureKind = "script_failure"
		}
	}
	return f, period{start: start, end: end}
}

func sourceOf(f entity.CIFacts) *ciSource {
	return &ciSource{runs: ciRunsOf(f), areas: []entity.ID{f.AreaID}, location: time.UTC, series: &entity.CalcSeries{Step: entity.StepWeek}}
}

func TestCI_V28AndIncompleteHistory(t *testing.T) {
	t.Parallel()
	cases := []struct {
		metric, est string
		value       float64
	}{
		{"I01", "runs", 1},
		{"I01", "jobs", 1},
		{"I02", "", .5},
		{"I03", "", .5},
		{"I04", "attempts", 1},
		{"I04", "seconds", 30},
		{"I04", "total_seconds", 130},
		{"I05", "", 0},
		{"I07", "script_failure", 1},
		{"I07", "unknown", 0},
	}
	for _, tt := range cases {
		t.Run(tt.metric+tt.est, func(t *testing.T) {
			f, p := ciFixture()
			s := sourceOf(f)
			pt := s.point(tt.metric, tt.est, p)
			require.Equal(t, entity.StateAssessed, pt.State)
			require.NotNil(t, pt.Value)
			assert.InDelta(t, tt.value, *pt.Value, 0)
			if tt.metric == "I02" || tt.metric == "I03" {
				assert.InDelta(t, 2, *pt.Denominator, 0, "matrix jobs remain separate")
			}
		})
	}
	for _, metric := range []string{"I02", "I03", "I04", "I10"} {
		t.Run("truncated "+metric, func(t *testing.T) {
			f, p := ciFixture()
			f.Runs[0].AttemptHistory = entity.AttemptHistoryTruncated
			s := sourceOf(f)
			est := ""
			if metric == "I04" {
				est = "seconds"
			}
			pt := s.point(metric, est, p)
			assert.Nil(t, pt.Value)
			if metric == "I04" {
				require.Equal(t, entity.StateBounded, pt.State)
				assert.Equal(t, &entity.Bound{Kind: entity.BoundLower, Value: 30}, pt.Bound)
			}
			if metric == "I10" {
				loaded := s.point(metric, "loaded", p)
				assert.Equal(t, &entity.Bound{Kind: entity.BoundLower, Value: 1}, loaded.Bound)
			}
		})
	}
}

func TestCI_V01TwelveMinutesIsALowerBound(t *testing.T) {
	t.Parallel()
	f, p := ciFixture()
	f.Runs[0].AttemptHistory = entity.AttemptHistoryTruncated
	seconds := int64(720)
	f.Attempts = f.Attempts[1:2]
	f.Attempts[0].DurationS = &seconds
	pt := sourceOf(f).point("I04", "seconds", p)
	require.Equal(t, entity.StateBounded, pt.State)
	assert.Nil(t, pt.Value)
	assert.Equal(t, &entity.Bound{Kind: entity.BoundLower, Value: 720}, pt.Bound)
}

func TestCI_OutcomeClassesAndExclusions(t *testing.T) {
	t.Parallel()
	for _, status := range _ciClasses {
		t.Run(status, func(t *testing.T) {
			f, p := ciFixture()
			f.Runs[0].NativeStatus = status
			f.Runs[0].Outcome = entity.CIUnknown
			f.Attempts = nil
			s := sourceOf(f)
			pt := s.point("I06", "runs:"+status, p)
			require.NotNil(t, pt.Value)
			assert.InDelta(t, 1, *pt.Value, 0)
			assert.InDelta(t, 1, *pt.Numerator, 0)
			pt = s.point("I01", "runs", p)
			if !ciTerminal(status) {
				assert.Nil(t, pt.Value)
				assert.Nil(t, pt.Denominator)
			} else {
				require.NotNil(t, pt.Denominator)
				assert.InDelta(t, 1, *pt.Denominator, 0)
			}
		})
	}
}

func TestCI_AllowedFailureUsesFinalAttemptAndSourceSemantics(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name                   string
		retry, allowed, gitlab bool
		want                   float64
		state                  entity.DataState
	}{
		{"final allowed failure", false, true, true, 1, entity.StateAssessed},
		{"retried pass", true, true, true, 0, entity.StateAssessed},
		{"failure not allowed", false, false, true, 0, entity.StateAssessed},
		{"semantics unavailable", false, true, false, 0, entity.StateNotApplicable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, p := ciFixture()
			f.Attempts[0].AllowFailure = tt.allowed
			if !tt.retry {
				f.Attempts = f.Attempts[:1]
			}
			if !tt.gitlab {
				f.Provider, f.Runs[0].AllowFailureSemantics = entity.ProviderJenkins, false
			}
			pt := sourceOf(f).point("I05", "", p)
			require.Equal(t, tt.state, pt.State)
			if tt.state == entity.StateAssessed {
				assert.InDelta(t, tt.want, *pt.Value, 0)
			}
		})
	}
}

func TestCI_AttemptGapsDurationsAndUnknownReasons(t *testing.T) {
	t.Parallel()
	f, p := ciFixture()
	f.Attempts = f.Attempts[1:2]
	s := sourceOf(f)
	assert.Nil(t, s.point("I02", "", p).Value, "a missing first attempt is not inferred")
	assert.Equal(t, entity.StateBounded, s.point("I04", "seconds", p).State)
	f, p = ciFixture()
	f.Attempts[1].DurationS = nil
	s = sourceOf(f)
	assert.Equal(t, entity.StateAssessed, s.point("I04", "attempts", p).State)
	assert.Equal(t, entity.StateBounded, s.point("I04", "seconds", p).State)
	f.Attempts[0].FailureKind = ""
	s = sourceOf(f)
	pt := s.point("I07", "unknown", p)
	assert.InDelta(t, 1, *pt.Value, 0)
	var evidence map[string]any
	require.NoError(t, json.Unmarshal(pt.Contributions[0].Evidence, &evidence))
	assert.Equal(t, "failed", evidence["native_status"])
	assert.Equal(t, "", evidence["failure_kind"])
	assert.NotContains(t, evidence, "code")
	assert.Equal(t, "timeout", ciClass("failed", entity.CIFailed, "job_execution_timeout"))
	assert.Len(t, ciFailureGroup(strings.Repeat("x", 100)), 31)
}

func TestCI_I10RequiresReconciledCIWindow(t *testing.T) {
	t.Parallel()
	f, p := ciFixture()
	for _, tt := range []struct {
		name    string
		change  func(*entity.AreaCollection)
		covered bool
	}{
		{"complete", func(*entity.AreaCollection) {}, true},
		{"all source history", func(s *entity.AreaCollection) { s.HistoryFrom = nil }, true},
		{"partial pagination", func(s *entity.AreaCollection) { s.Completeness = "partial" }, false},
		{"tracker reconciliation", func(s *entity.AreaCollection) { s.Reconcile.DataKind = "tasks" }, false},
		{"watermark behind", func(s *entity.AreaCollection) { s.Watermark = &p.start }, false},
		{"history boundary", func(s *entity.AreaCollection) { at := p.start.Add(time.Second); s.HistoryFrom = &at }, false},
		{"no reconciliation", func(s *entity.AreaCollection) { s.Reconcile = nil }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := sourceOf(f)
			a := entity.AreaCollection{
				AreaID: f.AreaID, ConnectionID: newID(), Completeness: entity.CollectionComplete, Watermark: &p.end, HistoryFrom: &p.start,
				Reconcile: &entity.ReconcileRun{DataKind: "ci", Outcome: entity.ReconcileComplete, FinishedAt: &p.end},
			}
			tt.change(&a)
			s.collection = []entity.AreaCollection{a}
			pt := s.point("I10", "", p)
			if tt.covered {
				require.Equal(t, entity.StateAssessed, pt.State)
				assert.InDelta(t, 1, *pt.Value, 0)
			} else {
				assert.Equal(t, entity.StateUnknown, pt.State)
				assert.Equal(t, entity.StateBounded, s.point("I10", "loaded", p).State)
			}
		})
	}
}

func TestCI_V41HalfOpenWindowsAndHash(t *testing.T) {
	t.Parallel()
	f, p := ciFixture()
	s := sourceOf(f)
	for _, tt := range []struct {
		name  string
		at    time.Time
		count float64
	}{
		{"before", p.start.Add(-time.Nanosecond), 0}, {"from", p.start, 1}, {"before to", p.end.Add(-time.Nanosecond), 1}, {"to", p.end, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s.runs[0].version.FinishedAt = &tt.at
			pt := s.point("I04", "attempts", p)
			assert.InDelta(t, tt.count, *pt.Value, 0)
		})
	}
	s = sourceOf(f)
	before := s.hash(p.start, p.end)
	f.Attempts = append(f.Attempts, entity.CIAttemptVersion{
		ID: newID(), AttemptID: newID(), RunID: f.Runs[0].RunID,
		JobKey: "test", ConfigFingerprint: []byte("linux"), AttemptNo: 3, NativeStatus: "success",
	})
	assert.NotEqual(t, before, sourceOf(f).hash(p.start, p.end), "late attempts invalidate the original run's window")
	out := sourceOf(f).stored([]period{p}, before, p.start, p.end)
	for _, pt := range out.Points {
		assert.NotEmpty(t, pt.Unit)
	}
}

func TestCI_MissingRunClockDoesNotBecomeZero(t *testing.T) {
	t.Parallel()
	f, p := ciFixture()
	f.Runs[0].FinishedAt = nil
	s := sourceOf(f)
	for _, metric := range []string{"I01", "I02", "I03", "I04", "I05", "I06", "I07", "I10"} {
		est := ""
		if metric == "I01" {
			est = "runs"
		}
		if metric == "I04" {
			est = "seconds"
		}
		if metric == "I06" {
			est = "runs:success"
		}
		if metric == "I07" {
			est = "unknown"
		}
		pt := s.point(metric, est, p)
		assert.Nil(t, pt.Value)
		assert.Equal(t, 1, pt.MissingCount)
	}
}

func TestCI_AsOfDoesNotUseFutureCompletion(t *testing.T) {
	t.Parallel()
	f, p := ciFixture()
	s := sourceOf(f)
	s.asOf = p.start.Add(time.Second)
	assert.Nil(t, s.point("I02", "", p).Value)
	assert.Nil(t, s.point("I03", "", p).Value)
	assert.Equal(t, entity.StateBounded, s.point("I04", "seconds", p).State)
	before := s.hash(p.start, p.end)
	s.asOf = p.end
	assert.InDelta(t, .5, *s.point("I02", "", p).Value, 0)
	assert.NotEqual(t, before, s.hash(p.start, p.end))
}

// jenkinsFacts are builds of one job as normalize stores them: results as native statuses and, for a build without
// an execution manifest, one attempt for the build itself and a cut history.
func jenkinsFacts(results ...string) (entity.CIFacts, period) {
	start, end := day(1), day(8)
	finished := start.Add(time.Minute)
	f := entity.CIFacts{AreaID: newID(), Provider: entity.ProviderJenkins}
	outcomes := map[string]entity.CIOutcome{
		"SUCCESS": entity.CISuccess, "FAILURE": entity.CIFailed, "UNSTABLE": entity.CIFailed, "ABORTED": entity.CICanceled, "NOT_BUILT": entity.CISkipped,
	}
	seconds := int64(60)
	for _, r := range results {
		run := newID()
		f.Runs = append(f.Runs, entity.CIRunVersion{
			ID: newID(), RunID: run, AreaID: f.AreaID, NativeStatus: r, Outcome: outcomes[r],
			StartedAt: &start, FinishedAt: &finished, AttemptHistory: entity.AttemptHistoryTruncated,
		})
		f.Attempts = append(f.Attempts, entity.CIAttemptVersion{
			ID: newID(), AttemptID: newID(), RunID: run, JobKey: "build", ConfigFingerprint: []byte("build"), AttemptNo: 1,
			NativeStatus: r, Outcome: outcomes[r], DurationS: &seconds, StartedAt: &start, FinishedAt: &finished,
		})
	}
	return f, period{start: start, end: end}
}

func TestCI_JenkinsResults(t *testing.T) {
	t.Parallel()
	f, p := jenkinsFacts("SUCCESS", "FAILURE", "UNSTABLE", "ABORTED", "NOT_BUILT")
	s := sourceOf(f)
	final := s.point("I01", "runs", p)
	require.NotNil(t, final.Value)
	assert.InDelta(t, 3, *final.Denominator, 0, "ABORTED and NOT_BUILT are not in the denominator")
	assert.InDelta(t, 1.0/3, *final.Value, 1e-9)
	for class, want := range map[string]float64{"unstable": 1, "failure": 1, "canceled": 1, "skipped": 1, "success": 1} {
		pt := s.point("I06", "runs:"+class, p)
		require.NotNil(t, pt.Numerator, class)
		assert.InDelta(t, want, *pt.Numerator, 0, class)
	}
	unstable := s.point("I06", "jobs:unstable", p)
	require.NotNil(t, unstable.Numerator)
	assert.InDelta(t, 1, *unstable.Numerator, 0, "the attempt of a build keeps UNSTABLE apart")
}

func TestCI_JenkinsBuildWithoutManifest(t *testing.T) {
	t.Parallel()
	f, p := jenkinsFacts("FAILURE")
	s := sourceOf(f)
	retries := s.point("I04", "seconds", p)
	assert.Equal(t, entity.StateBounded, retries.State, "inner retries of the build are unknown")
	assert.Nil(t, retries.Value)
	require.NotNil(t, retries.Bound)
	assert.Equal(t, entity.BoundLower, retries.Bound.Kind)
	for _, metric := range []string{"I02", "I03"} {
		assert.Nil(t, s.point(metric, "", p).Value, metric)
	}
}
