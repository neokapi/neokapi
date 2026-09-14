package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDecide_NotApplicableIsNeverAPass holds a report whose only analyzer had no
// rule to apply: it checked nothing, so the run did not run.
func TestDecide_NotApplicableIsNeverAPass(t *testing.T) {
	r := Report{
		Target: Target{Kind: "file", Blocks: 3},
		Execution: &Execution{Analyzers: []AnalyzerExecution{{
			ID: "voice.rules", Status: AnalyzerNotApplicable, Reason: "The voice profile declares no term or pattern.",
		}}},
	}
	r.Decide()
	assert.Equal(t, VerdictDidNotRun, r.Verdict)
	assert.False(t, r.Pass)
	assert.Equal(t, CauseContentNotChecked, r.DidNotRunCause)
	assert.Equal(t, []string{"no analyzer completed a check"}, r.DidNotRun)
}
