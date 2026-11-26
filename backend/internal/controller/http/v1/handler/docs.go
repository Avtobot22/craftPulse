package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

type documentation struct {
	uc        usecase.Documentation
	documents usecase.ServiceDocumentation
}

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

func (h *documentation) ListServiceDocuments(ctx context.Context, req openapi.ListServiceDocumentsRequestObject) (openapi.ListServiceDocumentsResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	const kind = "service_documents"
	key := s.OrganizationID.String() + ":" + s.UserID.String() + ":" + req.ServiceId.String()
	var after entity.ID
	if req.Params.Cursor != nil {
		if len(*req.Params.Cursor) > 512 {
			return nil, errMalformedPage
		}
		page, ok := decodeCursor(*req.Params.Cursor)
		if !ok || page.Kind != kind || page.Name != key || page.ID == uuid.Nil {
			return nil, errMalformedPage
		}
		after = entity.ID(page.ID)
	}
	page, err := h.documents.ListServiceDocuments(ctx, s, scope, entity.ID(req.ServiceId), after, limit)
	if err != nil {
		return nil, err
	}
	out := openapi.DocumentPage{
		DocumentationState: openapi.DataState(page.State), Data: make([]openapi.Document, len(page.Items)),
		AccessNotes: accessNotes(page.Notes), AccessLimited: len(page.Notes) != 0,
	}
	for i := range page.Items {
		d := &page.Items[i]
		out.Data[i] = openapi.Document{
			Id: uuid.UUID(d.DocumentID), Title: optional(d.Title), Path: new(d.Path), VersionRef: d.VersionRef,
			DocFormat: openapi.DocumentDocFormat(d.Format), LastCheckedAt: d.LastCheckedAt,
			OpenViolations: d.OpenViolations, SourceUrl: d.SourceURL,
		}
	}
	if page.Next != nil {
		out.NextCursor = new(encodeCursor(cursorData{Kind: kind, Name: key, ID: uuid.UUID(*page.Next)}))
	}
	return openapi.ListServiceDocuments200JSONResponse(out), nil
}

func (h *documentation) RequestDocumentationCheck(ctx context.Context, req openapi.RequestDocumentationCheckRequestObject) (openapi.RequestDocumentationCheckResponseObject, error) {
	s, scope, err := subjectScope(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil || req.Body.RootId == uuid.Nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/root_id", Code: entity.FieldRequired}}}
	}
	op, err := h.documents.RequestDocumentationCheck(ctx, s, scope, entity.ID(req.ServiceId), entity.ID(req.Body.RootId))
	if err != nil {
		return nil, err
	}
	location := "/api/v1/operations/" + op.ID.String()
	return openapi.RequestDocumentationCheck202JSONResponse{
		Body: operationBody(op), Headers: openapi.RequestDocumentationCheck202ResponseHeaders{Location: &location},
	}, nil
}
