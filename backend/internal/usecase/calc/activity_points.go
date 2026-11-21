package calc

import (
	"cmp"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const (
	_activityCut             = "history_truncated"
	_activityNotChecked      = "history_not_checked"
	_activitySourceMismatch  = "source_object_mismatch"
	_activityActorMissing    = "actor_missing"
	_activityTimeMissing     = "event_time_missing"
	_activityNegativeLag     = "observed_before_event"
	_activityCollectionError = "source_collection_failed"
)

type eventEvidence struct {
	EventID    entity.ID          `json:"event_id"`
	AccountID  *entity.ID         `json:"account_id,omitempty"`
	Connection entity.ID          `json:"connection_id,omitzero"`
	ActorKind  string             `json:"actor_kind"`
	Mode       entity.ObservedVia `json:"observed_via"`
	EventTime  time.Time          `json:"event_time"`
	ObservedAt time.Time          `json:"observed_at"`
}

func (e *activityEvent) evidence() []byte {
	return activityEvidence(eventEvidence{e.id, e.account, e.connection, e.actor, e.via, e.at, e.observed})
}

// activityTally keeps every event traceable to its object even when multiple events have the same actor.
type activityTally struct {
	*tally
	items map[entity.ID]int
}

func newActivityTally(p period) activityTally {
	return activityTally{newTally(p), make(map[entity.ID]int)}
}

func (t activityTally) addObject(id entity.ID, kind entity.ObjectKind, role entity.ContributionRole, value *float64, reason string, evidence []byte) {
	t.kind = kind
	t.items[id]++
	t.add(id, t.items[id], role, value, reason)
	t.contributions[len(t.contributions)-1].Evidence = evidence
}

func (s *activitySource) deliveryPoints(p period) []entity.SeriesPoint {
	type sample struct {
		tally  activityTally
		values []float64
	}
	samples := make(map[entity.ObservedVia]*sample, len(_observationModes))
	for _, mode := range _observationModes {
		samples[mode] = &sample{tally: newActivityTally(p)}
	}
	for i := range s.events {
		e := &s.events[i]
		sample := samples[e.via]
		if sample == nil || !p.has(e.observed) {
			continue
		}
		role, reason := entity.RoleNumerator, ""
		var value *float64
		switch {
		case e.at.IsZero():
			role, reason = entity.RoleExcluded, _activityTimeMissing
		case e.observed.Before(e.at):
			role, reason = entity.RoleExcluded, _activityNegativeLag
		default:
			delay := e.observed.Sub(e.at).Seconds()
			sample.values = append(sample.values, delay)
			value = num(delay)
		}
		sample.tally.addObject(e.object, e.kind, role, value, reason, e.evidence())
	}
	estimators := []string{EstimatorP50, _estimatorP95}
	var out []entity.SeriesPoint
	for _, mode := range _observationModes {
		sample := samples[mode]
		points := sample.tally.distribution(sample.values, estimators)
		for i := range points {
			out = append(out, activityPoint("A23", string(mode)+":"+estimators[i], UnitSeconds, points[i]))
		}
	}
	return out
}

func (s *activitySource) historyPoints(p period) []entity.SeriesPoint {
	t, bounds := newActivityTally(p), newActivityTally(p)
	checked, complete := 0, 0
	for i := range s.history {
		h := &s.history[i]
		if !p.has(h.CheckedAt) {
			continue
		}
		b := activityEvidence(h)
		if h.Checked {
			checked++
			t.addObject(h.ObjectID, h.Kind, entity.RoleDenominator, nil, "", b)
		}
		if h.Complete {
			complete++
			t.addObject(h.ObjectID, h.Kind, entity.RoleNumerator, num(1), "", b)
		} else {
			reason := entity.ReasonHistoryIncomplete
			switch {
			case h.Truncated:
				reason = _activityCut
			case !h.Checked:
				reason = _activityNotChecked
			case !h.Matches:
				reason = _activitySourceMismatch
			}
			t.addObject(h.ObjectID, h.Kind, entity.RoleExcluded, nil, reason, b)
		}
		if h.HistoryFrom != nil {
			bounds.addObject(h.ObjectID, h.Kind, entity.RoleNumerator, unixSeconds(h.HistoryFrom), "", b)
		} else {
			bounds.addObject(h.ObjectID, h.Kind, entity.RoleMissing, nil, entity.ReasonHistoryIncomplete, b)
		}
	}
	pt := t.ratio(float64(complete), float64(checked), checked, false)
	if checked == 0 && pt.ExcludedCount > 0 {
		pt.State, pt.ReasonCode = entity.StateUnknown, _activityNotChecked
	}
	return []entity.SeriesPoint{
		activityPoint("A24", "", UnitRatio, pt),
		activityPoint("A24", _historyFrom, UnitSeconds, earliestActivityPoint(bounds.tally)),
	}
}

func (s *activitySource) actorPoints(p period) []entity.SeriesPoint {
	t := newActivityTally(p)
	kinds := []string{ActorHuman, ActorBot, ActorSystem, ActorUnknown}
	byKind := make(map[string]activityTally, len(kinds))
	counts := make(map[string]int, len(kinds))
	for _, kind := range kinds {
		byKind[kind] = newActivityTally(p)
	}
	withAccount, known := 0, 0
	for i := range s.events {
		e := &s.events[i]
		if !p.has(e.at) {
			continue
		}
		b := e.evidence()
		if e.account == nil || e.account.IsZero() {
			t.addObject(e.object, e.kind, entity.RoleExcluded, nil, _activityActorMissing, b)
			continue
		}
		kind := ActorUnknown
		switch e.actor {
		case ActorHuman, ActorBot, ActorSystem:
			kind = e.actor
		}
		withAccount++
		t.addObject(e.object, e.kind, entity.RoleDenominator, nil, kind, b)
		if kind != ActorUnknown {
			known++
			t.addObject(e.object, e.kind, entity.RoleNumerator, num(1), kind, b)
		}
		byKind[kind].addObject(e.object, e.kind, entity.RoleNumerator, num(1), kind, b)
		counts[kind]++
	}
	out := make([]entity.SeriesPoint, 0, 1+len(kinds))
	out = append(out, activityPoint("A28", "", UnitRatio, t.ratio(float64(known), float64(withAccount), withAccount, false)))
	for _, kind := range kinds {
		point := byKind[kind].point()
		n := counts[kind]
		point.State, point.Value, point.N, point.LowSample = entity.StateAssessed, num(float64(n)), &n, n < _lowSample
		out = append(out, activityPoint("A28", kind, UnitCount, point))
	}
	return out
}

type activityStateKey struct {
	connection entity.ID
	kind       string
}

// statesAt rebuilds old points from reconciliation history and uses the captured Status fields for the live point.
func (s *activitySource) statesAt(at time.Time) []entity.AreaCollection {
	byKind := make(map[activityStateKey]*entity.ReconcileRun)
	byConnection := make(map[entity.ID]*entity.ReconcileRun)
	successful := make(map[activityStateKey]*entity.ReconcileRun)
	for i := range s.runs {
		run := s.runs[i]
		if run.StartedAt.After(at) {
			break
		}
		if run.FinishedAt != nil && run.FinishedAt.After(at) {
			run.FinishedAt, run.Outcome, run.ReasonCode = nil, entity.ReconcileRunning, ""
			run.ExpectedCount, run.ObservedCount, run.MissingCount, run.ChangedCount = nil, nil, nil, nil
		}
		key := activityStateKey{run.ConnectionID, run.DataKind}
		byKind[key], byConnection[run.ConnectionID] = &run, &run
		if run.Outcome == entity.ReconcileComplete {
			successful[key] = &run
		}
	}
	out := make([]entity.AreaCollection, 0, len(s.freshness))
	for i := range s.freshness {
		item := s.freshness[i]
		key := activityStateKey{item.ConnectionID, item.DataKind}
		if !at.Equal(s.asOf) || afterActivityTime(item.LastSuccessAt, at) || afterActivityTime(item.Watermark, at) ||
			afterActivityRun(item.Reconcile, at) || afterActivityRun(item.ConnectionReconcile, at) {
			item = entity.AreaCollection{
				ConnectionID: item.ConnectionID, DataKind: item.DataKind,
				Reconcile: byKind[key], ConnectionReconcile: byConnection[item.ConnectionID],
			}
			if ok := successful[key]; ok != nil {
				item.LastSuccessAt, item.Watermark = ok.FinishedAt, &ok.AsOf
			}
		}
		out = append(out, item)
	}
	return out
}

func afterActivityTime(t *time.Time, at time.Time) bool { return t != nil && t.After(at) }

func afterActivityRun(run *entity.ReconcileRun, at time.Time) bool {
	return run != nil && (run.StartedAt.After(at) || afterActivityTime(run.FinishedAt, at))
}

func (s *activitySource) freshnessPoints(p period) []entity.SeriesPoint {
	states := s.statesAt(p.end)
	estimators := freshnessEstimators()
	out := make([]entity.SeriesPoint, 0, len(estimators))
	for _, est := range estimators {
		kind, metric := activityFreshnessKind(est)
		t := newActivityTally(p)
		connections := make(map[entity.ID]bool)
		failed := false
		for i := range states {
			item := &states[i]
			if kind != "" && item.DataKind != kind || connections[item.ConnectionID] {
				continue
			}
			connections[item.ConnectionID] = true
			var value *time.Time
			reason := ""
			switch metric {
			case _lastSuccessAt:
				value = item.LastSuccessAt
			case _watermark:
				value = item.Watermark
			case _lastReconcileAt:
				run := item.Reconcile
				if kind == "" {
					run = item.ConnectionReconcile
				}
				reason = entity.ReasonReconcileMissing
				if run != nil {
					value, reason = run.FinishedAt, entity.ReasonReconcileUnfinished
					if value != nil {
						reason = run.Outcome
					}
				}
			}
			role := entity.RoleNumerator
			if value == nil {
				role, reason = entity.RoleMissing, cmp.Or(reason, entity.ReasonWatermarkMissing)
			}
			if item.LastErrorCode != "" || item.Reconcile != nil && item.Reconcile.Outcome == entity.ReconcileFailed {
				failed = true
				if value != nil {
					reason = _activityCollectionError
				}
			}
			t.addObject(item.ConnectionID, entity.ObjectConnection, role, unixSeconds(value), reason, activityEvidence(item))
		}
		pt := earliestActivityPoint(t.tally)
		if failed && pt.Value != nil {
			pt.State, pt.ReasonCode = entity.StateProvisional, _activityCollectionError
		}
		out = append(out, activityPoint("A27", est, UnitSeconds, pt))
	}
	return out
}

func earliestActivityPoint(t *tally) entity.Point {
	pt := t.point()
	var earliest *float64
	n := 0
	for _, c := range pt.Contributions {
		if c.Role != entity.RoleNumerator {
			continue
		}
		n++
		if earliest == nil || *c.Value < *earliest {
			earliest = c.Value
		}
	}
	pt.N, pt.LowSample = &n, n < _lowSample
	switch {
	case len(pt.Contributions) == 0:
		pt.State = entity.StateNotApplicable
	case earliest == nil:
		pt.State, pt.ReasonCode = entity.StateUnknown, t.reason
	case pt.MissingCount > 0:
		pt.State, pt.ReasonCode, pt.Value = entity.StateProvisional, t.reason, earliest
	default:
		pt.State, pt.Value = entity.StateAssessed, earliest
	}
	return pt
}

func unixSeconds(at *time.Time) *float64 {
	if at == nil {
		return nil
	}
	return num(float64(at.UnixMicro()) / 1e6)
}

func activityFreshnessKind(est string) (kind, metric string) {
	for _, kind := range _activityDataKinds {
		if len(est) > len(kind) && est[:len(kind)] == kind && est[len(kind)] == ':' {
			return kind, est[len(kind)+1:]
		}
	}
	return "", est
}

type activityLinks struct {
	mr     entity.ID
	opened time.Time
	pairs  []activityPair
}

type activityPair struct {
	OtherID   entity.ID         `json:"other_object_id"`
	Type      entity.LinkType   `json:"type"`
	Status    entity.LinkStatus `json:"status"`
	EdgeIDs   []entity.ID       `json:"edge_ids"`
	Ambiguous bool              `json:"ambiguous,omitempty"`
}

func activityLinksOf(mrs map[entity.ID]time.Time, ids []entity.ID, edges []entity.LinkEdge) []activityLinks {
	byMR := make(map[entity.ID]map[entity.ID]activityPair)
	for i := range edges {
		edge := &edges[i]
		mr, other := edge.To, edge.From
		if edge.Type == entity.LinkMRRun {
			mr, other = edge.From, edge.To
		}
		if _, ok := mrs[mr]; !ok {
			continue
		}
		if byMR[mr] == nil {
			byMR[mr] = make(map[entity.ID]activityPair)
		}
		pair := byMR[mr][other]
		pair.OtherID, pair.Type, pair.EdgeIDs = other, edge.Type, append(pair.EdgeIDs, edge.ID)
		if pair.Status == "" || edge.Status == entity.LinkConfirmed || edge.Status == entity.LinkCandidate && pair.Status == entity.LinkRejected {
			pair.Status = edge.Status
		}
		byMR[mr][other] = pair
	}
	out := make([]activityLinks, 0, len(byMR))
	for _, id := range ids {
		pairs := byMR[id]
		if len(pairs) == 0 {
			continue
		}
		possible := 0
		for _, pair := range pairs {
			if pair.Type == entity.LinkTaskMR && (pair.Status == entity.LinkCandidate || pair.Status == entity.LinkConfirmed) {
				possible++
			}
		}
		group := activityLinks{mr: id, opened: mrs[id]}
		for _, pair := range pairs {
			pair.Ambiguous = pair.Type == entity.LinkTaskMR && pair.Status == entity.LinkCandidate && possible > 1
			slices.SortFunc(pair.EdgeIDs, compareActivityIDs)
			group.pairs = append(group.pairs, pair)
		}
		slices.SortFunc(group.pairs, func(a, b activityPair) int { return compareActivityIDs(a.OtherID, b.OtherID) })
		out = append(out, group)
	}
	return out
}

func (s *activitySource) linkPoints(p period) []entity.SeriesPoint {
	estimators := definition("A30").Estimators
	tallies := make(map[string]activityTally, len(estimators))
	counts := make(map[string]int, len(estimators))
	for _, est := range estimators {
		tallies[est] = newActivityTally(p)
	}
	share := tallies["ambiguous_share"]
	for _, group := range s.links {
		if !p.has(group.opened) {
			continue
		}
		for _, pair := range group.pairs {
			b := activityEvidence(pair)
			status := string(pair.Status)
			if pair.Status == entity.LinkCandidate {
				status = "proposed"
				kind := _unambiguous
				if pair.Ambiguous {
					kind = _ambiguous
					share.addObject(group.mr, entity.ObjectMergeRequest, entity.RoleNumerator, num(1), kind, b)
				} else {
					share.addObject(group.mr, entity.ObjectMergeRequest, entity.RoleExcluded, nil, kind, b)
				}
				share.addObject(group.mr, entity.ObjectMergeRequest, entity.RoleDenominator, nil, kind, b)
				tallies[kind].addObject(group.mr, entity.ObjectMergeRequest, entity.RoleNumerator, num(1), kind, b)
				counts[kind]++
			} else {
				share.addObject(group.mr, entity.ObjectMergeRequest, entity.RoleExcluded, nil, status, b)
			}
			tallies[status].addObject(group.mr, entity.ObjectMergeRequest, entity.RoleNumerator, num(1), status, b)
			counts[status]++
		}
	}
	var out []entity.SeriesPoint
	for _, est := range estimators {
		if est == "ambiguous_share" {
			n := counts["proposed"]
			out = append(out, activityPoint("A30", est, UnitRatio, share.ratio(float64(counts[_ambiguous]), float64(n), n, false)))
			continue
		}
		pt := tallies[est].point()
		n := counts[est]
		pt.State, pt.Value, pt.N, pt.LowSample = entity.StateAssessed, num(float64(n)), &n, n < _lowSample
		out = append(out, activityPoint("A30", est, UnitCount, pt))
	}
	return out
}
