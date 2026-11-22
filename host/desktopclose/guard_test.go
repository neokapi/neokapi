package desktopclose

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorder collects the events and closer calls a guard makes.
type recorder struct {
	mu     sync.Mutex
	events []map[string]any
	closed []Route
}

func (r *recorder) emit(name string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == RequestedEvent {
		r.events = append(r.events, data.(map[string]any))
	}
}

func (r *recorder) closer(route Route) func() {
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.closed = append(r.closed, route)
	}
}

// manualTimer stands in for time.AfterFunc: fire runs the pending callback,
// as the timeout would.
type manualTimer struct {
	mu      sync.Mutex
	pending func()
}

func (m *manualTimer) after(_ time.Duration, f func()) func() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending = f
	return func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		stopped := m.pending != nil
		m.pending = nil
		return stopped
	}
}

func (m *manualTimer) fire() bool {
	m.mu.Lock()
	f := m.pending
	m.pending = nil
	m.mu.Unlock()
	if f == nil {
		return false
	}
	f()
	return true
}

// blockers is a settable Blockers source.
type blockers struct {
	mu   sync.Mutex
	list []Blocker
}

func (b *blockers) get() []Blocker {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Blocker(nil), b.list...)
}

func (b *blockers) set(list ...Blocker) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.list = list
}

var tabA = Blocker{ID: "a", Name: "compass", Reasons: []string{"unsaved"}}

func newGuard(r *recorder, timer *manualTimer, b *blockers) *Guard {
	g := &Guard{Blockers: b.get, Emit: r.emit, After: timer.after}
	g.SetClosers(r.closer(RouteWindow), r.closer(RouteQuit))
	return g
}

func TestGuard_NothingBlockingClosesAtOnce(t *testing.T) {
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	g := newGuard(r, timer, b)
	assert.True(t, g.Allow(RouteWindow))
	assert.True(t, g.Allow(RouteQuit))
	assert.Empty(t, r.events)
	assert.False(t, timer.fire(), "no wait starts")
}

func TestGuard_BlockerHoldsTheCloseAndAsksOnce(t *testing.T) {
	for _, route := range []Route{RouteWindow, RouteQuit} {
		t.Run(string(route), func(t *testing.T) {
			r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
			b.set(tabA)
			g := newGuard(r, timer, b)
			assert.False(t, g.Allow(route))
			assert.Equal(t, []map[string]any{{"route": string(route), "blockers": []Blocker{tabA}}}, r.events)
			assert.False(t, g.Allow(route), "a second request while unanswered asks nothing new")
			assert.Len(t, r.events, 1)
			assert.Empty(t, r.closed)
		})
	}
}

func TestGuard_ConfirmFinishesTheRouteThatAsked(t *testing.T) {
	for _, route := range []Route{RouteWindow, RouteQuit} {
		t.Run(string(route), func(t *testing.T) {
			r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
			b.set(tabA)
			g := newGuard(r, timer, b)
			assert.False(t, g.Allow(route))
			g.Acknowledge()
			g.Confirm()
			assert.Equal(t, []Route{route}, r.closed)
			// The closer's own close passes, and so does the quit a last
			// window close starts.
			assert.True(t, g.Allow(RouteWindow))
			assert.True(t, g.Allow(RouteQuit))
		})
	}
}

func TestGuard_QuitOutranksAWindowClose(t *testing.T) {
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	b.set(tabA)
	g := newGuard(r, timer, b)
	assert.False(t, g.Allow(RouteWindow))
	assert.False(t, g.Allow(RouteQuit))
	g.Confirm()
	assert.Equal(t, []Route{RouteQuit}, r.closed)
}

func TestGuard_CancelKeepsEverything(t *testing.T) {
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	b.set(tabA)
	g := newGuard(r, timer, b)
	assert.False(t, g.Allow(RouteQuit))
	g.Acknowledge()
	g.Cancel()
	assert.Empty(t, r.closed)
	assert.False(t, g.Allow(RouteWindow), "the next request is held and asks again")
	assert.Len(t, r.events, 2)
	g.Confirm()
	assert.Equal(t, []Route{RouteWindow}, r.closed)
}

func TestGuard_ConfirmWithNothingPendingClosesNothing(t *testing.T) {
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	g := newGuard(r, timer, b)
	g.Confirm()
	assert.Empty(t, r.closed)
}

func TestGuard_ClearedBlockersReleaseAHeldClose(t *testing.T) {
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	b.set(tabA)
	g := newGuard(r, timer, b)
	assert.False(t, g.Allow(RouteWindow))
	b.set()
	assert.True(t, g.Allow(RouteWindow))
}

func TestGuard_UnansweredCloseAsksNatively(t *testing.T) {
	for _, closeAnyway := range []bool{true, false} {
		r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
		b.set(tabA)
		g := newGuard(r, timer, b)
		anyway := 0
		g.OnCloseAnyway = func() { anyway++ }
		var asked []Blocker
		g.Fallback = func(route Route, list []Blocker) bool {
			assert.Equal(t, RouteWindow, route)
			asked = list
			return closeAnyway
		}
		assert.False(t, g.Allow(RouteWindow))
		require.True(t, timer.fire(), "a held close starts the wait")
		assert.Equal(t, []Blocker{tabA}, asked)
		if closeAnyway {
			assert.Equal(t, 1, anyway)
			assert.Equal(t, []Route{RouteWindow}, r.closed)
		} else {
			assert.Equal(t, 0, anyway)
			assert.Empty(t, r.closed)
			assert.False(t, g.Allow(RouteWindow), "after cancel the next close is held again")
			assert.Len(t, r.events, 2)
		}
	}
}

func TestGuard_AcknowledgedCloseNeverAsksNatively(t *testing.T) {
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	b.set(tabA)
	g := newGuard(r, timer, b)
	g.Fallback = func(Route, []Blocker) bool {
		t.Error("the fallback opened although the page answered")
		return true
	}
	assert.False(t, g.Allow(RouteWindow))
	g.Acknowledge()
	assert.False(t, timer.fire(), "the acknowledgement stops the wait")
}

// A second close while the fallback dialog is open neither asks the page
// again nor opens a second dialog; the dialog's answer finishes the stronger
// route.
func TestGuard_SecondRequestWhileTheFallbackShows(t *testing.T) {
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	b.set(tabA)
	g := newGuard(r, timer, b)
	dialogs := 0
	g.Fallback = func(Route, []Blocker) bool {
		dialogs++
		assert.False(t, g.Allow(RouteQuit))
		assert.False(t, timer.fire(), "no second wait starts while the dialog shows")
		return true
	}
	assert.False(t, g.Allow(RouteWindow))
	timer.fire()
	assert.Equal(t, 1, dialogs)
	assert.Len(t, r.events, 1)
	assert.Equal(t, []Route{RouteQuit}, r.closed)
}

// A quit from a signal (SIGTERM calls App.Quit once, and Wails v3.1.0 stops
// listening after the first signal) is held like any quit. With no page to
// answer, the fallback still offers a way out, and its answer quits.
func TestGuard_SignalQuitWithNoPage(t *testing.T) {
	r, timer, b := &recorder{}, &manualTimer{}, &blockers{}
	b.set(tabA)
	g := newGuard(r, timer, b)
	g.Fallback = func(route Route, _ []Blocker) bool {
		assert.Equal(t, RouteQuit, route)
		return true
	}
	assert.False(t, g.Allow(RouteQuit))
	timer.fire()
	assert.Equal(t, []Route{RouteQuit}, r.closed)
	assert.True(t, g.Allow(RouteQuit))
}

// The real timer: with a short timeout the fallback opens on its own.
func TestGuard_RealTimeoutOpensTheFallback(t *testing.T) {
	r, b := &recorder{}, &blockers{}
	b.set(tabA)
	asked := make(chan []Blocker, 1)
	g := &Guard{Blockers: b.get, Emit: r.emit, AckTimeout: 10 * time.Millisecond}
	g.Fallback = func(_ Route, list []Blocker) bool {
		asked <- list
		return false
	}
	assert.False(t, g.Allow(RouteWindow))
	select {
	case list := <-asked:
		assert.Equal(t, []Blocker{tabA}, list)
	case <-time.After(5 * time.Second):
		t.Fatal("the fallback never opened")
	}
}
