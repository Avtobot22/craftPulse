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
	events, err := reader.trackerEvents(ctx, &facts)
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	page := entity.TaskEventPage{Items: events, Notes: visible.Notes()}
	areas, err := reader.relatedEvents(ctx, s, scope, id, &page)
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	areas = addHistoryArea(areas, end.AreaID)
	selectTaskEvents(&page, cut, q)
	if err := reader.timelineSources(ctx, areas, &page); err != nil {
		return entity.TaskEventPage{}, err
	}
	structure, err := u.taskStructure(ctx, org, scope, cut.asOf)
	if err != nil {
		return entity.TaskEventPage{}, err
	}
	return page, reader.markLate(ctx, structure, page.Items)
}

func (r *taskRead) relatedEvents(ctx context.Context, s entity.Subject, scope entity.Scope, task entity.ID, page *entity.TaskEventPage) ([]entity.ID, error) {
	u := r.uc
	links, err := u.taskMRs(ctx, s, scope, _listTaskEvents, []entity.ID{task}, r.cut.seq, true)
	if err != nil || len(links.edges) == 0 {
		return nil, err
	}
	noteHiddenHistory(page, links.hidden, links.scope.Notes())
	byMR := groupHistoryEdges(links.edges)
	var mrs, areas []entity.ID
	for id, edges := range byMR {
		end := links.ends[id]
		if end.Kind != entity.ObjectMergeRequest {
			continue
		}
		facts, err := u.d.Tasks.Facts.TaskMRFacts(ctx, links.scope, r.org, end, r.cut.seq)
		if errors.Is(err, entity.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		events, err := r.mergeRequestEvents(ctx, links.scope, end, &facts)
		if err != nil {
			return nil, err
		}
		for i := range events {
			if holdsDuring(edges, events[i].EventTime) {
				page.Items = append(page.Items, events[i])
			}
		}
		mrs = append(mrs, id)
		areas = addHistoryArea(areas, end.AreaID)
	}
	if len(mrs) == 0 {
		return areas, nil
	}
	return r.relatedPipelineEvents(ctx, s, scope, mrs, byMR, areas, page)
}

func (r *taskRead) relatedPipelineEvents(ctx context.Context, s entity.Subject, scope entity.Scope, mrs []entity.ID, byMR map[entity.ID][]entity.LinkEdge,
	areas []entity.ID, page *entity.TaskEventPage,
) ([]entity.ID, error) {
	u := r.uc
	edges, err := u.d.CodeLinks.MREdgesAt(ctx, r.org, mrs, r.cut.seq)
	if err != nil {
		return nil, err
	}
	edges = pipelineEdges(edges, mrs)
	if len(edges) == 0 {
		return areas, nil
	}
	allowed, ends, err := u.taskRunEnds(ctx, s, scope, _listTaskEvents, edges)
	if err != nil {
		return nil, err
	}
	noteHiddenHistory(page, false, allowed.Notes())
	byRun := groupHistoryEdges(edges)
	runs := make([]entity.ID, 0, len(byRun))
	var runAreas []entity.ID
	for id := range byRun {
		end := ends[id]
		if end.Kind != entity.ObjectCIRun {
			noteHiddenHistory(page, true, nil)
			continue
		}
		runs = append(runs, id)
		runAreas = addHistoryArea(runAreas, end.AreaID)
		areas = addHistoryArea(areas, end.AreaID)
	}
	allowed, err = u.timelineObjectScope(ctx, s, allowed, _listTaskEvents, runAreas, entity.ObjectCIRun)
	if err != nil {
		return nil, err
	}
	noteHiddenHistory(page, false, allowed.Notes())
	facts, err := u.d.Tasks.Facts.TimelineFacts(ctx, allowed, r.org, runAreas, entity.TimelineFactQuery{
		Runs: runs, Kind: entity.ObjectCIRun, Knowledge: r.cut.seq, To: r.cut.asOf.Add(time.Microsecond),
	})
	if err != nil {
		return nil, err
	}
	for i := range facts {
		f := &facts[i]
		if !slices.ContainsFunc(byRun[f.RunID], func(e entity.LinkEdge) bool {
			return e.Validity.Contains(f.EventTime) && holdsDuring(byMR[e.From], f.EventTime)
		}) {
			continue
		}
		event, err := r.projectedEvent(ctx, allowed, f)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, event)
	}
	return areas, nil
}

func groupHistoryEdges(edges []entity.LinkEdge) map[entity.ID][]entity.LinkEdge {
	out := make(map[entity.ID][]entity.LinkEdge)
	for i := range edges {
		to := edges[i].To
		out[to] = append(out[to], edges[i])
	}
	return out
}

func holdsDuring(edges []entity.LinkEdge, at time.Time) bool {
	return slices.ContainsFunc(edges, func(e entity.LinkEdge) bool { return e.Validity.Contains(at) })
}

func addHistoryArea(areas []entity.ID, id entity.ID) []entity.ID {
	if !slices.Contains(areas, id) {
		return append(areas, id)
	}
	return areas
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
