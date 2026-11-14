package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpctrl "github.com/Avtobot22/craftPulse/backend/internal/controller/http"
	v1 "github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/handler"
	"github.com/Avtobot22/craftPulse/backend/internal/controller/http/v1/openapi"
	"github.com/Avtobot22/craftPulse/backend/internal/entity"
	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

const (
	_site  = "https://craftpulse.test"
	_token = "valid-session-token-of-the-browser-0123456"
)

// fakeLogins accepts only _token; authenticated counts the session lookups.
type fakeLogins struct {
	authenticated int
}

func (*fakeLogins) StartLogin(context.Context, string, string) (entity.LoginStart, error) {
	return entity.LoginStart{RedirectURL: "https://idp.test/authorize", Cookie: "login-cookie"}, nil
}

func (*fakeLogins) CompleteLogin(context.Context, entity.LoginCallback) (entity.LoginResult, error) {
	return entity.LoginResult{}, &entity.LoginError{Class: entity.LoginInvalidResponse}
}

func (f *fakeLogins) Authenticate(_ context.Context, token string) (entity.Subject, error) {
	f.authenticated++
	if token != _token {
		return entity.Subject{}, entity.ErrAuthRequired
	}
	return entity.Subject{UserID: entity.ID{1}}, nil
}

func (*fakeLogins) Logout(context.Context, entity.Subject) error { return nil }

type fakeAccess struct {
	err error
}

func (f fakeAccess) Authorize(context.Context, entity.Subject, entity.AccessRequest) (entity.Scope, error) {
	return entity.Scope{}, f.err
}

func (fakeAccess) Decide(_ context.Context, _ entity.Subject, scope entity.Scope, _ entity.SourceQuery) (entity.Scope, error) {
	return scope, nil
}

func (fakeAccess) CurrentUser(context.Context, entity.Subject) (entity.CurrentUser, error) {
	return entity.CurrentUser{}, nil
}

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
	logins  *fakeLogins
	routes  gin.RoutesInfo
}

func newFixture(t *testing.T, access fakeAccess) fixture {
	t.Helper()
	var logs bytes.Buffer
	reg := prometheus.NewRegistry()
	logins := &fakeLogins{}
	router := httpctrl.NewRouter(zerolog.New(&logs), reg, health.Probes{Ready: &health.Readiness{}}, v1.Deps{
		Handler:    handler.New(handler.Deps{Logins: logins, Access: access, Registerer: reg}),
		Logins:     logins,
		Access:     access,
		SiteOrigin: _site,
	})
	return fixture{
		handler: router, metrics: promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), logs: &logs,
		logins: logins, routes: router.Routes(),
	}
}

func withSession(r *http.Request) *http.Request {
	r.AddCookie(&http.Cookie{Name: "__Host-cp_session", Value: _token})
	return r
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
	f := newFixture(t, fakeAccess{})

	rec := f.do(t, withSession(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/api-keys", http.NoBody)))

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
	assert.Equal(t, "listApiKeys", entry.OperationID)
}

func TestApp_ErrorResponses(t *testing.T) {
	t.Parallel()

	oversized := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/imports", strings.NewReader("{}"))
	oversized.ContentLength = 25<<20 + 1
	tests := []struct {
		name       string
		giveReq    *http.Request
		giveAccess fakeAccess
		wantStatus int
		wantCode   string
	}{
		{
			name:       "no session",
			giveReq:    httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/me", http.NoBody),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "auth_required",
		},
		{
			name:       "ended session",
			giveReq:    withCookie(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/me", http.NoBody), "MARKER"),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "auth_required",
		},
		{
			name:       "area outside the scope",
			giveReq:    withSession(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/teams", http.NoBody)),
			giveAccess: fakeAccess{err: entity.ErrNotFound},
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "role without the action",
			giveReq:    withSession(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/teams", http.NoBody)),
			giveAccess: fakeAccess{err: entity.ErrForbidden},
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
		{
			name:       "unknown path",
			giveReq:    httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/nothing", http.NoBody),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "invalid parameter",
			giveReq:    withSession(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/api-keys?limit=MARKER", http.NoBody)),
			wantStatus: http.StatusBadRequest,
			wantCode:   "malformed_request",
		},
		{
			name:       "unknown query parameter",
			giveReq:    withSession(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/teams?sorting=MARKER", http.NoBody)),
			wantStatus: http.StatusBadRequest,
			wantCode:   "unknown_parameter",
		},
		{
			name: "versioned change without If-Match",
			giveReq: withOrigin(withSession(httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/api/v1/organization/structure",
				strings.NewReader("{}"))), _site),
			wantStatus: http.StatusPreconditionRequired,
			wantCode:   "precondition_required",
		},
		{
			name:       "body over limit",
			giveReq:    withOrigin(withSession(oversized), _site),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantCode:   "payload_too_large",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := newFixture(t, tt.giveAccess).do(t, tt.giveReq)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantCode, decodeProblem(t, rec).Code)
			assert.NotContains(t, rec.Body.String(), "MARKER")
		})
	}
}

func TestApp_ProcessStateAndMetrics(t *testing.T) {
	t.Parallel()
	f := newFixture(t, fakeAccess{})

	assert.Equal(t, http.StatusOK, f.do(t, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/livez", http.NoBody)).Code)
	assert.Equal(t, http.StatusServiceUnavailable, f.do(t, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", http.NoBody)).Code)
	f.do(t, withSession(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/api-keys", http.NoBody)))

	rec := httptest.NewRecorder()
	f.metrics.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", http.NoBody))
	assert.Contains(t, rec.Body.String(), `craftpulse_api_requests_total{operation_id="listApiKeys",status="501"} 1`)
}

func TestApp_OriginOfChangingRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		giveOrigin string
	}{
		{name: "foreign origin", giveOrigin: "https://evil.test"},
		{name: "empty origin", giveOrigin: ""},
		{name: "origin with a path", giveOrigin: _site + "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, fakeAccess{})
			req := withSession(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/session/logout", http.NoBody))

			rec := f.do(t, withOrigin(req, tt.giveOrigin))

			assert.Equal(t, http.StatusForbidden, rec.Code)
			assert.Equal(t, "origin_rejected", decodeProblem(t, rec).Code)
			assert.Zero(t, f.logins.authenticated, "the request is refused before the session is read")
		})
	}

	f := newFixture(t, fakeAccess{})
	req := withSession(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/session/logout", http.NoBody))
	rec := f.do(t, withOrigin(req, _site))
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Contains(t, rec.Header().Get("Set-Cookie"), "__Host-cp_session=;")
}

func TestApp_LoginIsServedFromSiteRoot(t *testing.T) {
	t.Parallel()
	f := newFixture(t, fakeAccess{err: entity.ErrForbidden})

	start := f.do(t, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/o/acme/login", http.NoBody))
	callback := f.do(t, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/o/acme/callback?state=x&code=y", http.NoBody))

	assert.Equal(t, http.StatusFound, start.Code)
	assert.Equal(t, "https://idp.test/authorize", start.Header().Get("Location"))
	assert.Contains(t, start.Header().Get("Set-Cookie"), "__Host-cp_login=login-cookie; Path=/; Max-Age=600; HttpOnly; Secure; SameSite=Lax")
	assert.Equal(t, http.StatusFound, callback.Code)
	assert.Equal(t, "/o/acme?login_error=invalid_response", callback.Header().Get("Location"))
	assert.Zero(t, f.logins.authenticated)
}

func TestApp_EveryRouteHasContractAction(t *testing.T) {
	t.Parallel()
	f := newFixture(t, fakeAccess{})

	for _, r := range f.routes {
		if r.Path == "/livez" || r.Path == "/readyz" || r.Path == "/version" {
			continue
		}
		key := r.Method + " " + strings.TrimPrefix(r.Path, "/api/v1")
		op, ok := openapi.Operations[key]
		if assert.True(t, ok, key) {
			assert.NotEmpty(t, op.Action, key)
		}
	}
}

func withCookie(r *http.Request, value string) *http.Request {
	r.AddCookie(&http.Cookie{Name: "__Host-cp_session", Value: value})
	return r
}

func withOrigin(r *http.Request, origin string) *http.Request {
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}
