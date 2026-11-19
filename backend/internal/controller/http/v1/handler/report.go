package handler

import (
	"context"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

type reports struct {
	operations usecase.Operations
	views      usecase.TeamReports
}

func (h *reports) RefreshSeries(ctx context.Context, req openapi.RefreshSeriesRequestObject) (openapi.RefreshSeriesResponseObject, error) {
	s, ok := middleware.SubjectFrom(ctx)
	if !ok {
		return nil, entity.ErrAuthRequired
	}
	op, err := h.operations.RequestSeriesRefresh(ctx, s, entity.ID(req.SeriesId))
	if err != nil {
		return nil, err
	}
	location := "/api/v1/operations/" + op.ID.String()
	return openapi.RefreshSeries202JSONResponse{Body: operationBody(op), Headers: openapi.RefreshSeries202ResponseHeaders{Location: &location}}, nil
}
