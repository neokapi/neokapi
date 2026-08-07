package observe

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Circuit-breaker and outbound-dependency series. The "dependency" label is a
// breaker name from bowrain/resilience — a bounded, code-declared identifier
// such as "ai:bedrock" or "connector:figma", never a workspace, URL, or job ID.
var (
	// BreakerState is the current position of each dependency's circuit:
	// 0 = closed, 1 = half-open, 2 = open. Ordered by severity so max() over
	// instances answers "is anything tripped anywhere".
	// PromQL: max by (dependency) (bowrain_dependency_breaker_state)
	// Alert:  max by (dependency) (bowrain_dependency_breaker_state) == 2
	BreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "bowrain",
			Subsystem: "dependency",
			Name:      "breaker_state",
			Help:      "Circuit breaker state per dependency (0=closed, 1=half-open, 2=open).",
		},
		[]string{"dependency", "kind"},
	)

	// BreakerTransitionsTotal counts state changes. A high open→half-open→open
	// rate means the dependency is flapping rather than simply down.
	// PromQL: rate(bowrain_dependency_breaker_transitions_total{to="open"}[15m])
	BreakerTransitionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bowrain",
			Subsystem: "dependency",
			Name:      "breaker_transitions_total",
			Help:      "Circuit breaker state transitions by dependency and direction.",
		},
		[]string{"dependency", "kind", "from", "to"},
	)

	// BreakerRejectionsTotal counts calls short-circuited without being
	// attempted. This is the load an open breaker is shedding — the number to
	// quote when explaining why work was deferred rather than run.
	// PromQL: rate(bowrain_dependency_breaker_rejections_total[5m])
	BreakerRejectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bowrain",
			Subsystem: "dependency",
			Name:      "breaker_rejections_total",
			Help:      "Calls rejected without being attempted because the circuit was open.",
		},
		[]string{"dependency", "kind"},
	)

	// DependencyCallsTotal counts attempted outbound calls by outcome:
	// success | failure (upstream trouble, counts toward tripping) |
	// rejected_upstream (the dependency answered with a permanent refusal) |
	// ignored (caller cancelled or caller-side fault) | rejected (open circuit).
	// PromQL: sum by (dependency) (rate(bowrain_dependency_calls_total{outcome="failure"}[5m]))
	DependencyCallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bowrain",
			Subsystem: "dependency",
			Name:      "calls_total",
			Help:      "Outbound dependency calls by dependency and outcome.",
		},
		[]string{"dependency", "kind", "outcome"},
	)

	// DependencyCallDuration measures outbound call latency. Rejections are not
	// observed here — they took no upstream time and would flatten the quantiles.
	// PromQL: histogram_quantile(0.99, rate(bowrain_dependency_call_duration_seconds_bucket[5m]))
	DependencyCallDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "bowrain",
			Subsystem: "dependency",
			Name:      "call_duration_seconds",
			Help:      "Outbound dependency call duration in seconds.",
			// LLM calls run to tens of seconds, so the default buckets (which
			// top out at 10s) would put every AI call in +Inf.
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60, 120},
		},
		[]string{"dependency", "kind"},
	)

	// JobsDeferredTotal counts jobs parked because a dependency was known-down.
	// A deferral is not a retry: it costs the job no attempt budget, so this
	// series is how an outage's cost shows up as waiting rather than failures.
	// PromQL: rate(bowrain_jobs_deferred_total[5m])
	JobsDeferredTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bowrain",
			Subsystem: "jobs",
			Name:      "deferred_total",
			Help:      "Jobs deferred (requeued without consuming a retry attempt) because a dependency was unavailable.",
		},
		[]string{"queue", "dependency"},
	)

	// EventsDroppedTotal counts events the in-process bus threw away because a
	// subscriber's buffer was full. It has no pending list and no redelivery,
	// so a drop here is permanent — this series is the only evidence it
	// happened, and a non-zero rate is the signal that the deployment has
	// outgrown the in-process bus and needs Redis.
	// PromQL: rate(bowrain_events_dropped_total[5m])
	EventsDroppedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bowrain",
			Subsystem: "events",
			Name:      "dropped_total",
			Help:      "Events dropped by the in-process event bus because a subscriber's buffer was full.",
		},
		[]string{"event_type"},
	)
)
