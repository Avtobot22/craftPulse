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
	Age *int64    `json:"task_age_seconds,omitempty"`
	MRs []crossMR `json:"merge_requests,omitempty"`
}

type crossMR struct {
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
			case estimator == _ciMissingReport:
				pt.ReasonCode = _reasonReportProfile
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
	facts := x.facts
	facts.AsOf = at
	ci := observationCI(&facts, x.runs)
	heads := observationRevisions(&facts, x.linked)
	out := make(map[entity.ID]crossMR, len(x.linked))
	for i := range x.review.mrs {
		m := &x.review.mrs[i]
		v := &m.version
		if !x.linked[v.MRID] || v.OpenedAt.After(at) {
			continue
		}
		b := crossReview(m, at)
		if run := ci.forMR(v, heads[v.MRID]); run != nil {
			b.PipelineID = &run.version.RunID
			b.CIClass = taskPipelineClass(&ci, run)
			if b.CIClass == "success" {
				o, ok := taskPipelineObservation(&facts, &ci, run, entity.ObjectRef{})
				b.Retry = ok && o.Kind == entity.ObservationCIRetried
				b.RetryKnown = run.complete
			}
		}
		out[v.MRID] = b
	}
	return out
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
