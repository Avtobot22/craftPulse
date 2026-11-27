package calc

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// CISeries finds or creates the weekly or monthly CI series of a team or repository.
func (u *UseCase) CISeries(ctx context.Context, orgID entity.ID, scope entity.ReviewScope, step entity.Step) (entity.CalcSeries, error) {
	if step != entity.StepWeek && step != entity.StepMonth {
		return entity.CalcSeries{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/step", Code: entity.FieldNotAllowedValue}}}
	}
	return u.scopedSeries(ctx, orgID, scope, entity.MetricSetCI, step)
}

type ciSource struct {
	window     entity.Window
	asOf       time.Time
	location   *time.Location
	series     *entity.CalcSeries
	runs       []ciRun
	areas      []entity.ID
	collection []entity.AreaCollection
}

type ciRun struct {
	version  entity.CIRunVersion
	provider entity.Provider
	jobs     []ciJob
	complete bool
}

type ciJob struct {
	id       entity.ID
	attempts []entity.CIAttemptVersion
	complete bool
}

func (u *UseCase) ciSource(ctx context.Context, job entity.CalcSliceJob, rev *entity.CalcRevision, series *entity.CalcSeries,
	areas []entity.ID, loc *time.Location,
) (*ciSource, error) {
	src := &ciSource{window: rev.Window, asOf: rev.AsOf, location: loc, series: series}
	src.window.Step = series.Step
	if job.Sources != nil {
		src.collection = *job.Sources
	}
	for _, area := range areas {
		facts, err := u.d.Facts.CIFacts(ctx, job.OrganizationID, area, rev.KnowledgeSeq)
		if err != nil {
			return nil, err
		}
		if facts.Provider == entity.ProviderJiraCloud || facts.Provider == entity.ProviderConfluenceCloud || facts.Provider == entity.ProviderConfluenceDC || facts.Provider == entity.ProviderGit || facts.Provider == entity.ProviderSonarQube {
			continue
		}
		facts.Attempts = slices.DeleteFunc(facts.Attempts, func(a entity.CIAttemptVersion) bool {
			return a.StartedAt != nil && !a.StartedAt.Before(rev.AsOf)
		})
		src.areas = append(src.areas, area)
		src.runs = append(src.runs, ciRunsOf(facts)...)
	}
	slices.SortFunc(src.runs, func(a, b ciRun) int { return bytes.Compare(a.version.RunID[:], b.version.RunID[:]) })
	return src, nil
}

func ciRunsOf(f entity.CIFacts) []ciRun {
	byRun := make(map[entity.ID][]entity.CIAttemptVersion)
	for i := range f.Attempts {
		a := &f.Attempts[i]
		byRun[a.RunID] = append(byRun[a.RunID], *a)
	}
	out := make([]ciRun, 0, len(f.Runs))
	for i := range f.Runs {
		v := &f.Runs[i]
		r := ciRun{version: *v, provider: f.Provider, complete: v.AttemptHistory == entity.AttemptHistoryComplete}
		jobs := make(map[entity.ID][]entity.CIAttemptVersion)
		runAttempts := byRun[v.RunID]
		for k := range runAttempts {
			a := &runAttempts[k]
			d := newDigest()
			d.text("ci_job/1")
			d.id(v.RunID)
			d.text(a.JobKey)
			d.bytes(a.ConfigFingerprint)
			id := entity.ID(d.sum()[:16])
			jobs[id] = append(jobs[id], *a)
		}
		for id, attempts := range jobs {
			slices.SortFunc(attempts, func(a, b entity.CIAttemptVersion) int {
				if a.AttemptNo != b.AttemptNo {
					return a.AttemptNo - b.AttemptNo
				}
				return bytes.Compare(a.AttemptID[:], b.AttemptID[:])
			})
			complete := r.version.AttemptHistory == entity.AttemptHistoryComplete
			for i := range attempts {
				complete = complete && attempts[i].AttemptNo == i+1
			}
			r.jobs = append(r.jobs, ciJob{id: id, attempts: attempts, complete: complete})
			r.complete = r.complete && complete
		}
		slices.SortFunc(r.jobs, func(a, b ciJob) int { return bytes.Compare(a.id[:], b.id[:]) })
		out = append(out, r)
	}
	return out
}

func (c *ciSource) periods() []period { return periods(c.window, c.asOf, c.location) }

func (r *ciRun) in(p period) bool {
	return r.version.FinishedAt != nil && p.has(*r.version.FinishedAt)
}

func (c *ciSource) hash(from, to time.Time) []byte {
	d := newDigest()
	d.text(entity.MetricSetCI)
	d.bytes(c.series.ScopeHash)
	d.text(string(c.series.Step))
	d.text(c.location.String())
	d.time(from)
	d.time(to)
	for i := range _ciDefinitions {
		d.text(_ciDefinitions[i].ID)
		d.int(int64(_ciDefinitions[i].Version))
	}
	for _, est := range c.estimators(definition("I07")) {
		d.text(est)
	}
	for i := range c.runs {
		r := &c.runs[i]
		if !r.in(period{start: from, end: to}) && !r.missingCompletion() {
			continue
		}
		d.id(r.version.ID)
		d.id(r.version.RunID)
		d.text(string(r.provider))
		d.text(r.version.NativeStatus)
		d.text(string(r.version.Outcome))
		d.text(c.runClass(r))
		d.optTime(r.version.StartedAt)
		d.optTime(r.version.FinishedAt)
		d.flag(r.complete)
		for _, j := range r.jobs {
			d.id(j.id)
			for k := range j.attempts {
				a := &j.attempts[k]
				d.id(a.ID)
				d.id(a.AttemptID)
				d.int(a.KnownSeq)
				d.int(int64(a.AttemptNo))
				d.text(a.NativeStatus)
				d.text(string(a.Outcome))
				d.text(c.attemptClass(a))
				d.flag(c.durationKnown(a))
				d.flag(a.AllowFailure)
				d.text(a.FailureKind)
				d.flag(a.DurationS != nil)
				if a.DurationS != nil {
					d.int(*a.DurationS)
				}
			}
		}
	}
	for _, p := range c.periods() {
		if p.start.Before(from) || !p.start.Before(to) {
			continue
		}
		d.flag(c.covered(p))
	}
	for i := range c.collection {
		a := &c.collection[i]
		d.id(a.AreaID)
		d.id(a.ConnectionID)
		d.text(a.Completeness)
		d.text(string(a.Phase))
		d.optTime(a.Watermark)
		d.optTime(a.HistoryFrom)
		if a.Reconcile != nil {
			d.id(a.Reconcile.ID)
			d.text(a.Reconcile.Outcome)
			d.text(a.Reconcile.ReasonCode)
		}
	}
	return d.sum()
}

func (c *ciSource) stored(ps []period, hash []byte, from, to time.Time) *entity.StoredPart {
	out := &entity.StoredPart{InputHash: hash, MetricSet: entity.MetricSetCI, From: from, To: to}
	for i := range _ciDefinitions {
		def := &_ciDefinitions[i]
		estimators := c.estimators(*def)
		for _, est := range estimators {
			for _, p := range ps {
				out.Points = append(out.Points, entity.SeriesPoint{MetricID: def.ID, Estimator: est, Unit: ciUnit(*def, est), Point: c.point(def.ID, est, p)})
			}
		}
	}
	return out
}

func (c *ciSource) estimators(def Definition) []string {
	if def.ID != "I07" {
		if len(def.Estimators) == 0 {
			return []string{""}
		}
		return def.Estimators
	}
	seen := map[string]bool{"unknown": true}
	for i := range c.runs {
		for _, j := range c.runs[i].jobs {
			for k := range j.attempts {
				a := &j.attempts[k]
				if ciFailure(c.attemptClass(a)) {
					seen[ciFailureGroup(a.FailureKind)] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func ciUnit(def Definition, est string) string {
	if def.ID == "I04" && est == "attempts" || def.ID == "I10" && est == "loaded" {
		return UnitCount
	}
	return def.Unit
}

func ciFailureGroup(reason string) string {
	if reason == "" {
		return "unknown"
	}
	if len(reason) <= 32 {
		return reason
	}
	d := newDigest()
	d.text(reason)
	return fmt.Sprintf("reason:%x", d.sum()[:12])
}

func ciEvidence(r *ciRun, a *entity.CIAttemptVersion) []byte {
	v := map[string]any{"run_id": r.version.RunID, "native_status": r.version.NativeStatus, "attempt_history": r.version.AttemptHistory}
	if a != nil {
		v["native_status"], v["attempt_id"], v["attempt_no"] = a.NativeStatus, a.AttemptID, a.AttemptNo
		v["job_key"], v["config_fingerprint"], v["allow_failure"] = a.JobKey, hex.EncodeToString(a.ConfigFingerprint), a.AllowFailure
		v["failure_kind"] = a.FailureKind
	}
	b, _ := json.Marshal(v)
	return b
}
