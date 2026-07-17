package commands

import (
	"testing"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/stretchr/testify/assert"
)

// TestRunNeedsConfirm covers the `kapi up` server-run confirmation gate (epic
// 019, theme B3): a run is gated only when it does paid AI work AND that work
// either exceeds the balance or crosses the size threshold.
func TestRunNeedsConfirm(t *testing.T) {
	tests := []struct {
		name string
		est  *apiclient.ConvergenceEstimate
		want bool
	}{
		{
			name: "no AI work never prompts",
			est: &apiclient.ConvergenceEstimate{
				Totals: apiclient.ConvergenceEstimateTotals{Pending: 100, ViaTM: 100, ViaAI: 0, TokenEstimate: 0},
			},
			want: false,
		},
		{
			name: "small AI work covered by balance does not prompt",
			est: &apiclient.ConvergenceEstimate{
				Totals:  apiclient.ConvergenceEstimateTotals{ViaAI: 10, TokenEstimate: 400},
				Credits: &apiclient.ConvergenceEstimateCredits{CoversAllAI: true},
			},
			want: false,
		},
		{
			name: "AI work exceeding balance prompts",
			est: &apiclient.ConvergenceEstimate{
				Totals:  apiclient.ConvergenceEstimateTotals{ViaAI: 10, TokenEstimate: 400},
				Credits: &apiclient.ConvergenceEstimateCredits{CoversAllAI: false, CoversAIUnits: 3},
			},
			want: true,
		},
		{
			name: "large AI work prompts even when covered",
			est: &apiclient.ConvergenceEstimate{
				Totals:  apiclient.ConvergenceEstimateTotals{ViaAI: 5000, TokenEstimate: confirmAITokenThreshold + 1},
				Credits: &apiclient.ConvergenceEstimateCredits{CoversAllAI: true},
			},
			want: true,
		},
		{
			name: "large AI work with no billing (self-hosted) still prompts on size",
			est: &apiclient.ConvergenceEstimate{
				Totals: apiclient.ConvergenceEstimateTotals{ViaAI: 5000, TokenEstimate: confirmAITokenThreshold + 1},
			},
			want: true,
		},
		{
			name: "nil estimate never prompts",
			est:  nil,
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, runNeedsConfirm(tc.est))
		})
	}
}
