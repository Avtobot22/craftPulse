package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

type documentation struct{ uc usecase.Documentation }

func (h *documentation) ListDocumentationRoots(ctx context.Context, req openapi.ListDocumentationRootsRequestObject) (openapi.ListDocumentationRootsResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	set, err := h.uc.ListRoots(ctx, s, scope, entity.ID(req.ServiceId))
	if err != nil {
		return nil, err
	}
	tag := etag(set.Version)
	return openapi.ListDocumentationRoots200JSONResponse{Body: rootsBody(set.Roots), Headers: openapi.ListDocumentationRoots200ResponseHeaders{ETag: &tag}}, nil
}

func (h *documentation) ReplaceDocumentationRoots(ctx context.Context, req openapi.ReplaceDocumentationRootsRequestObject) (openapi.ReplaceDocumentationRootsResponseObject, error) {
	s, _, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil || req.Body.Data == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/data", Code: entity.FieldRequired}}}
	}
	roots := make([]entity.DocumentationRoot, len(req.Body.Data))
	for i, r := range req.Body.Data {
		roots[i] = entity.DocumentationRoot{AreaID: entity.ID(r.AreaId), PathPrefix: r.PathPrefix, ProfileID: r.ProfileId, Audience: string(r.Audience)}
		if r.OwnerPersonId != nil {
			id := entity.ID(*r.OwnerPersonId)
			roots[i].OwnerPersonID = &id
		}
	}
	base, ok := parseETag(req.Params.IfMatch)
	if !ok {
		base = -1
	}
	set, err := h.uc.ReplaceRoots(ctx, s, entity.ID(req.ServiceId), base, entity.ID(req.Params.IdempotencyKey), roots)
	if err != nil {
		return nil, err
	}
	tag := etag(set.Version)
	return openapi.ReplaceDocumentationRoots200JSONResponse{Body: rootsBody(set.Roots), Headers: openapi.ReplaceDocumentationRoots200ResponseHeaders{ETag: &tag}}, nil
}

func rootsBody(roots []entity.DocumentationRoot) openapi.DocumentationRootList {
	out := openapi.DocumentationRootList{Data: make([]openapi.DocumentationRoot, len(roots))}
	for i := range roots {
		r := &roots[i]
		out.Data[i] = openapi.DocumentationRoot{Id: uuid.UUID(r.ID), AreaId: uuid.UUID(r.AreaID), PathPrefix: r.PathPrefix, ProfileId: r.ProfileID, Audience: openapi.DocumentationRootAudience(r.Audience)}
		if r.OwnerPersonID != nil {
			id := uuid.UUID(*r.OwnerPersonID)
			out.Data[i].OwnerPersonId = &id
		}
	}
	return out
}
