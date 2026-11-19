package calc

import (
	"cmp"
	"slices"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// _percentiles are the ranks of the estimators of a distribution.
var _percentiles = map[string]int{EstimatorP50: 50, EstimatorP85: 85}

// tally collects the contributions of a point; numerators are counted once per task.
type tally struct {
	p             period
	contributions []entity.Contribution
	numerators    int
	excluded      int
	missing       int
	reason        string
	// kind is the kind of the counted objects; zero is a task.
	kind entity.ObjectKind
}

func newTally(p period) *tally { return &tally{p: p} }

func (t *tally) add(id entity.ID, item int, role entity.ContributionRole, value *float64, reason string) {
	t.contributions = append(t.contributions, entity.Contribution{
		ObjectID: id, ItemNo: item, ObjectKind: cmp.Or(t.kind, entity.ObjectTask), Role: role, Value: value, ReasonCode: reason,
	})
	switch role {
	case entity.RoleNumerator:
		if item == 1 {
			t.numerators++
		}
	case entity.RoleExcluded:
		t.excluded++
	case entity.RoleMissing:
		t.missing++
		if t.reason == "" {
			t.reason = reason
		}
	case entity.RoleDenominator:
	}
}

func (t *tally) miss(tl *timeline) {
	reason := entity.ReasonHistoryIncomplete
	if tl.slice.Conflict {
		reason = entity.ReasonEventOrderConflict
	}
	t.add(tl.slice.TaskID, 1, entity.RoleMissing, nil, reason)
}

func (t *tally) point() entity.Point {
	return entity.Point{
		Start: t.p.start, End: t.p.end, ExcludedCount: t.excluded, MissingCount: t.missing, Contributions: t.contributions,
	}
}

// count is a point of a count: exact when every task of the scope is known, otherwise only its upper bound is
// proven and the known tasks are the numerator.
func (t *tally) count() entity.Point {
	pt := t.point()
	n := t.numerators
	pt.N, pt.LowSample = &n, n < _lowSample
	if t.missing == 0 {
		pt.State, pt.Value = entity.StateAssessed, num(float64(n))
		return pt
	}
	pt.State, pt.ReasonCode, pt.Numerator = entity.StateBounded, t.reason, num(float64(n))
	pt.Bound = &entity.Bound{Kind: entity.BoundUpper, Value: float64(n + t.missing)}
	return pt
}

// distribution is a point of every estimator over the sample; an empty sample has no percentile (BR-006).
func (t *tally) distribution(sample []float64, estimators []string) []entity.Point {
	slices.Sort(sample)
	out := make([]entity.Point, len(estimators))
	for i, est := range estimators {
		pt := t.point()
		pt.Contributions = slices.Clone(pt.Contributions)
		n := len(sample)
		pt.N, pt.LowSample = &n, n < _lowSample
		switch {
		case n == 0 && t.missing == 0:
			pt.State = entity.StateNotApplicable
		case n == 0:
			pt.State, pt.ReasonCode = entity.StateUnknown, t.reason
		case t.missing > 0:
			pt.State, pt.ReasonCode, pt.Value = entity.StateProvisional, t.reason, num(percentile(sample, _percentiles[est]))
		default:
			pt.State, pt.Value = entity.StateAssessed, num(percentile(sample, _percentiles[est]))
		}
		out[i] = pt
	}
	return out
}

// ratio is a point of numerator/denominator; without a denominator it is not applicable, and a ratio over
// tasks of an unknown state is UNKNOWN.
func (t *tally) ratio(numerator, denominator float64, n int, partial bool) entity.Point {
	pt := t.point()
	pt.N, pt.LowSample = &n, n < _lowSample
	switch {
	case denominator == 0 && t.missing == 0:
		pt.State = entity.StateNotApplicable
	case t.missing > 0 && (!partial || denominator == 0):
		pt.State, pt.ReasonCode = entity.StateUnknown, t.reason
	default:
		pt.State, pt.Value = entity.StateAssessed, num(numerator/denominator)
		pt.Numerator, pt.Denominator = num(numerator), num(denominator)
		if t.missing > 0 {
			pt.State, pt.ReasonCode = entity.StateProvisional, t.reason
		}
	}
	return pt
}

// percentile is the nearest rank sorted[ceil(p*n/100)-1] in integers, so 85% of 20 is the 17th value.
func percentile(sorted []float64, p int) float64 {
	k := (p*len(sorted) + 99) / 100
	return sorted[k-1]
}

func num(v float64) *float64 { return &v }

// startedPoint is T04: tasks whose first start falls in the period.
func startedPoint(tls []*timeline, p period) []entity.Point {
	t := newTally(p)
	for _, tl := range tls {
		switch tl.firstStartIn(p) {
		case memberIn:
			t.add(tl.slice.TaskID, 1, entity.RoleNumerator, num(1), "")
		case memberUnknown:
			t.miss(tl)
		case memberOut:
		}
	}
	return []entity.Point{t.count()}
}

// completedPoint is T05: tasks whose first accepted completion falls in the period; a cancellation and a
// repeated completion of the period are shown as exclusions.
func completedPoint(tls []*timeline, p period) []entity.Point {
	t := newTally(p)
	for _, tl := range tls {
		first := tl.firstCompletionIn(p)
		switch first {
		case memberIn:
			t.add(tl.slice.TaskID, 1, entity.RoleNumerator, num(1), "")
			continue
		case memberUnknown:
			t.miss(tl)
			continue
		case memberOut:
		}
		if term, ok := tl.terminationIn(p); ok {
			t.add(tl.slice.TaskID, 1, entity.RoleExcluded, nil, term.reason)
		} else if tl.repeatCompletionIn(p, first) {
			t.add(tl.slice.TaskID, 1, entity.RoleExcluded, nil, entity.ReasonRepeatCompletion)
		}
	}
	return []entity.Point{t.count()}
}

// cancelledPoint is T07: tasks with a terminal outcome without an accepted result in the period, by reason.
func cancelledPoint(tls []*timeline, p period) []entity.Point {
	t := newTally(p)
	for _, tl := range tls {
		if term, ok := tl.terminationIn(p); ok && !tl.slice.Conflict {
			t.add(tl.slice.TaskID, 1, entity.RoleNumerator, num(1), term.reason)
			continue
		}
		if !tl.historyCovers(p) {
			t.miss(tl)
		}
	}
	return []entity.Point{t.count()}
}

// cycleTimePoints are T08: first accepted completion - first start of the cohort first completed in the period.
func cycleTimePoints(tls []*timeline, p period) []entity.Point {
	t := newTally(p)
	var sample []float64
	for _, tl := range tls {
		s := &tl.slice
		switch tl.firstCompletionIn(p) {
		case memberIn:
			switch {
			case s.HistoryFrom != nil:
				t.miss(tl)
			case s.FirstStart == nil:
				t.add(s.TaskID, 1, entity.RoleExcluded, nil, entity.ReasonNoStart)
			default:
				d := s.FirstCompletion.Sub(*s.FirstStart).Seconds()
				sample = append(sample, d)
				t.add(s.TaskID, 1, entity.RoleNumerator, num(d), "")
			}
		case memberUnknown:
			t.miss(tl)
		case memberOut:
			if term, ok := tl.terminationIn(p); ok {
				t.add(s.TaskID, 1, entity.RoleExcluded, nil, term.reason)
			}
		}
	}
	return t.distribution(sample, definition("T08").Estimators)
}

// wipPoint is T12: tasks with an open episode at the end of the period, blocked ones included (BR-005).
func wipPoint(tls []*timeline, p period) []entity.Point {
	t := newTally(p)
	for _, tl := range tls {
		switch {
		case !tl.knownAt(p.end):
			t.miss(tl)
		case tl.episodeAt(p.end) != nil:
			t.add(tl.slice.TaskID, 1, entity.RoleNumerator, num(1), "")
		}
	}
	return []entity.Point{t.count()}
}

// blockedWIPPoint is T13: the blocked subset of the WIP at the end of the period and its share.
func blockedWIPPoint(tls []*timeline, p period) []entity.Point {
	t := newTally(p)
	wip, blocked := 0, 0
	for _, tl := range tls {
		if !tl.knownAt(p.end) {
			t.miss(tl)
			continue
		}
		ep := tl.episodeAt(p.end)
		if ep == nil {
			continue
		}
		wip++
		t.add(tl.slice.TaskID, 1, entity.RoleDenominator, num(1), "")
		if ep.classAt(p.end) == entity.IntervalBlocked {
			blocked++
			t.add(tl.slice.TaskID, 2, entity.RoleNumerator, num(1), "")
		}
	}
	return []entity.Point{t.ratio(float64(blocked), float64(wip), wip, false)}
}

// agePoints are T14: the age of the current episode of the tasks in WIP at the end of the period; the age from
// the first start is a second contribution of the task.
func agePoints(tls []*timeline, p period) []entity.Point {
	t := newTally(p)
	var sample []float64
	for _, tl := range tls {
		if !tl.knownAt(p.end) {
			t.miss(tl)
			continue
		}
		ep := tl.episodeAt(p.end)
		switch {
		case ep == nil:
			continue
		case !ep.startKnown:
			t.miss(tl)
			continue
		}
		age := p.end.Sub(ep.start).Seconds()
		sample = append(sample, age)
		t.add(tl.slice.TaskID, 1, entity.RoleNumerator, num(age), "")
		if s := &tl.slice; s.HistoryFrom == nil && s.FirstStart != nil {
			t.add(s.TaskID, 2, entity.RoleNumerator, num(p.end.Sub(*s.FirstStart).Seconds()), entity.ReasonFullAge)
		}
	}
	return t.distribution(sample, definition("T14").Estimators)
}

// blockedTimePoint is T15: the union of blocked intervals within the period and its share of the time in open
// episodes. One timeline of classes per task makes a status and a flag blocking at once count once (BR-005).
func blockedTimePoint(tls []*timeline, p period) []entity.Point {
	t := newTally(p)
	var blockedTotal, elapsedTotal float64
	tasks := 0
	for _, tl := range tls {
		if !tl.historyCovers(p) {
			t.miss(tl)
			continue
		}
		var blocked, elapsed float64
		for _, iv := range tl.slice.Intervals {
			d := overlap(iv.From, iv.To, p)
			elapsed += d
			if iv.Class == entity.IntervalBlocked {
				blocked += d
			}
		}
		if elapsed == 0 {
			continue
		}
		tasks++
		blockedTotal, elapsedTotal = blockedTotal+blocked, elapsedTotal+elapsed
		t.add(tl.slice.TaskID, 1, entity.RoleNumerator, num(blocked), "")
		t.add(tl.slice.TaskID, 2, entity.RoleDenominator, num(elapsed), "")
	}
	return []entity.Point{t.ratio(blockedTotal, elapsedTotal, tasks, true)}
}

// balancePoint is T18: first starts minus first completions of the period on the scale of T12.
func balancePoint(tls []*timeline, p period) []entity.Point {
	t := newTally(p)
	var sum float64
	for _, tl := range tls {
		started, completed := tl.firstStartIn(p), tl.firstCompletionIn(p)
		if started == memberUnknown || completed == memberUnknown {
			t.miss(tl)
			continue
		}
		if started == memberIn {
			sum++
			t.add(tl.slice.TaskID, 1, entity.RoleNumerator, num(1), entity.ReasonStarted)
		}
		if completed == memberIn {
			sum--
			t.add(tl.slice.TaskID, 2, entity.RoleNumerator, num(-1), entity.ReasonCompleted)
		}
	}
	return []entity.Point{balanceOf(t, sum)}
}
