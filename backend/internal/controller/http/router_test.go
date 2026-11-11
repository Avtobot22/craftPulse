package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

type problem struct {
	Type          string    `json:"type"`
	Status        int       `json:"status"`
	Code          string    `json:"code"`
	CorrelationID uuid.UUID `json:"correlation_id"`
}

type fixture struct {
	handler http.Handler
	metrics http.Handler
	logs    *bytes.Buffer
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	var logs bytes.Buffer
	reg := prometheus.NewRegistry()
	router := httpctrl.NewRouter(zerolog.New(&logs), reg, &health.Readiness{}, handler.New())
	return fixture{handler: router, metrics: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), logs: &logs}
}

func (f fixture) do(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) problem {
	t.Helper()
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	var p problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	require.Equal(t, rec.Code, p.Status)
	require.NotEqual(t, uuid.Nil, p.CorrelationID)
	return p
}

func TestApp_UnimplementedOperation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	rec := f.do(t, httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil))

	p := decodeProblem(t, rec)
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
	assert.Equal(t, "not_implemented", p.Code)
	assert.Equal(t, "urn:craftpulse:problem:not_implemented", p.Type)

	var entry struct {
		CorrelationID string `json:"correlation_id"`
		OperationID   string `json:"operation_id"`
		Time          string `json:"time"`
	}
	require.NoError(t, json.Unmarshal(f.logs.Bytes(), &entry))
	assert.Equal(t, p.CorrelationID.String(), entry.CorrelationID)
	assert.Equal(t, "listTeams", entry.OperationID)
}

func TestApp_ErrorResponses(t *testing.T) {
	t.Parallel()

	oversized := httptest.NewRequest(http.MethodPost, "/api/v1/imports", strings.NewReader("{}"))
	oversized.ContentLength = 25<<20 + 1
	tests := []struct {
		name       string
		giveReq    *http.Request
		wantStatus int
		wantCode   string
	}{
		{
			name:       "login is served from the site root",
			giveReq:    httptest.NewRequest(http.MethodGet, "/o/acme/login", nil),
			wantStatus: http.StatusNotImplemented,
			wantCode:   "not_implemented",
		},
		{
			name:       "unknown path",
			giveReq:    httptest.NewRequest(http.MethodGet, "/api/v1/nothing", nil),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "invalid parameter",
			giveReq:    httptest.NewRequest(http.MethodGet, "/api/v1/teams?limit=MARKER", nil),
			wantStatus: http.StatusBadRequest,
			wantCode:   "malformed_request",
		},
		{
			name:       "body over limit",
			giveReq:    oversized,
			wantStatus: http.StatusRequestEntityTooLarge,
			wantCode:   "payload_too_large",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := newFixture(t).do(t, tt.giveReq)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantCode, decodeProblem(t, rec).Code)
			assert.NotContains(t, rec.Body.String(), "MARKER")
		})
	}
}

func TestApp_ProcessStateAndMetrics(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	assert.Equal(t, http.StatusOK, f.do(t, httptest.NewRequest(http.MethodGet, "/livez", nil)).Code)
	assert.Equal(t, http.StatusServiceUnavailable, f.do(t, httptest.NewRequest(http.MethodGet, "/readyz", nil)).Code)
	f.do(t, httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil))

	rec := httptest.NewRecorder()
	f.metrics.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Contains(t, rec.Body.String(), `craftpulse_api_requests_total{operation_id="listTeams",status="501"} 1`)
}
