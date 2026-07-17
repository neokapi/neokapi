package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestReconcileSplit proves verification (c) at the reporting layer: given the
// TM hits recorded by the translation jobs, the produce emitter reports a
// non-zero ViaTM, and ViaTM + ViaAI always equals the reported Done. This is
// the truthful "TM N · AI M" the run standing carries, replacing the old
// hard-coded ViaTM=0.
func TestReconcileSplit(t *testing.T) {
	cases := []struct {
		name           string
		total, viaTM   int
		wantTM, wantAI int
	}{
		{"tm hits reported", 10, 4, 4, 6},
		{"all tm", 5, 5, 5, 0},
		{"no tm yet", 8, 0, 0, 8},
		{"tm over-reported clamps to total", 3, 9, 3, 0},
		{"negative tm clamps to zero", 4, -2, 0, 4},
		{"nothing done", 0, 2, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tm, ai := reconcileSplit(tc.total, tc.viaTM)
			assert.Equal(t, tc.wantTM, tm)
			assert.Equal(t, tc.wantAI, ai)
			if tc.total > 0 {
				assert.Equal(t, tc.total, tm+ai, "ViaTM + ViaAI must equal Done")
			}
		})
	}
}

// TestReconcileSplit_NonZeroViaTM is the explicit assertion for (c): when TM
// hits occur, ViaTM is reported non-zero (not the old hard-coded 0).
func TestReconcileSplit_NonZeroViaTM(t *testing.T) {
	tm, ai := reconcileSplit(20, 7)
	assert.Equal(t, 7, tm, "ViaTM must reflect the recycled blocks, not 0")
	assert.Equal(t, 13, ai)
}
