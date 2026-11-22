package calc

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

type reportSignals struct {
	review  reviewInput
	facts   entity.TaskObservationFacts
	taskMRs map[entity.ID][]entity.ID
	mrTasks map[entity.ID][]entity.ID
	linked  map[entity.ID]bool
	runs    map[entity.ID]entity.ID
	events  map[string][]activity
}

func (u *UseCase) reportSignals(ctx context.Context, org entity.ID, scope seriesScope, seq int64, in *input) (*reportSignals, error) {
	areas, err := u.scopeAreas(ctx, org, scope)
	if err != nil {
		return nil, err
	}
	facts := make([]entity.ReviewFacts, 0, len(areas))
	var mrIDs, repos []entity.ID
	f := entity.TaskObservationFacts{KnowledgeSeq: seq}
	for _, area := range areas {
		review, err := u.d.Facts.ReviewFacts(ctx, org, area, seq)
		if err != nil {
			return nil, err
		}
		if len(review.MRs) > 0 {
			repos = append(repos, area)
			for j := range review.MRs {
				mrIDs = append(mrIDs, review.MRs[j].MRID)
			}
		}
		f.MRRevisions = append(f.MRRevisions, review.MRRevisions...)
		facts = append(facts, review)
	}
	x := &reportSignals{
		facts: f, taskMRs: map[entity.ID][]entity.ID{}, mrTasks: map[entity.ID][]entity.ID{},
		linked: map[entity.ID]bool{}, runs: map[entity.ID]entity.ID{}, events: map[string][]activity{},
	}
	x.review = reviewInput{set: entity.MetricSetReview, mrs: reviewInputOf(facts, incompleteAreas(in.collection))}
	for _, a := range activitiesOf(facts) {
		if !a.at.Before(in.window.From) && a.at.Before(in.window.To) && a.at.Before(in.asOf) {
			key := a.actor + ":" + a.reason
			x.events[key] = append(x.events[key], a)
		}
	}
	if u.d.Links == nil || len(mrIDs) == 0 {
		return x, nil
	}
	if err := u.reportLinks(ctx, org, seq, mrIDs, x); err != nil {
		return nil, err
	}
	if len(x.runs) == 0 {
		return x, nil
	}
	for _, area := range repos {
		ci, err := u.d.Facts.CIFacts(ctx, org, area, seq)
		if err != nil {
			return nil, err
		}
		for i := range ci.Runs {
			run := &ci.Runs[i]
			if _, ok := x.runs[run.RunID]; ok {
				x.facts.CI.Runs = append(x.facts.CI.Runs, *run)
			}
		}
		for i := range ci.Attempts {
			a := &ci.Attempts[i]
			if _, ok := x.runs[a.RunID]; ok {
				x.facts.CI.Attempts = append(x.facts.CI.Attempts, *a)
			}
		}
	}
	return x, nil
}

func (u *UseCase) reportLinks(ctx context.Context, org entity.ID, seq int64, mrIDs []entity.ID, x *reportSignals) error {
	edges, err := u.d.Links.MREdgesAt(ctx, org, mrIDs, seq)
	if err != nil {
		return err
	}
	for i := range edges {
		e := &edges[i]
		if e.Type == entity.LinkTaskMR && e.Status == entity.LinkConfirmed {
			x.linked[e.To] = true
			x.taskMRs[e.From] = append(x.taskMRs[e.From], e.To)
			x.mrTasks[e.To] = append(x.mrTasks[e.To], e.From)
		}
	}
	for i := range edges {
		e := &edges[i]
		if e.Type == entity.LinkMRRun && e.Status == entity.LinkConfirmed && x.linked[e.From] {
			x.runs[e.To] = e.From
		}
	}
	for task, mrs := range x.taskMRs {
		x.taskMRs[task] = sortedIDs(mrs)
	}
	for mr, tasks := range x.mrTasks {
		x.mrTasks[mr] = sortedIDs(tasks)
	}
	return nil
}

func (x *reportSignals) hash(base []byte, from, to time.Time) []byte {
	d := newDigest()
	d.bytes(base)
	for _, ds := range [][]Definition{_reviewDefinitions, _eventDefinitions, _crossDefinitions} {
		for i := range ds {
			d.text(ds[i].ID)
			d.int(int64(ds[i].Version))
		}
	}
	for i := range x.review.mrs {
		m := &x.review.mrs[i]
		if m.version.OpenedAt.After(to) {
			continue
		}
		d.id(m.version.ID)
		d.flag(m.reviews)
		d.flag(m.complete)
		for _, task := range x.mrTasks[m.version.MRID] {
			d.id(task)
		}
		for j := range m.events {
			e := &m.events[j]
			if !e.EventTime.After(to) {
				d.id(e.ID)
				d.text(actorOf(m.kinds, e.ActorAccountID))
			}
		}
	}
	x.hashCI(d, to)
	for _, key := range slices.Sorted(maps.Keys(x.events)) {
		d.text(key)
		for _, a := range x.events[key] {
			if !a.at.Before(from) && a.at.Before(to) {
				d.id(a.id)
			}
		}
	}
	return d.sum()
}

func (x *reportSignals) hashCI(d *digest, to time.Time) {
	for i := range x.facts.MRRevisions {
		r := &x.facts.MRRevisions[i]
		if !r.CreatedAt.After(to) {
			d.id(r.ID)
		}
	}
	for i := range x.facts.CI.Runs {
		r := &x.facts.CI.Runs[i]
		if !runOrderTime(r).After(to) {
			d.id(r.ID)
			d.id(x.runs[r.RunID])
		}
	}
	for i := range x.facts.CI.Attempts {
		a := &x.facts.CI.Attempts[i]
		if attemptKnownBy(a, to) {
			d.id(a.ID)
		}
	}
}

func (x *reportSignals) points(part *entity.StoredPart, pr *prepared, periods []period) {
	mrTasks := map[entity.ID][]byte{}
	for id, tasks := range x.mrTasks {
		mrTasks[id], _ = json.Marshal(tasks)
	}
	for _, s := range x.review.series(periods) {
		for j := range s.Points {
			pt := s.Points[j]
			if pr.in.mapping == nil {
				pt = entity.Point{Start: pt.Start, End: pt.End, State: entity.StateNotEvaluated, ReasonCode: entity.ReasonMappingMissing}
			}
			part.Points = append(part.Points, signalPoint(s.MetricID, s.Estimator, s.Unit, pt, mrTasks))
		}
	}
	if pr.in.mapping != nil {
		for _, key := range slices.Sorted(maps.Keys(x.events)) {
			for _, p := range periods {
				pt := countActivity(x.events[key], p)
				part.Points = append(part.Points, signalPoint("A22", key, UnitCount, pt, mrTasks))
			}
		}
	}
	for _, p := range periods {
		part.Points = append(part.Points, x.crossPoints(pr, p)...)
	}
}

func signalPoint(metric, est, unit string, pt entity.Point, mrTasks map[entity.ID][]byte) entity.SeriesPoint {
	for i := range pt.Contributions {
		c := &pt.Contributions[i]
		if c.ObjectKind == entity.ObjectMergeRequest {
			c.Evidence = mrTasks[c.ObjectID]
		}
	}
	return entity.SeriesPoint{MetricID: metric, Estimator: est, Unit: unit, Point: pt}
}

func eventGroup(est string) *entity.SeriesGroup {
	actor, kind, ok := strings.Cut(est, ":")
	if !ok {
		return nil
	}
	return &entity.SeriesGroup{EventKind: kind, ActorKind: entity.PersonKind(actor)}
}

func countActivity(events []activity, p period) entity.Point {
	t := newTally(p)
	items := map[entity.ID]int{}
	for _, a := range events {
		if !p.has(a.at) {
			continue
		}
		items[a.object]++
		t.contributions = append(t.contributions, entity.Contribution{
			ObjectID: a.object, ItemNo: items[a.object], ObjectKind: a.kind, Role: entity.RoleNumerator, Value: num(1), ReasonCode: a.reason,
		})
	}
	return activityCount(t)
}

func activityCount(t *tally) entity.Point {
	pt := t.point()
	n := len(t.contributions)
	pt.State, pt.Value, pt.N, pt.LowSample = entity.StateAssessed, num(float64(n)), &n, n < _lowSample
	return pt
}
