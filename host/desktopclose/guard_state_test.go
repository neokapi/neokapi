package desktopclose

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every transition of the state machine, and each event out of order.

// heldGuard is a guard holding a window close, in state asked.
func heldGuard(t *testing.T) (*Guard, *recorder, *manualTimer, *blockers) {
	t.Helper()
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	b.set(tabA)
	g := newGuard(r, timer, b)
	require.False(t, g.Allow(RouteWindow))
	require.Equal(t, StateAsked, g.State())
	return g, r, timer, b
}

// toFallback opens the native dialog and holds it open until release is
// called with its answer, so a test can send events while it shows.
func toFallback(t *testing.T, g *Guard, timer *manualTimer) (release func(closeAnyway bool), done chan struct{}) {
	t.Helper()
	answers := make(chan bool)
	opened := make(chan struct{})
	g.Configure(func(g *Guard) {
		g.Fallback = func(Route, []Blocker) bool {
			close(opened)
			return <-answers
		}
	})
	done = make(chan struct{})
	go func() {
		defer close(done)
		timer.fire()
	}()
	<-opened
	require.Equal(t, StateFallback, g.State())
	return func(closeAnyway bool) { answers <- closeAnyway }, done
}

func TestState_IdleAllowsWithNoBlockers(t *testing.T) {
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	g := newGuard(r, timer, b)
	assert.Equal(t, StateIdle, g.State())
	assert.True(t, g.Allow(RouteQuit))
	assert.Equal(t, StateIdle, g.State())
}

func TestState_StrayEventsInIdleChangeNothing(t *testing.T) {
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	b.set(tabA)
	g := newGuard(r, timer, b)
	assert.False(t, g.Acknowledge(), "nothing pending: the page shows nothing")
	assert.False(t, g.Confirm(), "a confirm with nothing pending")
	g.Cancel()
	assert.Equal(t, StateIdle, g.State())
	assert.Empty(t, r.closed)
	assert.False(t, g.Allow(RouteWindow), "the guard was never confirmed: it still holds")
}

func TestState_AskedToAcknowledgedToConfirmed(t *testing.T) {
	g, r, timer, _ := heldGuard(t)
	assert.True(t, g.Acknowledge())
	assert.Equal(t, StateAcknowledged, g.State())
	assert.False(t, timer.fire(), "the acknowledgement stops the wait")
	assert.True(t, g.Confirm())
	assert.Equal(t, StateConfirmed, g.State())
	assert.Equal(t, []Route{RouteWindow}, r.closed)
}

func TestState_AskedToAcknowledgedToIdleByCancel(t *testing.T) {
	g, r, _, _ := heldGuard(t)
	g.Acknowledge()
	g.Cancel()
	assert.Equal(t, StateIdle, g.State())
	assert.Empty(t, r.closed)
}

// The page may answer before its acknowledgement arrives: both calls are
// asynchronous.
func TestState_ConfirmBeforeAcknowledge(t *testing.T) {
	g, r, timer, _ := heldGuard(t)
	assert.True(t, g.Confirm())
	assert.Equal(t, []Route{RouteWindow}, r.closed)
	assert.False(t, g.Acknowledge(), "the late acknowledgement asks for no dialog")
	assert.False(t, timer.fire(), "the confirm stopped the wait")
}

func TestState_CancelBeforeAcknowledge(t *testing.T) {
	g, _, timer, _ := heldGuard(t)
	g.Cancel()
	assert.Equal(t, StateIdle, g.State())
	assert.False(t, g.Acknowledge())
	assert.False(t, timer.fire())
}

func TestState_AcknowledgedAskedAgainRefreshesThePage(t *testing.T) {
	g, r, _, _ := heldGuard(t)
	g.Acknowledge()
	assert.False(t, g.Allow(RouteQuit))
	assert.Equal(t, StateAsked, g.State())
	assert.Len(t, r.events, 2)
	g.Acknowledge()
	g.Confirm()
	assert.Equal(t, []Route{RouteQuit}, r.closed, "the quit outranks the window close")
}

func TestState_BlockersGoneReleaseAPendingClose(t *testing.T) {
	g, _, timer, b := heldGuard(t)
	b.set()
	assert.True(t, g.Allow(RouteWindow))
	assert.Equal(t, StateIdle, g.State())
	assert.False(t, timer.fire(), "the wait stopped with the release")
}

func TestState_FallbackCloseAnyway(t *testing.T) {
	g, r, timer, _ := heldGuard(t)
	anyway := 0
	g.Configure(func(g *Guard) { g.OnCloseAnyway = func() { anyway++ } })
	release, done := toFallback(t, g, timer)
	release(true)
	<-done
	assert.Equal(t, StateConfirmed, g.State())
	assert.Equal(t, 1, anyway)
	assert.Equal(t, []Route{RouteWindow}, r.closed)
}

func TestState_FallbackCancel(t *testing.T) {
	g, r, timer, _ := heldGuard(t)
	release, done := toFallback(t, g, timer)
	release(false)
	<-done
	assert.Equal(t, StateIdle, g.State())
	assert.Empty(t, r.closed)
}

// Events that arrive while the native dialog shows: the page's late
// acknowledgement is told to show nothing, and its answers are ignored.
func TestState_EventsWhileTheFallbackShows(t *testing.T) {
	g, r, timer, _ := heldGuard(t)
	release, done := toFallback(t, g, timer)

	assert.False(t, g.Acknowledge(), "a late acknowledgement: the page shows no second dialog")
	assert.False(t, g.Confirm(), "the page's answer does not decide")
	g.Cancel()
	assert.Equal(t, StateFallback, g.State())
	assert.False(t, g.Allow(RouteQuit), "a second close is held and asks nothing new")
	assert.Len(t, r.events, 1)
	assert.Empty(t, r.closed)

	release(true)
	<-done
	assert.Equal(t, []Route{RouteQuit}, r.closed, "the native answer finishes the stronger route")
}

// The reviewer's sequence: the page acknowledges after the native dialog
// opened, the person cancels the native dialog, then the page sends Save all's
// confirm. Nothing closes, and the guard still holds the next close.
func TestState_LateConfirmAfterNativeCancelMarksNothingConfirmed(t *testing.T) {
	g, r, timer, _ := heldGuard(t)
	release, done := toFallback(t, g, timer)
	assert.False(t, g.Acknowledge())
	release(false)
	<-done
	assert.Equal(t, StateIdle, g.State())

	assert.False(t, g.Confirm())
	assert.Empty(t, r.closed)
	assert.NotEqual(t, StateConfirmed, g.State())
	assert.False(t, g.Allow(RouteWindow), "the next close is held and asked about")
	assert.Len(t, r.events, 2)
}

func TestState_ConfirmedAllowsEverything(t *testing.T) {
	g, _, _, b := heldGuard(t)
	g.Confirm()
	b.set(tabA)
	assert.True(t, g.Allow(RouteWindow))
	assert.True(t, g.Allow(RouteQuit))
	assert.False(t, g.Confirm(), "already confirmed: nothing pending")
	g.Cancel()
	assert.Equal(t, StateConfirmed, g.State())
}

func TestState_TimeoutAfterTheAnswerDoesNothing(t *testing.T) {
	g, r, _, _ := heldGuard(t)
	opened := false
	g.Configure(func(g *Guard) {
		g.Fallback = func(Route, []Blocker) bool { opened = true; return true }
	})
	g.Acknowledge()
	g.Confirm()
	// A timer that fired just as the answer arrived finds nothing to ask.
	g.unanswered()
	assert.False(t, opened)
	assert.Equal(t, []Route{RouteWindow}, r.closed)
}
