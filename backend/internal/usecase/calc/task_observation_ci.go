package calc

import (
	"bytes"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type taskCI struct {
	source ciSource
	byMR   map[entity.ID][]int
}

func observationCI(f *entity.TaskObservationFacts, links map[entity.ID]entity.ID) taskCI {
	out := taskCI{source: ciSource{asOf: f.AsOf}, byMR: map[entity.ID][]int{}}
	var selected entity.CIFacts
	kept := map[entity.ID]bool{}
	for i := range f.CI.Runs {
		run := &f.CI.Runs[i]
		if _, ok := links[run.RunID]; !ok || !knownOnCut(run.Knowledge, f.KnowledgeSeq) {
			continue
		}
		if started := runOrderTime(run); !started.IsZero() && !started.After(f.AsOf) {
			kept[run.RunID] = true
			selected.Runs = append(selected.Runs, *run)
		}
	}
	for i := range f.CI.Attempts {
		attempt := &f.CI.Attempts[i]
		if kept[attempt.RunID] && knownOnCut(attempt.Knowledge, f.KnowledgeSeq) && attemptKnownBy(attempt, f.AsOf) {
			version := *attempt
			if version.FinishedAt == nil && version.ObservedAt.After(f.AsOf) {
				version.NativeStatus, version.FailureKind, version.Outcome, version.DurationS = "", "", entity.CIUnknown, nil
			}
			selected.Attempts = append(selected.Attempts, version)
		}
	}
	out.source.runs = ciRunsOf(selected)
	for i := range out.source.runs {
		mr := links[out.source.runs[i].version.RunID]
		out.byMR[mr] = append(out.byMR[mr], i)
	}
	return out
}

func attemptKnownBy(a *entity.CIAttemptVersion, at time.Time) bool {
	switch {
	case a.StartedAt != nil:
		return !a.StartedAt.After(at)
	case a.FinishedAt != nil:
		return !a.FinishedAt.After(at)
	default:
		return !a.ObservedAt.After(at)
	}
}

func (c *taskCI) forMR(mr *entity.MRVersion, rev *entity.MRRevision) *ciRun {
	head := mr.HeadSHA
	if !snapshotKnownAt(mr.SourceUpdatedAt, mr.ObservedAt, c.source.asOf) {
		if rev == nil {
			return nil
		}
		head = rev.HeadSHA
	}
	if head == "" {
		return nil
	}
	var newest *ciRun
	for _, i := range c.byMR[mr.MRID] {
		run := &c.source.runs[i]
		v := &run.version
		if v.AreaID == mr.AreaID && v.SHA == head && (newest == nil || newerRun(v, &newest.version)) {
			newest = run
		}
	}
	return newest
}

func newerRun(a, b *entity.CIRunVersion) bool {
	diff := runOrderTime(a).Compare(runOrderTime(b))
	return diff > 0 || diff == 0 && bytes.Compare(a.RunID[:], b.RunID[:]) > 0
}

func runOrderTime(run *entity.CIRunVersion) time.Time {
	switch {
	case run.StartedAt != nil:
		return *run.StartedAt
	case run.FinishedAt != nil:
		return *run.FinishedAt
	case run.SourceUpdatedAt != nil:
		return *run.SourceUpdatedAt
	default:
		return run.ObservedAt
	}
}

func taskPipelineObservation(f *entity.TaskObservationFacts, c *taskCI, run *ciRun, ref entity.ObjectRef) (entity.TaskObservation, bool) {
	v := &run.version
	o := entity.TaskObservation{Basis: []entity.ObjectRef{ref}, Action: entity.ObservationCheckPipeline}
	if v.FinishedAt == nil && !snapshotKnownAt(v.SourceUpdatedAt, v.ObservedAt, f.AsOf) {
		return entity.TaskObservation{}, false
	}
	class := c.source.runClass(run)
	switch {
	case v.FinishedAt != nil && v.FinishedAt.After(f.AsOf), v.Outcome == entity.CIRunning && class == "unknown":
		start := runOrderTime(v)
		o.Kind, o.Since = entity.ObservationCIRunning, utc(&start)
	case ciFailure(class):
		o.Kind, o.Since = entity.ObservationCIFailed, utc(v.FinishedAt)
	case class == "success":
		o.Kind, o.Since = entity.ObservationCIRetried, utc(v.FinishedAt)
		o.Retry = taskRetry(run, f.AsOf)
		if o.Retry.Attempts == 0 && !o.Retry.ObservedOutcomeFlip {
			return entity.TaskObservation{}, false
		}
	default:
		return entity.TaskObservation{}, false
	}
	for _, job := range run.jobs {
		for i := range job.attempts {
			a := &job.attempts[i]
			if ciFailure(c.source.attemptClass(a)) || o.Retry != nil && a.AttemptNo > 1 {
				o.Basis = append(o.Basis, observationRef(f, entity.ObjectCIAttempt, a.AttemptID))
			}
		}
	}
	return o, true
}

func taskRetry(run *ciRun, at time.Time) *entity.TaskRetry {
	src := ciSource{runs: []ciRun{*run}, asOf: at}
	count := src.overheadOf("attempts", period{}, true)
	usage := src.overheadOf("seconds", period{}, true)
	attempts, attemptsLower := retryValue(&count)
	seconds, secondsLower := retryValue(&usage)
	out := &entity.TaskRetry{Attempts: attempts, Seconds: seconds, AttemptsLowerBound: attemptsLower, SecondsLowerBound: secondsLower}
	for i := range run.jobs {
		flipped, _ := src.retryFlip(&run.jobs[i])
		out.ObservedOutcomeFlip = out.ObservedOutcomeFlip || flipped
	}
	return out
}

func retryValue(p *entity.Point) (int64, bool) {
	if p.Bound != nil {
		return int64(p.Bound.Value), true
	}
	return int64(*p.Value), false
}
