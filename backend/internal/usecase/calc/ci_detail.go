package calc

import (
	"strconv"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// SummarizeRun uses the same logical jobs and I04 definitions as CI series, without a time window.
func (*UseCase) SummarizeRun(run entity.CIRun) entity.RunSummary {
	facts := entity.CIFacts{Runs: append([]entity.CIRunVersion{run.Run}, run.Children...), Attempts: run.Attempts}
	c := ciSource{runs: ciRunsOf(facts)}
	out := entity.RunSummary{UsageSeconds: c.overheadOf("total_seconds", period{}, true)}
	for i := range c.runs {
		r := &c.runs[i]
		for _, job := range r.jobs {
			out.AttemptsTotal += len(job.attempts)
			last := job.attempts[len(job.attempts)-1]
			if last.AttemptNo > 1 {
				out.RetriedJobs++
			}
			if r.version.AllowFailureSemantics && ciClass(r.version.NativeStatus, r.version.Outcome, "") == "success" && isAllowedFailure(&last, c.attemptClass(&last)) {
				out.AllowedFailures++
			}
		}
	}
	return out
}

// CalculateSummaryCI uses the CI series definitions for one pipeline, including its child runs.
func (*UseCase) CalculateSummaryCI(run entity.CIRun) ([]entity.SeriesPoint, []entity.RevisionInput) {
	facts := entity.CIFacts{Attempts: run.Attempts}
	if !run.Run.RunID.IsZero() {
		facts.Runs = append([]entity.CIRunVersion{run.Run}, run.Children...)
	}
	source := ciSource{runs: ciRunsOf(facts)}
	// A summary has no reporting period. Reuse the reducers with a fixed window covering source timestamps.
	window := period{end: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)}
	var points []entity.SeriesPoint
	var definitions []entity.RevisionInput
	for i := range _ciDefinitions {
		d := &_ciDefinitions[i]
		if d.ID == "I10" {
			continue
		}
		definitions = append(definitions, entity.RevisionInput{Kind: InputDefinition, ID: d.ID, Version: strconv.Itoa(d.Version)})
		for _, estimator := range source.estimators(*d) {
			point := source.point(d.ID, estimator, window)
			point.End = time.Time{}
			points = append(points, entity.SeriesPoint{MetricID: d.ID, Estimator: estimator, Unit: ciUnit(*d, estimator), Point: point})
		}
	}
	return points, definitions
}
