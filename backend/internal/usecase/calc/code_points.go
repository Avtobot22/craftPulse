package calc

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_reasonNoCodeReport  = "report_missing"
	_reasonPartialCode   = "report_partial"
	_reasonCodeUnknown   = "report_completeness_unknown"
	_reasonThreshold     = "profile_threshold_missing"
	_reasonFileInventory = "file_inventory_missing"
	_reasonNoFunctions   = "no_analyzed_functions"
	_reasonNoLines       = "no_analyzed_lines"
	_reasonCloneScope    = "clone_scope_unverified"
	_reasonInvalidLines  = "clone_lines_exceed_scope"
)

func (s *codeSource) points(p period) []entity.SeriesPoint {
	defs := codeDefinitions(s.format)
	var out []entity.SeriesPoint
	for i := range defs {
		def := &defs[i]
		estimators := def.Estimators
		if len(estimators) == 0 {
			estimators = []string{""}
		}
		for _, est := range estimators {
			point := entity.Point{Start: p.start, End: p.end}
			s.measure(&point, def.ID, est)
			out = append(out, entity.SeriesPoint{MetricID: def.ID, Estimator: est, Unit: def.Unit, Point: point})
		}
	}
	return out
}

func (s *codeSource) measure(p *entity.Point, id, estimator string) {
	if s.report == nil {
		p.State, p.ReasonCode = entity.StateNotEvaluated, _reasonNoCodeReport
		return
	}
	p.State = entity.StateAssessed
	switch id {
	case "C01":
		s.volume(p, estimator == _estTest)
	case "C04":
		s.functions(p, estimator, true)
	case "C05":
		s.functions(p, _estAboveThreshold, true)
	case "C06":
		s.functions(p, estimator, false)
	case "C08", "C09":
		s.clones(p, id == "C08")
	}
	s.completeness(p)
}

func (s *codeSource) volume(p *entity.Point, test bool) {
	if s.report.Facts.Files == nil {
		p.State, p.ReasonCode = entity.StateNotEvaluated, _reasonFileInventory
		return
	}
	var total int64
	for i, file := range *s.report.Facts.Files {
		if file.Test != test {
			continue
		}
		total += int64(file.NLOC)
		p.Contributions = append(p.Contributions, s.contribution(entity.ObjectCodeFile, i, entity.RoleNumerator, num(float64(file.NLOC)), "", file))
	}
	p.Value = num(float64(total))
}

func (s *codeSource) functions(p *entity.Point, estimator string, cc bool) {
	facts := &s.report.Facts
	above := estimator == _estAboveThreshold
	var threshold int
	if above {
		if facts.Thresholds == nil {
			p.State, p.ReasonCode = entity.StateNotEvaluated, _reasonThreshold
			return
		}
		threshold = facts.Thresholds.NLOC
		if cc {
			threshold = facts.Thresholds.CCN
		}
	}
	values := make([]float64, 0, len(facts.Functions))
	var count float64
	for i, fn := range facts.Functions {
		value := fn.NLOC
		if cc {
			value = fn.CCN
		}
		role := entity.RoleNumerator
		if above {
			if value > threshold {
				count++
			} else {
				role = entity.RoleExcluded
				if cc {
					role = entity.RoleDenominator
				}
			}
		}
		values = append(values, float64(value))
		p.Contributions = append(p.Contributions, s.contribution(entity.ObjectCodeFunction, i, role, num(float64(value)), "", fn))
	}
	p.N = new(len(values))
	switch {
	case above && cc:
		s.codeRatio(p, count, float64(len(values)), _reasonNoFunctions)
	case above:
		p.Value = num(count)
		p.ExcludedCount = len(values) - int(count)
	case len(values) == 0:
		p.State, p.ReasonCode = entity.StateNotApplicable, _reasonNoFunctions
	default:
		slices.Sort(values)
		pct := 100
		switch estimator {
		case EstimatorP50:
			pct = 50
		case _estP90:
			pct = 90
		}
		p.Value = num(percentile(values, pct))
	}
}

func (s *codeSource) clones(p *entity.Point, ratio bool) {
	facts := &s.report.Facts
	var total int64
	var values []int64
	if ratio {
		total, values = cloneLines(facts.Groups)
	}
	for i, group := range facts.Groups {
		value := 1.0
		if ratio {
			value = float64(values[i])
		}
		p.Contributions = append(p.Contributions, s.contribution(entity.ObjectCloneGroup, i, entity.RoleNumerator, num(value), "", group))
	}
	p.N = new(len(facts.Groups))
	if !ratio {
		p.Value = num(float64(len(facts.Groups)))
		return
	}
	if facts.PhysicalLines == nil {
		p.State, p.ReasonCode = entity.StateNotEvaluated, _reasonCloneScope
		return
	}
	lines := *facts.PhysicalLines
	p.Contributions = append(p.Contributions, s.contribution(entity.ObjectCodeReport, 0, entity.RoleDenominator, num(float64(lines)), "", nil))
	if total > lines {
		p.State, p.ReasonCode = entity.StateNotEvaluated, _reasonInvalidLines
		return
	}
	s.codeRatio(p, float64(total), float64(lines), _reasonNoLines)
}

func (*codeSource) codeRatio(p *entity.Point, n, d float64, reason string) {
	p.Numerator, p.Denominator = num(n), num(d)
	if d == 0 {
		p.State, p.ReasonCode = entity.StateNotApplicable, reason
		return
	}
	p.Value = num(n / d)
}

func (s *codeSource) completeness(p *entity.Point) {
	if s.report.Completeness == "complete" {
		return
	}
	reason := _reasonCodeUnknown
	if s.report.Completeness == "partial" {
		reason = _reasonPartialCode
	}
	if p.Value != nil {
		p.State, p.ReasonCode = entity.StateProvisional, reason
	}
	p.MissingCount = max(s.report.SkippedCount, len(s.report.Skipped))
	if p.MissingCount > 0 || reason == _reasonPartialCode {
		p.Contributions = append(p.Contributions, s.contribution(entity.ObjectCodeReport, 1, entity.RoleMissing, nil, reason, struct {
			Count int                    `json:"skipped_count"`
			Paths []entity.SkippedTarget `json:"skipped_targets"`
		}{Count: p.MissingCount, Paths: s.report.Skipped}))
	}
}

func (s *codeSource) contribution(kind entity.ObjectKind, ordinal int, role entity.ContributionRole, value *float64, reason string, evidence any) entity.Contribution {
	d := newDigest()
	d.id(s.report.ID)
	d.text(string(kind))
	d.int(int64(ordinal))
	var raw []byte
	if evidence != nil {
		raw, _ = json.Marshal(evidence) // callers supply only typed code facts
	}
	return entity.Contribution{
		ObjectID: entity.ID(d.sum()[:16]), ObjectKind: kind, ItemNo: 1, Role: role, Value: value, ReasonCode: reason, Evidence: raw,
	}
}

// cloneLines counts the union across all clone groups. Each line is credited to the first range
// covering it in file order, so the contributions add up even with overlapping groups.
func cloneLines(groups []entity.CloneGroup) (total int64, counts []int64) {
	type member struct {
		entity.CodeRange
		group int
	}
	var ranges []member
	for i, group := range groups {
		for _, r := range group.Members {
			ranges = append(ranges, member{r, i})
		}
	}
	slices.SortFunc(ranges, func(a, b member) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Start, b.Start), cmp.Compare(a.End, b.End), cmp.Compare(a.group, b.group))
	})
	var file string
	var end int
	counts = make([]int64, len(groups))
	for _, r := range ranges {
		if file != r.Path {
			file, end = r.Path, 0
		}
		n := int64(max(0, r.End-max(r.Start, end+1)+1))
		counts[r.group] += n
		total += n
		end = max(end, r.End)
	}
	return total, counts
}
