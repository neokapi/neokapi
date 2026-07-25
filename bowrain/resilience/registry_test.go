package resilience

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistrySharesOneBreakerPerDependency(t *testing.T) {
	r := NewRegistry()

	// This is the point of the registry: the server's synchronous path and the
	// worker's job path must observe ONE circuit for the same upstream, so an
	// outage seen by either short-circuits both.
	a := r.Get("ai:bedrock", KindAI)
	b := r.Get("ai:bedrock", KindAI)
	assert.Same(t, a, b)

	other := r.Get("ai:anthropic", KindAI)
	assert.NotSame(t, a, other, "different providers must not share a circuit")
}

func TestRegistryIsolatesDependencies(t *testing.T) {
	r := NewRegistry()
	r.Configure(Overrides{
		MinimumRequests: new(4),
		FailureRatio:    new(0.5),
		CooldownSeconds: new(60),
	})

	down := r.Get("ai:bedrock", KindAI)
	healthy := r.Get("connector:figma", KindConnector)

	for range 4 {
		_ = down.Do(t.Context(), func(context.Context) error { return errUpstream })
	}
	require.Equal(t, StateOpen, down.State())

	// A tripped AI provider must not take down an unrelated request path.
	assert.Equal(t, StateClosed, healthy.State())
	require.NoError(t, healthy.Do(t.Context(), func(context.Context) error { return nil }))
}

func TestRegistryAppliesOverridesToNewAndExistingBreakers(t *testing.T) {
	r := NewRegistry()
	existing := r.Get("ai:bedrock", KindAI)
	require.True(t, existing.Settings().Enabled)

	r.Configure(Overrides{Enabled: new(false)})

	assert.False(t, existing.Settings().Enabled, "existing breakers pick up the change")
	assert.False(t, r.Get("connector:figma", KindConnector).Settings().Enabled,
		"breakers created after the change inherit it")
}

func TestRegistryReconfigureResetsToClosed(t *testing.T) {
	r := NewRegistry()
	r.Configure(Overrides{MinimumRequests: new(4), FailureRatio: new(0.5), CooldownSeconds: new(60)})

	b := r.Get("ai:bedrock", KindAI)
	for range 4 {
		_ = b.Do(t.Context(), func(context.Context) error { return errUpstream })
	}
	require.Equal(t, StateOpen, b.State())

	// An operator changing the rules must stop enforcement made under the old
	// rules, never keep rejecting on a stale verdict.
	r.Configure(Overrides{MinimumRequests: new(50)})
	assert.Equal(t, StateClosed, b.State())
}

func TestRegistryKeepsPerKindDefaultsUnderPartialOverrides(t *testing.T) {
	r := NewRegistry()
	// Only the ratio is set; every other threshold must stay per-kind.
	r.Configure(Overrides{FailureRatio: new(0.9)})

	ai := r.Get("ai:bedrock", KindAI).Settings()
	mail := r.Get("mail:resend", KindMail).Settings()

	assert.InDelta(t, 0.9, ai.FailureRatio, 0.0001)
	assert.InDelta(t, 0.9, mail.FailureRatio, 0.0001)
	assert.Equal(t, DefaultsFor(KindAI).Cooldown, ai.Cooldown)
	assert.Equal(t, DefaultsFor(KindMail).Cooldown, mail.Cooldown,
		"mail keeps its longer cooldown; a single global number would flatten it")
	assert.NotEqual(t, ai.Cooldown, mail.Cooldown)
}

func TestSettingsApplyRejectsHarmfulValues(t *testing.T) {
	base := DefaultsFor(KindAI)

	// A breaker must not be configurable into a state that harms a healthy
	// dependency: a zero cooldown would spin, a zero ratio would trip on the
	// first call, a zero probe count would never recover.
	got := base.Apply(Overrides{
		FailureRatio:    new(0.0),
		MinimumRequests: new(0),
		WindowSeconds:   new(0),
		CooldownSeconds: new(0),
		HalfOpenProbes:  new(0),
	})
	assert.Equal(t, base, got)

	// Out-of-range ratios are refused too.
	assert.Equal(t, base.FailureRatio, base.Apply(Overrides{FailureRatio: new(1.5)}).FailureRatio)
	assert.Equal(t, base.FailureRatio, base.Apply(Overrides{FailureRatio: new(-1.0)}).FailureRatio)
}

func TestSettingsApplyAcceptsValidValues(t *testing.T) {
	got := DefaultsFor(KindAI).Apply(Overrides{
		Enabled:         new(false),
		FailureRatio:    new(0.25),
		MinimumRequests: new(7),
		WindowSeconds:   new(30),
		CooldownSeconds: new(45),
		HalfOpenProbes:  new(3),
	})
	assert.False(t, got.Enabled)
	assert.InDelta(t, 0.25, got.FailureRatio, 0.0001)
	assert.Equal(t, uint32(7), got.MinimumRequests)
	assert.Equal(t, 30*time.Second, got.Window)
	assert.Equal(t, 45*time.Second, got.Cooldown)
	assert.Equal(t, uint32(3), got.HalfOpenProbes)
}

func TestRegistryStatesReportsOnlyObservedDependencies(t *testing.T) {
	r := NewRegistry()
	assert.Empty(t, r.States(), "nothing called yet means nothing to report")

	r.Configure(Overrides{MinimumRequests: new(4), FailureRatio: new(0.5), CooldownSeconds: new(60)})
	down := r.Get("ai:bedrock", KindAI)
	r.Get("connector:figma", KindConnector)
	for range 4 {
		_ = down.Do(t.Context(), func(context.Context) error { return errUpstream })
	}

	states := r.States()
	require.Len(t, states, 2)
	// Sorted so the Health page does not reshuffle between polls.
	assert.Equal(t, "ai:bedrock", states[0].Dependency)
	assert.Equal(t, "connector:figma", states[1].Dependency)

	assert.Equal(t, string(StateOpen), states[0].State)
	assert.False(t, states[0].Healthy)
	assert.True(t, states[0].Enabled)
	assert.Positive(t, states[0].RetryAfterSeconds)

	assert.Equal(t, string(StateClosed), states[1].State)
	assert.True(t, states[1].Healthy)
	assert.Zero(t, states[1].RetryAfterSeconds, "a closed circuit advertises no retry delay")
}

func TestRegistryOpenListsTrippedDependencies(t *testing.T) {
	r := NewRegistry()
	r.Configure(Overrides{MinimumRequests: new(4), FailureRatio: new(0.5), CooldownSeconds: new(60)})

	assert.Empty(t, r.Open())

	b := r.Get("ai:bedrock", KindAI)
	for range 4 {
		_ = b.Do(t.Context(), func(context.Context) error { return errUpstream })
	}
	assert.Equal(t, []string{"ai:bedrock"}, r.Open())
}

func TestRegistryGetIsConcurrencySafe(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	seen := make([]*Breaker, 50)
	for i := range 50 {
		wg.Go(func() {
			seen[i] = r.Get("ai:bedrock", KindAI)
		})
	}
	wg.Wait()
	for _, b := range seen {
		assert.Same(t, seen[0], b, "concurrent first-use must not create duplicates")
	}
}

func TestDefaultRegistryIsProcessWide(t *testing.T) {
	// The package-level Get must resolve through the same registry the health
	// surface enumerates, or the operator would be shown a different breaker set
	// from the one the code is enforcing.
	assert.Same(t, Default().Get("ai:test-default", KindAI), Get("ai:test-default", KindAI))
}
