package calc

import (
	"bytes"
	"cmp"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

// TaskObservationRule adds observations using accessible facts on the same cut; the delay question is added last.
type TaskObservationRule func(*entity.TaskObservationFacts) []entity.TaskObservation

// TaskObservations evaluates the current episode without inferring a cause from candidates or missing facts.
func TaskObservations(facts *entity.TaskObservationFacts, rules ...TaskObservationRule) []entity.TaskObservation {
	task := observationRef(facts, entity.ObjectTask, facts.Task.TaskID)
	taskRef := []entity.ObjectRef{task}
	ep, group := observationEpisode(facts)
	out := taskWorkflowObservations(facts, ep, group, taskRef)
	mrs, runIDs, candidate := observationLinks(facts)
	checked := []entity.ObjectRef{task}
	if ep != nil {
		ci := observationCI(facts, runIDs)
		heads := observationRevisions(facts, mrs)
		reviews := observationReviews(facts)
		for i := range reviews {
			m := &reviews[i]
			if !mrs[m.version.MRID] || m.version.OpenedAt.After(facts.AsOf) {
				continue
			}
			ref := observationRef(facts, entity.ObjectMergeRequest, m.version.MRID)
			checked = append(checked, ref)
			if o, ok := taskReviewObservation(m, ref, facts.AsOf); ok {
				out = append(out, o)
			}
			if result := ci.forMR(&m.version, heads[m.version.MRID]); result != nil {
				ref := observationRef(facts, entity.ObjectCIRun, result.version.RunID)
				checked = append(checked, ref)
				if o, ok := taskPipelineObservation(facts, &ci, result, ref); ok {
					out = append(out, o)
				}
			}
		}
		if len(mrs) == 0 && !facts.TaskMRsUnavailable {
			out = append(out, noMRObservation(ep, taskRef, candidate))
		}
	}
	for _, rule := range rules {
		out = append(out, rule(facts)...)
	}
	if len(out) == 0 {
		out = append(out, entity.TaskObservation{
			Kind: entity.ObservationUnknown, Since: episodeStart(ep), Basis: checked,
			Action: entity.ObservationClarifyReason, Question: entity.QuestionDelayReason,
			SourceEventIDs: episodeEvents(ep),
		})
	}
	slices.SortStableFunc(out, compareObservation)
	return out
}

func observationEpisode(f *entity.TaskObservationFacts) (*episode, *entity.TargetGroup) {
	if f.Mapping == nil || f.Mapping.KnownSeq > f.KnowledgeSeq || !knownOnCut(f.Task.Knowledge, f.KnowledgeSeq) {
		return nil, nil
	}
	rs := rulesOf(f.Mapping)[f.InstanceID]
	if rs == nil {
		return nil, nil
	}
	var events []entity.TaskEvent
	for i := range f.TaskEvents {
		e := &f.TaskEvents[i]
		if e.TaskID == f.Task.TaskID && knownOnCut(e.Knowledge, f.KnowledgeSeq) {
			events = append(events, *e)
		}
	}
	tl := buildTimeline(&taskInput{version: f.Task, events: events, historyFrom: f.HistoryFrom}, rs, f.AsOf)
	if !tl.knownAt(f.AsOf) {
		return nil, nil
	}
	if tl.slice.TaskType == nil || tl.slice.SizeLabel == nil {
		return tl.episodeAt(f.AsOf), nil
	}
	return tl.episodeAt(f.AsOf), &entity.TargetGroup{TaskType: *tl.slice.TaskType, SizeLabel: *tl.slice.SizeLabel}
}

func taskWorkflowObservations(f *entity.TaskObservationFacts, ep *episode, group *entity.TargetGroup, task []entity.ObjectRef) []entity.TaskObservation {
	if ep == nil {
		return nil
	}
	var out []entity.TaskObservation
	last := ep.segments[len(ep.segments)-1]
	if last.class == entity.IntervalBlocked {
		var since *time.Time
		if ep.startKnown || len(last.events) != 0 {
			since = utc(&last.from)
		}
		out = append(out, entity.TaskObservation{
			Kind: entity.ObservationBlocked, Since: since, Basis: task, Action: entity.ObservationResolveBlocker,
			SourceEventIDs: slices.Clone(last.events),
		})
	}
	if target := observationTarget(f, group); target != nil && ep.startKnown && f.AsOf.Sub(ep.start).Seconds() > float64(target.DurationSeconds) {
		since := ep.start.Add(time.Duration(target.DurationSeconds) * time.Second).UTC()
		out = append(out, entity.TaskObservation{
			Kind: entity.ObservationOverTarget, Since: &since, Basis: task, Action: entity.ObservationReviewTarget,
			SourceEventIDs: episodeEvents(ep), TargetVersion: target,
		})
	}
	return out
}

func observationTarget(f *entity.TaskObservationFacts, group *entity.TargetGroup) *entity.TargetVersion {
	if group == nil {
		return nil
	}
	var best *entity.TargetVersion
	for i := range f.Targets {
		v := &f.Targets[i]
		if v.TargetGroup == *group && v.TeamID == f.Mapping.TeamID && v.KnownSeq <= f.KnowledgeSeq && (best == nil || v.VersionNo > best.VersionNo) {
			best = v
		}
	}
	if best == nil {
		return nil
	}
	v := *best
	return &v
}

func observationLinks(f *entity.TaskObservationFacts) (mrs map[entity.ID]bool, runs map[entity.ID]entity.ID, candidate bool) {
	mrs, runs = map[entity.ID]bool{}, map[entity.ID]entity.ID{}
	for i := range f.Links {
		edge := &f.Links[i]
		if edge.Type != entity.LinkTaskMR || edge.From != f.Task.TaskID || !knownOnCut(edge.Knowledge, f.KnowledgeSeq) {
			continue
		}
		candidate = candidate || edge.Status == entity.LinkCandidate
		if edge.Status == entity.LinkConfirmed {
			mrs[edge.To] = true
		}
	}
	for i := range f.Links {
		edge := &f.Links[i]
		if edge.Type == entity.LinkMRRun && mrs[edge.From] && edge.Status == entity.LinkConfirmed && knownOnCut(edge.Knowledge, f.KnowledgeSeq) {
			runs[edge.To] = edge.From
		}
	}
	return mrs, runs, candidate
}

func observationReviews(f *entity.TaskObservationFacts) []reviewMR {
	mrs := reviewInputOf(f.Reviews, incompleteAreas(f.ReviewCollection))
	out := mrs[:0]
	for i := range mrs {
		mr := &mrs[i]
		if !knownOnCut(mr.version.Knowledge, f.KnowledgeSeq) {
			continue
		}
		mr.events = slices.DeleteFunc(mr.events, func(e entity.MREvent) bool {
			return !knownOnCut(e.Knowledge, f.KnowledgeSeq) || e.EventTime.After(f.AsOf)
		})
		slices.SortFunc(mr.events, func(a, b entity.MREvent) int {
			return cmp.Or(a.EventTime.Compare(b.EventTime), bytes.Compare(a.ID[:], b.ID[:]))
		})
		out = append(out, *mr)
	}
	return out
}

func taskReviewObservation(m *reviewMR, ref entity.ObjectRef, at time.Time) (entity.TaskObservation, bool) {
	if !readyOpen(&m.version, m.events, at) || m.version.Draft {
		return entity.TaskObservation{}, false
	}
	wait := m.wait(at)
	if wait.Kind == entity.ReviewResponded {
		return entity.TaskObservation{}, false
	}
	o := entity.TaskObservation{Since: utc(&wait.Since), Basis: []entity.ObjectRef{ref}}
	for i := range m.events {
		e := &m.events[i]
		if (e.Kind == _eventReady && e.EventTime.Equal(wait.Since)) || e.Kind == _eventAssigned || e.Kind == _eventRemoved {
			o.SourceEventIDs = append(o.SourceEventIDs, e.ID)
		}
	}
	reviewersKnown := snapshotKnownAt(m.version.SourceUpdatedAt, m.version.ObservedAt, at) || hasReviewerEvents(m.events)
	switch {
	case !m.version.ReviewerField || !reviewersKnown:
		o.Kind, o.Action, o.Question = entity.ObservationUnknown, entity.ObservationClarifyReason, entity.QuestionReviewerAssigned
	case wait.Kind == entity.ReviewNoReviewer:
		o.Kind, o.Action = entity.ObservationAwaitingReviewer, entity.ObservationAssignReviewer
	case wait.Kind == entity.ReviewAwaitingFirst:
		o.Kind, o.Action = entity.ObservationAwaitingFirst, entity.ObservationRequestReview
	default:
		return entity.TaskObservation{}, false
	}
	return o, true
}

func hasReviewerEvents(events []entity.MREvent) bool {
	return slices.ContainsFunc(events, func(e entity.MREvent) bool {
		return e.Kind == _eventAssigned || e.Kind == _eventRemoved
	})
}

func observationRevisions(f *entity.TaskObservationFacts, mrs map[entity.ID]bool) map[entity.ID]*entity.MRRevision {
	heads := map[entity.ID]*entity.MRRevision{}
	for i := range f.MRRevisions {
		r := &f.MRRevisions[i]
		if !mrs[r.MRID] || r.CreatedAt.IsZero() || r.CreatedAt.After(f.AsOf) || !knownOnCut(r.Knowledge, f.KnowledgeSeq) {
			continue
		}
		prev := heads[r.MRID]
		if prev == nil || r.CreatedAt.After(prev.CreatedAt) || r.CreatedAt.Equal(prev.CreatedAt) && bytes.Compare(r.ID[:], prev.ID[:]) > 0 {
			heads[r.MRID] = r
		}
	}
	return heads
}

func snapshotKnownAt(updated *time.Time, observed, at time.Time) bool {
	if updated != nil {
		return !updated.After(at)
	}
	return !observed.After(at)
}

func noMRObservation(ep *episode, task []entity.ObjectRef, candidate bool) entity.TaskObservation {
	o := entity.TaskObservation{Kind: entity.ObservationNoMR, Since: episodeStart(ep), Basis: task, SourceEventIDs: episodeEvents(ep)}
	if candidate {
		o.Action = entity.ObservationLinkMR
	} else {
		o.Action, o.Question = entity.ObservationClarifyReason, entity.QuestionMRExists
	}
	return o
}

func episodeStart(ep *episode) *time.Time {
	if ep == nil || !ep.startKnown {
		return nil
	}
	return utc(&ep.start)
}

func episodeEvents(ep *episode) []entity.ID {
	if ep == nil {
		return nil
	}
	return slices.Clone(ep.segments[0].events)
}

func knownOnCut(k entity.Knowledge, seq int64) bool {
	return k.KnownSeq <= seq && (k.SupersededSeq == nil || seq < *k.SupersededSeq)
}

func observationRef(f *entity.TaskObservationFacts, kind entity.ObjectKind, id entity.ID) entity.ObjectRef {
	ref := f.Refs[id]
	ref.Kind, ref.ID = kind, id
	return ref
}

func compareObservation(a, b entity.TaskObservation) int {
	var at, bt time.Time
	if a.Since != nil {
		at = *a.Since
	}
	if b.Since != nil {
		bt = *b.Since
	}
	switch {
	case a.Since == nil && b.Since != nil:
		return 1
	case b.Since == nil && a.Since != nil:
		return -1
	}
	return cmp.Or(at.Compare(bt), cmp.Compare(a.Kind, b.Kind))
}
