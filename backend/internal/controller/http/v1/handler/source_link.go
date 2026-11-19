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

type sourceLinks struct {
	uc          usecase.SourceLinks
	callbackURL string
}

func (h *sourceLinks) ListSourceLinks(ctx context.Context, _ openapi.ListSourceLinksRequestObject) (openapi.ListSourceLinksResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	links, err := h.uc.ListSourceLinks(ctx, s)
	if err != nil {
		return nil, err
	}
	out := openapi.SourceLinkList{Data: make([]openapi.SourceLink, 0, len(links))}
	for _, l := range links {
		out.Data = append(out.Data, openapi.SourceLink{
			ConnectionId: uuid.UUID(l.ConnectionID), Provider: openapi.Provider(l.Provider),
			DisplayName: l.DisplayName, State: openapi.SourceLinkState(l.State), LinkedAt: l.LinkedAt,
		})
	}
	return openapi.ListSourceLinks200JSONResponse(out), nil
}

func (h *sourceLinks) StartSourceLink(ctx context.Context, req openapi.StartSourceLinkRequestObject) (openapi.StartSourceLinkResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	address, err := h.uc.StartSourceLink(ctx, s, entity.ID(req.ConnectionId), value(req.Params.ReturnTo), h.callbackURL)
	if err != nil {
		return nil, err
	}
	return openapi.StartSourceLink200JSONResponse{AuthorizationUrl: address}, nil
}

func (h *sourceLinks) CompleteSourceLink(ctx context.Context, req openapi.CompleteSourceLinkRequestObject) (openapi.CompleteSourceLinkResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	location := h.uc.CompleteSourceLink(ctx, s, entity.SourceLinkCallback{
		State: req.Params.State,
		Code:  entity.Secret(value(req.Params.Code)), Error: value(req.Params.Error),
	})
	return sourceLinkRedirect{location: location}, nil
}

func (h *sourceLinks) DeleteSourceLink(ctx context.Context, req openapi.DeleteSourceLinkRequestObject) (openapi.DeleteSourceLinkResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	if err := h.uc.DeleteSourceLink(ctx, s, entity.ID(req.ConnectionId)); err != nil {
		return nil, err
	}
	return openapi.DeleteSourceLink204Response{}, nil
}

type sourceLinkRedirect struct{ location string }

func (r sourceLinkRedirect) VisitCompleteSourceLinkResponse(w http.ResponseWriter) error {
	w.Header().Set("Location", r.location)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusFound)
	return nil
}
