package backend

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// manualTimer stands in for time.AfterFunc: fire runs the pending callback,
// as the timeout would.
type manualTimer struct {
	mu      sync.Mutex
	pending func()
	started int
}

func (m *manualTimer) after(_ time.Duration, f func()) func() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending = f
	m.started++
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

// fallbackGuard is a guard whose page never answers unless a test says so.
func fallbackGuard(r *recorder, timer *manualTimer, answer func(CloseRoute, []string) bool) *closeGuard {
	g := newTestGuard(r)
	g.after = timer.after
	g.fallback = answer
	g.tabNames = func(ids []string) []string {
		names := make([]string, len(ids))
		for i, id := range ids {
			names[i] = "project " + id
		}
		return names
	}
	return g
}

func TestCloseGuard_UnansweredCloseAsksNatively(t *testing.T) {
	r, timer := &recorder{}, &manualTimer{}
	var asked [][]string
	g := fallbackGuard(r, timer, func(route CloseRoute, names []string) bool {
		assert.Equal(t, CloseRouteWindow, route)
		asked = append(asked, names)
		return true
	})
	g.setUnsaved([]string{"a", "b"})
	assert.False(t, g.allow(CloseRouteWindow))
	require.True(t, timer.fire(), "a held close starts the wait")
	assert.Equal(t, [][]string{{"project a", "project b"}}, asked)
	assert.Equal(t, []CloseRoute{CloseRouteWindow}, r.closed, "close without saving runs the closer")
}

func TestCloseGuard_FallbackCancelKeepsTheWindow(t *testing.T) {
	r, timer := &recorder{}, &manualTimer{}
	g := fallbackGuard(r, timer, func(CloseRoute, []string) bool { return false })
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteQuit))
	timer.fire()
	assert.Empty(t, r.closed)
	// The next close is held and asked about again.
	assert.False(t, g.allow(CloseRouteQuit))
	assert.Len(t, r.events, 2)
}

func TestCloseGuard_AcknowledgedCloseNeverAsksNatively(t *testing.T) {
	r, timer := &recorder{}, &manualTimer{}
	g := fallbackGuard(r, timer, func(CloseRoute, []string) bool {
		t.Error("the fallback opened although the page answered")
		return true
	})
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteWindow))
	g.acknowledge()
	assert.False(t, timer.fire(), "the acknowledgement stops the wait")
	g.confirm()
	assert.Equal(t, []CloseRoute{CloseRouteWindow}, r.closed)
}

// A second close while the fallback dialog is open neither asks the page
// again nor opens a second dialog; the open dialog's answer finishes the
// stronger route.
func TestCloseGuard_SecondRequestWhileTheFallbackShows(t *testing.T) {
	r, timer := &recorder{}, &manualTimer{}
	var g *closeGuard
	dialogs := 0
	g = fallbackGuard(r, timer, func(CloseRoute, []string) bool {
		dialogs++
		// The person presses Cmd+Q while the dialog is up.
		assert.False(t, g.allow(CloseRouteQuit))
		assert.False(t, timer.fire(), "no second wait starts while the dialog shows")
		return true
	})
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteWindow))
	timer.fire()
	assert.Equal(t, 1, dialogs)
	assert.Len(t, r.events, 1, "the page is asked once")
	assert.Equal(t, []CloseRoute{CloseRouteQuit}, r.closed)
}

// A quit from a signal (SIGTERM calls App.Quit once, and Wails v3.1.0 stops
// listening after the first signal) is held like any quit. With no page to
// answer, the fallback still offers a way out, and its answer quits.
func TestCloseGuard_SignalQuitWithNoPage(t *testing.T) {
	r, timer := &recorder{}, &manualTimer{}
	g := fallbackGuard(r, timer, func(route CloseRoute, _ []string) bool {
		assert.Equal(t, CloseRouteQuit, route)
		return true
	})
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteQuit))
	// Later signals never reach the app; only the timeout moves things on.
	timer.fire()
	assert.Equal(t, []CloseRoute{CloseRouteQuit}, r.closed)
	assert.True(t, g.allow(CloseRouteQuit), "the confirmed quit passes the hook")
}

// The real timer: with a short timeout the fallback opens on its own.
func TestCloseGuard_RealTimeoutOpensTheFallback(t *testing.T) {
	r := &recorder{}
	asked := make(chan []string, 1)
	g := newTestGuard(r)
	g.ackTimeout = 10 * time.Millisecond
	g.fallback = func(_ CloseRoute, names []string) bool {
		asked <- names
		return false
	}
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteWindow))
	select {
	case names := <-asked:
		assert.Equal(t, []string{"a"}, names, "without a name resolver the ids are listed")
	case <-time.After(5 * time.Second):
		t.Fatal("the fallback never opened")
	}
}

// Clearing the list, as the page does when it unloads, releases the guard.
func TestCloseGuard_ClearedListReleasesAHeldClose(t *testing.T) {
	r, timer := &recorder{}, &manualTimer{}
	g := fallbackGuard(r, timer, func(CloseRoute, []string) bool { return false })
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteWindow))
	g.setUnsaved(nil)
	assert.True(t, g.allow(CloseRouteWindow))
}
