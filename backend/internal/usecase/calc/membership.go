package calc

import "time"

// membership tells whether a task belongs to a cohort of a period, or that the known history cannot tell.
type membership int

const (
	memberOut membership = iota + 1
	memberIn
	memberUnknown
)

// period is a point [start, end); end is bounded by as_of.
type period struct {
	start, end time.Time
}

func (p period) has(t time.Time) bool { return !t.Before(p.start) && t.Before(p.end) }

// firstIn places the first event of a kind, a start or a completion, in the period. With the full history it
// is the observed one. Without the history before historyFrom it lies in [created, historyFrom) when the
// beginning of the known history proves it happened; an observed event is never taken for the first one, so it
// only rules the period out when neither it nor the unknown span falls there (BR-004, TZ 4.3).
func (tl *timeline) firstIn(p period, proven bool, observed *time.Time) membership {
	if tl.slice.Conflict {
		if tl.slice.Created.Before(p.end) {
			return memberUnknown
		}
		return memberOut
	}
	h := tl.slice.HistoryFrom
	if h == nil {
		if observed != nil && p.has(*observed) {
			return memberIn
		}
		return memberOut
	}
	created := tl.slice.Created
	spanInside := !created.Before(p.start) && !h.After(p.end)
	spanOverlaps := created.Before(p.end) && h.After(p.start)
	switch {
	case proven && spanInside:
		return memberIn
	case proven && !spanOverlaps:
		return memberOut
	case proven:
		return memberUnknown
	case !spanOverlaps && (observed == nil || !p.has(*observed)):
		return memberOut
	}
	return memberUnknown
}

func (tl *timeline) firstStartIn(p period) membership {
	return tl.firstIn(p, tl.preStart, tl.observedStart())
}

func (tl *timeline) firstCompletionIn(p period) membership {
	var observed *time.Time
	if len(tl.completions) > 0 {
		observed = &tl.completions[0]
	}
	return tl.firstIn(p, tl.preCompletion, observed)
}

// observedStart is the first start seen in the known history.
func (tl *timeline) observedStart() *time.Time {
	for _, ep := range tl.episodes {
		if ep.startKnown {
			return &ep.start
		}
	}
	return nil
}

// historyCovers tells whether the known history describes the task over the whole period.
func (tl *timeline) historyCovers(p period) bool {
	if tl.slice.Conflict {
		return !tl.slice.Created.Before(p.end)
	}
	h := tl.slice.HistoryFrom
	return h == nil || !h.After(p.start) || !tl.slice.Created.Before(p.end)
}

// knownAt tells whether the state of the task at t is known.
func (tl *timeline) knownAt(t time.Time) bool {
	if tl.slice.Created.After(t) {
		return true
	}
	if tl.slice.Conflict {
		return false
	}
	h := tl.slice.HistoryFrom
	return h == nil || !t.Before(*h)
}

// terminationIn is the first terminal outcome without an accepted result in the period.
func (tl *timeline) terminationIn(p period) (termination, bool) {
	for _, t := range tl.terminations {
		if p.has(t.at) {
			return t, true
		}
	}
	return termination{}, false
}

// repeatCompletionIn tells whether an accepted completion of the period is not the first one of the task.
func (tl *timeline) repeatCompletionIn(p period, first membership) bool {
	for i, c := range tl.completions {
		if !p.has(c) {
			continue
		}
		if i > 0 || first == memberOut || tl.preCompletion {
			return true
		}
	}
	return false
}

// overlap is the length of [from, to) inside the period, in seconds.
func overlap(from, to time.Time, p period) float64 {
	if from.Before(p.start) {
		from = p.start
	}
	if to.After(p.end) {
		to = p.end
	}
	if !from.Before(to) {
		return 0
	}
	return to.Sub(from).Seconds()
}
