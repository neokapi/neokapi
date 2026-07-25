package resilience

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/sony/gobreaker/v2"
)

// State is a breaker's position in the closed → open → half-open cycle.
type State string

const (
	// StateClosed passes every call through. This is the cold-start state.
	StateClosed State = "closed"
	// StateHalfOpen admits a bounded number of probes after a cooldown.
	StateHalfOpen State = "half_open"
	// StateOpen rejects calls without attempting them.
	StateOpen State = "open"
)

// Breaker guards one dependency. Obtain one from a [Registry] rather than
// constructing it directly, so that its state is shared by every call site that
// talks to the same upstream and so it appears on the ctrl Health page.
//
// A Breaker is safe for concurrent use.
type Breaker struct {
	name string
	kind Kind

	mu       sync.RWMutex
	settings Settings
	cb       *gobreaker.CircuitBreaker[struct{}]
	openedAt time.Time

	// now is overridable in tests; nil means time.Now.
	now func() time.Time
}

func newBreaker(name string, kind Kind, s Settings) *Breaker {
	b := &Breaker{name: name, kind: kind}
	b.reconfigure(s)
	return b
}

func (b *Breaker) clock() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

// reconfigure rebuilds the underlying state machine. The rebuild resets the
// breaker to closed, which is the safe direction: an operator changing
// thresholds gets a clean slate rather than an inherited open circuit.
func (b *Breaker) reconfigure(s Settings) {
	cb := gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        b.name,
		MaxRequests: s.HalfOpenProbes,
		Interval:    s.Window,
		// Bucketing turns the fixed window into a rolling one, so failures do
		// not vanish wholesale at an arbitrary window boundary.
		BucketPeriod: s.Window / 6,
		Timeout:      s.Cooldown,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			if c.Requests < s.MinimumRequests {
				return false
			}
			return float64(c.TotalFailures)/float64(c.Requests) >= s.FailureRatio
		},
		// Only genuine upstream trouble counts. A 4xx is the caller's fault and
		// must never trip a breaker shared across workspaces.
		IsSuccessful: func(err error) bool {
			return err == nil || !IsUpstreamFailure(err)
		},
		// A cancelled caller says nothing about the dependency, so exclude it
		// from the counts entirely rather than scoring it as a success.
		IsExcluded: func(err error) bool {
			return errors.Is(err, context.Canceled) || isCallerFault(err)
		},
		OnStateChange: b.onStateChange,
	})

	b.mu.Lock()
	b.settings = s
	b.cb = cb
	b.openedAt = time.Time{}
	b.mu.Unlock()

	observe.BreakerState.WithLabelValues(b.name, string(b.kind)).Set(stateValue(StateClosed))
}

func (b *Breaker) onStateChange(name string, from, to gobreaker.State) {
	fromS, toS := translateState(from), translateState(to)

	if toS == StateOpen {
		b.mu.Lock()
		b.openedAt = b.clock()
		b.mu.Unlock()
	}

	observe.BreakerState.WithLabelValues(name, string(b.kind)).Set(stateValue(toS))
	observe.BreakerTransitionsTotal.WithLabelValues(name, string(b.kind), string(fromS), string(toS)).Inc()

	// Opening is an incident; the rest is recovery narration.
	ctx := context.Background()
	switch toS {
	case StateOpen:
		slog.ErrorContext(ctx, "circuit breaker opened; calls to this dependency are now rejected",
			"dependency", name, "kind", string(b.kind), "from", string(fromS),
			"cooldown", b.Settings().Cooldown.String())
		observe.CaptureError(
			&UnavailableError{Dependency: name, Kind: b.kind, RetryAfter: b.Settings().Cooldown},
			name,
			map[string]string{"kind": "circuit_breaker", "dependency": name, "dependency_kind": string(b.kind)},
		)
	case StateHalfOpen:
		slog.WarnContext(ctx, "circuit breaker probing dependency recovery",
			"dependency", name, "kind", string(b.kind))
	case StateClosed:
		slog.InfoContext(ctx, "circuit breaker closed; dependency recovered",
			"dependency", name, "kind", string(b.kind), "from", string(fromS))
	}
}

// Name is the breaker's dependency identifier, e.g. "ai:bedrock".
func (b *Breaker) Name() string { return b.name }

// Kind is the dependency class.
func (b *Breaker) Kind() Kind { return b.kind }

// Settings returns the thresholds currently in force.
func (b *Breaker) Settings() Settings {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.settings
}

// State reports the breaker's current position. Reading it also advances the
// underlying clock, so a breaker whose cooldown has elapsed reports half_open
// rather than a stale open.
func (b *Breaker) State() State {
	b.mu.RLock()
	cb := b.cb
	b.mu.RUnlock()
	return translateState(cb.State())
}

// retryAfter estimates the time until the circuit next admits a probe.
func (b *Breaker) retryAfter() time.Duration {
	b.mu.RLock()
	openedAt, cooldown := b.openedAt, b.settings.Cooldown
	b.mu.RUnlock()

	if openedAt.IsZero() {
		return cooldown
	}
	remaining := cooldown - b.clock().Sub(openedAt)
	if remaining < time.Second {
		return time.Second
	}
	return remaining
}

// Do runs fn under the breaker.
//
// When the circuit is open fn is not called at all and an [UnavailableError] is
// returned — the caller's cue to degrade rather than fail. Otherwise fn's own
// error is returned unchanged, so wrapping a call site in Do never alters its
// success path or its error values.
func (b *Breaker) Do(ctx context.Context, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	b.mu.RLock()
	cb, enabled := b.cb, b.settings.Enabled
	b.mu.RUnlock()

	start := b.clock()

	if !enabled {
		// Kill switch: still measured, never rejected.
		err := fn(ctx)
		b.record(start, err)
		return err
	}

	_, err := cb.Execute(func() (struct{}, error) {
		return struct{}{}, fn(ctx)
	})

	// gobreaker signals its own rejections with these two sentinels; fn never
	// produces them, so the discrimination is unambiguous.
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		observe.BreakerRejectionsTotal.WithLabelValues(b.name, string(b.kind)).Inc()
		observe.DependencyCallsTotal.WithLabelValues(b.name, string(b.kind), "rejected").Inc()
		return &UnavailableError{Dependency: b.name, Kind: b.kind, RetryAfter: b.retryAfter()}
	}

	b.record(start, err)
	return err
}

func (b *Breaker) record(start time.Time, err error) {
	outcome := "success"
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled) || isCallerFault(err):
		outcome = "ignored"
	case IsUpstreamFailure(err):
		outcome = "failure"
	default:
		// A permanent, non-upstream error: the dependency answered, it just
		// said no. Not a health signal.
		outcome = "rejected_upstream"
	}
	observe.DependencyCallsTotal.WithLabelValues(b.name, string(b.kind), outcome).Inc()
	observe.DependencyCallDuration.WithLabelValues(b.name, string(b.kind)).Observe(b.clock().Sub(start).Seconds())
}

// Do runs a value-returning call under a breaker. On an open circuit fn is not
// called and the zero value of T is returned alongside the [UnavailableError].
func Do[T any](ctx context.Context, b *Breaker, fn func(context.Context) (T, error)) (T, error) {
	var out T
	if b == nil {
		return fn(ctx)
	}
	err := b.Do(ctx, func(ctx context.Context) error {
		v, err := fn(ctx)
		out = v
		return err
	})
	if err != nil {
		var zero T
		if IsUnavailable(err) {
			return zero, err
		}
	}
	return out, err
}

func translateState(s gobreaker.State) State {
	switch s {
	case gobreaker.StateOpen:
		return StateOpen
	case gobreaker.StateHalfOpen:
		return StateHalfOpen
	default:
		return StateClosed
	}
}

// stateValue maps a state onto the numeric gauge value. Ordered by severity so
// max() over the series is the worst state seen.
func stateValue(s State) float64 {
	switch s {
	case StateHalfOpen:
		return 1
	case StateOpen:
		return 2
	default:
		return 0
	}
}
