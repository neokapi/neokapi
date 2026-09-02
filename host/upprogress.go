package host

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/convergence"
)

// convergeRenderer renders a convergence.Event stream for the terminal — the
// live face of `kapi up`. On a TTY it maintains an in-place region: one line
// per locale in the current pass (progress bar, unit counts, content memory/AI split)
// under a pass header, repainted as events arrive. On a plain stream (CI logs,
// pipes) it degrades to one line per meaningful event and never rewrites.
//
// The renderer draws on the progress stream (stderr by convention); the run's
// final structured summary prints separately on stdout, so `kapi up
// 2>/dev/null` still yields exactly the summary.
type convergeRenderer struct {
	w   io.Writer
	tty bool

	mu        sync.Mutex
	pass      int
	maxPasses int
	order     []string
	rows      map[string]*convergeRow
	drawn     int // lines currently occupied by the live region
}

type convergeRow struct {
	units, done, viaMemory, viaAI, viaDraft int
	state                                   string // queued | running | done
}

// NewConvergeRenderer builds a renderer; tty selects the in-place live region.
func NewConvergeRenderer(w io.Writer, tty bool) *convergeRenderer {
	return &convergeRenderer{w: w, tty: tty, rows: map[string]*convergeRow{}}
}

// NewConvergeEventRenderer exposes the `kapi up` progress renderer as an event
// sink for embedders whose events arrive from elsewhere — the kapi-bowrain
// plugin's remote venue feeds the server run's SSE stream through it, so a
// remote run and a local run are indistinguishable in the terminal.
func NewConvergeEventRenderer(w io.Writer, tty bool) func(convergence.Event) {
	return NewConvergeRenderer(w, tty).OnEvent
}

// OnEvent is the convergence event sink; the engine delivers events one at a
// time (serialized), so no internal ordering is needed beyond the lock guarding
// against a concurrent finish.
func (r *convergeRenderer) OnEvent(ev convergence.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch ev.Type {
	case convergence.EventLog:
		r.printAbove(ev.Message)
	case convergence.EventPassStart:
		r.pass, r.maxPasses = ev.Pass, ev.MaxPasses
		r.order = append([]string(nil), ev.Pending...)
		r.rows = map[string]*convergeRow{}
		for _, loc := range ev.Pending {
			r.rows[loc] = &convergeRow{state: "queued"}
		}
		if !r.tty {
			fmt.Fprintf(r.w, "pass %d/%d · catching up %s\n", ev.Pass, ev.MaxPasses, strings.Join(ev.Pending, ", "))
			return
		}
		r.redraw()
	case convergence.EventLocaleStart:
		if row := r.rows[ev.Locale]; row != nil {
			row.state = "running"
			row.units = ev.Units
		}
		r.redrawTTY()
	case convergence.EventUnitProgress:
		if row := r.rows[ev.Locale]; row != nil {
			row.done, row.viaMemory, row.viaAI, row.viaDraft = ev.Done, ev.ViaMemory, ev.ViaAI, ev.ViaDraft
		}
		r.redrawTTY()
	case convergence.EventLocaleDone:
		if row := r.rows[ev.Locale]; row != nil {
			row.state = "done"
			row.units, row.done, row.viaMemory, row.viaAI, row.viaDraft = ev.Units, ev.Done, ev.ViaMemory, ev.ViaAI, ev.ViaDraft
		}
		if !r.tty {
			fmt.Fprintf(r.w, "  %-10s %d/%d units%s\n", ev.Locale, ev.Done, ev.Units, producedSuffix(ev.ViaMemory, ev.ViaDraft, ev.ViaAI))
			return
		}
		r.redraw()
	case convergence.EventPassDone:
		// Finalize the region for this pass (its lines stay printed) and add
		// the pass summary beneath it.
		r.redrawTTY()
		r.drawn = 0
		checks := ""
		if ev.FailingChecks > 0 {
			checks = fmt.Sprintf(" · %d failing check(s)", ev.FailingChecks)
		}
		next := ""
		if len(ev.Pending) > 0 {
			next = " · still pending: " + strings.Join(ev.Pending, ", ")
		}
		fmt.Fprintf(r.w, "pass %d done · produced %d (%+d)%s%s\n", ev.Pass, ev.Produced, ev.ProducedDelta, checks, next)
	case convergence.EventMaterialized:
		fmt.Fprintf(r.w, "materialized %d target file(s)\n", ev.Files)
	case convergence.EventDone:
		// The structured summary that follows on stdout carries the outcome;
		// the progress stream needs no separate closing line.
	}
}

// producedSuffix renders where a locale's units came from. `drafts` counts the
// translations the block store already held, reused without a provider call. It
// appears only when the pass reused one, so a run that drafted everything
// afresh reads as content memory and AI alone, and a run that paid for nothing
// says so rather than quoting an AI count against no call (#2356).
func producedSuffix(viaMemory, viaDraft, viaAI int) string {
	if viaMemory == 0 && viaDraft == 0 && viaAI == 0 {
		return ""
	}
	if viaDraft == 0 {
		return fmt.Sprintf("  (content memory %d · AI %d)", viaMemory, viaAI)
	}
	return fmt.Sprintf("  (content memory %d · drafts %d · AI %d)", viaMemory, viaDraft, viaAI)
}

// printAbove writes a plain line above the live region.
func (r *convergeRenderer) printAbove(msg string) {
	if r.tty && r.drawn > 0 {
		// Clear the region, print the line, redraw beneath it.
		fmt.Fprintf(r.w, "\x1b[%dA", r.drawn)
		for range r.drawn {
			fmt.Fprint(r.w, "\x1b[2K\n")
		}
		fmt.Fprintf(r.w, "\x1b[%dA", r.drawn)
		r.drawn = 0
		fmt.Fprintln(r.w, msg)
		r.redraw()
		return
	}
	fmt.Fprintln(r.w, msg)
}

// redrawTTY repaints only on a TTY (plain streams print per-event lines).
func (r *convergeRenderer) redrawTTY() {
	if r.tty {
		r.redraw()
	}
}

// redraw repaints the live region in place: a pass header plus one line per
// locale in the current pass.
func (r *convergeRenderer) redraw() {
	if !r.tty {
		return
	}
	if r.drawn > 0 {
		fmt.Fprintf(r.w, "\x1b[%dA", r.drawn)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\x1b[2Kpass %d/%d\n", r.pass, r.maxPasses)
	lines := 1
	for _, loc := range r.order {
		row := r.rows[loc]
		if row == nil {
			continue
		}
		fmt.Fprintf(&b, "\x1b[2K  %-10s %s\n", loc, r.rowBody(row))
		lines++
	}
	fmt.Fprint(r.w, b.String())
	r.drawn = lines
}

// rowBody renders one locale's cell: state, bar, counts.
func (r *convergeRenderer) rowBody(row *convergeRow) string {
	switch row.state {
	case "queued":
		return "queued"
	case "done":
		return fmt.Sprintf("%s %d/%d%s ✓", bar(row.done, row.units), row.done, row.units, producedSuffix(row.viaMemory, row.viaDraft, row.viaAI))
	default:
		return fmt.Sprintf("%s %d/%d%s", bar(row.done, row.units), row.done, row.units, producedSuffix(row.viaMemory, row.viaDraft, row.viaAI))
	}
}

// bar renders a fixed-width unit-progress bar.
func bar(done, units int) string {
	const width = 14
	filled := 0
	if units > 0 {
		filled = min(done*width/units, width)
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}
