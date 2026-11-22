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
// The guard is a state machine; see State. Only one question is ever on
// screen: a page that acknowledges after the native dialog opened is told not
// to show its own (Acknowledge returns false), and an answer from a page that
// is not being asked is ignored.
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

// State is where a held close stands.
//
//	idle         nothing held
//	asked        RequestedEvent sent; waiting for the page to acknowledge
//	acknowledged the page shows its dialog; waiting for Confirm or Cancel
//	fallback     the native dialog shows; waiting for its answer
//	confirmed    the person agreed to close; every close goes ahead
//
// Cancel, and a native "cancel", return to idle.
type State string

const (
	StateIdle         State = "idle"
	StateAsked        State = "asked"
	StateAcknowledged State = "acknowledged"
	StateFallback     State = "fallback"
	StateConfirmed    State = "confirmed"
)

// Guard holds a window close or an application quit while Blockers reports
// anything. Set Blockers before the first Allow.
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

	mu       sync.Mutex
	state    State
	route    Route
	stopWait func() bool
	// onSettled runs, with the lock held, when a held close is answered and
	// the guard is back at idle (ExitOnSecondSignal resets its count there).
	onSettled func()
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

// State reports where the guard stands.
func (g *Guard) State() State {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stateLocked()
}

func (g *Guard) stateLocked() State {
	if g.state == "" {
		return StateIdle
	}
	return g.state
}

// Allow reports whether a close by route may go ahead now. Once confirmed,
// or with no blockers, it may. Otherwise the close is held:
//
//   - idle: the page is asked and the wait for its acknowledgement starts;
//   - acknowledged: the page is asked again (it refreshes its dialog);
//   - asked or fallback: the request only records its route, so a quit
//     outranks a window close when the pending question is answered.
func (g *Guard) Allow(route Route) bool {
	g.mu.Lock()
	state := g.stateLocked()
	if state == StateConfirmed {
		g.mu.Unlock()
		return true
	}
	blockers := g.blockersLocked()
	if len(blockers) == 0 {
		if state == StateAsked || state == StateAcknowledged {
			// What held the window is gone (saved, or the flow finished).
			g.resetLocked()
		}
		if state != StateFallback {
			g.mu.Unlock()
			return true
		}
	}
	if g.route == "" || route == RouteQuit {
		g.route = route
	}
	if state == StateAsked || state == StateFallback {
		g.mu.Unlock()
		return false
	}
	g.state = StateAsked
	emit := g.Emit
	g.startWaitLocked()
	g.mu.Unlock()
	if emit != nil {
		emit(RequestedEvent, map[string]any{"route": string(route), "blockers": blockers})
	}
	return false
}

// Acknowledge records that the page received the pending request. It returns
// true when the page should show its dialog: only from asked or
// acknowledged. When the native dialog already shows, or nothing is pending,
// it returns false and the page shows nothing, so one question is on screen.
func (g *Guard) Acknowledge() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch g.stateLocked() {
	case StateAsked, StateAcknowledged:
		g.state = StateAcknowledged
		g.stopWaitLocked()
		return true
	default:
		return false
	}
}

// Confirm finishes the pending close by the route that asked, and lets every
// later close go ahead. It applies only while the page is the one being asked
// (asked or acknowledged); otherwise it does nothing and returns false, so a
// late answer can never mark the guard confirmed with no close behind it. The
// closer runs without the lock held, because the close it starts passes
// through Allow again.
func (g *Guard) Confirm() bool {
	g.mu.Lock()
	switch g.stateLocked() {
	case StateAsked, StateAcknowledged:
	default:
		g.mu.Unlock()
		return false
	}
	closer := g.confirmLocked()
	g.mu.Unlock()
	if closer != nil {
		closer()
	}
	return true
}

// Cancel drops the pending close while the page is the one being asked. The
// next request is held and asked about again. It does nothing while the
// native dialog shows; that dialog's own answer decides.
func (g *Guard) Cancel() {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch g.stateLocked() {
	case StateAsked, StateAcknowledged:
		g.resetLocked()
	}
}

func (g *Guard) confirmLocked() func() {
	route := g.route
	g.stopWaitLocked()
	g.state = StateConfirmed
	g.route = ""
	return g.closers[route]
}

func (g *Guard) resetLocked() {
	g.stopWaitLocked()
	g.state = StateIdle
	g.route = ""
	if g.onSettled != nil {
		g.onSettled()
	}
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

// unanswered runs when the wait for the acknowledgement ends. Only from asked
// does it open the native dialog; an acknowledgement or an answer that
// arrived first wins.
func (g *Guard) unanswered() {
	g.mu.Lock()
	if g.stateLocked() != StateAsked || g.Fallback == nil {
		g.mu.Unlock()
		return
	}
	g.state = StateFallback
	g.stopWait = nil
	route := g.route
	blockers := g.blockersLocked()
	ask, anyway := g.Fallback, g.OnCloseAnyway
	g.mu.Unlock()

	closeAnyway := ask(route, blockers)
	if closeAnyway && anyway != nil {
		anyway()
	}

	g.mu.Lock()
	if g.stateLocked() != StateFallback {
		g.mu.Unlock()
		return
	}
	var closer func()
	if closeAnyway {
		closer = g.confirmLocked()
	} else {
		g.resetLocked()
	}
	g.mu.Unlock()
	if closer != nil {
		closer()
	}
}
