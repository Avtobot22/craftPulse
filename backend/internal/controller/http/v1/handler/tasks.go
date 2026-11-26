package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

type tasks struct{ uc usecase.TaskViews }

func (h *tasks) ListTasks(ctx context.Context, req openapi.ListTasksRequestObject) (openapi.ListTasksResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	q, filter, err := taskQuery(s, req.Params)
	if err != nil {
		return nil, err
	}
	page, err := h.uc.ListTasks(ctx, s, scope, q)
	if err != nil {
		return nil, err
	}
	out := openapi.TaskPage{Data: make([]openapi.TaskSummary, len(page.Items)), AccessLimited: len(page.Notes) > 0, AccessNotes: accessNotes(page.Notes)}
	for i := range page.Items {
		out.Data[i] = taskSummaryBody(&page.Items[i])
	}
	if n := page.Next; n != nil {
		c := cursorData{Kind: "tasks", Name: filter, At: n.AsOf, Seq: &n.Knowledge, ID: uuid.UUID(n.ID)}
		switch strings.TrimPrefix(q.Sort, "-") {
		case "age":
			c.TaskAge = n.Age
		case "updated_at":
			if n.UpdatedAt != nil {
				c.OpenedAt = *n.UpdatedAt
			}
		case "key":
			c.TaskKey = n.Key
		}
		raw := encodeCursor(c)
		out.NextCursor = &raw
	}
	return openapi.ListTasks200JSONResponse(out), nil
}

func taskQuery(s entity.Subject, p openapi.ListTasksParams) (entity.TaskViewQuery, string, error) {
	limit, err := pageLimit(p.Limit)
	if err != nil {
		return entity.TaskViewQuery{}, "", err
	}
	q := entity.TaskViewQuery{Limit: limit, Project: (*entity.ID)(p.ProjectId), Team: (*entity.ID)(p.TeamId), OverTarget: p.OverTarget, Sort: "-age"}
	if p.StatusMeaning != nil {
		if !p.StatusMeaning.Valid() {
			return q, "", errMalformedPage
		}
		q.Meaning = string(*p.StatusMeaning)
	}
	if p.Sort != nil {
		if !p.Sort.Valid() {
			return q, "", errMalformedPage
		}
		q.Sort = string(*p.Sort)
	}
	over := ""
	if p.OverTarget != nil {
		over = strconv.FormatBool(*p.OverTarget)
	}
	filter := strings.Join([]string{s.OrganizationID.String(), s.UserID.String(), s.APIKeyID.String(), queueID(q.Project), queueID(q.Team), q.Meaning, over, q.Sort}, ":")
	if p.Cursor == nil {
		return q, filter, nil
	}
	c, ok := decodeCursor(*p.Cursor)
	if !ok || c.Kind != "tasks" || c.Name != filter || c.At.IsZero() || c.ID == uuid.Nil || c.Seq == nil || *c.Seq < 0 ||
		c.TaskAge != nil && *c.TaskAge < 0 {
		return q, "", errMalformedPage
	}
	q.After = &entity.TaskViewCursor{AsOf: c.At, Knowledge: *c.Seq, ID: entity.ID(c.ID), Age: c.TaskAge, Key: c.TaskKey}
	if !c.OpenedAt.IsZero() {
		q.After.UpdatedAt = &c.OpenedAt
	}
	return q, filter, nil
}

func (h *tasks) GetTask(ctx context.Context, req openapi.GetTaskRequestObject) (openapi.GetTaskResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	detail, err := h.uc.GetTask(ctx, s, scope, entity.ID(req.TaskId))
	if err != nil {
		return nil, err
	}
	return openapi.GetTask200JSONResponse(taskDetailBody(&detail)), nil
}

func (h *tasks) ListTaskEvents(ctx context.Context, req openapi.ListTaskEventsRequestObject) (openapi.ListTaskEventsResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	binding := strings.Join([]string{s.OrganizationID.String(), s.UserID.String(), s.APIKeyID.String(), req.TaskId.String()}, ":")
	q := entity.TaskEventQuery{Limit: limit}
	if req.Params.Cursor != nil {
		c, ok := decodeCursor(*req.Params.Cursor)
		if !ok || c.Kind != "task-events" || c.Name != binding || c.At.IsZero() || c.ID == uuid.Nil ||
			c.Seq == nil || *c.Seq < 0 || c.OpenedAt.IsZero() || c.OpenedAt.After(c.At) {
			return nil, errMalformedPage
		}
		q.After = &entity.TaskEventCursor{AsOf: c.At, Knowledge: *c.Seq, EventTime: c.OpenedAt, ID: entity.ID(c.ID)}
	}
	page, err := h.uc.ListTaskEvents(ctx, s, scope, entity.ID(req.TaskId), q)
	if err != nil {
		return nil, err
	}
	out := openapi.TimelinePage{
		Data: make([]openapi.TimelineEvent, len(page.Items)), HistoryFrom: page.HistoryFrom,
		AccessLimited: len(page.Notes) > 0, AccessNotes: accessNotes(page.Notes),
	}
	for i := range page.Items {
		event := &page.Items[i]
		out.Data[i] = openapi.TimelineEvent{
			Id: uuid.UUID(event.ID), Object: taskObjectRef(event.Object), EventKind: event.Kind,
			Field: event.Field, BeforeValue: event.Before, AfterValue: event.After,
			Actor:     openapi.ActorRef{AccountId: uuidPtr(event.AccountID), Kind: openapi.ActorRefKind(event.ActorKind)},
			EventTime: event.EventTime, EventOffsetSeconds: event.EventOffsetS, ObservedAt: event.ObservedAt,
			ObservedVia: openapi.TimelineEventObservedVia(event.ObservedVia),
		}
	}
	if n := page.Next; n != nil {
		cursor := encodeCursor(cursorData{
			Kind: "task-events", Name: binding, At: n.AsOf, Seq: &n.Knowledge, OpenedAt: n.EventTime, ID: uuid.UUID(n.ID),
		})
		out.NextCursor = &cursor
	}
	return openapi.ListTaskEvents200JSONResponse(out), nil
}

func taskSummaryBody(task *entity.TaskSummary) openapi.TaskSummary {
	return openapi.TaskSummary{
		Id: uuid.UUID(task.Version.TaskID), Key: optional(task.Version.TaskKey), Title: optional(task.Version.Title),
		ProjectId: uuidPtr(task.ProjectID), TeamId: uuidPtr(task.TeamID), TaskType: task.Type, SizeLabel: task.Size,
		AgeSeconds: task.AgeSeconds, OverTarget: task.OverTarget, UpdatedAt: task.Version.SourceUpdatedAt, SourceUrl: task.SourceURL,
		Status: openapi.TaskStatus{NativeId: task.Version.StatusNativeID, Name: task.StatusName, Meaning: openapi.TaskStatusMeaning(task.Meaning)},
	}
}

func taskDetailBody(detail *entity.TaskDetail) openapi.TaskDetail {
	s := taskSummaryBody(&detail.TaskSummary)
	type block = struct {
		From time.Time  `json:"from"`
		To   *time.Time `json:"to"`
	}
	type observation = struct {
		Action   openapi.TaskDetailObservationsAction    `json:"action"`
		Basis    []openapi.ObjectRef                     `json:"basis"`
		Kind     openapi.TaskDetailObservationsKind      `json:"kind"`
		Question *openapi.TaskDetailObservationsQuestion `json:"question,omitempty"`
		Since    *time.Time                              `json:"since"`
	}
	out := openapi.TaskDetail{
		Id: s.Id, Key: s.Key, Title: s.Title, ProjectId: s.ProjectId, TeamId: s.TeamId, TaskType: s.TaskType, SizeLabel: s.SizeLabel,
		Status: s.Status, AgeSeconds: s.AgeSeconds, OverTarget: s.OverTarget, UpdatedAt: s.UpdatedAt, SourceUrl: s.SourceUrl,
		BlockedIntervals: make([]block, len(detail.Blocked)), Observations: make([]observation, len(detail.Observations)),
		Links: make([]openapi.Link, len(detail.Links)), Redactions: append([]string{}, detail.Redactions...),
	}
	if t := detail.Target; t != nil {
		out.Target = &struct {
			DurationSeconds int `json:"duration_seconds"`
			VersionNo       int `json:"version_no"`
		}{int(t.DurationSeconds), t.VersionNo}
	}
	for i, iv := range detail.Blocked {
		out.BlockedIntervals[i] = block{From: iv.From, To: iv.To}
	}
	for i, obs := range detail.Observations {
		o := &out.Observations[i]
		o.Action, o.Kind, o.Since = openapi.TaskDetailObservationsAction(obs.Action), openapi.TaskDetailObservationsKind(obs.Kind), obs.Since
		o.Basis = make([]openapi.ObjectRef, len(obs.Basis))
		for j, basis := range obs.Basis {
			o.Basis[j] = taskObjectRef(basis)
		}
		if obs.Question != "" {
			q := openapi.TaskDetailObservationsQuestion(obs.Question)
			o.Question = &q
		}
	}
	for i := range detail.Links {
		link := &detail.Links[i]
		e := &link.Edge
		out.Links[i] = openapi.Link{
			Id: uuid.UUID(e.ID), FromObjectId: uuid.UUID(e.From), LinkType: openapi.LinkLinkType(e.Type),
			Status: openapi.LinkStatus(e.Status), Method: openapi.LinkMethod(e.Method), DecidedAt: e.DecidedAt, To: taskObjectRef(link.To),
		}
	}
	return out
}

func taskObjectRef(ref entity.ObjectRef) openapi.ObjectRef {
	return openapi.ObjectRef{Kind: _refKinds[ref.Kind], Id: uuid.UUID(ref.ID), DisplayKey: ref.DisplayKey, Title: ref.Title, SourceUrl: ref.SourceURL}
}
