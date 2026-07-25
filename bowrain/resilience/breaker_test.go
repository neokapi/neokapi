package resilience

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSettings are thresholds tuned for fast, deterministic tests: a tiny
// volume floor and a sub-second cooldown. They are built directly rather than
// through Overrides because the operator-facing knobs are second-granular.
func testSettings() Settings {
	return Settings{
		Enabled:         true,
		FailureRatio:    0.5,
		MinimumRequests: 4,
		Window:          time.Minute,
		Cooldown:        60 * time.Millisecond,
		HalfOpenProbes:  1,
	}
}

var errUpstream = errors.New("upstream API error 503: service unavailable")

func failN(t *testing.T, b *Breaker, n int) {
	t.Helper()
	for range n {
		_ = b.Do(t.Context(), func(context.Context) error { return errUpstream })
	}
}

func TestBreakerStartsClosedAndPassesThrough(t *testing.T) {
	b := newBreaker("test:cold-start", KindAI, testSettings())

	// Cold start must never reject: a fresh process has observed nothing.
	assert.Equal(t, StateClosed, b.State())

	called := false
	err := b.Do(t.Context(), func(context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestBreakerOpensOnSustainedUpstreamFailure(t *testing.T) {
	b := newBreaker("test:opens", KindAI, testSettings())

	failN(t, b, 4) // MinimumRequests reached, ratio 1.0 >= 0.5
	assert.Equal(t, StateOpen, b.State())

	// The next call must not reach the dependency at all.
	called := false
	err := b.Do(t.Context(), func(context.Context) error {
		called = true
		return nil
	})
	assert.False(t, called, "an open circuit must not attempt the call")

	u, ok := AsUnavailable(err)
	require.True(t, ok, "open circuit must yield a typed UnavailableError, got %v", err)
	assert.Equal(t, "test:opens", u.Dependency)
	assert.Equal(t, KindAI, u.Kind)
	assert.GreaterOrEqual(t, u.RetryAfterSeconds(), 1, "Retry-After is always at least a second")
	assert.True(t, IsUnavailable(err))
}

func TestBreakerIgnoresVolumeBelowMinimum(t *testing.T) {
	b := newBreaker("test:min-volume", KindAI, testSettings())

	// Three total failures, below MinimumRequests=4: an idle dependency with a
	// couple of unlucky calls must not be declared down.
	failN(t, b, 3)
	assert.Equal(t, StateClosed, b.State())
}

func TestBreakerIgnoresCallerFaults(t *testing.T) {
	// A workspace with a bad API key produces a steady stream of 4xx. That must
	// never open the breaker every other workspace shares.
	permanent := errors.New("openai: API error 401: invalid api key")
	b := newBreaker("test:caller-fault", KindAI, testSettings())

	for range 20 {
		err := b.Do(t.Context(), func(context.Context) error { return permanent })
		require.ErrorIs(t, err, permanent, "the caller's own error is returned unchanged")
	}
	assert.Equal(t, StateClosed, b.State())
}

func TestBreakerIgnoresExplicitCallerFault(t *testing.T) {
	b := newBreaker("test:explicit-fault", KindAI, testSettings())

	// A call site that classified the error itself: even though the text looks
	// transient, CallerFault says it is not a health signal.
	for range 20 {
		_ = b.Do(t.Context(), func(context.Context) error {
			return CallerFault(errUpstream)
		})
	}
	assert.Equal(t, StateClosed, b.State())
}

func TestBreakerIgnoresCallerCancellation(t *testing.T) {
	b := newBreaker("test:cancel", KindAI, testSettings())

	for range 20 {
		_ = b.Do(t.Context(), func(context.Context) error {
			return fmt.Errorf("read body: %w", context.Canceled)
		})
	}
	assert.Equal(t, StateClosed, b.State(),
		"a user closing a tab says nothing about the dependency")
}

func TestBreakerCountsDeadlineExceeded(t *testing.T) {
	b := newBreaker("test:deadline", KindAI, testSettings())

	for range 4 {
		_ = b.Do(t.Context(), func(context.Context) error {
			return fmt.Errorf("call provider: %w", context.DeadlineExceeded)
		})
	}
	assert.Equal(t, StateOpen, b.State(),
		"a dependency too slow to answer is a dependency in trouble")
}

func TestBreakerRecoversThroughHalfOpen(t *testing.T) {
	b := newBreaker("test:recovers", KindAI, testSettings())

	failN(t, b, 4)
	require.Equal(t, StateOpen, b.State())

	// After the cooldown the circuit must admit a probe on its own — an open
	// breaker that never re-probes would wedge a recovered dependency.
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, StateHalfOpen, b.State())

	require.NoError(t, b.Do(t.Context(), func(context.Context) error { return nil }))
	assert.Equal(t, StateClosed, b.State())
}

func TestBreakerReopensWhenProbeFails(t *testing.T) {
	b := newBreaker("test:reopens", KindAI, testSettings())

	failN(t, b, 4)
	require.Equal(t, StateOpen, b.State())
	time.Sleep(80 * time.Millisecond)
	require.Equal(t, StateHalfOpen, b.State())

	// A still-broken dependency must go straight back to open, not accumulate
	// a second full window of failures first.
	_ = b.Do(t.Context(), func(context.Context) error { return errUpstream })
	assert.Equal(t, StateOpen, b.State())
}

func TestBreakerHalfOpenAdmitsBoundedProbes(t *testing.T) {
	s := testSettings()
	s.HalfOpenProbes = 1
	b := newBreaker("test:probe-cap", KindAI, s)

	failN(t, b, 4)
	time.Sleep(80 * time.Millisecond)
	require.Equal(t, StateHalfOpen, b.State())

	// Hold the single probe slot open, then prove a concurrent second call is
	// rejected rather than piling onto a dependency that may still be down.
	release := make(chan struct{})
	entered := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = b.Do(context.Background(), func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err := b.Do(t.Context(), func(context.Context) error {
		t.Error("second half-open probe must not reach the dependency")
		return nil
	})
	assert.True(t, IsUnavailable(err))

	close(release)
	wg.Wait()
}

func TestBreakerKillSwitchNeverRejects(t *testing.T) {
	s := testSettings()
	s.Enabled = false
	b := newBreaker("test:kill-switch", KindAI, s)

	// However badly the dependency behaves, a disabled breaker is a pure
	// pass-through — the escape hatch if a breaker ever misbehaves in prod.
	var calls atomic.Int64
	for range 50 {
		err := b.Do(t.Context(), func(context.Context) error {
			calls.Add(1)
			return errUpstream
		})
		require.ErrorIs(t, err, errUpstream)
		require.False(t, IsUnavailable(err))
	}
	assert.Equal(t, int64(50), calls.Load())
	assert.Equal(t, StateClosed, b.State())
}

func TestBreakerHonoursCallerContext(t *testing.T) {
	b := newBreaker("test:ctx", KindAI, testSettings())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := b.Do(ctx, func(context.Context) error {
		t.Error("a cancelled caller must not reach the dependency")
		return nil
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, IsUnavailable(err), "a cancelled caller is not an outage")
}

func TestDoGenericReturnsZeroValueWhenOpen(t *testing.T) {
	b := newBreaker("test:generic", KindAI, testSettings())
	failN(t, b, 4)
	require.Equal(t, StateOpen, b.State())

	out, err := Do(t.Context(), b, func(context.Context) (string, error) {
		t.Error("open circuit must not attempt the call")
		return "value", nil
	})
	assert.Empty(t, out)
	assert.True(t, IsUnavailable(err))
}

func TestDoGenericPassesValueThrough(t *testing.T) {
	b := newBreaker("test:generic-ok", KindAI, testSettings())

	out, err := Do(t.Context(), b, func(context.Context) (int, error) { return 42, nil })
	require.NoError(t, err)
	assert.Equal(t, 42, out)
}

func TestDoGenericWithNilBreakerIsPassThrough(t *testing.T) {
	// An unguarded call site must keep working rather than nil-panic.
	out, err := Do(t.Context(), nil, func(context.Context) (int, error) { return 7, nil })
	require.NoError(t, err)
	assert.Equal(t, 7, out)
}

func TestBreakerIsConcurrencySafe(t *testing.T) {
	b := newBreaker("test:concurrent", KindAI, testSettings())

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Do(context.Background(), func(context.Context) error {
				if i%2 == 0 {
					return errUpstream
				}
				return nil
			})
			_ = b.State()
			_ = b.Settings()
		}()
	}
	wg.Wait()
}
