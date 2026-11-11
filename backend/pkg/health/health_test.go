package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestReadyz(t *testing.T) {
	t.Parallel()

	pass := func(context.Context) error { return nil }
	fail := func(context.Context) error { return errors.New("db is down") }
	tests := []struct {
		name       string
		giveChecks []health.Check
		giveDrain  bool
		wantStatus int
	}{
		{name: "no checks", wantStatus: http.StatusServiceUnavailable},
		{name: "all pass", giveChecks: []health.Check{pass}, wantStatus: http.StatusOK},
		{name: "one fails", giveChecks: []health.Check{pass, fail}, wantStatus: http.StatusServiceUnavailable},
		{name: "draining", giveChecks: []health.Check{pass}, giveDrain: true, wantStatus: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ready := &health.Readiness{}
			for _, c := range tt.giveChecks {
				ready.Register(c)
			}
			if tt.giveDrain {
				ready.Drain()
			}

			rec := get(health.Handler(ready), "/readyz")

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestLivezAndVersion(t *testing.T) {
	t.Parallel()

	h := health.Handler(&health.Readiness{})

	assert.Equal(t, http.StatusOK, get(h, "/livez").Code)
	version := get(h, "/version")
	assert.Equal(t, http.StatusOK, version.Code)
	assert.JSONEq(t, `{"commit_sha":"unknown","schema_version":null}`, version.Body.String())
}
