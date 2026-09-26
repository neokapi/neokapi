package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvalSettle_MeasureIsMet runs Measure 3: every merge order establishes the
// expected rules and no other, and every rule ends at the same status.
func TestEvalSettle_MeasureIsMet(t *testing.T) {
	got, err := measureEvalSettle(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 24, got.Orders, "four machines merge in 24 orders")
	assert.Empty(t, got.Differing)
	assert.Equal(t, got.Expected, got.Established)
	assert.True(t, got.Met(), got.Verdict)
	assert.Equal(t, "contested", got.Outcome["utilise"], "a rival rule leaves a merged suggestion contested")
	assert.Equal(t, "contested", got.Outcome["e-mail"], "a correction away leaves it contested")
	assert.Equal(t, "suggested", got.Outcome["whitelist"], "standing alone settles nothing")

	var out strings.Builder
	renderEvalSettle(&out, &got)
	assert.Contains(t, out.String(), "## Measure 3: settling")
	assert.Contains(t, out.String(), "met: 3 of 7 rules established")
}
