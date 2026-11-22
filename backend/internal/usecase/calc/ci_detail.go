package calc

import "github.com/Avtobot22/craftPulse/backend/internal/entity"

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
