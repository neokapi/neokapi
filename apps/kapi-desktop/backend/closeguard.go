package backend

import "sync"

// CloseRoute names how the app was asked to close.
type CloseRoute string

const (
	// CloseRouteWindow is the window's close button or Close Window (Cmd+W).
	CloseRouteWindow CloseRoute = "window"
	// CloseRouteQuit is Quit from the menu, Cmd+Q, the Dock, or a logout.
	CloseRouteQuit CloseRoute = "quit"
)

// CloseRequestedEvent is emitted when a close is held because tabs have
// unsaved changes. The frontend answers with ConfirmClose or CancelClose.
const CloseRequestedEvent = "app:close-requested"

// SetUnsavedTabs records which project tabs have unsaved changes. The frontend
// calls it whenever that set changes. While it is not empty, a window close or
// a quit is held and CloseRequestedEvent asks the frontend what to do.
func (a *App) SetUnsavedTabs(tabIDs []string) {
	a.closeGuard.setUnsaved(tabIDs)
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
	a.closeGuard.mu.Unlock()
	return a.closeGuard.allow(route)
}

// SetClosers sets how a confirmed close finishes: closeWindow closes the
// window, quit ends the application.
func SetClosers(a *App, closeWindow, quit func()) {
	a.closeGuard.setClosers(closeWindow, quit)
}

// closeGuard holds a window close or an application quit while a project tab
// has unsaved changes. The frontend reports which tabs have changes through
// SetUnsavedTabs; the window hook and the quit hook ask GuardClose. A held
// close is finished by ConfirmClose, which runs the closer for the route that
// asked.
type closeGuard struct {
	mu        sync.Mutex
	unsaved   []string
	confirmed bool
	pending   CloseRoute
	emit      func(name string, data any)
	closers   map[CloseRoute]func()
}

// allow reports whether a close by route may go ahead now. With no unsaved
// changes, or once the person has confirmed, it may. Otherwise the close is
// held and the frontend is asked once per held request.
func (g *closeGuard) allow(route CloseRoute) bool {
	g.mu.Lock()
	if g.confirmed || len(g.unsaved) == 0 {
		g.mu.Unlock()
		return true
	}
	// A quit outranks a window close: confirming it must end the app.
	if g.pending == "" || route == CloseRouteQuit {
		g.pending = route
	}
	tabs := append([]string(nil), g.unsaved...)
	emit := g.emit
	g.mu.Unlock()
	if emit != nil {
		emit(CloseRequestedEvent, map[string]any{"route": string(route), "tabs": tabs})
	}
	return false
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
	g.pending = ""
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
	g.pending = ""
}
