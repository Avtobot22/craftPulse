package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/cookie"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/middleware"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/internal/usecase"
)

type taskEventsStub struct {
	usecase.Logins
	usecase.Access
	usecase.TaskViews
	page entity.TaskEventPage
}

func (*taskEventsStub) Authenticate(context.Context, string) (entity.Subject, error) {
	return entity.Subject{UserID: entity.ID{1}}, nil
}

func (*taskEventsStub) Authorize(context.Context, entity.Subject, entity.AccessRequest) (entity.Scope, error) {
	return entity.Scope{}, nil
}

func (s *taskEventsStub) ListTaskEvents(context.Context, entity.Subject, entity.Scope, entity.ID, entity.TaskEventQuery) (entity.TaskEventPage, error) {
	return s.page, nil
}

func TestTaskEventsRouteServesTimeline(t *testing.T) {
	t.Parallel()
	taskID := entity.ID{1, 2}
	at := time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC)
	stub := &taskEventsStub{page: entity.TaskEventPage{Items: []entity.TaskHistoryEvent{{
		ID: entity.ID{3}, Object: entity.ObjectRef{ID: taskID, Kind: entity.ObjectTask},
		Kind: "comment", ActorKind: entity.PersonBot, EventTime: at, ObservedAt: at, ObservedVia: entity.ViaWebhook,
	}}}}
	server := &Handler{tasks: tasks{uc: stub}}
	router := gin.New()
	router.ContextWithFallback = true
	api := router.Group("/api/v1", middleware.Operation(openapi.Operations), middleware.Authenticate(stub, ""), middleware.Authorize(stub, nil))
	openapi.RegisterHandlers(api, openapi.NewStrictHandler(server, nil))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/tasks/"+taskID.String()+"/events", http.NoBody)
	req.AddCookie(&http.Cookie{Name: cookie.Session, Value: "test-session"})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var page openapi.TimelinePage
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &page))
	require.Len(t, page.Data, 1)
	require.Equal(t, uuid.UUID(taskID), page.Data[0].Object.Id)
	require.Equal(t, openapi.ActorRefKind("bot"), page.Data[0].Actor.Kind)
	require.Empty(t, page.AccessNotes)
}
