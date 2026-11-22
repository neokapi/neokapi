package backend

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// recorder collects the events and closer calls a closeGuard makes.
type recorder struct {
	mu     sync.Mutex
	events []map[string]any
	closed []CloseRoute
}

func (r *recorder) emit(name string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == CloseRequestedEvent {
		r.events = append(r.events, data.(map[string]any))
	}
}

func newTestGuard(r *recorder) *closeGuard {
	g := &closeGuard{emit: r.emit}
	g.setClosers(
		func() { r.mu.Lock(); r.closed = append(r.closed, CloseRouteWindow); r.mu.Unlock() },
		func() { r.mu.Lock(); r.closed = append(r.closed, CloseRouteQuit); r.mu.Unlock() },
	)
	return g
}

func TestCloseGuard_NoUnsavedChangesClosesAtOnce(t *testing.T) {
	r := &recorder{}
	g := newTestGuard(r)
	assert.True(t, g.allow(CloseRouteWindow))
	assert.True(t, g.allow(CloseRouteQuit))
	assert.Empty(t, r.events, "nothing to ask about")
}

func TestCloseGuard_UnsavedChangesHoldTheCloseAndAskOnce(t *testing.T) {
	for _, route := range []CloseRoute{CloseRouteWindow, CloseRouteQuit} {
		t.Run(string(route), func(t *testing.T) {
			r := &recorder{}
			g := newTestGuard(r)
			g.setUnsaved([]string{"a", "b"})
			assert.False(t, g.allow(route))
			assert.Equal(t, []map[string]any{{"route": string(route), "tabs": []string{"a", "b"}}}, r.events)
			assert.Empty(t, r.closed, "a held close runs no closer")
		})
	}
}

func TestCloseGuard_ConfirmFinishesTheRouteThatAsked(t *testing.T) {
	for _, route := range []CloseRoute{CloseRouteWindow, CloseRouteQuit} {
		t.Run(string(route), func(t *testing.T) {
			r := &recorder{}
			g := newTestGuard(r)
			g.setUnsaved([]string{"a"})
			assert.False(t, g.allow(route))
			g.confirm()
			assert.Equal(t, []CloseRoute{route}, r.closed)
			// The closer's own close passes the guard, and so does the quit a
			// last window close starts.
			assert.True(t, g.allow(CloseRouteWindow))
			assert.True(t, g.allow(CloseRouteQuit))
		})
	}
}

func TestCloseGuard_QuitOutranksAWindowClose(t *testing.T) {
	r := &recorder{}
	g := newTestGuard(r)
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteQuit))
	assert.False(t, g.allow(CloseRouteWindow))
	g.confirm()
	assert.Equal(t, []CloseRoute{CloseRouteQuit}, r.closed)
}

func TestCloseGuard_CancelKeepsEverything(t *testing.T) {
	r := &recorder{}
	g := newTestGuard(r)
	g.setUnsaved([]string{"a"})
	assert.False(t, g.allow(CloseRouteQuit))
	g.cancel()
	assert.Empty(t, r.closed)
	// The next request is held again and asks again, by its own route.
	assert.False(t, g.allow(CloseRouteWindow))
	assert.Len(t, r.events, 2)
	g.confirm()
	assert.Equal(t, []CloseRoute{CloseRouteWindow}, r.closed)
}

func TestCloseGuard_ConfirmWithNothingPendingClosesNothing(t *testing.T) {
	r := &recorder{}
	g := newTestGuard(r)
	g.confirm()
	assert.Empty(t, r.closed)
}

// The bound methods and the hook function drive the guard the frontend and
// main.go see: a held quit reaches the frontend as CloseRequestedEvent, and
// ConfirmClose quits.
func TestGuardClose_ThroughTheApp(t *testing.T) {
	a := &App{}
	events := make(chan any, 1)
	InjectEventSink(a, func(name string, data any) {
		if name == CloseRequestedEvent {
			events <- data
		}
	})
	quit := make(chan struct{})
	SetClosers(a, func() { t.Error("the window closer ran for a quit") }, func() { close(quit) })

	assert.True(t, GuardClose(a, CloseRouteQuit), "no unsaved tabs: the quit goes ahead")

	a.SetUnsavedTabs([]string{"tab-1"})
	assert.False(t, GuardClose(a, CloseRouteQuit))
	assert.Equal(t, map[string]any{"route": "quit", "tabs": []string{"tab-1"}}, <-events)

	a.ConfirmClose()
	<-quit
	assert.True(t, GuardClose(a, CloseRouteQuit))
}

func TestCloseGuard_SavedTabsReleaseTheGuard(t *testing.T) {
	r := &recorder{}
	g := newTestGuard(r)
	g.setUnsaved([]string{"a"})
	g.setUnsaved(nil)
	assert.True(t, g.allow(CloseRouteWindow))
	assert.Empty(t, r.events)
}
