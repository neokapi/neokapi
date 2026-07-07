package commands

import (
	"errors"
	"slices"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/convergence"
)

// runAccumulator rolls a server run's convergence.Event stream up into the same
// cli.ConvergeOutput a local run produces, so `kapi up`'s final summary is
// identical whichever venue executed. The run's authoritative final standing
// (fetched after the stream closes) fills any gaps the stream left.
type runAccumulator struct {
	passes       int
	converged    bool
	sawDone      bool
	runState     string // terminal run state from the done event (converged|parked|failed|canceled)
	materialized int
	order        []string
	locales      map[string]*accLocale
}

type accLocale struct {
	units, done, viaTM, viaAI int
	state                     string // shippable | parked | pending
}

func (a *runAccumulator) observe(ev convergence.Event) {
	if a.locales == nil {
		a.locales = map[string]*accLocale{}
	}
	switch ev.Type {
	case convergence.EventPassDone:
		if ev.Pass > a.passes {
			a.passes = ev.Pass
		}
	case convergence.EventLocaleDone:
		l := a.locales[ev.Locale]
		if l == nil {
			l = &accLocale{}
			a.locales[ev.Locale] = l
			a.order = append(a.order, ev.Locale)
		}
		l.units, l.done, l.viaTM, l.viaAI = ev.Units, ev.Done, ev.ViaTM, ev.ViaAI
		if ev.State != "" {
			l.state = ev.State
		}
	case convergence.EventMaterialized:
		a.materialized = ev.Files
	case convergence.EventDone:
		a.sawDone = true
		a.runState = ev.State
		a.converged = ev.State == convergence.RunConverged
	}
}

// terminalError returns a non-nil error when the run ended in a failure state
// (failed or canceled), so `kapi up` exits non-zero instead of reporting a
// broken run as ordinary parked work. It prefers the streamed terminal state,
// falling back to the run's persisted final state.
func (a *runAccumulator) terminalError(final *apiclient.ConvergenceRun) error {
	state := a.runState
	if state == "" && final != nil {
		state = final.State
	}
	switch state {
	case convergence.RunFailed:
		msg := "server convergence run failed"
		if final != nil && final.Error != "" {
			msg += ": " + final.Error
		}
		return errors.New(msg)
	case convergence.RunCanceled:
		return errors.New("server convergence run was canceled")
	}
	return nil
}

// output builds the final ConvergeOutput, preferring the accumulated per-locale
// standing and falling back to the run's persisted final state for anything the
// stream did not carry (e.g. a run joined mid-flight, or a timeout).
func (a *runAccumulator) output(final *apiclient.ConvergenceRun) cli.ConvergeOutput {
	out := cli.ConvergeOutput{Flow: "default (server)", Passes: a.passes}
	converged := a.converged
	if !a.sawDone && final != nil {
		converged = final.State == "converged"
	}
	if final != nil && final.Passes > out.Passes {
		out.Passes = final.Passes
	}

	// Seed from the run's final standing, then overlay the (richer) streamed
	// per-locale counts.
	order := append([]string(nil), a.order...)
	rows := map[string]accLocale{}
	if final != nil {
		for _, ls := range final.Locales {
			if _, seen := rows[ls.Locale]; !seen {
				order = appendUnique(order, ls.Locale)
			}
			rows[ls.Locale] = accLocale{units: ls.Units, done: ls.Produced, viaTM: ls.ViaTM, viaAI: ls.ViaAI, state: ls.State}
		}
	}
	for _, loc := range a.order {
		l := a.locales[loc]
		merged := rows[loc] // seeded from final standing (carries authoritative state)
		merged.units, merged.done, merged.viaTM, merged.viaAI = l.units, l.done, l.viaTM, l.viaAI
		// The live stream leaves per-locale state empty (it is a whole-pass
		// verdict); keep the authoritative state from the final standing, and
		// only let a streamed state win if one was actually carried.
		if l.state != "" {
			merged.state = l.state
		}
		rows[loc] = merged
	}

	for _, loc := range order {
		l := rows[loc]
		res := cli.ConvergeLocaleResult{Locale: loc, Pct: map[string]int{}}
		pct := 100
		if l.units > 0 {
			pct = l.done * 100 / l.units
		}
		switch l.state {
		case convergence.LocaleShippable:
			res.Shippable = true
			res.Pct["draft"], res.Pct["translated"] = 100, 100
		case convergence.LocaleParked:
			res.Parked = true
			res.Pct["draft"], res.Pct["translated"] = pct, pct
			out.ParkedScopes = append(out.ParkedScopes, cli.ParkedScope{Locale: loc})
		default:
			res.Pct["draft"], res.Pct["translated"] = pct, pct
		}
		out.Locales = append(out.Locales, res)
	}

	out.Converged = converged && len(out.ParkedScopes) == 0
	out.MaterializedFiles = a.materialized
	return out
}

func appendUnique(s []string, v string) []string {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}
