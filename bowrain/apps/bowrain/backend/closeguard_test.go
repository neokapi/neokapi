package backend

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/host/desktopclose"
)

// The hold, the acknowledgement and the fallback are tested in
// host/desktopclose. These tests cover what Bowrain Desktop adds: the page's
// waiting saves are what hold the window.

func bowrainCloseApp(t *testing.T) (*App, chan map[string]any, chan desktopclose.Route) {
	t.Helper()
	a := &App{}
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

func TestGuardClose_NoWaitingSavesClosesAtOnce(t *testing.T) {
	a, _, _ := bowrainCloseApp(t)
	assert.True(t, GuardClose(a, desktopclose.RouteWindow))
	assert.True(t, GuardClose(a, desktopclose.RouteQuit))
}

func TestGuardClose_WaitingSavesHoldUntilThePageConfirms(t *testing.T) {
	a, events, closed := bowrainCloseApp(t)
	a.SetPendingSaves(true)
	assert.False(t, GuardClose(a, desktopclose.RouteWindow))
	event := <-events
	assert.Equal(t, "window", event["route"])
	assert.Equal(t, []desktopclose.Blocker{{
		ID: "saves", Name: "Changes not yet saved", Reasons: []string{PendingSavesReason},
	}}, event["blockers"])

	a.AcknowledgeClose()
	a.ConfirmClose()
	select {
	case route := <-closed:
		assert.Equal(t, desktopclose.RouteWindow, route)
	case <-time.After(5 * time.Second):
		t.Fatal("the confirmed close did not close")
	}
}

func TestGuardClose_FlushedSavesReleaseTheWindow(t *testing.T) {
	a, _, _ := bowrainCloseApp(t)
	a.SetPendingSaves(true)
	assert.False(t, GuardClose(a, desktopclose.RouteQuit))
	a.AcknowledgeClose()
	a.CancelClose()
	a.SetPendingSaves(false)
	assert.True(t, GuardClose(a, desktopclose.RouteQuit))
}
