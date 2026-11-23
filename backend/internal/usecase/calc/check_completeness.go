package calc

import (
	"encoding/json"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_objectExpectedCheck  entity.ObjectKind = "expected_check"
	_objectExpectedReport entity.ObjectKind = "expected_report"
)

// ExpectedCheck is one logical check from the configured graph, with its final visible outcome if available.
type ExpectedCheck struct {
	Key     string
	JobID   entity.ID
	Outcome entity.CIOutcome
}

// CheckCompletenessInput carries the expectations and the reports in the selected pipeline snapshot.
type CheckCompletenessInput struct {
	AreaID   entity.ID
	HeadSHA  string
	AsOf     time.Time
	KnownSeq int64
	Checks   []ExpectedCheck
	Expected []entity.ExpectedReport
	Received []FindingScan
}

// CalculateCheckCompleteness evaluates I08 and I09 without treating absence of expectations as complete coverage.
func (*UseCase) CalculateCheckCompleteness(in CheckCompletenessInput) []entity.SeriesPoint {
	return []entity.SeriesPoint{missingChecks(&in), missingReports(&in)}
}

func missingChecks(in *CheckCompletenessInput) entity.SeriesPoint {
	p := completenessPoint("I08", in.AsOf, "expected_checks_missing")
	checks := make([]ExpectedCheck, 0, len(in.Checks))
	indices := make(map[string]int, len(in.Checks))
	for _, check := range in.Checks {
		if i, ok := indices[check.Key]; ok {
			if !hasOutcome(checks[i].Outcome) && hasOutcome(check.Outcome) {
				checks[i] = check
			}
			continue
		}
		indices[check.Key] = len(checks)
		checks = append(checks, check)
	}
	n := 0
	items := make(map[entity.ID]int)
	for _, check := range checks {
		known := hasOutcome(check.Outcome)
		c := entity.Contribution{ObjectID: check.JobID, ObjectKind: entity.ObjectCIJob, Role: entity.RoleDenominator, Value: num(1)}
		if c.ObjectID.IsZero() {
			c.ObjectID, c.ObjectKind = in.AreaID, _objectExpectedCheck
		}
		if !known {
			n++
			c.Role, c.ReasonCode = entity.RoleNumerator, "check_outcome_missing"
		}
		c.Evidence, _ = json.Marshal(struct {
			Key     string           `json:"key"`
			Outcome entity.CIOutcome `json:"outcome,omitempty"`
		}{check.Key, check.Outcome})
		appendExpected(&p.Point, c, items)
	}
	finishCompleteness(&p.Point, n, len(checks))
	return p
}

func hasOutcome(outcome entity.CIOutcome) bool {
	switch outcome {
	case entity.CISuccess, entity.CIFailed, entity.CICanceled, entity.CISkipped:
		return true
	default:
		return false
	}
}

func missingReports(in *CheckCompletenessInput) entity.SeriesPoint {
	p := completenessPoint("I09", in.AsOf, "expected_reports_missing")
	wanted := make(map[entity.ExpectedReport]bool, len(in.Expected))
	kindCounts := make(map[string]int)
	for _, w := range in.Expected {
		if !wanted[w] {
			kindCounts[w.Kind]++
			wanted[w] = true
		}
	}
	received := make(map[string][]*FindingScan)
	for i := range in.Received {
		r := &in.Received[i]
		if in.HeadSHA != "" && r.SHA == in.HeadSHA && r.KnownSeq <= in.KnownSeq {
			received[r.Kind] = append(received[r.Kind], r)
		}
	}
	n := 0
	items := make(map[entity.ID]int)
	for _, w := range in.Expected {
		if !wanted[w] {
			continue
		}
		delete(wanted, w)
		c := expectedReportContribution(in, w, kindCounts[w.Kind] == 1, received[w.Kind])
		if c.Role == entity.RoleNumerator {
			n++
		}
		appendExpected(&p.Point, c, items)
	}
	finishCompleteness(&p.Point, n, len(p.Contributions))
	return p
}

func expectedReportContribution(in *CheckCompletenessInput, w entity.ExpectedReport, single bool, reports []*FindingScan) entity.Contribution {
	c := entity.Contribution{ObjectID: w.ComponentID, ObjectKind: _objectExpectedReport, Value: num(1)}
	if c.ObjectID.IsZero() {
		c.ObjectID = in.AreaID
	}
	current := metricState{entity.StateNotEvaluated, _reasonReportMissing}
	var actual *entity.ID
	for _, r := range reports {
		if !sameReportScope(w, r, single) {
			continue
		}
		state := scanState(r, in.HeadSHA, in.KnownSeq)
		if actual == nil || current.state == entity.StateUnknown || state.state == entity.StateAssessed {
			actual, current = &r.ID, state
		}
		if state.state == entity.StateAssessed {
			break
		}
	}
	c.Role, c.ReasonCode = entity.RoleNumerator, current.reason
	if current.state == entity.StateAssessed {
		c.Role = entity.RoleDenominator
	}
	c.Evidence, _ = json.Marshal(struct {
		Kind        string     `json:"kind"`
		ComponentID entity.ID  `json:"component_id,omitzero"`
		Language    string     `json:"language,omitempty"`
		ReportID    *entity.ID `json:"report_id,omitempty"`
	}{w.Kind, w.ComponentID, w.Language, actual})
	return c
}

func sameReportScope(w entity.ExpectedReport, r *FindingScan, single bool) bool {
	component := r.ComponentID != nil && *r.ComponentID == w.ComponentID ||
		r.ComponentID == nil && (w.ComponentID.IsZero() || single)
	return component && (w.Language == "" || r.Language == w.Language || r.Language == "" && single)
}

func appendExpected(p *entity.Point, c entity.Contribution, items map[entity.ID]int) {
	items[c.ObjectID]++
	c.ItemNo = items[c.ObjectID]
	p.Contributions = append(p.Contributions, c)
}

func completenessPoint(id string, at time.Time, missing string) entity.SeriesPoint {
	return entity.SeriesPoint{
		MetricID: id, Unit: UnitRatio,
		Point: entity.Point{End: at, State: entity.StateNotApplicable, ReasonCode: missing},
	}
}

func finishCompleteness(p *entity.Point, n, total int) {
	if total == 0 {
		return
	}
	p.State, p.ReasonCode = entity.StateAssessed, ""
	p.Value, p.Numerator, p.Denominator = num(float64(n)/float64(total)), num(float64(n)), num(float64(total))
	p.N = &total
}
