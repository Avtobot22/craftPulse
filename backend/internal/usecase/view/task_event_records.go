package view

import (
	"cmp"
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *taskRead) trackerEvents(ctx context.Context, facts *entity.AreaFacts) ([]entity.TaskHistoryEvent, error) {
	actors, err := r.uc.d.Tasks.Facts.TaskActorKinds(ctx, r.org, r.cut.seq, facts.Events)
	if err != nil {
		return nil, err
	}
	src, err := r.instance(ctx, facts.InstanceID)
	if err != nil {
		return nil, err
	}
	objects := make(map[entity.ID]entity.ObjectRef, len(facts.Tasks))
	for i := range facts.Tasks {
		task := &facts.Tasks[i]
		objects[task.TaskID] = entity.ObjectRef{
			ID: task.TaskID, Kind: entity.ObjectTask, DisplayKey: taskKey(task.TaskKey),
			Title: taskKey(task.Title), SourceURL: trackerTaskURL(src, task.TaskKey),
		}
	}
	out := make([]entity.TaskHistoryEvent, 0, len(facts.Events))
	for i := range facts.Events {
		event := &facts.Events[i]
		ref, ok := objects[event.TaskID]
		if !ok {
			continue
		}
		out = append(out, entity.TaskHistoryEvent{
			ID: event.ID, Object: ref, AreaID: facts.AreaID, Kind: string(event.Kind), Field: taskKey(event.FieldNativeID),
			Before:    taskEventValue(event.FieldNativeID, event.BeforeValue, event.BeforeAccountID),
			After:     taskEventValue(event.FieldNativeID, event.AfterValue, event.AfterAccountID),
			AccountID: event.ActorAccountID, ActorKind: historyActor(actors, event.ActorAccountID),
			EventTime: event.EventTime, EventOffsetS: event.EventOffsetS, ObservedAt: event.ObservedAt, ObservedVia: event.ObservedVia,
		})
	}
	return out, nil
}

func taskEventValue(field string, value *string, account *entity.ID) *string {
	if account != nil {
		key := account.String()
		return &key
	}
	if field == "assignee" || field == "reporter" || field == "creator" {
		return nil
	}
	return value
}

func historyActor(kinds map[entity.ID]entity.PersonKind, id *entity.ID) entity.PersonKind {
	if id == nil {
		return entity.PersonUnknown
	}
	return cmp.Or(kinds[*id], entity.PersonUnknown)
}

func (r *taskRead) mergeRequestEvents(ctx context.Context, scope entity.Scope, end entity.LinkEnd, facts *entity.ReviewFacts) ([]entity.TaskHistoryEvent, error) {
	src, err := r.instance(ctx, end.InstanceID)
	if err != nil {
		return nil, err
	}
	versions := make(map[entity.ID]*entity.MRVersion, len(facts.MRs))
	for i := range facts.MRs {
		v := &facts.MRs[i]
		versions[v.MRID] = v
	}
	objects := make(map[entity.ID]entity.ObjectRef)
	out := make([]entity.TaskHistoryEvent, 0, len(facts.MREvents))
	for i := range facts.MREvents {
		event := &facts.MREvents[i]
		mr := versions[event.MRID]
		if mr == nil {
			continue
		}
		ref, ok := objects[event.MRID]
		if !ok {
			end.ID = event.MRID
			ref, err = r.taskMRRef(ctx, scope, end, mr)
			if err != nil {
				return nil, err
			}
			objects[event.MRID] = ref
		}
		if src.Provider == entity.ProviderGitLab && ref.SourceURL != nil {
			if key, found := strings.CutPrefix(event.EventKey, "note/"); found {
				note, _, _ := strings.Cut(key, "/")
				if id, err := strconv.ParseUint(note, 10, 64); err == nil && id > 0 {
					link := *ref.SourceURL + "#note_" + strconv.FormatUint(id, 10)
					ref.SourceURL = &link
				}
			}
		}
		e := entity.TaskHistoryEvent{
			ID: event.ID, Object: ref, AreaID: end.AreaID, Kind: event.Kind, After: taskKey(event.ReviewState),
			AccountID: event.ActorAccountID, ActorKind: historyActor(facts.ActorKinds, event.ActorAccountID),
			EventTime: event.EventTime, EventOffsetS: event.EventOffsetS, ObservedAt: event.ObservedAt, ObservedVia: event.ObservedVia,
		}
		switch event.Kind {
		case "reviewer_assigned":
			e.Field, e.After = new("reviewer"), taskEventValue("reviewer", nil, event.TargetAccountID)
		case "reviewer_removed":
			e.Field, e.Before, e.After = new("reviewer"), taskEventValue("reviewer", nil, event.TargetAccountID), nil
		case "head_pushed":
			e.Field, e.After = new("head_sha"), taskKey(event.HeadSHA)
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *taskRead) projectedEvent(ctx context.Context, scope entity.Scope, f *entity.TimelineFact) (entity.TaskHistoryEvent, error) {
	src, err := r.instance(ctx, f.InstanceID)
	if err != nil {
		return entity.TaskHistoryEvent{}, err
	}
	repo, err := r.repository(ctx, scope, f.AreaID)
	if err != nil {
		return entity.TaskHistoryEvent{}, err
	}
	ref := entity.ObjectRef{Kind: f.ObjectKind, ID: f.ObjectID, DisplayKey: taskKey(f.DisplayKey), Title: taskKey(f.Title)}
	actor := entity.PersonSystem
	switch string(f.ObjectKind) {
	case string(entity.ObjectCIRun):
		switch string(src.Provider) {
		case string(entity.ProviderGitLab):
			ref.SourceURL = gitLabCIURL(src, repo, f.ProjectPath, "/-/pipelines/"+url.PathEscape(f.NativeID))
		case string(entity.ProviderGitHub):
			ref.SourceURL = sourceURL(repo.WebURL, "/actions/runs/"+url.PathEscape(f.NativeID))
		}
	case string(entity.ObjectCIAttempt):
		switch string(src.Provider) {
		case string(entity.ProviderGitLab):
			ref.SourceURL = gitLabCIURL(src, repo, f.ProjectPath, "/-/jobs/"+url.PathEscape(f.NativeID))
		case string(entity.ProviderGitHub):
			ref.SourceURL = sourceURL(repo.WebURL, "/actions/runs/"+url.PathEscape(f.ParentNativeID)+"/job/"+url.PathEscape(f.NativeID))
		}
	case string(entity.ObjectDocument):
		actor = entity.PersonUnknown
		parts := strings.Split(f.DisplayKey, "/")
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		path := f.Value + "/" + strings.Join(parts, "/")
		switch string(src.Provider) {
		case string(entity.ProviderGitLab):
			ref.SourceURL = sourceURL(repo.WebURL, "/-/blob/"+path)
		case string(entity.ProviderGitHub):
			ref.SourceURL = sourceURL(repo.WebURL, "/blob/"+path)
		}
	}
	return entity.TaskHistoryEvent{
		Object: ref, ID: f.ID, AreaID: f.AreaID, Kind: f.Kind, After: taskKey(f.Value), ActorKind: actor,
		EventTime: f.EventTime, ObservedAt: f.ObservedAt, EventOffsetS: f.EventOffsetS, ObservedVia: f.ObservedVia,
	}, nil
}

func gitLabCIURL(src entity.SourceInstance, repo entity.RepositoryVersion, manifestPath, route string) *string {
	if link := sourceURL(repo.WebURL, route); link != nil {
		return link
	}
	// CI can discover a project before the MR collector has stored its repository metadata.
	parts := strings.Split(cmp.Or(repo.Path, manifestPath), "/")
	if len(parts) < 2 {
		return nil
	}
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil
		}
		parts[i] = url.PathEscape(part)
	}
	return sourceURL(cmp.Or(src.BaseURL, src.InstanceKey), "/"+strings.Join(parts, "/")+route)
}
