package calc

import (
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _connectionMetrics count connections, not tasks: their points do not depend on which tasks a reader sees.
var _connectionMetrics = map[string]bool{"T40": true, "T42": true}

// regather computes a point again from a part of its stored contributions with the rule of its definition,
// as the formula would over the tasks of that part (TZ 12.7). The roles and items are those the formula wrote:
// item 1 is the task itself, item 2 its second contribution.
func regather(sp *entity.SeriesPoint, cs []entity.Contribution) entity.Point {
	p := period{start: sp.Start, end: sp.End}
	if isTechnicalCross(sp.MetricID, sp.Estimator) {
		return technicalPoint(sp.MetricID, p, cs)
	}
	if isCrossMetric(sp.MetricID) {
		return crossRatio(p, cs)
	}
	t := newTally(p)
	var (
		sample           []float64
		numerators, dens int
		numSum, denSum   float64
	)
	for i := range cs {
		c := &cs[i]
		t.add(c.ObjectID, c.ItemNo, c.Role, c.Value, c.ReasonCode)
		switch c.Role {
		case entity.RoleNumerator:
			numerators++
			numSum += valueOf(c.Value)
			if c.ItemNo == 1 && c.Value != nil {
				sample = append(sample, *c.Value)
			}
		case entity.RoleDenominator:
			dens++
			denSum += valueOf(c.Value)
		case entity.RoleExcluded, entity.RoleMissing:
		}
	}
	switch sp.MetricID {
	case "T08", "T14", "A06":
		return pick(t.distribution(sample, definition(sp.MetricID).Estimators), sp)
	case "A07":
		pt := pick(t.distribution(sample, definition(sp.MetricID).Estimators), sp)
		for i := range cs {
			if cs[i].ReasonCode == _eventComment && pt.ReasonCode == "" {
				pt.ReasonCode = entity.ReasonExternalResponse
			}
		}
		return pt
	case "A22":
		return activityCount(t)
	case "T13", "A10":
		return t.ratio(float64(numerators), float64(dens), dens, false)
	case "T15":
		return t.ratio(numSum, denSum, dens, true)
	case "T41":
		if sp.Estimator == CoverageTransition {
			return t.ratio(numSum, denSum, int(denSum), true)
		}
		return t.ratio(float64(numerators), float64(dens), dens, true)
	case "T10", "T23", "T38", "T39":
		return t.ratio(float64(numerators), float64(dens), dens, true)
	case "T11":
		return overTarget(t, numerators, dens)
	case "T18":
		return balanceOf(t, numSum)
	}
	return t.count()
}

// pick takes the point of the estimator of the stored point among those of a distribution.
func pick(points []entity.Point, sp *entity.SeriesPoint) entity.Point {
	for i, est := range definition(sp.MetricID).Estimators {
		if est == sp.Estimator {
			return points[i]
		}
	}
	return points[0]
}

// overTarget is the point of T11: the tasks over their target among those with a target.
func overTarget(t *tally, over, with int) entity.Point {
	pt := t.point()
	pt.N, pt.LowSample = &with, with < _lowSample
	switch {
	case with == 0 && t.missing == 0:
		pt.State = entity.StateNotApplicable
	case with == 0:
		pt.State, pt.ReasonCode = entity.StateUnknown, t.reason
	default:
		pt.State, pt.Value = entity.StateAssessed, num(float64(over))
		pt.Numerator, pt.Denominator = num(float64(over)), num(float64(with))
		if t.missing > 0 {
			pt.State, pt.ReasonCode = entity.StateProvisional, t.reason
		}
	}
	return pt
}

// balanceOf is the point of T18: the sum of +1 for first starts and -1 for first completions.
func balanceOf(t *tally, sum float64) entity.Point {
	pt := t.point()
	if t.missing > 0 {
		pt.State, pt.ReasonCode = entity.StateUnknown, t.reason
		return pt
	}
	pt.State, pt.Value = entity.StateAssessed, num(sum)
	return pt
}

// regathered tells whether a point depends on the tasks of its cohort: a point of connections, one the formula
// did not evaluate or one of a retired definition stays as stored for any reader.
func regathered(sp *entity.SeriesPoint) bool {
	_, known := lookup(sp.MetricID)
	return known && !_connectionMetrics[sp.MetricID] && sp.State != entity.StateNotEvaluated && sp.ReasonCode != entity.ReasonAsOfOnly
}

func valueOf(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
