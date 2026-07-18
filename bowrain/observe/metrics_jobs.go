package observe

import (
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Background-job and business metrics. These are the worker's golden signals —
// the server exposes only HTTP metrics, so without these the entire async
// pipeline (translation, extraction, brand-scan, convergence) is unobservable.

var (
	// JobsProcessedTotal counts terminal job outcomes by queue and result.
	// outcome ∈ {success, failed, transient}. queue is a bounded label
	// (translation|extraction|brand-scan|...), never a job ID.
	// PromQL:  sum by (queue) (rate(bowrain_jobs_processed_total{outcome="failed"}[5m]))
	// Alert:   rate(bowrain_jobs_processed_total{outcome="failed"}[10m]) > 0.1
	JobsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bowrain",
			Subsystem: "jobs",
			Name:      "processed_total",
			Help:      "Total jobs processed by queue and terminal outcome (success|failed|transient).",
		},
		[]string{"queue", "outcome"},
	)

	// JobDuration measures end-to-end job processing latency by queue.
	// PromQL: histogram_quantile(0.95, sum by (le,queue) (rate(bowrain_jobs_duration_seconds_bucket[5m])))
	JobDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "bowrain",
			Subsystem: "jobs",
			Name:      "duration_seconds",
			Help:      "Job processing duration in seconds by queue.",
			// Jobs run longer than HTTP requests (AI calls); use wider buckets.
			Buckets: []float64{0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"queue"},
	)

	// JobsInFlight tracks jobs currently being processed, by queue.
	// PromQL: bowrain_jobs_in_flight
	JobsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "bowrain",
			Subsystem: "jobs",
			Name:      "in_flight",
			Help:      "Jobs currently being processed, by queue.",
		},
		[]string{"queue"},
	)

	// ConvergenceRunsTotal counts convergence-loop run outcomes.
	// outcome ∈ {completed, parked, failed, canceled}. This is the core product
	// loop — a rising failed/parked rate is a direct product-health signal.
	// PromQL: sum by (outcome) (rate(bowrain_convergence_runs_total[15m]))
	ConvergenceRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bowrain",
			Subsystem: "convergence",
			Name:      "runs_total",
			Help:      "Convergence run terminal outcomes (completed|parked|failed|canceled).",
		},
		[]string{"outcome"},
	)
)

// ObserveJob records a completed job's outcome and duration in one call. queue
// must be a bounded label value; outcome is success|failed|transient.
func ObserveJob(queue, outcome string, started time.Time) {
	JobsProcessedTotal.WithLabelValues(queue, outcome).Inc()
	JobDuration.WithLabelValues(queue).Observe(time.Since(started).Seconds())
}

// RegisterDBStats exposes a database/sql pool's live stats as gauges. Call once
// per pool (name distinguishes multiple pools). Cheap: read on scrape.
//
// PromQL: bowrain_db_pool_in_use / bowrain_db_pool_open  (saturation)
// Alert:  bowrain_db_pool_wait_count increasing → pool too small
func RegisterDBStats(db *sql.DB, name string) {
	labels := prometheus.Labels{"pool": name}
	factory := promauto.With(prometheus.DefaultRegisterer)
	factory.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "bowrain", Subsystem: "db_pool", Name: "open", ConstLabels: labels,
		Help: "Open connections (in use + idle).",
	}, func() float64 { return float64(db.Stats().OpenConnections) })
	factory.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "bowrain", Subsystem: "db_pool", Name: "in_use", ConstLabels: labels,
		Help: "Connections currently in use.",
	}, func() float64 { return float64(db.Stats().InUse) })
	factory.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "bowrain", Subsystem: "db_pool", Name: "idle", ConstLabels: labels,
		Help: "Idle connections.",
	}, func() float64 { return float64(db.Stats().Idle) })
	factory.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "bowrain", Subsystem: "db_pool", Name: "wait_count", ConstLabels: labels,
		Help: "Total number of connections waited for (monotonic).",
	}, func() float64 { return float64(db.Stats().WaitCount) })
}
