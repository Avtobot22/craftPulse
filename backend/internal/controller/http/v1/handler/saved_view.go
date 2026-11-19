package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
)

const _savedViewCursor = "saved_view"

func (h *reports) CreateSavedView(ctx context.Context, req openapi.CreateSavedViewRequestObject) (openapi.CreateSavedViewResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if req.Body == nil {
		return nil, errMalformedForm
	}
	filters, err := json.Marshal(req.Body.Filters)
	if err != nil {
		return nil, fmt.Errorf("handler: encode saved filters: %w", err)
	}
	v, err := h.views.CreateSavedView(ctx, s, entity.SavedView{
		Name: req.Body.Name, SeriesID: entity.ID(req.Body.SeriesId), Filters: filters, PinnedRevisionID: (*entity.ID)(req.Body.PinnedRevisionId),
	})
	if err != nil {
		return nil, err
	}
	body, err := savedViewBody(&v)
	if err != nil {
		return nil, err
	}
	location := "/api/v1/saved-views/" + v.ID.String()
	return openapi.CreateSavedView201JSONResponse{Body: body, Headers: openapi.CreateSavedView201ResponseHeaders{Location: &location}}, nil
}

func (h *reports) ListSavedViews(ctx context.Context, req openapi.ListSavedViewsRequestObject) (openapi.ListSavedViewsResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	limit, err := pageLimit(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	q := entity.SavedViewQuery{Limit: limit}
	if req.Params.Cursor != nil {
		c, ok := decodeCursor(*req.Params.Cursor)
		if !ok || c.Kind != _savedViewCursor || c.Name != s.UserID.String() || c.At.IsZero() || c.ID == uuid.Nil {
			return nil, errMalformedPage
		}
		q.After = &entity.SavedViewKey{CreatedAt: c.At, ID: entity.ID(c.ID)}
	}
	page, err := h.views.ListSavedViews(ctx, s, q)
	if err != nil {
		return nil, err
	}
	out := openapi.SavedViewPage{Data: make([]openapi.SavedView, len(page.Items))}
	for i := range page.Items {
		if out.Data[i], err = savedViewBody(&page.Items[i]); err != nil {
			return nil, err
		}
	}
	if n := page.Next; n != nil {
		next := encodeCursor(cursorData{Kind: _savedViewCursor, Name: s.UserID.String(), At: n.CreatedAt, ID: uuid.UUID(n.ID)})
		out.NextCursor = &next
	}
	return openapi.ListSavedViews200JSONResponse(out), nil
}

func (h *reports) GetSavedView(ctx context.Context, req openapi.GetSavedViewRequestObject) (openapi.GetSavedViewResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	v, err := h.views.OpenSavedView(ctx, s, entity.ID(req.SavedViewId))
	if err != nil {
		return nil, err
	}
	out, err := savedViewBody(&v.View)
	if err != nil {
		return nil, err
	}
	r := teamReport(&v.Report, &v.Query)
	out.Report = &r
	return openapi.GetSavedView200JSONResponse(out), nil
}

func (h *reports) DeleteSavedView(ctx context.Context, req openapi.DeleteSavedViewRequestObject) (openapi.DeleteSavedViewResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if err := h.views.DeleteSavedView(ctx, s, entity.ID(req.SavedViewId)); err != nil {
		return nil, err
	}
	return openapi.DeleteSavedView204Response{}, nil
}

func savedViewBody(v *entity.SavedView) (openapi.SavedView, error) {
	out := openapi.SavedView{Id: uuid.UUID(v.ID), Name: v.Name, SeriesId: uuid.UUID(v.SeriesID), PinnedRevisionId: uuidPtr(v.PinnedRevisionID), CreatedAt: v.CreatedAt}
	if err := json.Unmarshal(v.Filters, &out.Filters); err != nil {
		return openapi.SavedView{}, fmt.Errorf("handler: decode saved filters: %w", err)
	}
	return out, nil
}
