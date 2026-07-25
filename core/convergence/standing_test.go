package convergence_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A run that clears every gate: each locale's standing follows its coverage,
// and the terminal event settles the run as converged.
func TestStanding_ConvergedRun(t *testing.T) {
	s := convergence.NewStanding()
	for _, ev := range []convergence.Event{
		{Type: convergence.EventPassStart, Pass: 1, Pending: []string{"fr-FR", "de-DE"}},
		{Type: convergence.EventLocaleStart, Pass: 1, Locale: "fr-FR", Units: 10},
		{Type: convergence.EventLocaleStart, Pass: 1, Locale: "de-DE", Units: 4},
		{Type: convergence.EventUnitProgress, Pass: 1, Locale: "fr-FR", Done: 6, ViaMemory: 2, ViaAI: 4},
		{Type: convergence.EventLocaleDone, Pass: 1, Locale: "fr-FR", Units: 10, Done: 10, ViaMemory: 3, ViaAI: 7},
		{Type: convergence.EventLocaleDone, Pass: 1, Locale: "de-DE", Units: 4, Done: 4, ViaAI: 4},
		{Type: convergence.EventPassDone, Pass: 1, Produced: 14, ProducedDelta: 14},
		{Type: convergence.EventMaterialized, Files: 2},
		{Type: convergence.EventDone, State: convergence.RunConverged},
	} {
		s.Observe(ev)
	}

	state, done := s.RunState()
	assert.Equal(t, convergence.RunConverged, state)
	assert.True(t, done)
	assert.Equal(t, 1, s.Passes())
	assert.Equal(t, 2, s.MaterializedFiles())

	locales := s.Locales()
	require.Len(t, locales, 2)
	// Arrival order, not map order.
	assert.Equal(t, "fr-FR", locales[0].Locale)
	assert.Equal(t, "de-DE", locales[1].Locale)
	assert.Equal(t, convergence.LocaleShippable, locales[0].State)
	assert.Equal(t, convergence.LocaleStanding{
		Locale: "fr-FR", State: convergence.LocaleShippable,
		Units: 10, Produced: 10, ViaMemory: 3, ViaAI: 7,
	}, locales[0])
}

// The engine's locale_done carries no per-locale state — the verdict is the
// pass's post-derivation. A locale left in pass_done's Pending is pending, and
// once the loop stalls (done: parked) it is parked work for a human.
func TestStanding_PendingLocaleParksOnStall(t *testing.T) {
	s := convergence.NewStanding()
	for _, ev := range []convergence.Event{
		{Type: convergence.EventPassStart, Pass: 1, Pending: []string{"fr-FR", "de-DE"}},
		// fr-FR fully covered; de-DE short of its gate.
		{Type: convergence.EventLocaleDone, Pass: 1, Locale: "fr-FR", Units: 10, Done: 10},
		{Type: convergence.EventLocaleDone, Pass: 1, Locale: "de-DE", Units: 10, Done: 6},
		// Post-derivation: de-DE still fails a bound check, fr-FR cleared.
		{Type: convergence.EventPassDone, Pass: 1, Produced: 16, FailingChecks: 4, Pending: []string{"de-DE"}},
		{Type: convergence.EventDone, State: convergence.RunParked},
	} {
		s.Observe(ev)
	}

	assert.Equal(t, 4, s.FailingChecks())
	locales := s.Locales()
	require.Len(t, locales, 2)
	assert.Equal(t, convergence.LocaleShippable, locales[0].State, "fr-FR cleared its gate")
	assert.Equal(t, convergence.LocaleParked, locales[1].State, "de-DE was pending when the loop stalled")
}

// A locale fully covered by the pass's flow run but demoted by the post-pass
// check derivation reads pending, not shippable: the coverage-derived verdict
// on locale_done is provisional, and pass_done is authoritative.
func TestStanding_PassDerivationOverridesCoverage(t *testing.T) {
	s := convergence.NewStanding()
	s.Observe(convergence.Event{Type: convergence.EventPassStart, Pass: 1, Pending: []string{"fr-FR"}})
	s.Observe(convergence.Event{Type: convergence.EventLocaleDone, Pass: 1, Locale: "fr-FR", Units: 10, Done: 10})
	require.Equal(t, convergence.LocaleShippable, s.Locales()[0].State)

	s.Observe(convergence.Event{Type: convergence.EventPassDone, Pass: 1, FailingChecks: 2, Pending: []string{"fr-FR"}})
	assert.Equal(t, convergence.LocalePending, s.Locales()[0].State)
}

// A stream cut before its terminal event reports done=false, so a caller knows
// to fall back to whatever the run row persisted rather than trusting a
// half-folded standing.
func TestStanding_CutStreamIsNotDone(t *testing.T) {
	s := convergence.NewStanding()
	s.Observe(convergence.Event{Type: convergence.EventLocaleStart, Pass: 1, Locale: "fr-FR", Units: 10})

	state, done := s.RunState()
	assert.Empty(t, state)
	assert.False(t, done)
	assert.Equal(t, convergence.LocalePending, s.Locales()[0].State)
}

// A venue that does carry an explicit per-locale state on locale_done wins over
// the coverage-derived one.
func TestStanding_ExplicitLocaleStateWins(t *testing.T) {
	s := convergence.NewStanding()
	s.Observe(convergence.Event{
		Type: convergence.EventLocaleDone, Pass: 1, Locale: "fr-FR",
		Units: 10, Done: 10, State: convergence.LocaleParked,
	})
	assert.Equal(t, convergence.LocaleParked, s.Locales()[0].State)
}
