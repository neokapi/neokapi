package convergence

import "sync"

// LocaleStanding is one locale's rolled-up standing in a run: what the run
// produced for it, and the ship verdict that follows.
type LocaleStanding struct {
	Locale    string `json:"locale"`
	State     string `json:"state"` // shippable | parked | pending
	Units     int    `json:"units,omitempty"`
	Produced  int    `json:"produced,omitempty"`
	ViaMemory int    `json:"viaTM,omitempty"`
	ViaAI     int    `json:"viaAI,omitempty"`
	ViaDraft  int    `json:"viaDrafts,omitempty"`
}

// Standing folds a run's Event stream into its per-locale standing and the
// run-level counters. It is *the* fold: the server persists a run's standing
// from it, and a client following the same stream (`kapi up` against a server
// venue) derives the identical standing rather than reimplementing the state
// machine. Two folds over one protocol is how a locale ends up reported as
// "pending" on one surface and "shippable" on another.
//
// Safe for concurrent use: Observe runs on the emitter's goroutine while a
// request handler snapshots.
type Standing struct {
	mu            sync.Mutex
	passes        int
	failingChecks int
	materialized  int
	runState      string
	done          bool
	order         []string
	locales       map[string]LocaleStanding
}

// NewStanding returns an empty standing — the fold's identity.
func NewStanding() *Standing {
	return &Standing{locales: map[string]LocaleStanding{}}
}

// Observe folds one event in.
func (s *Standing) Observe(ev Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locales == nil {
		s.locales = map[string]LocaleStanding{}
	}

	switch ev.Type {
	case EventPassStart:
		s.passes = max(s.passes, ev.Pass)

	case EventLocaleStart:
		cur := s.row(ev.Locale)
		cur.Units = ev.Units
		cur.State = LocalePending
		s.locales[ev.Locale] = cur

	case EventUnitProgress, EventLocaleDone:
		cur := s.row(ev.Locale)
		if ev.Units > 0 {
			cur.Units = ev.Units
		}
		cur.Produced, cur.ViaMemory, cur.ViaAI, cur.ViaDraft = ev.Done, ev.ViaMemory, ev.ViaAI, ev.ViaDraft
		if ev.Type == EventLocaleDone {
			// The engine's locale_done is state-less — a locale's ship verdict
			// is a whole-pass judgement, settled by the post-pass derivation
			// below. Derive it from coverage here so a stream that ends mid-run
			// still reports something honest, and let an explicit State win
			// when a venue does carry one.
			switch {
			case ev.State != "":
				cur.State = ev.State
			case cur.Units > 0 && cur.Produced >= cur.Units:
				cur.State = LocaleShippable
			default:
				cur.State = LocalePending
			}
		}
		s.locales[ev.Locale] = cur

	case EventPassDone:
		s.passes = ev.Pass
		s.failingChecks = ev.FailingChecks
		// Post-derivation is authoritative: whoever is still short of their
		// gate is pending (the park candidates if the loop stalls); everyone
		// else has cleared it.
		pending := map[string]bool{}
		for _, l := range ev.Pending {
			pending[l] = true
		}
		for l, cur := range s.locales {
			if pending[l] {
				cur.State = LocalePending
			} else if cur.State == LocalePending {
				cur.State = LocaleShippable
			}
			s.locales[l] = cur
		}

	case EventMaterialized:
		s.materialized = ev.Files

	case EventDone:
		s.runState, s.done = ev.State, true
		if ev.State == RunParked {
			// The loop stalled: everything short of its gate is parked work
			// for a human, not still-in-flight.
			for l, cur := range s.locales {
				if cur.State != LocaleShippable {
					cur.State = LocaleParked
					s.locales[l] = cur
				}
			}
		}

	case EventLog:
		// No structured shape; surfaces render it as a log line.
	}
}

// row returns the locale's current standing, registering it in arrival order on
// first sight. Callers hold s.mu.
func (s *Standing) row(locale string) LocaleStanding {
	cur, ok := s.locales[locale]
	if !ok {
		s.order = append(s.order, locale)
		cur.Locale = locale
	}
	return cur
}

// Locales returns the per-locale standing in the order the locales first
// appeared in the stream.
func (s *Standing) Locales() []LocaleStanding {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LocaleStanding, 0, len(s.order))
	for _, l := range s.order {
		out = append(out, s.locales[l])
	}
	return out
}

// Passes is the highest pass number the run reached.
func (s *Standing) Passes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.passes
}

// FailingChecks is the count of produced units failing the project's bound
// checks after the last settled pass.
func (s *Standing) FailingChecks() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failingChecks
}

// MaterializedFiles is the number of files the post-loop materialize step
// wrote (zero when it did not run).
func (s *Standing) MaterializedFiles() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.materialized
}

// RunState reports the run's terminal state (converged | parked | failed |
// canceled) and whether the terminal event arrived at all — a stream that was
// cut short reports done=false, and the caller falls back to whatever the run
// row persisted.
func (s *Standing) RunState() (state string, done bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runState, s.done
}
