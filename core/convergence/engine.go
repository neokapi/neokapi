package convergence

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"
)

// This file is the venue-neutral convergence loop — the ONE implementation of
// "reconcile toward the ship gates" that every venue drives (AD-033 semantics):
// the CLI/desktop run it over working-tree files, a Bowrain server runs it
// over its block store. Venues supply the IO through LoopFuncs; the loop owns
// the semantics — pass barrier, per-locale fan-out, stall-parks-the-rest,
// never-an-error on pending target work — and the Event protocol emission.

// Emitter delivers convergence events one at a time. Concurrent producers
// (per-locale workers) serialize on it, so consumers (renderers, NDJSON
// encoders, SSE writers, UI bridges) never need their own locking. A nil
// callback makes every Emit a no-op.
type Emitter struct {
	mu sync.Mutex
	fn func(Event)
}

// NewEmitter builds an emitter over the given callback (nil is allowed).
func NewEmitter(fn func(Event)) *Emitter {
	return &Emitter{fn: fn}
}

// Emit delivers one event.
func (e *Emitter) Emit(ev Event) {
	if e == nil || e.fn == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.fn(ev)
}

// PassState is one derivation of the project's standing — recomputed from the
// venue's source of truth before and after every pass (state is derived, never
// tracked).
type PassState struct {
	// Pending are the locales still short of their gate (the candidates to
	// park if the loop stalls), in the project's target order.
	Pending []string
	// Produced is the progress metric: units at ≥ draft (any committed
	// target) summed across scopes. A pass that does not raise it has
	// stalled.
	Produced int
	// FailingChecks counts produced units demoted by the venue's bound
	// checks (they read at draft for gating).
	FailingChecks int
	// UnitTotals is each locale's total unit count — the denominator its
	// live progress renders against.
	UnitTotals map[string]int
	// Detail carries the venue's rich derivation (coverage rows, check
	// exclusions) through the loop to its finish step, opaquely.
	Detail any
}

// SyncResult reports a pre-pass source sync (drift re-extract): how much was
// re-extracted and why. A nil result means the store was already in sync.
type SyncResult struct {
	Files  int
	Blocks int
	Reason string
}

// LoopFuncs are the venue's IO callbacks. Sync is optional; Derive and
// Produce are required.
type LoopFuncs struct {
	// Sync brings the venue's block store back in sync with its source of
	// truth before a pass (the CLI's working-tree drift re-extract; a
	// server's connector re-ingest). Optional.
	Sync func(ctx context.Context) (*SyncResult, error)
	// Derive recomputes the standing: coverage per scope, bound-check
	// demotions, pending locales.
	Derive func(ctx context.Context) (PassState, error)
	// Produce runs one locale's flow for one pass. It reports live progress
	// by emitting unit_progress events on emit (already serialized and
	// throttle-friendly) and returns the locale's final produced counts for
	// the pass.
	Produce func(ctx context.Context, locale string, pass int, emit *Emitter) (PassProduction, error)
}

// PassProduction is what one locale's pass produced: how many units carry a
// committed target, and where those targets came from.
//
// The split tells a reader what the pass cost. ViaMemory is the project's own
// reviewed wording, recycled. ViaDraft is a translation the block store already
// held, reused without a provider call: the same work as ViaAI, made on an
// earlier run and paid for then. Counting a reused draft as ViaAI would quote a
// bill nobody was sent, which is what a parked locale's every run would report
// (#2356).
type PassProduction struct {
	Done      int
	ViaMemory int
	ViaAI     int
	ViaDraft  int
}

// LoopOptions are the loop's knobs.
type LoopOptions struct {
	// UntilGate loops passes until every gated scope ships or a pass stalls;
	// false runs a single pass.
	UntilGate bool
	// MaxPasses caps the loop (>= 1).
	MaxPasses int
	// Jobs is the per-pass locale concurrency (>= 1).
	Jobs int
}

// LoopResult is the loop's outcome: how many passes ran and the final
// standing. Parked work is a normal outcome, never an error.
type LoopResult struct {
	Passes int
	Final  PassState
}

// Loop runs the convergence loop: per pass, sync the store, derive the
// standing, fan the pending locales out Jobs at a time, re-derive, and stop
// when everything ships, a pass makes no progress (the remainder parks), or
// the pass cap is reached. Events stream through emit; the venue emits its
// own materialized/done events after its finish step (materialization is
// venue policy, not loop semantics).
func Loop(ctx context.Context, opts LoopOptions, f LoopFuncs, emit *Emitter) (LoopResult, error) {
	if opts.MaxPasses < 1 {
		opts.MaxPasses = 1
	}
	if opts.Jobs < 1 {
		opts.Jobs = 1
	}
	if emit == nil {
		emit = NewEmitter(nil)
	}

	passes := 0
	for {
		// A cancelled run (Ctrl-C, a UI's Cancel, a server shutdown) stops
		// between passes; mid-pass cancellation propagates through the
		// errgroup context.
		if err := ctx.Err(); err != nil {
			return LoopResult{Passes: passes}, err
		}

		var synced *SyncResult
		if f.Sync != nil {
			s, err := f.Sync(ctx)
			if err != nil {
				return LoopResult{Passes: passes}, err
			}
			synced = s
			if synced != nil {
				emit.Emit(Event{Type: EventLog, Stage: StageSync, Message: fmt.Sprintf(
					"Extracted %d block(s) from %d file(s) into the project store (%s).",
					synced.Blocks, synced.Files, synced.Reason)})
			}
		}

		state, err := f.Derive(ctx)
		if err != nil {
			return LoopResult{Passes: passes}, err
		}
		if len(state.Pending) == 0 {
			// Already converged before this pass (or after the previous one).
			return LoopResult{Passes: passes, Final: state}, nil
		}

		before := state.Produced
		passes++
		passStart := Event{
			Type:      EventPassStart,
			Pass:      passes,
			MaxPasses: opts.MaxPasses,
			Pending:   state.Pending,
		}
		if synced != nil {
			passStart.ExtractedFiles = synced.Files
			passStart.ExtractedBlocks = synced.Blocks
		}
		emit.Emit(passStart)

		// Fan the pass out per locale: locales are independent within a pass,
		// so each runs on its own worker, at most Jobs at a time. The pass
		// barrier stays — coverage, checks, and parking reason about the
		// whole pass (g.Wait) before the next one starts.
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(opts.Jobs)
		for _, loc := range state.Pending {
			g.Go(func() error {
				emit.Emit(Event{
					Type:   EventLocaleStart,
					Pass:   passes,
					Locale: loc,
					Units:  state.UnitTotals[loc],
				})
				produced, err := f.Produce(gctx, loc, passes, emit)
				if err != nil {
					return fmt.Errorf("converge %s: %w", loc, err)
				}
				emit.Emit(Event{
					Type:      EventLocaleDone,
					Pass:      passes,
					Locale:    loc,
					Units:     state.UnitTotals[loc],
					Done:      produced.Done,
					ViaMemory: produced.ViaMemory,
					ViaAI:     produced.ViaAI,
					ViaDraft:  produced.ViaDraft,
				})
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return LoopResult{Passes: passes}, err
		}

		// Post-pass derivation: the venue re-runs its bound checks over what
		// the pass produced — a failing unit reads at draft, so it cannot
		// lift its locale over the gate.
		after, err := f.Derive(ctx)
		if err != nil {
			return LoopResult{Passes: passes}, err
		}
		emit.Emit(Event{
			Type:          EventPassDone,
			Stage:         StageChecks,
			Pass:          passes,
			MaxPasses:     opts.MaxPasses,
			Produced:      after.Produced,
			ProducedDelta: after.Produced - before,
			FailingChecks: after.FailingChecks,
			Pending:       after.Pending,
		})

		if !opts.UntilGate || len(after.Pending) == 0 {
			return LoopResult{Passes: passes, Final: after}, nil
		}
		// Stop looping when capped or when a full pass produced nothing new —
		// the remaining locales park (the flow can't advance them unaided).
		if passes >= opts.MaxPasses || after.Produced <= before {
			return LoopResult{Passes: passes, Final: after}, nil
		}
	}
}
