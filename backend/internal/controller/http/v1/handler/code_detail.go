package handler

import (
	"context"
	"encoding/hex"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

type codeDetails struct{ uc usecase.CodeDetails }

func (h *codeDetails) GetMergeRequest(ctx context.Context, req openapi.GetMergeRequestRequestObject) (openapi.GetMergeRequestResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	detail, err := h.uc.MergeRequest(ctx, s, scope, entity.ID(req.MergeRequestId))
	if err != nil {
		return nil, err
	}
	v := &detail.Version
	links := make([]openapi.Link, len(detail.Links))
	for i := range detail.Links {
		link := &detail.Links[i]
		links[i] = openapi.Link{
			Id: uuid.UUID(link.Edge.ID), FromObjectId: uuid.UUID(v.MRID), LinkType: openapi.LinkLinkType(link.Edge.Type),
			Method: openapi.LinkMethod(link.Edge.Method), Status: openapi.LinkStatus(link.Edge.Status), DecidedAt: link.Edge.DecidedAt,
			To: openapi.ObjectRef{Id: uuid.UUID(link.To.ID), Kind: openapi.ObjectRefKindTask, DisplayKey: link.To.DisplayKey, Title: link.To.Title},
		}
	}
	limited, notes := len(detail.Notes) > 0, accessNotes(detail.Notes)
	out := openapi.MergeRequest{
		Id: uuid.UUID(v.MRID), Iid: v.IID, Title: optional(v.Title), State: openapi.MergeRequestState(v.State), Draft: v.Draft,
		Area:    openapi.AreaRef{Id: uuid.UUID(v.AreaID), Kind: openapi.AreaRefKindRepository, DisplayName: detail.Repository.Name},
		Author:  openapi.ActorRef{Kind: openapi.ActorRefKind(detail.AuthorKind), AccountId: uuidPtr(v.AuthorAccountID)},
		HeadSha: v.HeadSHA, BaseSha: optional(v.BaseSHA), OpenedAt: v.OpenedAt, ReadyAt: v.ReadyAt, SourceUrl: detail.SourceURL,
		Links: &links, AccessLimited: &limited, AccessNotes: &notes,
	}
	return openapi.GetMergeRequest200JSONResponse(out), nil
}

func (h *codeDetails) GetPipeline(ctx context.Context, req openapi.GetPipelineRequestObject) (openapi.GetPipelineResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	detail, err := h.uc.Pipeline(ctx, s, scope, entity.ID(req.PipelineId))
	if err != nil {
		return nil, err
	}
	run, summary := &detail.Run.Run, &detail.Summary
	out := openapi.Pipeline{
		Id: uuid.UUID(run.RunID), MergeRequestId: uuidPtr(run.MRID), Sha: run.SHA, RefName: run.RefName,
		NativeStatus: run.NativeStatus, Outcome: openapi.PipelineOutcome(run.Outcome), StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		AttemptHistory: openapi.PipelineAttemptHistory(detail.Run.History), AttemptsTotal: summary.AttemptsTotal,
		RetriedJobs: summary.RetriedJobs, AllowedFailures: summary.AllowedFailures,
	}
	// Parsed reports are not available in normalize yet; an unexamined run has no synthetic test counts.
	out.Reports = make([]struct {
		AnalysisKind openapi.AnalysisKind         `json:"analysis_kind"`
		State        openapi.PipelineReportsState `json:"state"`
	}, 0)
	out.Tests.State = openapi.DataState(entity.StateNotEvaluated)
	return openapi.GetPipeline200JSONResponse(out), nil
}

func (h *codeDetails) ListPipelineAttempts(ctx context.Context, req openapi.ListPipelineAttemptsRequestObject) (openapi.ListPipelineAttemptsResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	q := entity.AttemptQuery{Limit: limit}
	if req.Params.Outcome != nil {
		if !req.Params.Outcome.Valid() {
			return nil, errMalformedPage
		}
		q.Outcome = entity.CIOutcome(*req.Params.Outcome)
	}
	// A cursor belongs to one tenant, reader, pipeline and filter. Source access is checked on every page.
	filter := s.OrganizationID.String() + ":" + s.UserID.String() + ":" + req.PipelineId.String() + ":" + string(q.Outcome)
	if req.Params.Cursor != nil {
		c, ok := decodeCursor(*req.Params.Cursor)
		if !ok || c.Kind != "pipeline_attempt" || c.Name != filter || c.At.IsZero() || c.ID == uuid.Nil {
			return nil, errMalformedPage
		}
		after := entity.ID(c.ID)
		q.After, q.IssuedAt = &after, &c.At
	}
	page, err := h.uc.PipelineAttempts(ctx, s, scope, entity.ID(req.PipelineId), q)
	if err != nil {
		return nil, err
	}
	out := openapi.AttemptPage{Data: make([]openapi.Attempt, len(page.Items)), UsageSeconds: metricValue("seconds", &page.UsageSeconds)}
	for i := range page.Items {
		a := &page.Items[i]
		var duration *int
		if a.DurationS != nil {
			n := int(*a.DurationS)
			duration = &n
		}
		out.Data[i] = openapi.Attempt{
			Id: uuid.UUID(a.AttemptID), JobKey: a.JobKey, ConfigFingerprint: hex.EncodeToString(a.ConfigFingerprint), AttemptNo: a.AttemptNo,
			Outcome: openapi.AttemptOutcome(a.Outcome), NativeStatus: a.NativeStatus, AllowFailure: a.AllowFailure,
			FailureKind: optional(a.FailureKind), RunnerClass: optional(a.RunnerClass), StartedAt: a.StartedAt, FinishedAt: a.FinishedAt, DurationSeconds: duration,
		}
	}
	if page.Next != nil {
		next := encodeCursor(cursorData{Kind: "pipeline_attempt", Name: filter, At: page.IssuedAt, ID: uuid.UUID(*page.Next)})
		out.NextCursor = &next
	}
	return openapi.ListPipelineAttempts200JSONResponse(out), nil
}
