package backend

import (
	"sync"
	"time"
)

// CloseRoute names how the app was asked to close.
type CloseRoute string

const (
	// CloseRouteWindow is the window's close button or Close Window (Cmd+W).
	CloseRouteWindow CloseRoute = "window"
	// CloseRouteQuit is Quit from the menu, Cmd+Q, the Dock, a logout, a
	// signal, or the updater handing over to its helper.
	CloseRouteQuit CloseRoute = "quit"
)

// CloseRequestedEvent is emitted when a close is held because tabs have
// unsaved changes. The frontend acknowledges it with AcknowledgeClose and
// answers with ConfirmClose or CancelClose.
const CloseRequestedEvent = "app:close-requested"

// closeAckTimeout is how long a held close waits for the page to acknowledge
// CloseRequestedEvent before the native fallback dialog asks instead.
const closeAckTimeout = 4 * time.Second

// SetUnsavedTabs records which project tabs have unsaved changes. The frontend
// calls it whenever that set changes, and with an empty list when the page
// unloads. While it is not empty, a window close or a quit is held and
// CloseRequestedEvent asks the frontend what to do.
func (a *App) SetUnsavedTabs(tabIDs []string) {
	a.closeGuard.setUnsaved(tabIDs)
}

// AcknowledgeClose tells the guard that the page received the close request
// and is showing its dialog, so the native fallback stays closed.
func (a *App) AcknowledgeClose() {
	a.closeGuard.acknowledge()
}

// ConfirmClose lets the held close go ahead, after the person saved or
// discarded every listed tab. Every later close goes ahead too.
func (a *App) ConfirmClose() {
	// The close it starts can call back into the quit hook on the main
	// thread, so the binding call returns first.
	go a.closeGuard.confirm()
}

// CancelClose drops the held close and keeps the window open.
func (a *App) CancelClose() {
	a.closeGuard.cancel()
}

// GuardClose is the window-close and quit hook's decision: true lets the close
// by route go ahead, false holds it and asks the frontend. It is a function
// rather than a method so the frontend bindings leave it out.
func GuardClose(a *App, route CloseRoute) bool {
	a.closeGuard.mu.Lock()
	if a.closeGuard.emit == nil {
		// The quit hook runs on the main thread; the event goes out after it
		// returns.
		a.closeGuard.emit = func(name string, data any) { go a.emitEvent(name, data) }
	}
	if a.closeGuard.tabNames == nil {
		a.closeGuard.tabNames = a.unsavedTabNames
	}
	a.closeGuard.mu.Unlock()
	return a.closeGuard.allow(route)
}

// SetClosers sets how a confirmed close finishes: closeWindow closes the
// window, quit ends the application.
func SetClosers(a *App, closeWindow, quit func()) {
	a.closeGuard.setClosers(closeWindow, quit)
}

// SetCloseFallback sets the native dialog that asks when the page does not
// acknowledge a held close in time: it lists the tabs with unsaved changes
// and returns true to close without saving, false to cancel.
func SetCloseFallback(a *App, ask func(route CloseRoute, tabNames []string) bool) {
	a.closeGuard.mu.Lock()
	defer a.closeGuard.mu.Unlock()
	a.closeGuard.fallback = ask
}

// unsavedTabNames names the tabs for the fallback dialog, by project name.
func (a *App) unsavedTabNames(tabIDs []string) []string {
	names := make([]string, 0, len(tabIDs))
	for _, id := range tabIDs {
		name := id
		if op := a.getOpenProject(id); op != nil && op.Project != nil && op.Project.Name != "" {
			name = op.Project.Name
		}
		names = append(names, name)
	}
	return names
}

// closeGuard holds a window close or an application quit while a project tab
// has unsaved changes. The frontend reports which tabs have changes through
// SetUnsavedTabs; the window hook and the quit hook ask GuardClose. A held
// close is finished by ConfirmClose, which runs the closer for the route that
// asked.
//
// A page that never answers (a render error, a crashed web content process)
// must not leave the app impossible to close. A held close therefore waits
// ackTimeout for AcknowledgeClose, then asks through the native fallback
// dialog, which names the tabs and offers to close without saving or cancel.
type closeGuard struct {
	mu        sync.Mutex
	unsaved   []string
	confirmed bool
	pending   CloseRoute
	// acked is set once the page acknowledged the pending request.
	acked bool
	// asking is set while the fallback dialog is open.
	asking   bool
	stopWait func() bool

	emit     func(name string, data any)
	closers  map[CloseRoute]func()
	fallback func(route CloseRoute, tabNames []string) bool
	tabNames func(tabIDs []string) []string

	// ackTimeout and after are the wait and the timer; tests shorten or
	// replace them.
	ackTimeout time.Duration
	after      func(d time.Duration, f func()) (stop func() bool)
}

// allow reports whether a close by route may go ahead now. With no unsaved
// changes, or once the person has confirmed, it may. Otherwise the close is
// held. The first held request asks the page and starts the wait for its
// acknowledgement; a request while one is pending only records the route (a
// quit outranks a window close) and asks again once the page has answered
// the first.
func (g *closeGuard) allow(route CloseRoute) bool {
	g.mu.Lock()
	if g.confirmed || len(g.unsaved) == 0 {
		g.mu.Unlock()
		return true
	}
	waiting := g.pending != "" && (!g.acked || g.asking)
	if g.pending == "" || route == CloseRouteQuit {
		g.pending = route
	}
	if waiting {
		g.mu.Unlock()
		return false
	}
	g.acked = false
	tabs := append([]string(nil), g.unsaved...)
	emit := g.emit
	g.startWaitLocked()
	g.mu.Unlock()
	if emit != nil {
		emit(CloseRequestedEvent, map[string]any{"route": string(route), "tabs": tabs})
	}
	return false
}

// startWaitLocked starts the wait for the page's acknowledgement. g.mu is held.
func (g *closeGuard) startWaitLocked() {
	if g.stopWait != nil {
		g.stopWait()
	}
	timeout := g.ackTimeout
	if timeout <= 0 {
		timeout = closeAckTimeout
	}
	after := g.after
	if after == nil {
		after = func(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }
	}
	g.stopWait = after(timeout, g.unanswered)
}

// unanswered runs when the page did not acknowledge in time. It opens the
// fallback dialog and applies the answer.
func (g *closeGuard) unanswered() {
	g.mu.Lock()
	if g.acked || g.asking || g.pending == "" || g.confirmed || g.fallback == nil {
		g.mu.Unlock()
		return
	}
	g.asking = true
	route := g.pending
	ids := append([]string(nil), g.unsaved...)
	names, ask := g.tabNames, g.fallback
	g.mu.Unlock()

	labels := ids
	if names != nil {
		labels = names(ids)
	}
	closeWithoutSaving := ask(route, labels)

	g.mu.Lock()
	g.asking = false
	g.mu.Unlock()
	if closeWithoutSaving {
		g.confirm()
	} else {
		g.cancel()
	}
}

// acknowledge records that the page received the pending request.
func (g *closeGuard) acknowledge() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pending == "" {
		return
	}
	g.acked = true
	if g.stopWait != nil {
		g.stopWait()
		g.stopWait = nil
	}
}

// setUnsaved records the tabs that have unsaved changes.
func (g *closeGuard) setUnsaved(tabIDs []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.unsaved = append([]string(nil), tabIDs...)
}

// setClosers sets what finishes a held close, for each route.
func (g *closeGuard) setClosers(closeWindow, quit func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closers = map[CloseRoute]func(){CloseRouteWindow: closeWindow, CloseRouteQuit: quit}
}

// confirm lets every later close go ahead and finishes the held one, if any,
// by the route that asked. The closer runs without the lock held, because the
// close it starts passes through allow again.
func (g *closeGuard) confirm() {
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

// cancel drops the held close. The next request is held and asked about again.
func (g *closeGuard) cancel() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resetLocked()
}

// resetLocked clears the pending request and its wait. g.mu is held.
func (g *closeGuard) resetLocked() {
	g.pending = ""
	g.acked = false
	if g.stopWait != nil {
		g.stopWait()
		g.stopWait = nil
	}
}
