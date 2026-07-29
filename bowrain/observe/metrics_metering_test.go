package observe

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeteringDiscarded(t *testing.T) {
	tests := []struct {
		name     string
		meter    string
		quantity float64
		wantUnit string
		wantAdd  float64
	}{
		{"ai tokens", MeterAITokens, 1200, "tokens", 1200},
		{"agent tokens", MeterAgentTokens, 40, "tokens", 40},
		{"runner seconds", MeterRunnerSeconds, 12.5, "seconds", 12.5},
		{"unknown meter still counted", "invented", 3, "unknown", 3},
		// A meter write can fail with nothing to attribute (a zero-token
		// provider response); the failure still counts, the quantity does not.
		{"zero quantity counts the failure only", MeterAITokens, 0, "tokens", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := testutil.ToFloat64(MeteringDiscardedTotal.WithLabelValues(tc.meter))
			beforeUnits := testutil.ToFloat64(MeteringUnrecordedTotal.WithLabelValues(tc.meter, tc.wantUnit))

			MeteringDiscarded(context.Background(), tc.meter, tc.quantity,
				assert.AnError, "operation", "test")

			assert.Equal(t, before+1,
				testutil.ToFloat64(MeteringDiscardedTotal.WithLabelValues(tc.meter)),
				"every discarded write is counted")
			assert.InDelta(t, beforeUnits+tc.wantAdd,
				testutil.ToFloat64(MeteringUnrecordedTotal.WithLabelValues(tc.meter, tc.wantUnit)), 0.0001,
				"the unrecorded quantity accumulates under the meter's own unit")
		})
	}
}

// TestMeteringDiscardedUnitsAreDeclared guards the pairing the call sites rely
// on: every meter constant has a unit, so no call site can mislabel one.
func TestMeteringDiscardedUnitsAreDeclared(t *testing.T) {
	for _, meter := range []string{MeterAITokens, MeterAgentTokens, MeterRunnerSeconds} {
		unit, ok := meterUnits[meter]
		require.True(t, ok, "meter %q has no declared unit", meter)
		assert.NotEmpty(t, unit)
	}
}
