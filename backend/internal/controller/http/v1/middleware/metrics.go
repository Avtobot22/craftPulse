package middleware

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
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
			Buckets: prometheus.DefBuckets,
		}, []string{"operation_id"}),
	}
	reg.MustRegister(m.requests, m.duration)
	return m
}

func (m *Metrics) observe(operationID string, status int, elapsed time.Duration) {
	m.requests.WithLabelValues(operationID, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(operationID).Observe(elapsed.Seconds())
}
