package view

import (
	"context"
	"slices"
	"time"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) ListTasks(ctx context.Context, s entity.Subject, scope entity.Scope, q entity.TaskViewQuery) (entity.TaskViewPage, error) {
	if q.Limit < 1 {
		return entity.TaskViewPage{}, entity.ErrInvalidCursor
	}
	org := s.OrganizationID
	cut, err := u.taskCut(ctx, org, q.After)
	if err != nil {
		return entity.TaskViewPage{}, err
	}
	structure, err := u.taskStructure(ctx, org, scope, cut.asOf)
	if err != nil {
		return entity.TaskViewPage{}, err
	}
	areas, err := u.listTrackerAreas(ctx, org, structure, q)
	if err != nil {
		return entity.TaskViewPage{}, err
	}
	if len(areas) == 0 {
		if q.After == nil {
			return entity.TaskViewPage{EmptyReason: entity.TaskSourceNotConfigured}, nil
		}
		return entity.TaskViewPage{}, nil
	}
	visible, err := u.taskListScope(ctx, s, scope, areas)
	if err != nil {
		return entity.TaskViewPage{}, err
	}
	page := entity.TaskViewPage{Notes: visible.Notes()}
	areas = slices.DeleteFunc(areas, func(id entity.ID) bool { return !visible.Area(id) })
	if len(areas) == 0 {
		return page, nil
	}
	read, err := u.taskRead(ctx, org, cut, structure, areas)
	if err != nil {
		return entity.TaskViewPage{}, err
	}
	hasTasks := false
	for _, id := range areas {
		facts, err := u.d.Tasks.Facts.VisibleAreaFacts(ctx, visible, org, id, cut.seq)
		if err != nil {
			return entity.TaskViewPage{}, err
		}
		if err := read.keepCurrentTasks(ctx, visible, &facts); err != nil {
			return entity.TaskViewPage{}, err
		}
		hasTasks = hasTasks || len(facts.Tasks) > 0
		rows, err := read.listRows(ctx, &facts, q)
		if err != nil {
			return entity.TaskViewPage{}, err
		}
		page.Items = append(page.Items, rows...)
	}
	if q.After == nil && !visible.Limited() && !hasTasks && waitingForTaskHistory(areas, read.collection) {
		page.EmptyReason = entity.TaskHistoryNotReceived
	}
	slices.SortFunc(page.Items, func(a, b entity.TaskSummary) int {
		return compareTaskPosition(taskPosition(&a, cut), taskPosition(&b, cut), q.Sort)
	})
	if len(page.Items) > q.Limit {
		next := taskPosition(&page.Items[q.Limit-1], cut)
		page.Next, page.Items = &next, page.Items[:q.Limit]
	}
	ids := make([]entity.ID, len(page.Items))
	for i := range page.Items {
		ids[i] = page.Items[i].Version.TaskID
	}
	hidden, err := u.hiddenTaskLinks(ctx, s, scope, ids, cut.seq)
	if hidden && !visible.Limited() {
		page.Notes = []entity.AccessNote{{Code: entity.NoteRestricted}}
	}
	return page, err
}

// Import-only areas have no collection state. They do not imply a pending sync.
func waitingForTaskHistory(areas []entity.ID, collection []entity.AreaCollection) bool {
	pending := make(map[entity.ID]struct{}, len(areas))
	for i := range collection {
		item := &collection[i]
		if item.HistoryFrom != nil || item.LastSuccessAt != nil {
			return false
		}
		pending[item.AreaID] = struct{}{}
	}
	return len(pending) == len(areas)
}

func (u *UseCase) hiddenTaskLinks(ctx context.Context, s entity.Subject, scope entity.Scope, ids []entity.ID, seq int64) (bool, error) {
	links, err := u.taskMRs(ctx, s, scope, _listTasks, ids, seq, false)
	if err != nil || links.hidden || len(links.confirmed) == 0 {
		return links.hidden, err
	}
	edges, err := u.d.CodeLinks.MREdgesAt(ctx, s.OrganizationID, links.confirmed, seq)
	if err != nil {
		return false, err
	}
	runs := pipelineEdges(edges, links.confirmed)
	if len(runs) == 0 {
		return false, nil
	}
	_, ends, err := u.taskRunEnds(ctx, s, scope, _listTasks, runs)
	return slices.ContainsFunc(runs, func(e entity.LinkEdge) bool { return ends[e.To].Kind != entity.ObjectCIRun }), err
}

func (u *UseCase) listTrackerAreas(ctx context.Context, org entity.ID, st taskStructure, q entity.TaskViewQuery) ([]entity.ID, error) {
	var ids []entity.ID
	for id, projects := range st.projects {
		if slices.ContainsFunc(projects, func(project entity.ID) bool {
			return (q.Project == nil || project == *q.Project) && (q.Team == nil || slices.Contains(st.teams[project], *q.Team))
		}) {
			ids = append(ids, id)
		}
	}
	ends, err := u.d.Code.CurrentObjects(ctx, org, ids)
	ids = ids[:0]
	for _, end := range ends {
		if end.Kind == entity.ObjectTrackerProject {
			ids = append(ids, end.ID)
		}
	}
	return ids, err
}

func (u *UseCase) taskListScope(ctx context.Context, s entity.Subject, scope entity.Scope, areas []entity.ID) (entity.Scope, error) {
	scope, err := u.d.SourceRights.Decide(ctx, s, scope, entity.SourceQuery{Operation: _listTasks, Areas: areas})
	if err != nil {
		return scope, err
	}
	objects, err := u.d.Code.RestrictedObjects(ctx, scope, s.OrganizationID, areas)
	if err != nil || len(objects) == 0 {
		return scope, err
	}
	return u.d.SourceRights.Decide(ctx, s, scope, entity.SourceQuery{Operation: _listTasks, Objects: objects})
}

// A new page may see a task move or be deleted after the cursor was issued.
func (r *taskRead) keepCurrentTasks(ctx context.Context, scope entity.Scope, facts *entity.AreaFacts) error {
	ids := make([]entity.ID, len(facts.Tasks))
	for i := range facts.Tasks {
		ids[i] = facts.Tasks[i].TaskID
	}
	ends, err := r.uc.d.Code.CurrentObjects(ctx, r.org, ids)
	if err != nil {
		return err
	}
	live := make(map[entity.ID]bool, len(ends))
	allowed := scope.Objects()
	for _, end := range ends {
		live[end.ID] = end.AreaID == facts.AreaID && (!end.Restricted || slices.Contains(allowed, end.ID))
	}
	facts.Tasks = slices.DeleteFunc(facts.Tasks, func(task entity.TaskVersion) bool { return !live[task.TaskID] })
	return nil
}

func (r *taskRead) listRows(ctx context.Context, facts *entity.AreaFacts, q entity.TaskViewQuery) ([]entity.TaskSummary, error) {
	events := make(map[entity.ID][]entity.TaskEvent, len(facts.Tasks))
	for i := range facts.Events {
		e := &facts.Events[i]
		events[e.TaskID] = append(events[e.TaskID], *e)
	}
	var out []entity.TaskSummary
	for i := range facts.Tasks {
		task := &facts.Tasks[i]
		if task.CreatedSrcAt.After(r.cut.asOf) {
			continue
		}
		row, _, err := r.summary(ctx, task, facts.InstanceID, events[task.TaskID])
		if err != nil {
			return nil, err
		}
		if q.Project != nil {
			row.ProjectID = q.Project
		}
		if q.Team != nil && (row.TeamID == nil || *row.TeamID != *q.Team) ||
			q.Meaning != "" && row.Meaning != q.Meaning ||
			q.OverTarget != nil && (row.OverTarget == nil || *row.OverTarget != *q.OverTarget) ||
			q.After != nil && compareTaskPosition(taskPosition(&row, r.cut), *q.After, q.Sort) <= 0 {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func taskHistory(collection []entity.AreaCollection) map[entity.ID]*time.Time {
	out := make(map[entity.ID]*time.Time, len(collection))
	for i := range collection {
		item := &collection[i]
		if item.HistoryFrom != nil {
			prev := out[item.AreaID]
			if prev == nil || item.HistoryFrom.Before(*prev) {
				out[item.AreaID] = item.HistoryFrom
			}
		}
	}
	return out
}
