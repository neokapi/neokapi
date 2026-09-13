package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestShipGateRollupAdd pins how one computed pair is tallied, which the SQL
// rollup mirrors for the pairs it counts itself. A dimension that governs the
// locale but holds no result leaves a clean pair not checked, and a dimension
// that governs nothing there leaves it alone.
func TestShipGateRollupAdd(t *testing.T) {
	cases := []struct {
		name  string
		v     ShipGateVerdict
		voice ShipGateVoice
		want  ShipGateCounts
	}{
		{
			name:  "a failing pair is failing whatever else holds",
			v:     ShipGateVerdict{Fails: true, Terms: TermComplianceViolation},
			voice: ShipGateVoice{Governed: true},
			want:  ShipGateCounts{Failing: 1},
		},
		{
			name:  "a checked clean pair with a passing score is clean",
			v:     ShipGateVerdict{Terms: TermComplianceCompliant},
			voice: ShipGateVoice{Governed: true, Scored: true},
			want:  ShipGateCounts{Clean: 1, Scored: 1},
		},
		{
			name: "terminology with no verdict leaves a clean pair not checked",
			v:    ShipGateVerdict{Terms: TermComplianceUnchecked},
			want: ShipGateCounts{Clean: 1, TermsNotChecked: 1, NotChecked: 1},
		},
		{
			name:  "a governing voice bar with no score leaves a clean pair not checked",
			v:     ShipGateVerdict{Terms: TermComplianceCompliant},
			voice: ShipGateVoice{Governed: true},
			want:  ShipGateCounts{Clean: 1, NotChecked: 1},
		},
		{
			name: "terminology that governs nothing owes no verdict",
			v:    ShipGateVerdict{Terms: TermComplianceNotGoverned},
			want: ShipGateCounts{Clean: 1},
		},
		{
			name: "an unscored pair where no voice profile governs owes no score",
			v:    ShipGateVerdict{Terms: TermComplianceCompliant},
			want: ShipGateCounts{Clean: 1},
		},
		{
			name:  "a score below the bar is a verdict rather than a missing result",
			v:     ShipGateVerdict{Terms: TermComplianceUnchecked},
			voice: ShipGateVoice{Governed: true, Scored: true, BelowBar: true},
			want:  ShipGateCounts{Clean: 1, Scored: 1, CleanBelowBar: 1, TermsNotChecked: 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r ShipGateRollup
			r.Add("col", "fr", tc.v, tc.voice)
			assert.Equal(t, tc.want, r.CountsFor("col", "fr"))
		})
	}
}
