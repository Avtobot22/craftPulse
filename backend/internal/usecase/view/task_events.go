package view

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _listTaskEvents = "listTaskEvents"

func (u *UseCase) ListTaskEvents(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID, q entity.TaskEventQuery) (entity.TaskEventPage, error) {
	if q.Limit < 1 || q.Limit > 200 {
		return entity.TaskEventPage{}, entity.ErrInvalidCursor
	}
	org := s.OrganizationID
	var after *entity.TaskViewCursor
	if q.After != nil {
		after = &entity.TaskViewCursor{AsOf: q.After.AsOf, Knowledge: q.After.Knowledge}
	}
	cut, err := u.taskCut(ctx, org, after)
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	visible, ends, err := u.codeScope(ctx, s, scope, _listTaskEvents, []entity.ID{id})
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	end := ends[id]
	if end.Kind != entity.ObjectTask {
		return entity.TaskEventPage{}, entity.ErrNotFound
	}
	facts, err := u.d.Tasks.Facts.VisibleTaskFacts(ctx, visible, org, end, cut.seq)
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	if len(facts.Tasks) == 0 {
		return entity.TaskEventPage{}, entity.ErrNotFound
	}

	reader := taskRead{uc: u, org: org, cut: cut, instances: make(map[entity.ID]entity.SourceInstance)}
	events, err := reader.trackerEvents(ctx, end, &facts)
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	collection, err := u.d.Tasks.Collection.AreaCollection(ctx, org, []entity.ID{end.AreaID})
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	page := entity.TaskEventPage{Items: events, Notes: visible.Notes()}
	addHistoryBounds(&page, collection)
	if err := reader.relatedEvents(ctx, s, scope, id, &page); err != nil {
		return entity.TaskEventPage{}, err
	}
	selectTaskEvents(&page, cut, q)
	return page, nil
}

func (r *taskRead) relatedEvents(ctx context.Context, s entity.Subject, scope entity.Scope, task entity.ID, page *entity.TaskEventPage) error {
	u := r.uc
	links, err := u.taskMRs(ctx, s, scope, _listTaskEvents, []entity.ID{task}, r.cut.seq, true)
	if err != nil || len(links.edges) == 0 {
		return err
	}
	noteHiddenHistory(page, links.hidden, links.scope.Notes())
	var mrs, areas []entity.ID
	for _, id := range links.confirmed {
		if slices.Contains(mrs, id) {
			continue
		}
		end := links.ends[id]
		facts, err := u.d.Tasks.Facts.TaskMRFacts(ctx, links.scope, r.org, end, r.cut.seq)
		if errors.Is(err, entity.ErrNotFound) {
			noteHiddenHistory(page, true, nil)
			continue
		}
		if err != nil {
			return err
		}
		events, err := r.mergeRequestEvents(ctx, links.scope, end, &facts)
		if err != nil {
			return err
		}
		page.Items = append(page.Items, events...)
		mrs = append(mrs, id)
		areas = addHistoryArea(areas, end.AreaID)
	}
	if len(mrs) == 0 {
		return nil
	}
	collection, err := u.d.Tasks.Collection.MRCollection(ctx, r.org, areas)
	if err != nil {
		return err
	}
	addHistoryBounds(page, collection)
	return r.relatedPipelineEvents(ctx, s, scope, mrs, page)
}

func (r *taskRead) relatedPipelineEvents(ctx context.Context, s entity.Subject, scope entity.Scope, mrs []entity.ID, page *entity.TaskEventPage) error {
	u := r.uc
	edges, err := u.d.CodeLinks.MREdgesAt(ctx, r.org, mrs, r.cut.seq)
	if err != nil {
		return err
	}
	edges = pipelineEdges(edges, mrs)
	if len(edges) == 0 {
		return nil
	}
	allowed, ends, err := u.taskRunEnds(ctx, s, scope, _listTaskEvents, edges)
	if err != nil {
		return err
	}
	noteHiddenHistory(page, false, allowed.Notes())
	seen := make(map[entity.ID]bool, len(edges))
	var areas []entity.ID
	for i := range edges {
		id := edges[i].To
		if seen[id] {
			continue
		}
		seen[id] = true
		end := ends[id]
		if end.Kind != entity.ObjectCIRun {
			noteHiddenHistory(page, true, nil)
			continue
		}
		run, err := u.d.Code.RunAt(ctx, allowed, r.org, id, r.cut.seq)
		if errors.Is(err, entity.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		events, err := r.pipelineHistory(ctx, allowed, end, &run)
		if err != nil {
			return err
		}
		page.Items = append(page.Items, events...)
		areas = addHistoryArea(areas, end.AreaID)
	}
	if len(areas) == 0 {
		return nil
	}
	collection, err := u.d.Tasks.Collection.CICollection(ctx, r.org, areas)
	if err != nil {
		return err
	}
	addHistoryBounds(page, collection)
	return nil
}

func addHistoryArea(areas []entity.ID, id entity.ID) []entity.ID {
	if !slices.Contains(areas, id) {
		return append(areas, id)
	}
	return areas
}

func addHistoryBounds(page *entity.TaskEventPage, collection []entity.AreaCollection) {
	for i := range collection {
		bound := collection[i].HistoryFrom
		if bound != nil && (page.HistoryFrom == nil || bound.After(*page.HistoryFrom)) {
			page.HistoryFrom = bound
		}
	}
}

func noteHiddenHistory(page *entity.TaskEventPage, hidden bool, notes []entity.AccessNote) {
	if hidden && len(notes) == 0 {
		notes = []entity.AccessNote{{Code: entity.NoteRestricted}}
	}
	for _, note := range notes {
		if !slices.Contains(page.Notes, note) {
			page.Notes = append(page.Notes, note)
		}
	}
}

func compareHistoryPosition(aTime time.Time, aID entity.ID, bTime time.Time, bID entity.ID) int {
	return cmp.Or(bTime.Compare(aTime), bytes.Compare(aID[:], bID[:]))
}

func selectTaskEvents(page *entity.TaskEventPage, cut taskCut, q entity.TaskEventQuery) {
	page.Items = slices.DeleteFunc(page.Items, func(e entity.TaskHistoryEvent) bool {
		return e.EventTime.After(cut.asOf) ||
			(q.After != nil && compareHistoryPosition(e.EventTime, e.ID, q.After.EventTime, q.After.ID) <= 0)
	})
	slices.SortFunc(page.Items, func(a, b entity.TaskHistoryEvent) int {
		return compareHistoryPosition(a.EventTime, a.ID, b.EventTime, b.ID)
	})
	if len(page.Items) > q.Limit {
		last := &page.Items[q.Limit-1]
		page.Next = &entity.TaskEventCursor{AsOf: cut.asOf, Knowledge: cut.seq, EventTime: last.EventTime, ID: last.ID}
		page.Items = page.Items[:q.Limit]
	}
}
