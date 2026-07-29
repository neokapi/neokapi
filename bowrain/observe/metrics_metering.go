package observe

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Usage metering is deliberately fail-open: the AI call has already been made
// and already cost real money by the time the meter is written, so a meter that
// is down must not also cost the customer the work. What fail-open must never
// mean is fail-silent — spend nobody recorded is spend nobody can bill, cap, or
// explain, and a discarded meter write leaves no trace anywhere else.
//
// Every discarded write therefore lands here: one log line naming the meter and
// the quantity, and two counters so an operator can answer "did we fail to
// meter anything today, and how much?" without reading logs at all.

// Meter names. Bounded label values — never a workspace, job, or model id.
const (
	// MeterAITokens is the ai_usage abuse-cap ledger (jobs.QuotaStore.RecordUsage).
	MeterAITokens = "ai_tokens"
	// MeterAgentTokens is the @bravo per-message token ledger (AgentStore.RecordUsage).
	MeterAgentTokens = "agent_tokens"
	// MeterRunnerSeconds is container/runner wall time (QuotaStore.RecordRunnerUsage).
	MeterRunnerSeconds = "runner_seconds"
)

// meterUnits maps each meter to the unit its quantity is measured in, so a call
// site cannot mislabel one.
var meterUnits = map[string]string{
	MeterAITokens:      "tokens",
	MeterAgentTokens:   "tokens",
	MeterRunnerSeconds: "seconds",
}

var (
	// MeteringDiscardedTotal counts usage-metering writes that failed and were
	// dropped rather than retried or escalated.
	// PromQL: sum by (meter) (increase(bowrain_metering_discarded_total[24h]))
	// Alert:  increase(bowrain_metering_discarded_total[1h]) > 0
	MeteringDiscardedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bowrain",
			Subsystem: "metering",
			Name:      "discarded_total",
			Help:      "Usage-metering writes that failed and were discarded, by meter.",
		},
		[]string{"meter"},
	)

	// MeteringUnrecordedTotal accumulates the quantity those discarded writes
	// were carrying — the "how much" half of the question. Units differ per
	// meter (tokens, seconds), hence the unit label; never sum across meters.
	// PromQL: sum by (meter, unit) (increase(bowrain_metering_unrecorded_total[24h]))
	MeteringUnrecordedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bowrain",
			Subsystem: "metering",
			Name:      "unrecorded_total",
			Help:      "Usage quantity that failed to be metered, by meter and unit.",
		},
		[]string{"meter", "unit"},
	)
)

// MeteringDiscarded records one usage-metering write that failed and was
// deliberately not retried. meter must be one of the Meter* constants; quantity
// is the amount that went unrecorded, in that meter's unit. attrs are extra
// slog key/value pairs identifying the work (job id, workspace, operation) and
// must not carry content or credentials.
func MeteringDiscarded(ctx context.Context, meter string, quantity float64, err error, attrs ...any) {
	unit, ok := meterUnits[meter]
	if !ok {
		unit = "unknown"
	}
	MeteringDiscardedTotal.WithLabelValues(meter).Inc()
	if quantity > 0 {
		MeteringUnrecordedTotal.WithLabelValues(meter, unit).Add(quantity)
	}

	args := make([]any, 0, len(attrs)+8)
	args = append(args, "meter", meter, "unit", unit, "quantity", quantity, "error", err)
	args = append(args, attrs...)
	slog.ErrorContext(ctx, "usage metering write discarded", args...)
}
