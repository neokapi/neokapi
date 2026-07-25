package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestReconcileSplit proves verification (c) at the reporting layer: given the
// content-memory hits recorded by the translation jobs, the produce emitter reports a
// non-zero ViaMemory, and ViaMemory + ViaAI always equals the reported Done. This is
// the truthful "content memory N · AI M" the run standing carries, replacing the old
// hard-coded ViaMemory=0.
func TestReconcileSplit(t *testing.T) {
	cases := []struct {
		name               string
		total, viaMemory   int
		wantMemory, wantAI int
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
			tm, ai := reconcileSplit(tc.total, tc.viaMemory)
			assert.Equal(t, tc.wantMemory, tm)
			assert.Equal(t, tc.wantAI, ai)
			if tc.total > 0 {
				assert.Equal(t, tc.total, tm+ai, "ViaMemory + ViaAI must equal Done")
			}
		})
	}
}

// TestReconcileSplit_NonZeroViaMemory is the explicit assertion for (c): when content memory
// hits occur, ViaMemory is reported non-zero (not the old hard-coded 0).
func TestReconcileSplit_NonZeroViaMemory(t *testing.T) {
	tm, ai := reconcileSplit(20, 7)
	assert.Equal(t, 7, tm, "ViaMemory must reflect the recycled blocks, not 0")
	assert.Equal(t, 13, ai)
}
