package backend

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/host/desktopclose"
)

// The hold, the acknowledgement, the timeout and the fallback are tested in
// host/desktopclose. These tests cover what Kapi Desktop adds: which tabs hold
// the window and why, the event the page receives, and stopping a running
// flow when the fallback closes anyway.

// closeApp is an App with a runner, its closers recorded, and every event the
// guard sends captured.
func closeApp(t *testing.T) (*App, chan map[string]any, chan desktopclose.Route) {
	t.Helper()
	a := &App{runState: &runner{}}
	events := make(chan map[string]any, 4)
	InjectEventSink(a, func(name string, data any) {
		if name == desktopclose.RequestedEvent {
			events <- data.(map[string]any)
		}
	})
	closed := make(chan desktopclose.Route, 4)
	SetClosers(a,
		func() { closed <- desktopclose.RouteWindow },
		func() { closed <- desktopclose.RouteQuit })
	return a, events, closed
}

func setRunning(a *App, tabID string) {
	a.runState.mu.Lock()
	defer a.runState.mu.Unlock()
	a.runState.tabID = tabID
	if tabID == "" {
		a.runState.state = RunStateComplete
	} else {
		a.runState.state = RunStateRunning
	}
}

func blockerOf(id string, reasons ...string) desktopclose.Blocker {
	return desktopclose.Blocker{ID: id, Name: id, Reasons: reasons}
}

func TestCloseBlockers_EachTabOnceWithItsReasons(t *testing.T) {
	for _, tc := range []struct {
		name    string
		unsaved []string
		running string
		want    []desktopclose.Blocker
	}{
		{name: "nothing"},
		{name: "only unsaved", unsaved: []string{"a", "b"},
			want: []desktopclose.Blocker{blockerOf("a", CloseReasonUnsaved), blockerOf("b", CloseReasonUnsaved)}},
		{name: "only running", running: "c",
			want: []desktopclose.Blocker{blockerOf("c", CloseReasonRunning)}},
		{name: "both", unsaved: []string{"a"}, running: "c",
			want: []desktopclose.Blocker{blockerOf("a", CloseReasonUnsaved), blockerOf("c", CloseReasonRunning)}},
		{name: "one tab with both", unsaved: []string{"a"}, running: "a",
			want: []desktopclose.Blocker{blockerOf("a", CloseReasonUnsaved, CloseReasonRunning)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := closeApp(t)
			a.SetUnsavedTabs(tc.unsaved)
			setRunning(a, tc.running)
			assert.Equal(t, tc.want, a.closeBlockers())
		})
	}
}

func TestGuardClose_UnsavedTabsHoldAndConfirmCloses(t *testing.T) {
	a, events, closed := closeApp(t)
	assert.True(t, GuardClose(a, desktopclose.RouteQuit), "nothing holds the window")

	a.SetUnsavedTabs([]string{"tab-1"})
	assert.False(t, GuardClose(a, desktopclose.RouteQuit))
	assert.Equal(t, map[string]any{
		"route":    "quit",
		"blockers": []desktopclose.Blocker{blockerOf("tab-1", CloseReasonUnsaved)},
	}, <-events)

	a.AcknowledgeClose()
	a.closeGuard().Confirm()
	assert.Equal(t, desktopclose.RouteQuit, <-closed)
	assert.True(t, GuardClose(a, desktopclose.RouteQuit))
}

// A running flow holds the window with nothing unsaved, and a finished one
// releases it.
func TestGuardClose_RunningFlowHoldsTheWindow(t *testing.T) {
	a, events, _ := closeApp(t)
	setRunning(a, "t1")
	assert.False(t, GuardClose(a, desktopclose.RouteWindow))
	assert.Equal(t, []desktopclose.Blocker{blockerOf("t1", CloseReasonRunning)}, (<-events)["blockers"])
	a.AcknowledgeClose()
	a.CancelClose()

	setRunning(a, "")
	assert.True(t, GuardClose(a, desktopclose.RouteWindow))
}

// Clearing the list, as the page does when it unloads, releases the window.
func TestGuardClose_ClearedListReleasesTheWindow(t *testing.T) {
	a, _, _ := closeApp(t)
	a.SetUnsavedTabs([]string{"a"})
	assert.False(t, GuardClose(a, desktopclose.RouteWindow))
	a.SetUnsavedTabs(nil)
	assert.True(t, GuardClose(a, desktopclose.RouteWindow))
}

// When the page never answers, the native fallback lists the tabs with their
// reasons, and closing anyway cancels the running flow before the close.
func TestGuardClose_FallbackCancelsTheRunningFlow(t *testing.T) {
	a, _, closed := closeApp(t)
	canceled := make(chan struct{})
	a.runState.cancel = func() { close(canceled) }
	setRunning(a, "t1")
	a.SetUnsavedTabs([]string{"t2"})

	var asked []desktopclose.Blocker
	answered := make(chan struct{})
	a.closeGuard().Configure(func(g *desktopclose.Guard) { g.AckTimeout = 10 * time.Millisecond })
	SetCloseFallback(a, func(route desktopclose.Route, blockers []desktopclose.Blocker) bool {
		assert.Equal(t, desktopclose.RouteQuit, route)
		asked = blockers
		close(answered)
		return true
	})
	assert.False(t, GuardClose(a, desktopclose.RouteQuit))

	select {
	case <-answered:
	case <-time.After(5 * time.Second):
		t.Fatal("the fallback never opened")
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("closing anyway did not cancel the running flow")
	}
	assert.Equal(t, []desktopclose.Blocker{
		blockerOf("t2", CloseReasonUnsaved),
		blockerOf("t1", CloseReasonRunning),
	}, asked)
	select {
	case route := <-closed:
		assert.Equal(t, desktopclose.RouteQuit, route)
	case <-time.After(5 * time.Second):
		t.Fatal("closing anyway did not close")
	}
}
