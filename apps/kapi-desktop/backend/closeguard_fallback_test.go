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

// fakeRunner is the runner as the guard sees it: the tabs running a flow,
// and a stop that the fallback's "close anyway" calls.
type fakeRunner struct {
	mu      sync.Mutex
	tabs    []string
	stopped int
}

func (f *fakeRunner) running() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.tabs...)
}

func (f *fakeRunner) stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped++
	f.tabs = nil
}

func (f *fakeRunner) set(tabs ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tabs = tabs
}

// fallbackGuard is a guard whose page never answers unless a test says so.
func fallbackGuard(r *recorder, timer *manualTimer, runs *fakeRunner, answer func(CloseRoute, []CloseBlocker) bool) *closeGuard {
	g := newTestGuard(r)
	g.after = timer.after
	g.fallback = answer
	g.tabName = func(id string) string { return "project " + id }
	g.running = runs.running
	g.stopRuns = runs.stop
	return g
}

func blocker(id string, reasons ...CloseReason) CloseBlocker {
	return CloseBlocker{TabID: id, Name: "project " + id, Reasons: reasons}
}

func TestCloseGuard_FallbackNamesEachTabWithItsReasons(t *testing.T) {
	for _, tc := range []struct {
		name    string
		unsaved []string
		running []string
		want    []CloseBlocker
	}{
		{name: "only unsaved", unsaved: []string{"a", "b"},
			want: []CloseBlocker{blocker("a", CloseReasonUnsaved), blocker("b", CloseReasonUnsaved)}},
		{name: "only running", running: []string{"c"},
			want: []CloseBlocker{blocker("c", CloseReasonRunning)}},
		{name: "both", unsaved: []string{"a"}, running: []string{"c"},
			want: []CloseBlocker{blocker("a", CloseReasonUnsaved), blocker("c", CloseReasonRunning)}},
		{name: "one tab with both", unsaved: []string{"a"}, running: []string{"a"},
			want: []CloseBlocker{blocker("a", CloseReasonUnsaved, CloseReasonRunning)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, timer, runs := &recorder{}, &manualTimer{}, &fakeRunner{}
			runs.set(tc.running...)
			var asked []CloseBlocker
			g := fallbackGuard(r, timer, runs, func(route CloseRoute, blockers []CloseBlocker) bool {
				assert.Equal(t, CloseRouteWindow, route)
				asked = blockers
				return true
			})
			g.setUnsaved(tc.unsaved)
			assert.False(t, g.allow(CloseRouteWindow))
			require.True(t, timer.fire(), "a held close starts the wait")
			assert.Equal(t, tc.want, asked)
			assert.Equal(t, 1, runs.stopped, "close anyway stops the running flows")
			assert.Equal(t, []CloseRoute{CloseRouteWindow}, r.closed)
		})
	}
}

// A running flow holds the window as unsaved changes do, and the page is
// told which tabs run one.
func TestCloseGuard_RunningFlowHoldsTheClose(t *testing.T) {
	r, timer, runs := &recorder{}, &manualTimer{}, &fakeRunner{}
	g := fallbackGuard(r, timer, runs, func(CloseRoute, []CloseBlocker) bool { return false })
	runs.set("c")
	assert.False(t, g.allow(CloseRouteQuit))
	assert.Equal(t, []map[string]any{{"route": "quit", "tabs": []string{}, "running": []string{"c"}}}, r.events)
	g.acknowledge()
	g.cancel()

	// The flow finishes: nothing holds the window any more.
	runs.set()
	assert.True(t, g.allow(CloseRouteQuit))
}

// Through the App, the runner's own state is what holds the window: a
// running flow with nothing unsaved holds it, and a finished one does not.
func TestGuardClose_RunnerStateHoldsTheWindow(t *testing.T) {
	a := &App{runState: &runner{}}
	a.runState.tabID, a.runState.state = "t1", RunStateRunning
	SetClosers(a, func() {}, func() {})
	assert.False(t, GuardClose(a, CloseRouteWindow))
	a.CancelClose()

	a.runState.mu.Lock()
	a.runState.state = RunStateComplete
	a.runState.mu.Unlock()
	assert.True(t, GuardClose(a, CloseRouteWindow))
}

func TestCloseGuard_FallbackCancelKeepsTheWindow(t *testing.T) {
	r, timer, runs := &recorder{}, &manualTimer{}, &fakeRunner{}
	runs.set("c")
	g := fallbackGuard(r, timer, runs, func(CloseRoute, []CloseBlocker) bool { return false })
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteQuit))
	timer.fire()
	assert.Empty(t, r.closed)
	assert.Equal(t, 0, runs.stopped, "cancel keeps the flow running")
	// The next close is held and asked about again.
	assert.False(t, g.allow(CloseRouteQuit))
	assert.Len(t, r.events, 2)
}

func TestCloseGuard_AcknowledgedCloseNeverAsksNatively(t *testing.T) {
	r, timer, runs := &recorder{}, &manualTimer{}, &fakeRunner{}
	g := fallbackGuard(r, timer, runs, func(CloseRoute, []CloseBlocker) bool {
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
	r, timer, runs := &recorder{}, &manualTimer{}, &fakeRunner{}
	var g *closeGuard
	dialogs := 0
	g = fallbackGuard(r, timer, runs, func(CloseRoute, []CloseBlocker) bool {
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
	r, timer, runs := &recorder{}, &manualTimer{}, &fakeRunner{}
	g := fallbackGuard(r, timer, runs, func(route CloseRoute, _ []CloseBlocker) bool {
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
	asked := make(chan []CloseBlocker, 1)
	g := newTestGuard(r)
	g.ackTimeout = 10 * time.Millisecond
	g.fallback = func(_ CloseRoute, blockers []CloseBlocker) bool {
		asked <- blockers
		return false
	}
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteWindow))
	select {
	case blockers := <-asked:
		assert.Equal(t, []CloseBlocker{{TabID: "a", Name: "a", Reasons: []CloseReason{CloseReasonUnsaved}}}, blockers,
			"without a name resolver the id is the name")
	case <-time.After(5 * time.Second):
		t.Fatal("the fallback never opened")
	}
}

// Clearing the list, as the page does when it unloads, releases the guard.
func TestCloseGuard_ClearedListReleasesAHeldClose(t *testing.T) {
	r, timer, runs := &recorder{}, &manualTimer{}, &fakeRunner{}
	g := fallbackGuard(r, timer, runs, func(CloseRoute, []CloseBlocker) bool { return false })
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteWindow))
	g.setUnsaved(nil)
	assert.True(t, g.allow(CloseRouteWindow))
}
