package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAdapterCertificateTelemetry(t *testing.T) {
	t.Parallel()
	expiry := time.Now().Add(20 * 24 * time.Hour)
	handler := adapterMetrics(expiry)
	for _, path := range []string{"/livez", "/metrics"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody))
		assert.Equal(t, http.StatusOK, rec.Code)
		if path == "/metrics" {
			assert.Contains(t, rec.Body.String(),
				fmt.Sprintf("craftpulse_adapter_tls_not_after_timestamp_seconds %g", float64(expiry.Unix())))
		}
	}

	for _, tc := range []struct {
		expiry time.Time
		status int
	}{{expiry, http.StatusOK}, {time.Now().Add(-time.Hour), http.StatusServiceUnavailable}} {
		rec := httptest.NewRecorder()
		adapterBeforeExpiry(handler, tc.expiry).ServeHTTP(rec,
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/livez", http.NoBody))
		assert.Equal(t, tc.status, rec.Code)
	}
}
