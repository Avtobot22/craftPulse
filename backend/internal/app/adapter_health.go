package app

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Avtobot22/craftPulse/backend/pkg/health"
)

// Enforce certificate expiry even on TLS connections opened before it expired.
func adapterBeforeExpiry(next http.Handler, notAfter time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !time.Now().Before(notAfter) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// The internal metrics listener reports the loaded certificate even when its TLS handshake fails.
func adapterMetrics(notAfter time.Time) http.Handler {
	expiry := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "craftpulse_adapter_tls_not_after_timestamp_seconds",
		Help: "Expiry timestamp of the TLS certificate loaded by the adapter.",
	})
	expiry.Set(float64(notAfter.Unix()))
	reg := prometheus.NewRegistry()
	reg.MustRegister(expiry)
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.Handle("GET /livez", health.Handler(health.Probes{}))
	return mux
}
