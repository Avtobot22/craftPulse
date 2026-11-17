package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

type links struct{ uc usecase.Links }

// _refKinds are the objects of normalize a link may end at, by the kinds of ObjectRef.
var _refKinds = map[entity.ObjectKind]openapi.ObjectRefKind{
	entity.ObjectTask: openapi.ObjectRefKindTask, "merge_request": openapi.ObjectRefKindMergeRequest,
	"ci_run": openapi.ObjectRefKindPipeline, "ci_attempt": openapi.ObjectRefKindAttempt, "document": openapi.ObjectRefKindDocument,
	"deployment": openapi.ObjectRefKindDeployment, "artifact": openapi.ObjectRefKindArtifact, "incident": openapi.ObjectRefKindIncident,
}

func (h *links) DecideLink(ctx context.Context, req openapi.DecideLinkRequestObject) (openapi.DecideLinkResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/decision", Code: entity.FieldRequired}}}
	}
	res, err := h.uc.DecideLink(ctx, s, scope, entity.ID(req.LinkId), entity.LinkDecision(req.Body.Decision), entity.ID(req.Params.IdempotencyKey))
	if err != nil {
		return nil, err
	}
	e := &res.Edge
	return openapi.DecideLink200JSONResponse{
		Id: uuid.UUID(e.ID), LinkType: openapi.LinkLinkType(e.Type), Method: openapi.LinkMethod(e.Method), Status: openapi.LinkStatus(e.Status),
		FromObjectId: uuid.UUID(e.From), DecidedAt: e.DecidedAt,
		To: openapi.ObjectRef{Kind: _refKinds[res.To.Kind], Id: uuid.UUID(res.To.ID), DisplayKey: res.To.DisplayKey, Title: res.To.Title},
	}, nil
}
