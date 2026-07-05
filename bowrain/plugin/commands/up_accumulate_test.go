package commands

import (
	"testing"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunAccumulator_PreservesFinalStandingState: the live locale_done stream
// carries no per-locale State (a whole-pass verdict), so the accumulator must
// take shippable/parked from the run's authoritative final standing rather than
// clobbering it with the streamed empty state (regression: a converged server
// run printed every locale as "pending").
func TestRunAccumulator_PreservesFinalStandingState(t *testing.T) {
	acc := &runAccumulator{}
	// Stream: a pass, per-locale done (no State), terminal converged.
	acc.observe(convergence.Event{Type: convergence.EventLocaleStart, Locale: "fr-FR", Units: 10})
	acc.observe(convergence.Event{Type: convergence.EventLocaleDone, Locale: "fr-FR", Units: 10, Done: 10, ViaAI: 10})
	acc.observe(convergence.Event{Type: convergence.EventPassDone, Pass: 1})
	acc.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunConverged})

	final := &apiclient.ConvergenceRun{
		State:  "converged",
		Passes: 1,
		Locales: []apiclient.ConvergenceLocaleStanding{
			{Locale: "fr-FR", State: convergence.LocaleShippable, Units: 10, Produced: 10, ViaAI: 10},
		},
	}
	out := acc.output(final)

	require.Len(t, out.Locales, 1)
	assert.Equal(t, "fr-FR", out.Locales[0].Locale)
	assert.True(t, out.Locales[0].Shippable, "final-standing shippable state must survive the streamed overlay")
	assert.True(t, out.Converged)
	assert.Empty(t, out.ParkedScopes)
	assert.NoError(t, acc.terminalError(final))
}

// TestRunAccumulator_ParkedFromFinalStanding: a parked run's parked locale is
// taken from the final standing and recorded as a parked scope.
func TestRunAccumulator_ParkedFromFinalStanding(t *testing.T) {
	acc := &runAccumulator{}
	acc.observe(convergence.Event{Type: convergence.EventLocaleDone, Locale: "de-DE", Units: 10, Done: 6, ViaAI: 6})
	acc.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunParked})

	final := &apiclient.ConvergenceRun{
		State: "parked",
		Locales: []apiclient.ConvergenceLocaleStanding{
			{Locale: "de-DE", State: convergence.LocaleParked, Units: 10, Produced: 6},
		},
	}
	out := acc.output(final)
	require.Len(t, out.Locales, 1)
	assert.True(t, out.Locales[0].Parked)
	assert.False(t, out.Converged)
	require.Len(t, out.ParkedScopes, 1)
	assert.Equal(t, "de-DE", out.ParkedScopes[0].Locale)
	assert.NoError(t, acc.terminalError(final))
}

// TestRunAccumulator_TerminalError: a failed/canceled run surfaces a non-nil
// error (so `kapi up` exits non-zero), while converged/parked do not.
func TestRunAccumulator_TerminalError(t *testing.T) {
	failed := &runAccumulator{}
	failed.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunFailed})
	err := failed.terminalError(&apiclient.ConvergenceRun{State: "failed", Error: "provider outage"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider outage")

	canceled := &runAccumulator{}
	canceled.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunCanceled})
	assert.Error(t, canceled.terminalError(nil))

	// Fallback to the persisted final state when the stream was cut before the
	// terminal frame (sawDone never set).
	cut := &runAccumulator{}
	assert.Error(t, cut.terminalError(&apiclient.ConvergenceRun{State: "failed"}))

	ok := &runAccumulator{}
	ok.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunConverged})
	assert.NoError(t, ok.terminalError(nil))
}
