package calc

import (
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// withinTargetPoints is T10: the tasks first completed in the period within the target of their type and size
// in force at the knowledge number, among those with a known start, size and target. A task without them is
// missing with the reason; no target of another group is taken (BR-007).
func withinTargetPoints(tls []*timeline, p period, x *pointContext) []entity.Point {
	t := newTally(p)
	within, eligible := 0, 0
	for _, tl := range tls {
		s := &tl.slice
		switch tl.firstCompletionIn(p) {
		case memberOut:
			continue
		case memberUnknown:
			t.miss(tl)
			continue
		case memberIn:
		}
		if s.HistoryFrom != nil {
			t.miss(tl)
			continue
		}
		limit, reason := x.target(s)
		if s.FirstStart == nil {
			limit, reason = 0, entity.ReasonNoStart
		}
		if reason != "" {
			t.add(s.TaskID, 1, entity.RoleMissing, nil, reason)
			continue
		}
		cycle := s.FirstCompletion.Sub(*s.FirstStart).Seconds()
		eligible++
		t.add(s.TaskID, 1, entity.RoleDenominator, num(cycle), "")
		if cycle <= float64(limit) {
			within++
			t.add(s.TaskID, 2, entity.RoleNumerator, num(1), "")
		}
	}
	return []entity.Point{t.ratio(float64(within), float64(eligible), eligible, true)}
}

// overTargetPoints is T11 at t = end of the period: the tasks in WIP whose current episode is older than their
// target, among those with a target and a known start of the episode. Without a target there is no excess.
func overTargetPoints(tls []*timeline, p period, x *pointContext) []entity.Point {
	t := newTally(p)
	over, with := 0, 0
	for _, tl := range tls {
		s := &tl.slice
		if !tl.knownAt(p.end) {
			t.miss(tl)
			continue
		}
		ep := tl.episodeAt(p.end)
		if ep == nil {
			continue
		}
		limit, reason := x.target(s)
		if !ep.startKnown {
			limit, reason = 0, entity.ReasonNoStart
		}
		if reason != "" {
			t.add(s.TaskID, 1, entity.RoleMissing, nil, reason)
			continue
		}
		age := p.end.Sub(ep.start).Seconds()
		with++
		t.add(s.TaskID, 1, entity.RoleDenominator, num(age/float64(limit)), "")
		if age > float64(limit) {
			over++
			t.add(s.TaskID, 2, entity.RoleNumerator, num(1), "")
		}
	}
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
	return []entity.Point{pt}
}

// reopenedPoints is T23: the tasks first completed in the period reopened in [first, first + H), among those
// whose window ended by as_of. The start of the window is inside and its end outside; a task whose window is
// still open is immature, excluded and listed (BR-008).
func reopenedPoints(tls []*timeline, p period, x *pointContext) []entity.Point {
	t := newTally(p)
	reopened, mature := 0, 0
	for _, tl := range tls {
		s := &tl.slice
		switch tl.firstCompletionIn(p) {
		case memberOut:
			continue
		case memberUnknown:
			t.miss(tl)
			continue
		case memberIn:
		}
		if s.HistoryFrom != nil || s.FirstCompletion == nil {
			t.miss(tl)
			continue
		}
		first := *s.FirstCompletion
		end := first.Add(x.followUp)
		if end.After(x.asOf) {
			t.add(s.TaskID, 1, entity.RoleExcluded, nil, entity.ReasonImmature)
			continue
		}
		mature++
		t.add(s.TaskID, 1, entity.RoleDenominator, num(1), "")
		if tl.reopenedIn(first, end) {
			reopened++
			t.add(s.TaskID, 2, entity.RoleNumerator, num(1), "")
		}
	}
	return []entity.Point{t.ratio(float64(reopened), float64(mature), mature, true)}
}

// target is the duration of the target of the group of a task with the reason it has none.
func (x *pointContext) target(s *entity.TaskSlice) (seconds int64, reason string) {
	switch {
	case s.TaskType == nil:
		return 0, entity.ReasonTypeUnknown
	case s.SizeLabel == nil:
		return 0, entity.ReasonSizeUnknown
	}
	limit, ok := x.targets[entity.TargetGroup{TaskType: *s.TaskType, SizeLabel: *s.SizeLabel}]
	if !ok {
		return 0, entity.ReasonTargetMissing
	}
	return limit, ""
}

// reopenedIn tells whether an episode began in [from, to): work again after the first completion.
func (tl *timeline) reopenedIn(from, to time.Time) bool {
	for _, ep := range tl.episodes {
		if ep.startKnown && !ep.start.Before(from) && ep.start.Before(to) {
			return true
		}
	}
	return false
}
