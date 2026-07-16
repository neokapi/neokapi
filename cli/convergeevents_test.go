package cli

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUp_EventStream: a convergence run emits the convergence.Event protocol —
// pass_start, per-locale locale_start/locale_done (concurrently produced,
// serially delivered), pass_done with post-derivation counts, and a final done
// event — the one stream every surface (CLI renderer, NDJSON, desktop, server
// SSE) renders a run from.
func TestUp_EventStream(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO", "de-DE"}, gate.Gate{"translated": {Pct: 100}})

	var events []convergence.Event
	out, err := a.RunUp(context.Background(), recipe, "", UpOptions{
		UntilGate: true,
		Jobs:      2,
		OnEvent:   func(ev convergence.Event) { events = append(events, ev) },
	})
	require.NoError(t, err)
	require.True(t, out.Converged)
	require.NotEmpty(t, events)

	// The stream opens with pass 1 over both pending locales (after any
	// auto-extract log lines) and closes with exactly one done event.
	structural := make([]convergence.Event, 0, len(events))
	for _, ev := range events {
		if ev.Type != convergence.EventLog {
			structural = append(structural, ev)
		}
	}
	events = structural
	first, last := events[0], events[len(events)-1]
	assert.Equal(t, convergence.EventPassStart, first.Type)
	assert.Equal(t, 1, first.Pass)
	assert.ElementsMatch(t, []string{"nb-NO", "de-DE"}, first.Pending)
	assert.Equal(t, convergence.EventDone, last.Type)
	assert.Equal(t, convergence.RunConverged, last.State)

	starts := map[string]convergence.Event{}
	dones := map[string]convergence.Event{}
	var passDone *convergence.Event
	doneCount := 0
	for _, ev := range events {
		switch ev.Type {
		case convergence.EventLocaleStart:
			starts[ev.Locale] = ev
		case convergence.EventLocaleDone:
			dones[ev.Locale] = ev
		case convergence.EventPassDone:
			passDone = &ev
		case convergence.EventDone:
			doneCount++
		}
	}
	assert.Equal(t, 1, doneCount, "exactly one done event")

	// Both locales report a start with the unit total and a done with every
	// unit produced (the fixture has two source units per locale).
	for _, loc := range []string{"nb-NO", "de-DE"} {
		st, ok := starts[loc]
		require.True(t, ok, "locale_start for %s", loc)
		assert.Equal(t, 2, st.Units, "%s unit total", loc)
		dn, ok := dones[loc]
		require.True(t, ok, "locale_done for %s", loc)
		assert.Equal(t, 2, dn.Done, "%s produced units", loc)
	}

	// Post-derivation pass summary: all four units produced, nothing pending.
	require.NotNil(t, passDone)
	assert.Equal(t, 4, passDone.Produced)
	assert.Empty(t, passDone.Pending)
}

// TestUp_JobsFlag: `up --jobs 1` (sequential) and the concurrent default land
// on the same converged standing — concurrency changes wall-clock, never the
// result.
func TestUp_JobsFlag(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO", "de-DE"}, gate.Gate{"translated": {Pct: 100}})

	out, err := runUp(t, a, recipe, "--jobs", "1")
	require.NoError(t, err, out)
	assert.Contains(t, out, "Up to date: every gated scope is shippable")
}
