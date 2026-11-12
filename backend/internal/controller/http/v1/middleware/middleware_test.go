package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestRecovery_HidesPanicDetails(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	e := gin.New()
	e.GET("/panic",
		Correlate(zerolog.New(&logs)),
		Observe(NewMetrics(prometheus.NewRegistry())),
		Recovery(),
		func(*gin.Context) { panic("boom at db 10.0.0.5") },
	)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/panic", http.NoBody))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), `"code":"internal_error"`)
	assert.NotContains(t, rec.Body.String(), "10.0.0.5")
	assert.NotContains(t, rec.Body.String(), "goroutine")
	assert.Contains(t, logs.String(), "boom at db 10.0.0.5")
}
