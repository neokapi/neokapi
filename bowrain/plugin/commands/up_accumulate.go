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
// identical whichever venue executed.
//
// The fold itself is convergence.Standing — the very fold the server persists
// its run standing from, so client and server can't disagree about a locale's
// verdict. What is left here is the venue's own concern: mapping that standing
// onto the CLI's output type, and falling back to the run's persisted state for
// anything the stream did not carry (a run joined mid-flight, a cut stream).
type runAccumulator struct {
	standing *convergence.Standing
}

func newRunAccumulator() *runAccumulator {
	return &runAccumulator{standing: convergence.NewStanding()}
}

func (a *runAccumulator) observe(ev convergence.Event) { a.standing.Observe(ev) }

// terminalError returns a non-nil error when the run ended in a failure state
// (failed or canceled), so `kapi up` exits non-zero instead of reporting a
// broken run as ordinary parked work. It prefers the streamed terminal state,
// falling back to the run's persisted final state.
func (a *runAccumulator) terminalError(final *apiclient.ConvergenceRun) error {
	state, _ := a.standing.RunState()
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

// output builds the final ConvergeOutput, preferring the standing the stream
// produced and falling back to the run's persisted final state for anything the
// stream did not carry.
func (a *runAccumulator) output(final *apiclient.ConvergenceRun) cli.ConvergeOutput {
	runState, sawDone := a.standing.RunState()
	out := cli.ConvergeOutput{Flow: "default (server)", Passes: a.standing.Passes()}

	converged := runState == convergence.RunConverged
	if !sawDone && final != nil {
		converged = final.State == convergence.RunConverged
	}
	if final != nil && final.Passes > out.Passes {
		out.Passes = final.Passes
	}

	// Both sides ran the same fold over the same events, so they agree — except
	// where one of them is incomplete. A stream that reached its terminal event
	// is complete and is the richer view (it saw every pass); a stream that was
	// cut short is not, and the run row the server wrote after the loop is.
	// Whichever is authoritative wins; the other only fills locales it missed.
	streamed := a.standing.Locales()
	var persisted []convergence.LocaleStanding
	if final != nil {
		persisted = final.Locales
	}
	primary, secondary := streamed, persisted
	if !sawDone {
		primary, secondary = persisted, streamed
	}

	var order []string
	rows := map[string]convergence.LocaleStanding{}
	for _, ls := range secondary {
		order = appendUnique(order, ls.Locale)
		rows[ls.Locale] = ls
	}
	for _, ls := range primary {
		order = appendUnique(order, ls.Locale)
		rows[ls.Locale] = ls
	}

	for _, loc := range order {
		l := rows[loc]
		res := cli.ConvergeLocaleResult{Locale: loc, Pct: map[string]int{}}
		pct := 100
		if l.Units > 0 {
			pct = l.Produced * 100 / l.Units
		}
		switch l.State {
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
	out.MaterializedFiles = a.standing.MaterializedFiles()
	return out
}

func appendUnique(s []string, v string) []string {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}
