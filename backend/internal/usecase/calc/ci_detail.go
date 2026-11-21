package calc

import "github.com/Avtobot22/craftPulse/backend/internal/entity"

// SummarizeRun uses the same logical jobs and I04 definitions as CI series, without a time window.
func (*UseCase) SummarizeRun(run entity.CIRun, provider entity.Provider) entity.RunSummary {
	facts := entity.CIFacts{Provider: provider, Runs: append([]entity.CIRunVersion{run.Run}, run.Children...), Attempts: run.Attempts}
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
			if provider == entity.ProviderGitLab && ciClass(run.Run.NativeStatus, run.Run.Outcome, "") == "success" && last.AllowFailure && ciFailure(ciClass(last.NativeStatus, last.Outcome, last.FailureKind)) {
				out.AllowedFailures++
			}
		}
	}
	return out
}
