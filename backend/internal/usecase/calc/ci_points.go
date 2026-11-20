package calc

import (
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func ciClass(native string, outcome entity.CIOutcome, failure string) string {
	if failure == "job_execution_timeout" || failure == "stuck_or_timeout_failure" {
		return "timeout"
	}
	if v, ok := _ciNativeClasses[native]; ok {
		return v
	}
	switch outcome {
	case entity.CISuccess:
		return "success"
	case entity.CIFailed:
		return "failure"
	case entity.CICanceled:
		return "canceled"
	case entity.CISkipped:
		return "skipped"
	case entity.CIRunning, entity.CIUnknown:
		return "unknown"
	}
	return "unknown"
}

func ciFailure(class string) bool {
	return class == "failure" || class == "timeout" || class == "unstable"
}
func ciTerminal(class string) bool { return class == "success" || ciFailure(class) }

func (r *ciRun) missingCompletion() bool {
	return r.version.FinishedAt == nil && ciTerminal(ciClass(r.version.NativeStatus, r.version.Outcome, ""))
}

func (c *ciSource) runClass(r *ciRun) string {
	if !c.asOf.IsZero() && r.version.FinishedAt != nil && r.version.FinishedAt.After(c.asOf) {
		return "unknown"
	}
	return ciClass(r.version.NativeStatus, r.version.Outcome, "")
}

func (c *ciSource) attemptClass(a *entity.CIAttemptVersion) string {
	if !c.asOf.IsZero() && a.FinishedAt != nil && a.FinishedAt.After(c.asOf) {
		return "unknown"
	}
	return ciClass(a.NativeStatus, a.Outcome, a.FailureKind)
}

func (c *ciSource) durationKnown(a *entity.CIAttemptVersion) bool {
	return a.DurationS != nil && (c.asOf.IsZero() || a.FinishedAt == nil || !a.FinishedAt.After(c.asOf))
}

func ciTally(p period) *tally {
	t := newTally(p)
	t.kind = entity.ObjectCIRun
	return t
}

func ciAdd(t *tally, r *ciRun, a *entity.CIAttemptVersion, job *ciJob, role entity.ContributionRole, value *float64, reason string) {
	id, kind := r.version.RunID, entity.ObjectCIRun
	if a != nil {
		id, kind = a.AttemptID, entity.ObjectCIAttempt
	}
	if job != nil {
		id, kind = job.id, entity.ObjectCIJob
	}
	t.kind = kind
	item := map[entity.ContributionRole]int{entity.RoleDenominator: 1, entity.RoleNumerator: 2, entity.RoleExcluded: 3, entity.RoleMissing: 4}[role]
	t.add(id, item, role, value, reason)
	t.contributions[len(t.contributions)-1].Evidence = ciEvidence(r, a)
}

func (c *ciSource) point(metric, est string, p period) entity.Point {
	var pt entity.Point
	switch metric {
	case "I01", "I06":
		pt = c.outcomes(metric, est, p)
	case "I02", "I03":
		pt = c.logical(metric, p)
	case "I04":
		pt = c.overhead(est, p)
	case "I05":
		pt = c.allowed(p)
	case "I07":
		pt = c.failures(est, p)
	case "I10":
		pt = c.completeness(est, p)
	}
	for i := range c.runs {
		r := &c.runs[i]
		if !r.missingCompletion() {
			continue
		}
		pt.MissingCount++
		pt.ReasonCode = "run_clock_missing"
		pt.Contributions = append(pt.Contributions, entity.Contribution{
			ObjectID: r.version.RunID, ObjectKind: entity.ObjectCIRun,
			ItemNo: 4, Role: entity.RoleMissing, ReasonCode: pt.ReasonCode, Evidence: ciEvidence(r, nil),
		})
		if metric == "I04" || metric == "I07" || metric == "I10" && est == "loaded" {
			if pt.Value != nil {
				pt.Bound = &entity.Bound{Kind: entity.BoundLower, Value: *pt.Value}
			}
			pt.State, pt.Value = entity.StateBounded, nil
		} else if metric != "I10" && pt.Value != nil {
			pt.State = entity.StateProvisional
		} else {
			pt.State, pt.Value = entity.StateUnknown, nil
		}
	}
	return pt
}

func (c *ciSource) outcomes(metric, est string, p period) entity.Point {
	t := ciTally(p)
	var numerator, denominator float64
	kind, class, _ := strings.Cut(est, ":")
	add := func(r *ciRun, a *entity.CIAttemptVersion, job *ciJob, status string) {
		if metric == "I01" && !ciTerminal(status) {
			role := entity.RoleExcluded
			if status == "unknown" {
				role = entity.RoleMissing
			}
			ciAdd(t, r, a, job, role, nil, status)
			return
		}
		denominator++
		ciAdd(t, r, a, job, entity.RoleDenominator, num(1), status)
		if metric == "I01" && status == "success" || metric == "I06" && status == class {
			numerator++
			ciAdd(t, r, a, job, entity.RoleNumerator, num(1), status)
		}
	}
	for i := range c.runs {
		r := &c.runs[i]
		if !r.in(p) {
			continue
		}
		if kind == "runs" {
			add(r, nil, nil, c.runClass(r))
			continue
		}
		for j := range r.jobs {
			job := &r.jobs[j]
			a := &job.attempts[len(job.attempts)-1]
			add(r, a, job, c.attemptClass(a))
		}
		if !r.complete {
			ciAdd(t, r, nil, nil, entity.RoleMissing, nil, entity.ReasonHistoryIncomplete)
		}
	}
	return t.ratio(numerator, denominator, int(denominator), true)
}

func (c *ciSource) logical(metric string, p period) entity.Point {
	t := ciTally(p)
	var numerator, denominator float64
	for i := range c.runs {
		r := &c.runs[i]
		if !r.in(p) {
			continue
		}
		if !r.complete {
			ciAdd(t, r, nil, nil, entity.RoleMissing, nil, entity.ReasonHistoryIncomplete)
		}
		for j := range r.jobs {
			job := &r.jobs[j]
			first := &job.attempts[0]
			if !job.complete {
				ciAdd(t, r, first, job, entity.RoleMissing, nil, entity.ReasonHistoryIncomplete)
				continue
			}
			firstClass := c.attemptClass(first)
			if !ciTerminal(firstClass) {
				role := entity.RoleExcluded
				if firstClass == "unknown" {
					role = entity.RoleMissing
				}
				ciAdd(t, r, first, job, role, nil, "first_outcome_unknown")
				continue
			}
			pass := firstClass == "success"
			if metric == "I03" {
				pass = false
				failed := false
				known := true
				for k := range job.attempts {
					a := &job.attempts[k]
					cl := c.attemptClass(a)
					known = known && ciTerminal(cl)
					pass = pass || failed && cl == "success"
					failed = failed || ciFailure(cl)
				}
				if !known {
					ciAdd(t, r, first, job, entity.RoleExcluded, nil, "outcomes_not_comparable")
					continue
				}
			}
			denominator++
			ciAdd(t, r, first, job, entity.RoleDenominator, num(1), "")
			if pass {
				numerator++
				ciAdd(t, r, first, job, entity.RoleNumerator, num(1), "")
			}
		}
	}
	return t.ratio(numerator, denominator, int(denominator), true)
}

func (c *ciSource) overhead(est string, p period) entity.Point {
	t := ciTally(p)
	var sum float64
	n := 0
	for i := range c.runs {
		r := &c.runs[i]
		if !r.in(p) {
			continue
		}
		if !r.complete {
			ciAdd(t, r, nil, nil, entity.RoleMissing, nil, entity.ReasonHistoryIncomplete)
		}
		for _, j := range r.jobs {
			for k := range j.attempts {
				a := &j.attempts[k]
				if est != "total_seconds" && a.AttemptNo <= 1 {
					continue
				}
				v := 1.0
				if est != "attempts" {
					if !c.durationKnown(a) {
						ciAdd(t, r, a, nil, entity.RoleMissing, nil, "duration_missing")
						continue
					}
					v = float64(*a.DurationS)
				}
				sum += v
				n++
				ciAdd(t, r, a, nil, entity.RoleNumerator, num(v), "")
			}
		}
	}
	pt := t.point()
	pt.N, pt.LowSample = &n, n < _lowSample
	pt.State, pt.Value = entity.StateAssessed, num(sum)
	if t.missing > 0 {
		pt.State, pt.Value, pt.ReasonCode = entity.StateBounded, nil, t.reason
		pt.Bound = &entity.Bound{Kind: entity.BoundLower, Value: sum}
	}
	return pt
}

func (c *ciSource) allowed(p period) entity.Point {
	t := ciTally(p)
	var numerator, denominator float64
	for i := range c.runs {
		r := &c.runs[i]
		if !r.in(p) || c.runClass(r) != "success" {
			continue
		}
		if r.provider != entity.ProviderGitLab {
			ciAdd(t, r, nil, nil, entity.RoleExcluded, nil, "allow_failure_unavailable")
			continue
		}
		if !r.complete {
			ciAdd(t, r, nil, nil, entity.RoleMissing, nil, entity.ReasonHistoryIncomplete)
			continue
		}
		allowed, known := false, true
		for _, j := range r.jobs {
			a := &j.attempts[len(j.attempts)-1]
			cl := c.attemptClass(a)
			known = known && cl != "unknown"
			allowed = allowed || a.AllowFailure && ciFailure(cl)
		}
		if !known {
			ciAdd(t, r, nil, nil, entity.RoleMissing, nil, "final_outcome_unknown")
			continue
		}
		denominator++
		ciAdd(t, r, nil, nil, entity.RoleDenominator, num(1), "")
		if allowed {
			numerator++
			ciAdd(t, r, nil, nil, entity.RoleNumerator, num(1), "allow_failure")
		}
	}
	return t.ratio(numerator, denominator, int(denominator), true)
}

func (c *ciSource) failures(est string, p period) entity.Point {
	t := ciTally(p)
	n := 0
	for i := range c.runs {
		r := &c.runs[i]
		if !r.in(p) {
			continue
		}
		if !r.complete {
			ciAdd(t, r, nil, nil, entity.RoleMissing, nil, entity.ReasonHistoryIncomplete)
		}
		for _, j := range r.jobs {
			for k := range j.attempts {
				a := &j.attempts[k]
				if !ciFailure(c.attemptClass(a)) || ciFailureGroup(a.FailureKind) != est {
					continue
				}
				n++
				ciAdd(t, r, a, nil, entity.RoleNumerator, num(1), "source_failure_reason")
			}
		}
	}
	pt := t.point()
	pt.State, pt.Value, pt.N, pt.LowSample = entity.StateAssessed, num(float64(n)), &n, n < _lowSample
	if t.missing > 0 {
		pt.State, pt.Value, pt.ReasonCode = entity.StateBounded, nil, t.reason
		pt.Bound = &entity.Bound{Kind: entity.BoundLower, Value: float64(n)}
	}
	return pt
}
