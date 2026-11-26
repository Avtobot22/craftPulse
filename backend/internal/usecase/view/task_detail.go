package view

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (u *UseCase) GetTask(ctx context.Context, s entity.Subject, scope entity.Scope, id entity.ID) (entity.TaskDetail, error) {
	org := s.OrganizationID
	cut, err := u.taskCut(ctx, org, nil)
	if err != nil {
		return entity.TaskDetail{}, err
	}
	visible, ends, err := u.codeScope(ctx, s, scope, _getTask, []entity.ID{id})
	if err != nil {
		return entity.TaskDetail{}, err
	}
	end := ends[id]
	if end.Kind != entity.ObjectTask {
		return entity.TaskDetail{}, entity.ErrNotFound
	}
	area, err := u.d.Tasks.Facts.VisibleTaskFacts(ctx, visible, org, end, cut.seq)
	if err != nil {
		return entity.TaskDetail{}, err
	}
	if len(area.Tasks) == 0 {
		return entity.TaskDetail{}, entity.ErrNotFound
	}
	structure, err := u.taskStructure(ctx, org, scope, cut.asOf)
	if err != nil {
		return entity.TaskDetail{}, err
	}
	read, err := u.taskRead(ctx, org, cut, structure, []entity.ID{end.AreaID})
	if err != nil {
		return entity.TaskDetail{}, err
	}
	summary, facts, err := read.summary(ctx, &area.Tasks[0], end.InstanceID, area.Events)
	if err != nil {
		return entity.TaskDetail{}, err
	}
	facts.Refs = map[entity.ID]entity.ObjectRef{id: {
		ID: id, Kind: entity.ObjectTask, DisplayKey: taskKey(summary.Version.TaskKey), Title: taskKey(summary.Version.Title), SourceURL: summary.SourceURL,
	}}
	out := entity.TaskDetail{TaskSummary: summary}
	if err := read.linkedFacts(ctx, s, scope, facts, &out); err != nil {
		return entity.TaskDetail{}, err
	}
	for _, provider := range u.d.Tasks.Observations {
		out.Observations = append(out.Observations, provider(facts)...)
	}
	u.d.Tasks.Calculator.SortTaskObservations(out.Observations)
	return out, nil
}

type taskMRLinks struct {
	scope     entity.Scope
	edges     []entity.LinkEdge
	ends      map[entity.ID]entity.LinkEnd
	confirmed []entity.ID
	hidden    bool
}

func (u *UseCase) taskMRs(ctx context.Context, s entity.Subject, scope entity.Scope, op string, tasks []entity.ID, seq int64, onlyConfirmed bool) (taskMRLinks, error) {
	if len(tasks) == 0 {
		return taskMRLinks{}, nil
	}
	edges, err := u.d.CodeLinks.TaskMREdgesAt(ctx, s.OrganizationID, tasks, seq)
	if err != nil {
		return taskMRLinks{}, err
	}
	edges = slices.DeleteFunc(edges, func(e entity.LinkEdge) bool {
		return e.Type != entity.LinkTaskMR || e.Status == entity.LinkRejected || !slices.Contains(tasks, e.From) ||
			(onlyConfirmed && e.Status != entity.LinkConfirmed)
	})
	if len(edges) == 0 {
		return taskMRLinks{}, nil
	}
	// A confirmation is stored alongside its original candidate. Show the pair once.
	confirmed := make(map[[2]entity.ID]bool)
	for i := range edges {
		e := &edges[i]
		if e.Status == entity.LinkConfirmed {
			confirmed[[2]entity.ID{e.From, e.To}] = true
		}
	}
	edges = slices.DeleteFunc(edges, func(e entity.LinkEdge) bool {
		return e.Status == entity.LinkCandidate && confirmed[[2]entity.ID{e.From, e.To}]
	})
	ids := make([]entity.ID, 0, len(edges))
	for i := range edges {
		ids = append(ids, edges[i].To)
	}
	allowed, ends, err := u.codeScope(ctx, s, scope, op, ids)
	if err != nil {
		return taskMRLinks{}, err
	}
	out := taskMRLinks{scope: allowed, edges: edges, ends: ends}
	for i := range edges {
		edge := &edges[i]
		if ends[edge.To].Kind != entity.ObjectMergeRequest {
			out.hidden = true
		} else if edge.Status == entity.LinkConfirmed {
			out.confirmed = append(out.confirmed, edge.To)
		}
	}
	return out, nil
}

func (r *taskRead) linkedFacts(ctx context.Context, s entity.Subject, scope entity.Scope, f *entity.TaskObservationFacts, out *entity.TaskDetail) error {
	u := r.uc
	links, err := u.taskMRs(ctx, s, scope, _getTask, []entity.ID{f.Task.TaskID}, f.KnowledgeSeq, false)
	if err != nil {
		return err
	}
	var confirmed, reviewAreas []entity.ID
	heads := map[entity.ID]string{}
	hidden := links.hidden
	for i := range links.edges {
		edge := &links.edges[i]
		end := links.ends[edge.To]
		if end.Kind != entity.ObjectMergeRequest {
			continue
		}
		facts, err := u.d.Tasks.Facts.TaskMRFacts(ctx, links.scope, r.org, end, r.cut.seq)
		if errors.Is(err, entity.ErrNotFound) {
			hidden = true
			continue
		}
		if err != nil {
			return err
		}
		mr := &facts.MRs[0]
		ref, err := r.taskMRRef(ctx, links.scope, end, mr)
		if err != nil {
			return err
		}
		f.Refs[end.ID] = ref
		f.Links = append(f.Links, *edge)
		out.Links = append(out.Links, entity.TaskViewLink{Edge: *edge, To: ref})
		if edge.Status == entity.LinkConfirmed {
			confirmed = append(confirmed, end.ID)
			heads[end.ID] = mr.HeadSHA
			if !slices.Contains(reviewAreas, mr.AreaID) {
				reviewAreas = append(reviewAreas, mr.AreaID)
			}
			f.Reviews = append(f.Reviews, facts)
			f.MRRevisions = append(f.MRRevisions, facts.MRRevisions...)
		}
	}
	f.TaskMRsUnavailable = hidden
	if len(confirmed) > 0 {
		f.ReviewCollection, err = u.d.Tasks.Collection.MRCollection(ctx, r.org, reviewAreas)
		if err != nil {
			return err
		}
		runsHidden, err := r.linkedRuns(ctx, s, scope, f, out, confirmed, heads)
		if err != nil {
			return err
		}
		hidden = hidden || runsHidden
	}
	if hidden {
		out.Redactions = []string{"links", "observations"}
	}
	return nil
}

func (r *taskRead) taskMRRef(ctx context.Context, scope entity.Scope, end entity.LinkEnd, mr *entity.MRVersion) (entity.ObjectRef, error) {
	ref := entity.ObjectRef{ID: end.ID, Kind: entity.ObjectMergeRequest, Title: taskKey(mr.Title)}
	if mr.IID == nil {
		return ref, nil
	}
	key := "!" + strconv.Itoa(*mr.IID)
	ref.DisplayKey = &key
	repo, err := r.uc.d.Code.Repository(ctx, scope, r.org, end.AreaID)
	if errors.Is(err, entity.ErrNotFound) {
		return ref, nil
	}
	if err != nil {
		return ref, err
	}
	src, err := r.instance(ctx, end.InstanceID)
	if err != nil {
		return ref, err
	}
	if src.Provider == entity.ProviderGitHub {
		key = "#" + strconv.Itoa(*mr.IID)
	}
	ref.SourceURL = mergeRequestURL(repo.WebURL, src.Provider, *mr.IID)
	return ref, nil
}

func pipelineEdges(edges []entity.LinkEdge, mrs []entity.ID) []entity.LinkEdge {
	return slices.DeleteFunc(edges, func(e entity.LinkEdge) bool {
		return e.Type != entity.LinkMRRun || e.Status != entity.LinkConfirmed || !slices.Contains(mrs, e.From)
	})
}

func (u *UseCase) taskRunEnds(ctx context.Context, s entity.Subject, scope entity.Scope, op string, edges []entity.LinkEdge) (entity.Scope, map[entity.ID]entity.LinkEnd, error) {
	ids := make([]entity.ID, len(edges))
	for i := range edges {
		ids[i] = edges[i].To
	}
	return u.codeScope(ctx, s, scope, op, ids)
}

func (r *taskRead) linkedRuns(ctx context.Context, s entity.Subject, scope entity.Scope, f *entity.TaskObservationFacts, out *entity.TaskDetail,
	mrs []entity.ID, heads map[entity.ID]string,
) (bool, error) {
	u := r.uc
	runs, err := u.d.CodeLinks.MREdgesAt(ctx, r.org, mrs, r.cut.seq)
	if err != nil {
		return false, err
	}
	runs = pipelineEdges(runs, mrs)
	if len(runs) == 0 {
		return false, nil
	}
	allowed, ends, err := u.taskRunEnds(ctx, s, scope, _getTask, runs)
	if err != nil {
		return false, err
	}
	hidden := false
	for i := range runs {
		edge := &runs[i]
		end := ends[edge.To]
		if end.Kind != entity.ObjectCIRun {
			hidden = true
			continue
		}
		run, err := u.d.Code.RunAt(ctx, allowed, r.org, edge.To, r.cut.seq)
		if errors.Is(err, entity.ErrNotFound) {
			continue
		}
		if err != nil {
			return false, err
		}
		if run.Run.SHA == "" || run.Run.SHA != heads[edge.From] {
			continue
		}
		ref := entity.ObjectRef{Kind: entity.ObjectCIRun, ID: edge.To}
		if end.NativeID != "" {
			key := "#" + end.NativeID
			ref.DisplayKey = &key
		}
		f.Refs[edge.To] = ref
		f.Links = append(f.Links, *edge)
		f.CI.Runs = append(f.CI.Runs, run.Run)
		for i := range run.Attempts {
			if run.Attempts[i].RunID == run.Run.RunID {
				f.CI.Attempts = append(f.CI.Attempts, run.Attempts[i])
			}
		}
		out.Links = append(out.Links, entity.TaskViewLink{Edge: *edge, To: ref})
	}
	return hidden, nil
}
