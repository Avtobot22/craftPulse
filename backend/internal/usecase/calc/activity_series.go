package calc

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// ActivitySeries finds or creates a weekly or monthly series of one team or connection.
func (u *UseCase) ActivitySeries(ctx context.Context, org entity.ID, scope entity.ActivityScope, step entity.Step) (entity.CalcSeries, error) {
	if step != entity.StepWeek && step != entity.StepMonth {
		return entity.CalcSeries{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/step", Code: entity.FieldNotAllowedValue}}}
	}
	if scope.TeamID.IsZero() == scope.ConnectionID.IsZero() {
		return entity.CalcSeries{}, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/scope", Code: entity.FieldNotAllowedValue}}}
	}
	if !scope.TeamID.IsZero() {
		return u.scopedSeries(ctx, org, entity.ReviewScope{TeamID: scope.TeamID}, entity.MetricSetActivity, step)
	}
	raw, err := json.Marshal(struct {
		Kind         string    `json:"kind"`
		ConnectionID entity.ID `json:"connection_id"`
	}{_scopeConnection, scope.ConnectionID})
	if err != nil {
		return entity.CalcSeries{}, fmt.Errorf("calc: encode activity scope: %w", err)
	}
	return u.ensureScopedSeries(ctx, org, raw, entity.MetricSetActivity, step)
}

type activitySource struct {
	window    entity.Window
	asOf      time.Time
	location  *time.Location
	series    *entity.CalcSeries
	events    []activityEvent
	history   []entity.ActivityHistory
	freshness []entity.AreaCollection
	runs      []entity.ReconcileRun
	links     []activityLinks
}

type activityEvent struct {
	id, object, connection entity.ID
	kind                   entity.ObjectKind
	via                    entity.ObservedVia
	at, observed           time.Time
	account                *entity.ID
	actor                  string
}

func (u *UseCase) activitySource(ctx context.Context, job entity.CalcSliceJob, rev *entity.CalcRevision, series *entity.CalcSeries,
	scope seriesScope, areas []entity.ID, loc *time.Location,
) (*activitySource, error) {
	org := job.OrganizationID
	src := &activitySource{window: rev.Window, asOf: rev.AsOf, location: loc, series: series}
	src.window.Step = series.Step
	areaConnections := make(map[entity.ID]entity.ID)
	connections := make(map[entity.ID]bool)
	seenStates := make(map[activityStateKey]bool)
	if job.Sources != nil {
		for i := range *job.Sources {
			item := &(*job.Sources)[i]
			areaConnections[item.AreaID] = item.ConnectionID
			connections[item.ConnectionID] = true
			k := activityStateKey{item.ConnectionID, item.DataKind}
			if !seenStates[k] {
				seenStates[k] = true
				src.freshness = append(src.freshness, *item)
			}
		}
	}
	var err error
	history, err := u.d.Activity.ActivityHistory(ctx, org, areas, scope.ConnectionID, rev.StartedAt)
	if err != nil {
		return nil, err
	}
	if len(connections) > 0 {
		ids := slices.SortedFunc(maps.Keys(connections), compareActivityIDs)
		src.runs, err = u.d.Activity.ActivityReconciliations(ctx, org, ids, rev.Window.From, rev.StartedAt)
		if err != nil {
			return nil, err
		}
	}
	mrs := make(map[entity.ID]time.Time)
	objects := make(map[entity.ID]activityObject)
	taskAreas := make(map[entity.ID]bool)
	for i := range history {
		h := &history[i]
		if h.Key.ObjectKind == string(entity.ObjectTask) {
			taskAreas[h.Source.AreaID] = true
		}
	}
	events := make([]activityEvent, 0)
	receipts := make([]entity.ID, 0)
	for _, area := range areas {
		facts, err := u.d.Facts.ReviewFacts(ctx, org, area, rev.KnowledgeSeq)
		if err != nil {
			return nil, err
		}
		for i := range facts.MRs {
			mr := &facts.MRs[i]
			objects[mr.MRID] = activityObject{area, mr.OpenedAt}
			if scope.ConnectionID.IsZero() || areaConnections[area] == scope.ConnectionID {
				mrs[mr.MRID] = mr.OpenedAt
			}
		}
		if taskAreas[area] {
			facts, err := u.d.Facts.AreaFacts(ctx, org, area, rev.KnowledgeSeq)
			if err != nil {
				return nil, err
			}
			for i := range facts.Tasks {
				task := &facts.Tasks[i]
				objects[task.TaskID] = activityObject{area, task.CreatedSrcAt}
			}
		}
		for i := range facts.MREvents {
			e := &facts.MREvents[i]
			events = append(events, activityEvent{
				id: e.ID, object: e.MRID, kind: entity.ObjectMergeRequest, at: e.EventTime,
				observed: e.ObservedAt, via: e.ObservedVia, account: e.ActorAccountID, actor: actorOf(facts.ActorKinds, e.ActorAccountID),
			})
			receipts = append(receipts, e.Record.ReceiptID)
		}
		for i := range facts.TaskEvents {
			e := &facts.TaskEvents[i]
			events = append(events, activityEvent{
				id: e.ID, object: e.TaskID, kind: entity.ObjectTask, at: e.EventTime,
				observed: e.ObservedAt, via: e.ObservedVia, account: e.ActorAccountID, actor: actorOf(facts.ActorKinds, e.ActorAccountID),
			})
			receipts = append(receipts, e.Record.ReceiptID)
		}
	}
	if len(history) > 0 {
		ends, err := u.d.Facts.CurrentObjects(ctx, org, slices.Collect(maps.Keys(objects)))
		if err != nil {
			return nil, err
		}
		src.history = activityHistoryOf(history, objects, ends)
	}
	if len(receipts) > 0 {
		keys := slices.Clone(receipts)
		slices.SortFunc(keys, compareActivityIDs)
		origins, err := u.d.Activity.ActivityReceiptConnections(ctx, org, slices.Compact(keys))
		if err != nil {
			return nil, err
		}
		seen := make(map[entity.ID]bool)
		for i := range events {
			event := &events[i]
			event.connection = origins[receipts[i]]
			if (scope.ConnectionID.IsZero() || event.connection == scope.ConnectionID) && !seen[event.id] {
				seen[event.id] = true
				src.events = append(src.events, *event)
			}
		}
		slices.SortFunc(src.events, func(a, b activityEvent) int { return compareActivityIDs(a.id, b.id) })
	}
	if u.d.Links != nil && len(mrs) > 0 {
		ids := slices.SortedFunc(maps.Keys(mrs), compareActivityIDs)
		edges, err := u.d.Links.MREdgesAt(ctx, org, ids, rev.KnowledgeSeq)
		if err != nil {
			return nil, err
		}
		src.links = activityLinksOf(mrs, ids, edges)
	}
	return src, nil
}

type activityObject struct {
	area   entity.ID
	opened time.Time
}

func activityHistoryOf(evidence []entity.ActivityHistoryEvidence, objects map[entity.ID]activityObject, ends []entity.LinkEnd) []entity.ActivityHistory {
	type sourceObject struct {
		area entity.ID
		key  entity.SourceKey
	}
	known := make(map[sourceObject]entity.ID, len(ends))
	for _, end := range ends {
		k := entity.SourceKey{InstanceID: end.InstanceID, Kind: end.Kind, NativeNS: end.NativeNS, NativeID: end.NativeID}
		known[sourceObject{objects[end.ID].area, k}] = end.ID
	}
	byObject := make(map[entity.ID]entity.ActivityHistory)
	for i := range evidence {
		e := &evidence[i]
		key := entity.SourceKey{InstanceID: e.Source.InstanceID, Kind: entity.ObjectKind(e.Key.ObjectKind), NativeNS: e.Key.NativeNS, NativeID: e.Key.NativeID}
		id := known[sourceObject{e.Source.AreaID, key}]
		h := entity.ActivityHistory{ObjectID: id, ConnectionID: e.Source.ConnectionID, Kind: key.Kind, OpenedAt: objects[id].opened}
		if id.IsZero() {
			// A checked source object can be missing locally. Keep a stable drill-down key across revisions.
			d := newDigest()
			d.text("activity_history_object/1")
			d.id(key.InstanceID)
			d.text(string(key.Kind))
			d.text(key.NativeNS)
			d.text(key.NativeID)
			h.ObjectID = entity.ID(d.sum()[:16])
		}
		if e.Check != nil {
			h.Checked, h.Matches, h.ReconcileID = true, e.Check.Matches, e.Check.RunID
			h.CheckedAt = e.Check.CheckedAt
		}
		switch {
		case e.Cut != nil && e.Recovery == nil:
			h.Truncated, h.HistoryFrom, h.HistoryReceiptID = true, e.Cut.HistoryFrom, e.Cut.ReceiptID
			if h.CheckedAt.IsZero() {
				h.CheckedAt = e.Cut.ObservedAt
			}
		case !id.IsZero() && e.Recovery != nil:
			h.Complete, h.HistoryFrom, h.HistoryReceiptID = h.Matches, &h.OpenedAt, e.Recovery.ReceiptID
		case !id.IsZero() && h.Matches && !e.Check.PaginationReceipt.IsZero():
			h.Complete, h.HistoryFrom, h.HistoryReceiptID = true, &h.OpenedAt, e.Check.PaginationReceipt
			if from := e.Source.HistoryFrom; from != nil && from.After(h.OpenedAt) {
				h.Complete, h.HistoryFrom = false, from
			}
		}
		previous, seen := byObject[h.ObjectID]
		if !seen || betterActivityHistory(h, previous) {
			byObject[h.ObjectID] = h
		}
	}
	return slices.SortedFunc(maps.Values(byObject), func(a, b entity.ActivityHistory) int { return compareActivityIDs(a.ObjectID, b.ObjectID) })
}

func betterActivityHistory(a, b entity.ActivityHistory) bool {
	switch {
	case a.Complete != b.Complete:
		return a.Complete
	case a.Checked != b.Checked:
		return a.Checked
	case a.Truncated != b.Truncated:
		return a.Truncated
	default:
		return compareActivityIDs(a.ConnectionID, b.ConnectionID) < 0
	}
}

func compareActivityIDs(a, b entity.ID) int { return slices.Compare(a[:], b[:]) }

func (s *activitySource) periods() []period { return periods(s.window, s.asOf, s.location) }

func (s *activitySource) hash(from, to time.Time) []byte {
	d := newDigest()
	d.text(entity.MetricSetActivity)
	d.bytes(s.series.ScopeHash)
	d.text(string(s.series.Step))
	d.text(s.location.String())
	d.time(from)
	d.time(to)
	for i := range _activityDefinitions {
		def := &_activityDefinitions[i]
		d.text(def.ID)
		d.int(int64(def.Version))
	}
	part := period{start: from, end: to}
	for i := range s.events {
		e := &s.events[i]
		if part.has(e.at) || part.has(e.observed) {
			d.id(e.id)
			d.id(e.connection)
			d.text(e.actor)
		}
	}
	for i := range s.history {
		h := &s.history[i]
		if part.has(h.CheckedAt) {
			d.bytes(activityEvidence(h))
		}
	}
	for _, links := range s.links {
		if part.has(links.opened) {
			d.id(links.mr)
			d.time(links.opened)
			d.bytes(activityEvidence(links.pairs))
		}
	}
	for _, p := range s.periods() {
		if !part.has(p.start) {
			continue
		}
		d.time(p.end)
		d.bytes(activityEvidence(s.statesAt(p.end)))
	}
	return d.sum()
}

func (s *activitySource) stored(ps []period, hash []byte, from, to time.Time) *entity.StoredPart {
	out := &entity.StoredPart{InputHash: hash, MetricSet: entity.MetricSetActivity, From: from, To: to}
	for _, p := range ps {
		out.Points = append(out.Points, s.deliveryPoints(p)...)
		out.Points = append(out.Points, s.historyPoints(p)...)
		out.Points = append(out.Points, s.freshnessPoints(p)...)
		out.Points = append(out.Points, s.actorPoints(p)...)
		out.Points = append(out.Points, s.linkPoints(p)...)
	}
	return out
}

func activityEvidence(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func activityPoint(id, est, unit string, p entity.Point) entity.SeriesPoint {
	return entity.SeriesPoint{MetricID: id, Estimator: est, Unit: unit, Point: p}
}
