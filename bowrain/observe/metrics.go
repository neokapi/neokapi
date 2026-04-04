package observe

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal counts total HTTP requests.
	// PromQL: rate(bowrain_http_requests_total[5m])
	// Alert:  rate(bowrain_http_requests_total{status=~"5.."}[5m]) > 1
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bowrain",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total HTTP requests by method, route pattern, and status code.",
		},
		[]string{"method", "route", "status"},
	)

	// HTTPRequestDuration measures request latency.
	// PromQL: histogram_quantile(0.99, rate(bowrain_http_request_duration_seconds_bucket[5m]))
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "bowrain",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)

	// HTTPRequestsInFlight tracks concurrent in-flight requests.
	// PromQL: bowrain_http_requests_in_flight
	HTTPRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "bowrain",
		Subsystem: "http",
		Name:      "requests_in_flight",
		Help:      "Current number of HTTP requests being processed.",
	})
)
