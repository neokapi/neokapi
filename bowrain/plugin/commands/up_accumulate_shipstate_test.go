package commands

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A server run holds every locale to one bar: fully translated and clear of the
// bound checks (the orchestrator's pending test). No locale of a server run is
// without a bar, so `kapi up` against a server reports each one shippable or
// withheld, and never not gated.

// TestRunAccumulator_ServerStandingIsGated: the final standing's states map to
// the ship states, each gated.
func TestRunAccumulator_ServerStandingIsGated(t *testing.T) {
	acc := newRunAccumulator()
	acc.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunParked})

	out := acc.output(&apiclient.ConvergenceRun{
		State: "parked",
		Locales: []apiclient.ConvergenceLocaleStanding{
			{Locale: "fr-FR", State: convergence.LocaleShippable, Units: 10, Produced: 10},
			{Locale: "de-DE", State: convergence.LocaleParked, Units: 10, Produced: 6},
			{Locale: "ja-JP", State: convergence.LocalePending, Units: 10, Produced: 2},
		},
	})

	require.Len(t, out.Locales, 3)
	want := map[string]convergence.ShipState{
		"fr-FR": convergence.ShipStateShippable,
		"de-DE": convergence.ShipStateWithheld,
		"ja-JP": convergence.ShipStateWithheld,
	}
	for _, l := range out.Locales {
		assert.True(t, l.Gated, l.Locale)
		assert.Equal(t, want[l.Locale], l.ShipState, l.Locale)
	}
}

// TestRunAccumulator_ConvergedServerRunClaimsItsBar: the summary of a converged
// server run keeps its verdict rather than reading as a project with no gates.
func TestRunAccumulator_ConvergedServerRunClaimsItsBar(t *testing.T) {
	acc := newRunAccumulator()
	acc.observe(convergence.Event{Type: convergence.EventDone, State: convergence.RunConverged})

	out := acc.output(&apiclient.ConvergenceRun{
		State:  "converged",
		Passes: 1,
		Locales: []apiclient.ConvergenceLocaleStanding{
			{Locale: "fr-FR", State: convergence.LocaleShippable, Units: 10, Produced: 10},
		},
	})

	var b bytes.Buffer
	require.NoError(t, out.FormatText(&b))
	assert.Contains(t, b.String(), "Up to date: every gated scope is shippable.")
	assert.NotContains(t, b.String(), "not gated")
	assert.NotContains(t, b.String(), "No ship gates are declared")
}
