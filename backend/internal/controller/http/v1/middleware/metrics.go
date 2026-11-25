package middleware

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	denials  *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "craftpulse_api_requests_total",
			Help: "Requests handled by api by operationId and HTTP status.",
		}, []string{"operation_id", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "craftpulse_api_request_duration_seconds",
			Help:    "Request duration by operationId.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 2.5, 5, 10},
		}, []string{"operation_id"}),
		denials: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "craftpulse_api_access_denied_total",
			Help: "Requests refused by access, by subject and reason; growth for one subject means ID guessing.",
		}, []string{"subject", "reason"}),
	}
	reg.MustRegister(m.requests, m.duration, m.denials)
	return m
}

func (m *Metrics) observe(operationID string, status int, elapsed time.Duration) {
	m.requests.WithLabelValues(operationID, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(operationID).Observe(elapsed.Seconds())
}

func (m *Metrics) denied(subject, reason string) {
	m.denials.WithLabelValues(subject, reason).Inc()
}
