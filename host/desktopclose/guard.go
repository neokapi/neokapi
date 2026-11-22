// Package desktopclose holds a desktop app's window close or quit while
// something in the app would be lost by it, and asks the page what to do.
//
// A desktop app's window hook and quit hook call Guard.Allow. While Blockers
// reports anything, the close is held and RequestedEvent goes to the page,
// which acknowledges it (Acknowledge) and answers with Confirm or Cancel. A
// page that never answers (a render error, a crashed web content process, a
// signal quit with no page) must not leave the app impossible to close. A
// held close therefore waits AckTimeout for the acknowledgement. Then it asks
// through Fallback, a native dialog that names each blocker with its reasons.
//
// The package has no UI and no Wails dependency: Kapi Desktop and Bowrain
// Desktop each supply the blockers, the event emitter, the closers and the
// native dialog.
package desktopclose

import (
	"sync"
	"time"
)

// Route names how the app was asked to close.
type Route string

const (
	// RouteWindow is the window's close button or Close Window (Cmd+W).
	RouteWindow Route = "window"
	// RouteQuit is Quit from the menu, Cmd+Q, the Dock, a logout, a signal,
	// or the updater handing over to its helper.
	RouteQuit Route = "quit"
)

// RequestedEvent is emitted when a close is held.
const RequestedEvent = "app:close-requested"

// DefaultAckTimeout is how long a held close waits for the page to
// acknowledge RequestedEvent before Fallback asks instead.
const DefaultAckTimeout = 4 * time.Second

// Blocker is one thing that holds the window, with each reason it does.
type Blocker struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Reasons []string `json:"reasons"`
}

// Guard holds a window close or an application quit while Blockers reports
// anything. The zero value is not usable; set Blockers first.
type Guard struct {
	// Blockers lists what holds the window now. It is called with the
	// guard's lock held and must not call back into the guard.
	Blockers func() []Blocker
	// Emit delivers RequestedEvent to the page.
	Emit func(name string, data any)
	// Fallback is the native dialog for a page that does not acknowledge in
	// time. It returns true to close anyway, false to cancel.
	Fallback func(route Route, blockers []Blocker) bool
	// OnCloseAnyway runs before the close a Fallback answer of true lets
	// through, such as stopping running work.
	OnCloseAnyway func()
	// AckTimeout is the wait for the page; DefaultAckTimeout when zero.
	AckTimeout time.Duration
	// After starts the wait; time.AfterFunc when nil. Tests replace it.
	After func(d time.Duration, f func()) (stop func() bool)

	mu        sync.Mutex
	confirmed bool
	pending   Route
	acked     bool
	asking    bool
	stopWait  func() bool
	closers   map[Route]func()
}

// SetClosers sets how a confirmed close finishes: closeWindow closes the
// window, quit ends the application.
func (g *Guard) SetClosers(closeWindow, quit func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closers = map[Route]func(){RouteWindow: closeWindow, RouteQuit: quit}
}

// Configure runs f with the guard's lock held, so an app can set its fields
// after hooks may already call Allow.
func (g *Guard) Configure(f func(g *Guard)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	f(g)
}

// Allow reports whether a close by route may go ahead now. With no blockers,
// or once the person has confirmed, it may. Otherwise the close is held. The
// first held request asks the page and starts the wait for its
// acknowledgement; a request while one is pending only records the route (a
// quit outranks a window close) and asks again once the page has answered
// the first.
func (g *Guard) Allow(route Route) bool {
	g.mu.Lock()
	blockers := g.blockersLocked()
	if g.confirmed || len(blockers) == 0 {
		g.mu.Unlock()
		return true
	}
	waiting := g.pending != "" && (!g.acked || g.asking)
	if g.pending == "" || route == RouteQuit {
		g.pending = route
	}
	if waiting {
		g.mu.Unlock()
		return false
	}
	g.acked = false
	emit := g.Emit
	g.startWaitLocked()
	g.mu.Unlock()
	if emit != nil {
		emit(RequestedEvent, map[string]any{"route": string(route), "blockers": blockers})
	}
	return false
}

// Acknowledge records that the page received the pending request and shows
// its own dialog, so Fallback stays closed.
func (g *Guard) Acknowledge() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending == "" {
		return
	}
	g.acked = true
	g.stopWaitLocked()
}

// Confirm lets every later close go ahead and finishes the held one, if any,
// by the route that asked. The closer runs without the lock held, because
// the close it starts passes through Allow again.
func (g *Guard) Confirm() {
	g.mu.Lock()
	g.confirmed = true
	route := g.pending
	g.resetLocked()
	closer := g.closers[route]
	g.mu.Unlock()
	if closer != nil {
		closer()
	}
}

// Cancel drops the held close. The next request is held and asked about
// again.
func (g *Guard) Cancel() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resetLocked()
}

func (g *Guard) blockersLocked() []Blocker {
	if g.Blockers == nil {
		return nil
	}
	return g.Blockers()
}

func (g *Guard) startWaitLocked() {
	g.stopWaitLocked()
	timeout := g.AckTimeout
	if timeout <= 0 {
		timeout = DefaultAckTimeout
	}
	after := g.After
	if after == nil {
		after = func(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }
	}
	g.stopWait = after(timeout, g.unanswered)
}

func (g *Guard) stopWaitLocked() {
	if g.stopWait != nil {
		g.stopWait()
		g.stopWait = nil
	}
}

func (g *Guard) resetLocked() {
	g.pending = ""
	g.acked = false
	g.stopWaitLocked()
}

// unanswered runs when the page did not acknowledge in time. It opens
// Fallback and applies the answer.
func (g *Guard) unanswered() {
	g.mu.Lock()
	if g.acked || g.asking || g.pending == "" || g.confirmed || g.Fallback == nil {
		g.mu.Unlock()
		return
	}
	g.asking = true
	route := g.pending
	blockers := g.blockersLocked()
	ask, anyway := g.Fallback, g.OnCloseAnyway
	g.mu.Unlock()

	closeAnyway := ask(route, blockers)

	g.mu.Lock()
	g.asking = false
	g.mu.Unlock()
	if !closeAnyway {
		g.Cancel()
		return
	}
	if anyway != nil {
		anyway()
	}
	g.Confirm()
}
