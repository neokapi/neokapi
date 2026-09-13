package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// A run learns whether its flow holds a check step when the collector is armed,
// before any file is read. A check flow whose files were all skipped builds no
// chain and taps nothing, and it still reports: its report is did_not_run.
func TestBeginFlowFindings_ACheckFlowThatReadNothingDidNotRun(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	var got []FlowFindings
	a.flowFindingsSink = func(f FlowFindings) { got = append(got, f) }

	a.beginFlowFindings("qa")()

	require.Len(t, got, 1, "a flow with a check step reports even when no chain was built")
	assert.Equal(t, 0, got[0].Files)
	assert.Equal(t, 0, got[0].Blocks)
	assert.Equal(t, check.CauseNothingToCheck, got[0].DidNotRunCause)
	assert.NotEmpty(t, got[0].DidNotRun)
}

// A flow with no check step has no findings to report, whatever it read.
func TestBeginFlowFindings_AFlowWithoutAChecksReportsNothing(t *testing.T) {
	a := &App{}
	a.InitRegistries()
	var got []FlowFindings
	a.flowFindingsSink = func(f FlowFindings) { got = append(got, f) }

	a.beginFlowFindings("pseudo-translate")()

	assert.Empty(t, got)
}
