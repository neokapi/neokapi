package commands

import (
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunAccumulator_PreservesFinalStandingState: the live locale_done stream
// carries no per-locale State (a whole-pass verdict), so the accumulator must
// take shippable/parked from the run's authoritative final standing rather than
// clobbering it with the streamed empty state (regression: a converged server
// run printed every locale as "pending").
func TestRunAccumulator_PreservesFinalStandingState(t *testing.T) {
	acc := newRunAccumulator()
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

// TestRunAccumulator_ProducedAndWaiting: the summary of a server run reports
// what its passes wrote and the drafts its last pass held for review, as a
// local run's summary does.
func TestRunAccumulator_ProducedAndWaiting(t *testing.T) {
	acc := newRunAccumulator()
	acc.observe(convergence.Event{Type: convergence.EventPassDone, Pass: 1, Produced: 10, ProducedDelta: 10, WaitingForReview: 4})
	acc.observe(convergence.Event{Type: convergence.EventPassDone, Pass: 2, Produced: 10, WaitingForReview: 4})
	acc.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunParked})
	out := acc.output(nil)
	assert.Equal(t, 10, out.Produced)
	assert.Equal(t, 4, out.WaitingForReview)
}

// TestRunAccumulator_ParkedFromFinalStanding: a parked run's parked locale is
// taken from the final standing and recorded as a parked scope.
func TestRunAccumulator_ParkedFromFinalStanding(t *testing.T) {
	acc := newRunAccumulator()
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
	failed := newRunAccumulator()
	failed.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunFailed})
	err := failed.terminalError(&apiclient.ConvergenceRun{State: "failed", Error: "provider outage"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider outage")

	canceled := newRunAccumulator()
	canceled.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunCanceled})
	require.Error(t, canceled.terminalError(nil))

	// Fallback to the persisted final state when the stream was cut before the
	// terminal frame (sawDone never set).
	cut := newRunAccumulator()
	require.Error(t, cut.terminalError(&apiclient.ConvergenceRun{State: "failed"}))

	ok := newRunAccumulator()
	ok.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunConverged})
	assert.NoError(t, ok.terminalError(nil))
}
