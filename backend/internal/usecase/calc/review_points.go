package calc

import (
	"bytes"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// reviewInput is what a part of the review metrics or of the events reads: the MRs and events of the areas of the
// scope at the knowledge cutoff of the revision.
type reviewInput struct {
	set      string
	window   entity.Window
	asOf     time.Time
	location *time.Location
	mrs      []reviewMR
	events   []activity
}

type reviewMR struct {
	version  entity.MRVersion
	events   []entity.MREvent
	kinds    map[entity.ID]entity.PersonKind
	reviews  bool
	complete bool
}

func (m *reviewMR) wait(at time.Time) entity.ReviewWait {
	return ReviewWaitOf(&m.version, m.events, m.kinds, m.reviews, m.complete, at)
}

// activity is an event of a task or an MR as A22 counts it.
type activity struct {
	id, object entity.ID
	kind       entity.ObjectKind
	reason     string
	at         time.Time
	actor      string
}

// reviewInputOf joins the events of the MRs of the areas to them; the history of an area is complete unless a
// connection of it has not reconciled without differences.
func reviewInputOf(facts []entity.ReviewFacts, incomplete map[entity.ID]bool) []reviewMR {
	var out []reviewMR
	for i := range facts {
		f := &facts[i]
		byMR := map[entity.ID][]entity.MREvent{}
		for j := range f.MREvents {
			byMR[f.MREvents[j].MRID] = append(byMR[f.MREvents[j].MRID], f.MREvents[j])
		}
		for j := range f.MRs {
			v := f.MRs[j]
			out = append(out, reviewMR{version: v, events: byMR[v.MRID], kinds: f.ActorKinds, reviews: f.ReviewsGiven, complete: !incomplete[f.AreaID]})
		}
	}
	slices.SortFunc(out, func(a, b reviewMR) int { return bytes.Compare(a.version.MRID[:], b.version.MRID[:]) })
	return out
}

// activitiesOf are the events of tasks and MRs of the areas with the kinds of their participants.
func activitiesOf(facts []entity.ReviewFacts) []activity {
	var out []activity
	for i := range facts {
		f := &facts[i]
		for j := range f.MREvents {
			e := &f.MREvents[j]
			out = append(out, activity{
				id: e.ID, object: e.MRID, kind: entity.ObjectMergeRequest, reason: "mr:" + e.Kind, at: e.EventTime, actor: actorOf(f.ActorKinds, e.ActorAccountID),
			})
		}
		for j := range f.TaskEvents {
			e := &f.TaskEvents[j]
			out = append(out, activity{
				id: e.ID, object: e.TaskID, kind: entity.ObjectTask, reason: "task:" + string(e.Kind), at: e.EventTime, actor: actorOf(f.ActorKinds, e.ActorAccountID),
			})
		}
	}
	slices.SortFunc(out, func(a, b activity) int { return bytes.Compare(a.id[:], b.id[:]) })
	return out
}

func actorOf(kinds map[entity.ID]entity.PersonKind, account *entity.ID) string {
	if account == nil {
		return ActorUnknown
	}
	if k, ok := kinds[*account]; ok && k != "" {
		return string(k)
	}
	return ActorUnknown
}

type reviewCalc func(in *reviewInput, p period) []entity.Point

var _reviewMetrics = map[string]reviewCalc{"A05": waitingPoint, "A06": waitAgePoints, "A07": firstReviewPoints, "A10": noReviewerPoint, "A22": eventPoints}

// series are the points of the definitions of the set over the periods.
func (in *reviewInput) series(periods []period) []entity.Series {
	var out []entity.Series
	defs := definitionsOf(in.set)
	for d := range defs {
		def := &defs[d]
		estimators := def.Estimators
		if len(estimators) == 0 {
			estimators = []string{""}
		}
		ss := make([]entity.Series, len(estimators))
		for i, est := range estimators {
			ss[i] = entity.Series{MetricID: def.ID, DefinitionVersion: def.Version, Unit: def.Unit, Estimator: est}
		}
		for _, p := range periods {
			pts := _reviewMetrics[def.ID](in, p)
			for i := range pts {
				ss[i].Points = append(ss[i].Points, pts[i])
			}
		}
		out = append(out, ss...)
	}
	return out
}

func mrTally(p period) *tally {
	t := newTally(p)
	t.kind = entity.ObjectMergeRequest
	return t
}

// waitingPoint is A05: the ready open MRs at the end of the period, each with the kind of its wait.
func waitingPoint(in *reviewInput, p period) []entity.Point {
	t := mrTally(p)
	for i := range in.mrs {
		m := &in.mrs[i]
		if !readyOpen(&m.version, m.events, p.end) {
			continue
		}
		w := m.wait(p.end)
		t.add(m.version.MRID, 1, entity.RoleNumerator, num(float64(w.WaitSeconds)), string(w.Kind))
	}
	return []entity.Point{t.count()}
}

// waitAgePoints is A06: the age of the waits for a first response at the end of the period; an MR whose
// history is incomplete is missing.
func waitAgePoints(in *reviewInput, p period) []entity.Point {
	t := mrTally(p)
	var sample []float64
	for i := range in.mrs {
		m := &in.mrs[i]
		if !readyOpen(&m.version, m.events, p.end) {
			continue
		}
		w := m.wait(p.end)
		switch w.Kind {
		case entity.ReviewUnknown:
			t.add(m.version.MRID, 1, entity.RoleMissing, nil, entity.ReasonHistoryIncomplete)
		case entity.ReviewNoReviewer, entity.ReviewAwaitingFirst:
			age := float64(w.WaitSeconds)
			sample = append(sample, age)
			t.add(m.version.MRID, 1, entity.RoleNumerator, &age, fallbackReason(w))
		case entity.ReviewResponded:
		}
	}
	return t.distribution(sample, definition("A06").Estimators)
}

// firstReviewPoints is A07: the time from ready to the first qualifying response of the MRs responded in the period.
func firstReviewPoints(in *reviewInput, p period) []entity.Point {
	t := mrTally(p)
	var sample []float64
	comments := false
	for i := range in.mrs {
		m := &in.mrs[i]
		w := m.wait(p.end)
		if w.Kind != entity.ReviewResponded || !p.has(w.Since) {
			continue
		}
		d := float64(w.WaitSeconds)
		sample = append(sample, d)
		t.add(m.version.MRID, 1, entity.RoleNumerator, &d, w.ResponseKind)
		comments = comments || w.ResponseKind == _eventComment
	}
	pts := t.distribution(sample, definition("A07").Estimators)
	if comments {
		for i := range pts {
			if pts[i].ReasonCode == "" {
				pts[i].ReasonCode = entity.ReasonExternalResponse
			}
		}
	}
	return pts
}

// noReviewerPoint is A10: the ready open MRs without a reviewer among those whose field of reviewers is available.
func noReviewerPoint(in *reviewInput, p period) []entity.Point {
	t := mrTally(p)
	var numerator, denominator float64
	for i := range in.mrs {
		m := &in.mrs[i]
		if !readyOpen(&m.version, m.events, p.end) {
			continue
		}
		if !m.version.ReviewerField {
			t.add(m.version.MRID, 1, entity.RoleExcluded, nil, entity.ReasonReviewerUnknown)
			continue
		}
		denominator++
		t.add(m.version.MRID, 1, entity.RoleDenominator, nil, "")
		if !reviewerAssigned(&m.version, m.events, p.end) {
			numerator++
			t.add(m.version.MRID, 2, entity.RoleNumerator, nil, "")
		}
	}
	return []entity.Point{t.ratio(numerator, denominator, int(denominator), false)}
}

// eventPoints is A22: the events of the period by the kind of their participant, one series per kind.
func eventPoints(in *reviewInput, p period) []entity.Point {
	estimators := definition("A22").Estimators
	out := make([]entity.Point, len(estimators))
	for i, est := range estimators {
		t := newTally(p)
		items := map[entity.ID]int{}
		n := 0
		for _, a := range in.events {
			if a.actor != est || !p.has(a.at) {
				continue
			}
			items[a.object]++
			n++
			t.contributions = append(t.contributions, entity.Contribution{
				ObjectID: a.object, ItemNo: items[a.object], ObjectKind: a.kind, Role: entity.RoleNumerator, Value: num(1), ReasonCode: a.reason,
			})
		}
		pt := t.point()
		pt.State, pt.Value, pt.N, pt.LowSample = entity.StateAssessed, num(float64(n)), &n, n < _lowSample
		out[i] = pt
	}
	return out
}

func fallbackReason(w entity.ReviewWait) string {
	if w.FallbackOpen {
		return entity.ReasonFallbackOpen
	}
	return string(w.Kind)
}
