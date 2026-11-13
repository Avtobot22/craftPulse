package handler

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

type uploads struct{ tokens usecase.UploadTokens }

func (h *uploads) CreateUploadToken(ctx context.Context, req openapi.CreateUploadTokenRequestObject) (openapi.CreateUploadTokenResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil {
		return nil, &entity.ValidationError{Fields: []entity.FieldError{{Field: "/project_id", Code: entity.FieldRequired}}}
	}
	t, value, err := h.tokens.CreateUploadToken(ctx, s, entity.ID(req.Body.ProjectId))
	if err != nil {
		return nil, err
	}
	location := "/api/v1/upload-tokens/" + t.ID.String()
	return uploadCreatedResponse{openapi.CreateUploadToken201JSONResponse{Headers: openapi.CreateUploadToken201ResponseHeaders{Location: &location}, Body: openapi.UploadTokenCreated{Id: uuid.UUID(t.ID), ProjectId: uuid.UUID(t.ProjectID), Slot: openapi.UploadTokenCreatedSlot(t.Slot), CreatedAt: t.CreatedAt, RevokedAt: t.RevokedAt, Value: value.Value()}}}, nil
}

func (h *uploads) ListUploadTokens(ctx context.Context, req openapi.ListUploadTokensRequestObject) (openapi.ListUploadTokensResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	project := entity.ID(req.Params.ProjectId)
	var after *entity.AuditKey
	if req.Params.Cursor != nil {
		c, ok := decodeCursor(*req.Params.Cursor)
		if !ok || c.Kind != "upload_token" || c.Name != project.String() || c.At.IsZero() || c.ID == uuid.Nil {
			return nil, errMalformedPage
		}
		after = &entity.AuditKey{OccurredAt: c.At, ID: entity.ID(c.ID)}
	}
	page, err := h.tokens.ListUploadTokens(ctx, s, project, limit, after)
	if err != nil {
		return nil, err
	}
	out := openapi.UploadTokenPage{Data: make([]openapi.UploadToken, 0, len(page.Items))}
	for _, t := range page.Items {
		out.Data = append(out.Data, openapi.UploadToken{Id: uuid.UUID(t.ID), ProjectId: uuid.UUID(t.ProjectID), Slot: openapi.UploadTokenSlot(t.Slot), CreatedAt: t.CreatedAt, RevokedAt: t.RevokedAt})
	}
	if page.Next != nil {
		c := encodeCursor(cursorData{Kind: "upload_token", Name: project.String(), At: page.Next.OccurredAt, ID: uuid.UUID(page.Next.ID)})
		out.NextCursor = &c
	}
	return openapi.ListUploadTokens200JSONResponse(out), nil
}

func (h *uploads) RevokeUploadToken(ctx context.Context, req openapi.RevokeUploadTokenRequestObject) (openapi.RevokeUploadTokenResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if err := h.tokens.RevokeUploadToken(ctx, s, entity.ID(req.UploadTokenId)); err != nil {
		return nil, err
	}
	return openapi.RevokeUploadToken204Response{}, nil
}

type uploadCreatedResponse struct {
	openapi.CreateUploadToken201JSONResponse
}

func (r uploadCreatedResponse) VisitCreateUploadTokenResponse(w http.ResponseWriter) error {
	w.Header().Set("Cache-Control", "no-store")
	return r.CreateUploadToken201JSONResponse.VisitCreateUploadTokenResponse(w)
}
