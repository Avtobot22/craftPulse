package view

import (
	"cmp"
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

func (r *taskRead) trackerEvents(ctx context.Context, end entity.LinkEnd, facts *entity.AreaFacts) ([]entity.TaskHistoryEvent, error) {
	actors, err := r.uc.d.Tasks.Facts.TaskActorKinds(ctx, r.org, r.cut.seq, facts.Events)
	if err != nil {
		return nil, err
	}
	task := &facts.Tasks[0]
	src, err := r.instance(ctx, end.InstanceID)
	if err != nil {
		return nil, err
	}
	ref := entity.ObjectRef{
		ID: end.ID, Kind: entity.ObjectTask, DisplayKey: taskKey(task.TaskKey),
		Title: taskKey(task.Title), SourceURL: trackerTaskURL(src, task.TaskKey),
	}
	out := make([]entity.TaskHistoryEvent, 0, len(facts.Events))
	for i := range facts.Events {
		event := &facts.Events[i]
		out = append(out, entity.TaskHistoryEvent{
			ID: event.ID, Object: ref, Kind: string(event.Kind), Field: taskKey(event.FieldNativeID),
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
	ref, err := r.taskMRRef(ctx, scope, end, &facts.MRs[0])
	if err != nil {
		return nil, err
	}
	src, err := r.instance(ctx, end.InstanceID)
	if err != nil {
		return nil, err
	}
	out := make([]entity.TaskHistoryEvent, 0, len(facts.MREvents))
	for i := range facts.MREvents {
		event := &facts.MREvents[i]
		object := ref
		if src.Provider == entity.ProviderGitLab && ref.SourceURL != nil {
			if key, found := strings.CutPrefix(event.EventKey, "note/"); found {
				note, _, _ := strings.Cut(key, "/")
				if id, err := strconv.ParseUint(note, 10, 64); err == nil && id > 0 {
					link := *ref.SourceURL + "#note_" + strconv.FormatUint(id, 10)
					object.SourceURL = &link
				}
			}
		}
		out = append(out, entity.TaskHistoryEvent{
			ID: event.ID, Object: object, Kind: event.Kind, After: taskKey(event.ReviewState),
			AccountID: event.ActorAccountID, ActorKind: historyActor(facts.ActorKinds, event.ActorAccountID),
			EventTime: event.EventTime, EventOffsetS: event.EventOffsetS, ObservedAt: event.ObservedAt, ObservedVia: event.ObservedVia,
		})
	}
	return out, nil
}

func (r *taskRead) pipelineHistory(ctx context.Context, scope entity.Scope, end entity.LinkEnd, run *entity.CIRun) ([]entity.TaskHistoryEvent, error) {
	ref := entity.ObjectRef{Kind: entity.ObjectCIRun, ID: end.ID}
	if end.NativeID != "" {
		ref.DisplayKey = taskKey("#" + end.NativeID)
	}
	src, err := r.instance(ctx, end.InstanceID)
	if err != nil {
		return nil, err
	}
	if src.Provider == entity.ProviderGitLab {
		repo, err := r.uc.d.Code.Repository(ctx, scope, r.org, end.AreaID)
		if err != nil && !errors.Is(err, entity.ErrNotFound) {
			return nil, err
		}
		if id, err := strconv.ParseUint(end.NativeID, 10, 64); err == nil && id > 0 {
			ref.SourceURL = sourceURL(repo.WebURL, "/-/pipelines/"+strconv.FormatUint(id, 10))
		}
	}

	var out []entity.TaskHistoryEvent
	v := &run.Run
	out = appendCIEvent(out, ref, "pipeline_started", v.StartedAt, v.ObservedAt, "")
	out = appendCIEvent(out, ref, "pipeline_finished", v.FinishedAt, v.ObservedAt, v.Outcome)
	for i := range run.Attempts {
		a := &run.Attempts[i]
		if a.RunID != v.RunID {
			continue
		}
		attempt := entity.ObjectRef{Kind: entity.ObjectCIAttempt, ID: a.AttemptID, DisplayKey: taskKey(a.JobKey), SourceURL: ref.SourceURL}
		out = appendCIEvent(out, attempt, "attempt_started", a.StartedAt, a.ObservedAt, "")
		out = appendCIEvent(out, attempt, "attempt_finished", a.FinishedAt, a.ObservedAt, a.Outcome)
	}
	return out, nil
}

func appendCIEvent(out []entity.TaskHistoryEvent, object entity.ObjectRef, kind string, at *time.Time, observed time.Time, outcome entity.CIOutcome) []entity.TaskHistoryEvent {
	if at == nil || at.IsZero() {
		return out
	}
	// A CI version supplies both timestamps without a separate actor or observation-mode field.
	_, offset := at.Zone()
	return append(out, entity.TaskHistoryEvent{
		ID: entity.ID(uuid.NewSHA1(uuid.UUID(object.ID), []byte(kind))), Object: object, Kind: kind, After: taskKey(string(outcome)),
		ActorKind: entity.PersonUnknown, EventTime: *at, EventOffsetS: offset, ObservedAt: observed, ObservedVia: entity.ViaIncremental,
	})
}
