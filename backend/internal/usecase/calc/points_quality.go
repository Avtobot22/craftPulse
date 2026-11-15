package calc

import (
	"bytes"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// fieldPoints is T38 at t = end of the period, field by field in the order of its estimators. The size is
// taken at the first start (BR-007), so it counts the tasks started by t; the start counts the tasks started or
// completed by t; the type and the team count every task created by t. A team of M0 is the current structure,
// so every task of the scope has it.
func fieldPoints(tls []*timeline, p period, _ *pointContext) []entity.Point {
	fields := []struct {
		eligible func(tl *timeline) bool
		known    func(tl *timeline) bool
		reason   string
	}{
		{func(tl *timeline) bool { return tl.startedBy(p.end) }, func(tl *timeline) bool { return tl.slice.SizeLabel != nil }, entity.ReasonSizeUnknown},
		{nil, func(tl *timeline) bool { return tl.slice.TaskType != nil }, entity.ReasonTypeUnknown},
		{nil, func(*timeline) bool { return true }, ""},
		{
			func(tl *timeline) bool { return tl.startedBy(p.end) || tl.completedBy(p.end) },
			func(tl *timeline) bool { return tl.slice.FirstStart != nil && tl.slice.FirstStart.Before(p.end) }, entity.ReasonNoStart,
		},
	}
	out := make([]entity.Point, len(fields))
	for i, f := range fields {
		t := newTally(p)
		known, eligible := 0, 0
		for _, tl := range tls {
			if !tl.slice.Created.Before(p.end) || (f.eligible != nil && !f.eligible(tl)) {
				continue
			}
			eligible++
			if !f.known(tl) {
				t.add(tl.slice.TaskID, 1, entity.RoleDenominator, num(1), f.reason)
				continue
			}
			known++
			t.add(tl.slice.TaskID, 1, entity.RoleDenominator, num(1), "")
			t.add(tl.slice.TaskID, 2, entity.RoleNumerator, num(1), "")
		}
		out[i] = t.ratio(float64(known), float64(eligible), eligible, true)
	}
	return out
}

// historyPoints is T39 at t = end of the period: the tasks created by t with a confirmed full history. A history
// is confirmed when it has no boundary and no conflict of order and every connection of its area reconciled
// without differences; an area only imported is told by its facts.
func historyPoints(tls []*timeline, p period, x *pointContext) []entity.Point {
	t := newTally(p)
	full, checked := 0, 0
	for _, tl := range tls {
		s := &tl.slice
		if !s.Created.Before(p.end) {
			continue
		}
		checked++
		reason := ""
		switch {
		case s.Conflict:
			reason = entity.ReasonEventOrderConflict
		case s.HistoryFrom != nil:
			reason = entity.ReasonHistoryIncomplete
		case x.incomplete[tl.area]:
			reason = entity.ReasonCollectionPartial
		}
		t.add(s.TaskID, 1, entity.RoleDenominator, num(1), reason)
		if reason == "" {
			full++
			t.add(s.TaskID, 2, entity.RoleNumerator, num(1), "")
		}
	}
	return []entity.Point{t.ratio(float64(full), float64(checked), checked, true)}
}

// coveragePoints is T41 at t = end of the period: the tasks created by t all of whose statuses entered by t have
// a confirmed meaning, and the statuses entered with one among all entered. A task with a status without a rule
// is in the denominator with status_unmapped.
func coveragePoints(tls []*timeline, p period, _ *pointContext) []entity.Point {
	tasks, transitions := newTally(p), newTally(p)
	mappedTasks, all, mapped, entered := 0, 0, 0, 0
	for _, tl := range tls {
		s := &tl.slice
		if !s.Created.Before(p.end) {
			continue
		}
		all++
		n, m := 0, 0
		for _, e := range tl.entries {
			if !e.at.Before(p.end) {
				break
			}
			n++
			if e.mapped {
				m++
			}
		}
		entered, mapped = entered+n, mapped+m
		transitions.add(s.TaskID, 1, entity.RoleDenominator, num(float64(n)), "")
		transitions.add(s.TaskID, 2, entity.RoleNumerator, num(float64(m)), "")
		if n == 0 || m < n {
			tasks.add(s.TaskID, 1, entity.RoleDenominator, num(1), entity.ReasonStatusUnmapped)
			continue
		}
		mappedTasks++
		tasks.add(s.TaskID, 1, entity.RoleDenominator, num(1), "")
		tasks.add(s.TaskID, 2, entity.RoleNumerator, num(1), "")
	}
	return []entity.Point{
		tasks.ratio(float64(mappedTasks), float64(all), all, true),
		transitions.ratio(float64(mapped), float64(entered), entered, true),
	}
}

// freshnessPoints is T40 at the last point of the window: as_of minus the watermark and minus the last success of
// the stalest connection of the team, each connection a contribution. Earlier points do not describe the
// collection now and are not applicable.
func freshnessPoints(_ []*timeline, p period, x *pointContext) []entity.Point {
	if !p.end.Equal(x.last) {
		return asOfOnly(p, 2)
	}
	conns := x.connections()
	out := make([]entity.Point, 2)
	for i, at := range []func(c *entity.AreaCollection) *float64{
		func(c *entity.AreaCollection) *float64 { return x.age(c.Watermark) },
		func(c *entity.AreaCollection) *float64 { return x.age(c.LastSuccessAt) },
	} {
		pt := entity.Point{Start: p.start, End: p.end}
		var stalest *float64
		n := 0
		for _, c := range conns {
			v := at(c)
			if v == nil {
				pt.MissingCount++
				pt.ReasonCode = entity.ReasonWatermarkMissing
				pt.Contributions = append(pt.Contributions, connectionContribution(c.ConnectionID, entity.RoleMissing, nil, entity.ReasonWatermarkMissing))
				continue
			}
			n++
			pt.Contributions = append(pt.Contributions, connectionContribution(c.ConnectionID, entity.RoleNumerator, v, ""))
			if stalest == nil || *v > *stalest {
				stalest = v
			}
		}
		pt.N, pt.LowSample = &n, n < _lowSample
		switch {
		case len(conns) == 0:
			pt.State = entity.StateNotApplicable
		case stalest == nil:
			pt.State = entity.StateUnknown
		case pt.MissingCount > 0:
			pt.State, pt.Value = entity.StateProvisional, stalest
		default:
			pt.State, pt.Value = entity.StateAssessed, stalest
		}
		out[i] = pt
	}
	return out
}

// variancePoints is T42 at the last point of the window: the tasks missing from craftPulse and those with another
// version than the source in the last finished reconciliation of every connection of the team, with the
// expected number as the denominator.
func variancePoints(_ []*timeline, p period, x *pointContext) []entity.Point {
	if !p.end.Equal(x.last) {
		return asOfOnly(p, 2)
	}
	conns := x.connections()
	out := make([]entity.Point, 2)
	for i, count := range []func(r *entity.ReconcileRun) *int{
		func(r *entity.ReconcileRun) *int { return r.MissingCount },
		func(r *entity.ReconcileRun) *int { return r.ChangedCount },
	} {
		pt := entity.Point{Start: p.start, End: p.end}
		differing, expected, n := 0, 0, 0
		for _, c := range conns {
			r := c.Reconcile
			reason := ""
			switch {
			case r == nil:
				reason = entity.ReasonReconcileMissing
			case r.FinishedAt == nil || count(r) == nil || r.ExpectedCount == nil:
				reason = entity.ReasonReconcileUnfinished
			}
			if reason != "" {
				pt.MissingCount++
				pt.ReasonCode = reason
				pt.Contributions = append(pt.Contributions, connectionContribution(c.ConnectionID, entity.RoleMissing, nil, reason))
				continue
			}
			n++
			differing, expected = differing+*count(r), expected+*r.ExpectedCount
			pt.Contributions = append(pt.Contributions, connectionContribution(c.ConnectionID, entity.RoleNumerator, num(float64(*count(r))), ""))
		}
		pt.N, pt.LowSample = &n, false
		switch {
		case len(conns) == 0:
			pt.State = entity.StateNotApplicable
		case n == 0:
			pt.State = entity.StateNotEvaluated
		default:
			pt.State, pt.Value = entity.StateAssessed, num(float64(differing))
			pt.Numerator, pt.Denominator = num(float64(differing)), num(float64(expected))
			if pt.MissingCount > 0 {
				pt.State = entity.StateProvisional
			}
		}
		out[i] = pt
	}
	return out
}

// connections are the collections of the areas of the team once per connection, in the order of their ID.
func (x *pointContext) connections() []*entity.AreaCollection {
	var out []*entity.AreaCollection
	for i := range x.collection {
		c := &x.collection[i]
		if !slices.ContainsFunc(out, func(o *entity.AreaCollection) bool { return o.ConnectionID == c.ConnectionID }) {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b *entity.AreaCollection) int { return bytes.Compare(a.ConnectionID[:], b.ConnectionID[:]) })
	return out
}

// age is as_of minus the moment in seconds, never negative; nil without the moment.
func (x *pointContext) age(at *time.Time) *float64 {
	if at == nil {
		return nil
	}
	return num(max(x.asOf.Sub(*at).Seconds(), 0))
}

func asOfOnly(p period, n int) []entity.Point {
	out := make([]entity.Point, n)
	for i := range out {
		out[i] = entity.Point{Start: p.start, End: p.end, State: entity.StateNotApplicable, ReasonCode: entity.ReasonAsOfOnly}
	}
	return out
}

func connectionContribution(id entity.ID, role entity.ContributionRole, value *float64, reason string) entity.Contribution {
	return entity.Contribution{ObjectID: id, ItemNo: 1, ObjectKind: entity.ObjectConnection, Role: role, Value: value, ReasonCode: reason}
}

// startedBy tells whether the task started before t, before its known history included.
func (tl *timeline) startedBy(t time.Time) bool {
	if tl.preStart {
		return true
	}
	start := tl.observedStart()
	return start != nil && start.Before(t)
}

// completedBy tells whether the task was completed before t, before its known history included.
func (tl *timeline) completedBy(t time.Time) bool {
	return tl.preCompletion || (len(tl.completions) > 0 && tl.completions[0].Before(t))
}
