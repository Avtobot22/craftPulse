package view

import (
	"context"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _listProjectTimeline = "listProjectTimeline"

func (u *UseCase) ListProjectTimeline(ctx context.Context, s entity.Subject, scope entity.Scope, project entity.ID, q entity.ProjectTimelineQuery) (entity.TaskEventPage, error) {
	if project.IsZero() || !slices.Contains(scope.Projects(), project) {
		return entity.TaskEventPage{}, entity.ErrNotFound
	}
	if q.Limit < 1 || q.Limit > 200 || !q.From.Before(q.To) {
		return entity.TaskEventPage{}, entity.ErrInvalidCursor
	}
	var after *entity.TaskViewCursor
	if q.After != nil {
		after = &entity.TaskViewCursor{AsOf: q.After.AsOf, Knowledge: q.After.Knowledge}
	}
	org := s.OrganizationID
	cut, err := u.taskCut(ctx, org, after)
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	structure, err := u.taskStructure(ctx, org, scope, cut.asOf)
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	if q.RevisionID != nil {
		if err := u.timelineRevision(ctx, org, structure.teams[project], &q, &cut); err != nil {
			return entity.TaskEventPage{}, err
		}
	}

	var areas []entity.ID
	for id, projects := range structure.projects {
		if slices.Contains(projects, project) {
			areas = append(areas, id)
		}
	}
	visible, err := u.d.SourceRights.Decide(ctx, s, scope, entity.SourceQuery{Operation: _listProjectTimeline, Areas: areas})
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	page := entity.TaskEventPage{}
	noteHiddenHistory(&page, slices.ContainsFunc(areas, func(id entity.ID) bool { return !visible.Area(id) }), visible.Notes())
	areas = slices.DeleteFunc(areas, func(id entity.ID) bool { return !visible.Area(id) })
	visible, err = u.timelineObjectScope(ctx, s, visible, _listProjectTimeline, areas, q.ObjectKind)
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	noteHiddenHistory(&page, false, visible.Notes())

	reader := taskRead{uc: u, org: org, cut: cut, instances: make(map[entity.ID]entity.SourceInstance)}
	if err := reader.projectEvents(ctx, visible, areas, &q, &page); err != nil {
		return entity.TaskEventPage{}, err
	}
	selectTaskEvents(&page, cut, q.TaskEventQuery)
	if err := reader.timelineSources(ctx, areas, &page); err != nil {
		return entity.TaskEventPage{}, err
	}
	// An area may also be linked to a different project. Use only this project's teams.
	for _, area := range areas {
		structure.projects[area] = []entity.ID{project}
	}
	return page, reader.markLate(ctx, structure, page.Items)
}

func (u *UseCase) timelineRevision(ctx context.Context, org entity.ID, teams []entity.ID, q *entity.ProjectTimelineQuery, cut *taskCut) error {
	rev, err := u.d.Tasks.Revisions.TimelineRevision(ctx, org, *q.RevisionID, teams)
	if err != nil {
		return err
	}
	if q.After != nil && q.After.Knowledge != rev.KnowledgeSeq {
		return entity.ErrInvalidCursor
	}
	cut.seq = rev.KnowledgeSeq
	return nil
}

func (u *UseCase) timelineObjectScope(ctx context.Context, s entity.Subject, scope entity.Scope, op string, areas []entity.ID,
	kind entity.ObjectKind,
) (entity.Scope, error) {
	if len(areas) == 0 {
		return scope, nil
	}
	objects, err := u.d.Code.RestrictedObjects(ctx, scope, s.OrganizationID, areas)
	if err != nil {
		return entity.Scope{}, err
	}
	objects = slices.DeleteFunc(objects, func(o entity.RestrictedObject) bool {
		return kind != "" && o.Kind != kind && (kind != entity.ObjectCIRun || o.Kind != entity.ObjectCIAttempt)
	})
	if len(objects) == 0 {
		return scope, nil
	}
	return u.d.SourceRights.Decide(ctx, s, scope, entity.SourceQuery{Operation: op, Objects: objects})
}

func (r *taskRead) projectEvents(ctx context.Context, scope entity.Scope, areas []entity.ID, q *entity.ProjectTimelineQuery, page *entity.TaskEventPage) error {
	for _, area := range areas {
		events, err := r.projectAreaEvents(ctx, scope, area, q)
		if err != nil {
			return err
		}
		page.Items = append(page.Items, events...)
	}
	for _, kind := range []entity.ObjectKind{entity.ObjectCIRun, entity.ObjectDocument} {
		if q.ObjectKind != "" && q.ObjectKind != kind {
			continue
		}
		facts, err := r.uc.d.Tasks.Facts.TimelineFacts(ctx, scope, r.org, areas, entity.TimelineFactQuery{
			Kind: kind, Knowledge: r.cut.seq, From: q.From, To: q.To,
		})
		if err != nil {
			return err
		}
		for i := range facts {
			event, err := r.projectedEvent(ctx, scope, &facts[i])
			if err != nil {
				return err
			}
			page.Items = append(page.Items, event)
		}
	}
	page.Items = slices.DeleteFunc(page.Items, func(e entity.TaskHistoryEvent) bool {
		return e.EventTime.Before(q.From) || !e.EventTime.Before(q.To) || q.ActorKind != "" && e.ActorKind != q.ActorKind
	})
	return nil
}

func (r *taskRead) projectAreaEvents(ctx context.Context, scope entity.Scope, area entity.ID, q *entity.ProjectTimelineQuery) ([]entity.TaskHistoryEvent, error) {
	var events []entity.TaskHistoryEvent
	facts := r.uc.d.Tasks.Facts
	if q.ObjectKind == "" || q.ObjectKind == entity.ObjectTask {
		f, err := facts.VisibleAreaFacts(ctx, scope, r.org, area, r.cut.seq)
		if err != nil {
			return nil, err
		}
		f.Events = slices.DeleteFunc(f.Events, func(e entity.TaskEvent) bool {
			return e.EventTime.Before(q.From) || !e.EventTime.Before(q.To)
		})
		if len(f.Events) > 0 {
			events, err = r.trackerEvents(ctx, &f)
			if err != nil {
				return nil, err
			}
		}
	}
	if q.ObjectKind == "" || q.ObjectKind == entity.ObjectMergeRequest {
		f, err := facts.TimelineMRFacts(ctx, scope, r.org, area, r.cut.seq)
		if err != nil {
			return nil, err
		}
		f.MREvents = slices.DeleteFunc(f.MREvents, func(e entity.MREvent) bool {
			return e.EventTime.Before(q.From) || !e.EventTime.Before(q.To)
		})
		if len(f.MREvents) > 0 {
			mrs, err := r.mergeRequestEvents(ctx, scope, entity.LinkEnd{AreaID: area, InstanceID: f.InstanceID}, &f)
			if err != nil {
				return nil, err
			}
			events = append(events, mrs...)
		}
	}
	return events, nil
}

func (r *taskRead) markLate(ctx context.Context, structure taskStructure, items []entity.TaskHistoryEvent) error {
	if len(items) == 0 {
		return nil
	}
	var teams []entity.ID
	byArea := make(map[entity.ID][]entity.ID)
	from, observedBefore := items[0].EventTime, items[0].ObservedAt
	for i := range items {
		event := &items[i]
		if event.EventTime.Before(from) {
			from = event.EventTime
		}
		if event.ObservedAt.After(observedBefore) {
			observedBefore = event.ObservedAt
		}
		if _, ok := byArea[event.AreaID]; ok {
			continue
		}
		for _, project := range structure.projects[event.AreaID] {
			for _, team := range structure.teams[project] {
				byArea[event.AreaID] = addHistoryArea(byArea[event.AreaID], team)
				teams = addHistoryArea(teams, team)
			}
		}
	}
	publications, err := r.uc.d.Tasks.Revisions.TimelinePublications(ctx, r.org, teams, from, observedBefore)
	if err != nil {
		return err
	}
	for i := range items {
		e := &items[i]
		e.Late = slices.ContainsFunc(publications, func(p entity.TimelinePublication) bool {
			return slices.Contains(byArea[e.AreaID], p.TeamID) && p.Knowledge <= r.cut.seq &&
				!e.EventTime.After(p.AsOf) && e.ObservedAt.After(p.PublishedAt)
		})
	}
	return nil
}

func (r *taskRead) timelineSources(ctx context.Context, areas []entity.ID, page *entity.TaskEventPage) error {
	items, err := r.uc.d.Tasks.Collection.TimelineCollection(ctx, r.org, areas)
	if err != nil {
		return err
	}
	index := make(map[entity.ID]int)
	page.Sources = make([]entity.TimelineSource, 0)
	for i := range items {
		item := &items[i]
		pos, ok := index[item.ConnectionID]
		if !ok {
			src, err := r.instance(ctx, item.InstanceID)
			if err != nil {
				return err
			}
			pos = len(page.Sources)
			index[item.ConnectionID] = pos
			page.Sources = append(page.Sources, entity.TimelineSource{
				ConnectionID: item.ConnectionID, Provider: src.Provider, Completeness: "complete",
			})
		}
		source := &page.Sources[pos]
		source.HistoryFrom = laterHistoryBound(source.HistoryFrom, item.HistoryFrom)
		page.HistoryFrom = laterHistoryBound(page.HistoryFrom, item.HistoryFrom)
		switch item.Completeness {
		case "partial":
			source.Completeness = "partial"
		case "complete":
		default:
			if source.Completeness != "partial" {
				source.Completeness = "unknown"
			}
		}
	}
	slices.SortFunc(page.Sources, func(a, b entity.TimelineSource) int {
		return compareHistoryPosition(time.Time{}, a.ConnectionID, time.Time{}, b.ConnectionID)
	})
	return nil
}

func laterHistoryBound(a, b *time.Time) *time.Time {
	if b != nil && (a == nil || b.After(*a)) {
		return b
	}
	return a
}
