package calc

import (
	"encoding/json"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_reasonNoTaskMR        = "no_merge_request"
	_reasonPipelineUnknown = "pipeline_unknown"
	_reasonReportProfile   = "expected_report_profile_unavailable"
)

// crossEvidence stores the facts needed to recalculate a task's one contribution after repository checks.
type crossEvidence struct {
	UnknownMembership bool  `json:"unknown_membership,omitempty"`
	OverTarget        *bool `json:"over_target,omitempty"`
	UnknownLink       bool  `json:"unknown_link,omitempty"`

	Age *int64    `json:"task_age_seconds,omitempty"`
	MRs []crossMR `json:"merge_requests,omitempty"`
}

type crossMR struct {
	AllowFailureSupported bool                   `json:"allow_failure_supported,omitempty"`
	Tasks                 []entity.ID            `json:"tasks,omitempty"`
	Technical             *entity.TechnicalCross `json:"technical,omitempty"`
	PipelineReports       *entity.TechnicalCross `json:"pipeline_reports,omitempty"`
	FailToPass            *bool                  `json:"fail_to_pass,omitempty"`
	AllowedFailure        *bool                  `json:"allowed_failure,omitempty"`

	ID            entity.ID              `json:"id"`
	ReviewSignal  entity.ObservationKind `json:"review_signal,omitempty"`
	ReviewerKnown bool                   `json:"reviewer_known,omitempty"`
	ReviewReady   bool                   `json:"review_ready,omitempty"`
	ResponseKnown bool                   `json:"response_known,omitempty"`
	WaitSeconds   int64                  `json:"wait_seconds,omitempty"`
	PipelineID    *entity.ID             `json:"pipeline_id,omitempty"`
	CIClass       string                 `json:"ci_class,omitempty"`
	Retry         bool                   `json:"retry,omitempty"`
	RetryKnown    bool                   `json:"retry_known,omitempty"`
}

type crossTask struct {
	contribution entity.Contribution
	basis        crossEvidence
}

func (x *reportSignals) crossPoints(pr *prepared, p period) []entity.SeriesPoint {
	var tasks []crossTask
	if pr.in.mapping != nil {
		facts := x.observationsAt(p.end)
		for _, tl := range pr.tls {
			ep := tl.episodeAt(p.end)
			if !tl.knownAt(p.end) || ep == nil {
				continue
			}
			b := crossEvidence{}
			if ep.startKnown {
				age := seconds(ep.start, p.end)
				b.Age = &age
			}
			for _, id := range x.taskMRs[tl.slice.TaskID] {
				if m, exists := facts[id]; exists {
					b.MRs = append(b.MRs, m)
				}
			}
			// All evidence fields have a total JSON representation.
			raw, _ := json.Marshal(b)
			tasks = append(tasks, crossTask{
				basis:        b,
				contribution: entity.Contribution{ObjectID: tl.slice.TaskID, ItemNo: 1, ObjectKind: entity.ObjectTask, Evidence: raw},
			})
		}
	}
	var points []entity.SeriesPoint
	for i := range _crossDefinitions {
		def := &_crossDefinitions[i]
		for _, estimator := range def.Estimators {
			pt := entity.Point{Start: p.start, End: p.end, State: entity.StateNotEvaluated}
			switch {
			case pr.in.mapping == nil:
				pt.ReasonCode = entity.ReasonMappingMissing
			case estimator == _ciMissingReport && x.technical == nil:
				pt.ReasonCode = _reasonReportProfile
			case estimator == _ciMissingReport:
				continue // The technical evidence supplies this X02 slice.
			default:
				cs := make([]entity.Contribution, len(tasks))
				for i := range tasks {
					cs[i] = tasks[i].contribution
					crossScore(&cs[i], estimator, &tasks[i].basis)
				}
				pt = crossRatio(p, cs)
			}
			points = append(points, entity.SeriesPoint{MetricID: def.ID, Estimator: estimator, Unit: def.Unit, Point: pt})
		}
	}
	return points
}

func crossScore(c *entity.Contribution, est string, b *crossEvidence) {
	c.Value = nil
	if len(b.MRs) == 0 {
		c.Role, c.ReasonCode = entity.RoleExcluded, _reasonNoTaskMR
		return
	}
	var known, unknown, hit bool
	var wait int64
	for i := range b.MRs {
		m := &b.MRs[i]
		valid, missing, matched := m.signal(est)
		known, unknown, hit = known || valid, unknown || missing, hit || matched
		if matched {
			wait = max(wait, m.WaitSeconds)
		}
	}
	switch {
	case hit:
		c.Role, c.ReasonCode = entity.RoleNumerator, est
		if est == _reviewNoReviewer || est == _reviewNoResponse {
			c.Value = num(float64(wait))
		}
	case known:
		c.Role, c.ReasonCode = entity.RoleDenominator, ""
	case unknown:
		c.Role, c.ReasonCode = entity.RoleMissing, _reasonPipelineUnknown
		if est == _reviewNoReviewer || est == _reviewNoResponse {
			c.ReasonCode = entity.ReasonReviewerUnknown
		}
	default:
		c.Role, c.ReasonCode = entity.RoleExcluded, "not_applicable"
	}
}

func (m *crossMR) signal(est string) (valid, missing, match bool) {
	switch est {
	case _reviewNoReviewer:
		return m.ReviewerKnown, !m.ReviewerKnown, m.ReviewSignal == entity.ObservationAwaitingReviewer
	case _reviewNoResponse:
		wait := m.ReviewSignal == entity.ObservationAwaitingReviewer || m.ReviewSignal == entity.ObservationAwaitingFirst
		return m.ResponseKnown, m.ReviewReady && !m.ResponseKnown, wait
	case _ciFailed:
		failed := m.CIClass == "failure" || m.CIClass == "timeout"
		return failed || m.CIClass == "success", m.CIClass == "" || m.CIClass == "unknown", failed
	case _ciRetried:
		return m.RetryKnown || m.Retry, m.CIClass == "success" && !m.RetryKnown || m.CIClass == "" || m.CIClass == "unknown", m.Retry
	}
	return false, false, false
}

func crossRatio(p period, cs []entity.Contribution) entity.Point {
	t := newTally(p)
	var n, denominator int
	for i := range cs {
		c := &cs[i]
		t.add(c.ObjectID, c.ItemNo, c.Role, c.Value, c.ReasonCode)
		if c.Role == entity.RoleNumerator {
			n++
		}
		if c.Role == entity.RoleNumerator || c.Role == entity.RoleDenominator {
			denominator++
		}
	}
	pt := t.ratio(float64(n), float64(denominator), len(cs), true)
	pt.Contributions = cs
	pt.LowSample = denominator < _lowSample
	return pt
}

func (x *reportSignals) observationsAt(at time.Time) map[entity.ID]crossMR {
	mrs, _, _ := x.crossObservations(at, false)
	return mrs
}

func (x *reportSignals) crossObservations(at time.Time, allRuns bool) (mrs map[entity.ID]crossMR, technicalHeads []entity.TechnicalHead, pipelines map[entity.ID][]crossMR) {
	facts := x.facts
	facts.AsOf = at
	ci := observationCI(&facts, x.runs)
	heads := observationRevisions(&facts, x.linked)
	mrs = make(map[entity.ID]crossMR, len(x.linked))
	if allRuns {
		pipelines = map[entity.ID][]crossMR{}
	}
	for i := range x.review.mrs {
		m := &x.review.mrs[i]
		v := &m.version
		if !x.linked[v.MRID] || v.OpenedAt.After(at) {
			continue
		}
		b := crossReview(m, at)
		head := entity.TechnicalHead{MRVersion: *v}
		head.HeadSHA = observationHead(v, heads[v.MRID], at)
		if run := ci.forMR(v, heads[v.MRID]); run != nil {
			b.setPipeline(&facts, &ci, run)
			head.PipelineID, head.PipelineStarted = run.version.RunID, run.version.StartedAt
		}
		mrs[v.MRID] = b
		if head.HeadSHA == "" {
			continue
		}
		if allRuns {
			for _, i := range ci.byMR[v.MRID] {
				run := &ci.source.runs[i]
				if run.version.ParentRunID != nil || run.version.AreaID != v.AreaID || run.version.SHA != head.HeadSHA {
					continue
				}
				b := crossMR{ID: v.MRID}
				b.setPipeline(&facts, &ci, run)
				pipelines[v.MRID] = append(pipelines[v.MRID], b)
				if b.CIClass == "success" {
					head.SuccessfulPipelines = append(head.SuccessfulPipelines, entity.TechnicalPipeline{
						ID: run.version.RunID, ProfileAt: runOrderTime(&run.version),
					})
				}
			}
		}
		technicalHeads = append(technicalHeads, head)
	}
	return mrs, technicalHeads, pipelines
}

func (m *crossMR) setPipeline(facts *entity.TaskObservationFacts, ci *taskCI, run *ciRun) {
	m.PipelineID = &run.version.RunID
	m.CIClass = taskPipelineClass(ci, run)
	if m.CIClass != "success" {
		return
	}
	o, ok := taskPipelineObservation(facts, ci, run, entity.ObjectRef{})
	m.Retry = ok && o.Kind == entity.ObservationCIRetried
	m.RetryKnown = run.complete
	flipped, known := false, run.complete
	for i := range run.jobs {
		hit, knownRetry := ci.source.retryFlip(&run.jobs[i])
		flipped = flipped || hit
		known = known && knownRetry
	}
	if flipped || known {
		m.FailToPass = new(flipped)
	}
	m.AllowFailureSupported = run.version.AllowFailureSemantics
	if !run.complete || !run.version.AllowFailureSemantics {
		return
	}
	known, allowed := true, false
	for _, job := range run.jobs {
		last := &job.attempts[len(job.attempts)-1]
		class := ci.source.attemptClass(last)
		known = known && class != "unknown"
		allowed = allowed || isAllowedFailure(last, class)
	}
	if known || allowed {
		m.AllowedFailure = new(allowed)
	}
}

func crossReview(m *reviewMR, at time.Time) crossMR {
	// Review history is ordered by event time, so a slice preserves the caller's snapshot.
	mr := *m
	for i := range mr.events {
		if mr.events[i].EventTime.After(at) {
			mr.events = mr.events[:i]
			break
		}
	}
	b := crossMR{ID: mr.version.MRID, ReviewerKnown: taskReviewerKnown(&mr, at)}
	b.ReviewReady = taskReviewReady(&mr, at)
	if !b.ReviewReady {
		return b
	}
	o, ok := taskReviewObservation(&mr, entity.ObjectRef{}, at)
	if ok && (o.Kind == entity.ObservationAwaitingReviewer || o.Kind == entity.ObservationAwaitingFirst) {
		b.ReviewSignal = o.Kind
		b.WaitSeconds = seconds(*o.Since, at)
		b.ResponseKnown = true
	}
	b.ResponseKnown = b.ResponseKnown || mr.wait(at).Kind == entity.ReviewResponded
	return b
}
