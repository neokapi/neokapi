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

// CloseRequestedEvent is emitted when a close is held because a tab has
// unsaved changes or a running flow. The frontend acknowledges it with
// AcknowledgeClose and answers with ConfirmClose or CancelClose.
const CloseRequestedEvent = "app:close-requested"

// closeAckTimeout is how long a held close waits for the page to acknowledge
// CloseRequestedEvent before the native fallback dialog asks instead.
const closeAckTimeout = 4 * time.Second

// CloseReason says why a tab holds the window.
type CloseReason string

const (
	// CloseReasonUnsaved is a tab with unsaved changes.
	CloseReasonUnsaved CloseReason = "unsaved"
	// CloseReasonRunning is a tab whose flow is running.
	CloseReasonRunning CloseReason = "running"
)

// CloseBlocker is one tab that holds the window, for the native fallback
// dialog.
type CloseBlocker struct {
	TabID   string
	Name    string
	Reasons []CloseReason
}

// SetUnsavedTabs records which project tabs have unsaved changes. The frontend
// calls it whenever that set changes, and with an empty list when the page
// unloads. While it is not empty, or a flow is running, a window close or a
// quit is held and CloseRequestedEvent asks the frontend what to do.
func (a *App) SetUnsavedTabs(tabIDs []string) {
	a.closeGuard.setUnsaved(tabIDs)
}

// AcknowledgeClose tells the guard that the page received the close request
// and is showing its dialog, so the native fallback stays closed.
func (a *App) AcknowledgeClose() {
	a.closeGuard.acknowledge()
}

// ConfirmClose lets the held close go ahead, after the person saved or
// discarded every listed tab and stopped every running flow. Every later
// close goes ahead too.
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
	if a.closeGuard.tabName == nil {
		a.closeGuard.tabName = a.tabDisplayName
	}
	if a.closeGuard.running == nil {
		a.closeGuard.running = a.runningTabs
		a.closeGuard.stopRuns = func() { a.cancelRun("") }
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
// acknowledge a held close in time. It lists the tabs that hold the window,
// each with its reasons, and returns true to stop running flows and close
// without saving, false to cancel.
func SetCloseFallback(a *App, ask func(route CloseRoute, blockers []CloseBlocker) bool) {
	a.closeGuard.mu.Lock()
	defer a.closeGuard.mu.Unlock()
	a.closeGuard.fallback = ask
}

// tabDisplayName names a tab by its project name, or by its id.
func (a *App) tabDisplayName(tabID string) string {
	if op := a.getOpenProject(tabID); op != nil && op.Project != nil && op.Project.Name != "" {
		return op.Project.Name
	}
	return tabID
}

// runningTabs returns the tab whose flow is running, if any.
func (a *App) runningTabs() []string {
	if a.runState == nil {
		return nil
	}
	a.runState.mu.Lock()
	defer a.runState.mu.Unlock()
	if a.runState.state != RunStateRunning || a.runState.tabID == "" {
		return nil
	}
	return []string{a.runState.tabID}
}

// closeGuard holds a window close or an application quit while a project tab
// has unsaved changes or a running flow. The frontend reports unsaved tabs
// through SetUnsavedTabs; running flows come from the runner. The window hook
// and the quit hook ask GuardClose. A held close is finished by ConfirmClose,
// which runs the closer for the route that asked.
//
// A page that never answers (a render error, a crashed web content process)
// must not leave the app impossible to close. A held close therefore waits
// ackTimeout for AcknowledgeClose, then asks through the native fallback
// dialog, which names the tabs and offers to close anyway or cancel.
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
	fallback func(route CloseRoute, blockers []CloseBlocker) bool
	tabName  func(tabID string) string
	running  func() []string
	stopRuns func()

	// ackTimeout and after are the wait and the timer; tests shorten or
	// replace them.
	ackTimeout time.Duration
	after      func(d time.Duration, f func()) (stop func() bool)
}

// runningLocked asks the runner which tabs run a flow. g.mu is held; the
// runner never calls into the guard, so the two locks cannot cross.
func (g *closeGuard) runningLocked() []string {
	if g.running == nil {
		return nil
	}
	return g.running()
}

// allow reports whether a close by route may go ahead now. With nothing
// unsaved and no flow running, or once the person has confirmed, it may.
// Otherwise the close is held. The first held request asks the page and
// starts the wait for its acknowledgement; a request while one is pending
// only records the route (a quit outranks a window close) and asks again once
// the page has answered the first.
func (g *closeGuard) allow(route CloseRoute) bool {
	g.mu.Lock()
	running := g.runningLocked()
	if g.confirmed || (len(g.unsaved) == 0 && len(running) == 0) {
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
	unsaved := append([]string{}, g.unsaved...)
	emit := g.emit
	g.startWaitLocked()
	g.mu.Unlock()
	if emit != nil {
		emit(CloseRequestedEvent, map[string]any{
			"route":   string(route),
			"tabs":    unsaved,
			"running": append([]string{}, running...),
		})
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

// blockersLocked lists the tabs that hold the window, unsaved ones first,
// each once with every reason it has. g.mu is held.
func (g *closeGuard) blockersLocked() []CloseBlocker {
	var out []CloseBlocker
	index := map[string]int{}
	add := func(id string, reason CloseReason) {
		if i, ok := index[id]; ok {
			out[i].Reasons = append(out[i].Reasons, reason)
			return
		}
		index[id] = len(out)
		out = append(out, CloseBlocker{TabID: id, Name: id, Reasons: []CloseReason{reason}})
	}
	for _, id := range g.unsaved {
		add(id, CloseReasonUnsaved)
	}
	for _, id := range g.runningLocked() {
		add(id, CloseReasonRunning)
	}
	return out
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
	blockers := g.blockersLocked()
	name, ask, stop := g.tabName, g.fallback, g.stopRuns
	g.mu.Unlock()

	if name != nil {
		for i := range blockers {
			blockers[i].Name = name(blockers[i].TabID)
		}
	}
	closeAnyway := ask(route, blockers)

	g.mu.Lock()
	g.asking = false
	g.mu.Unlock()
	if !closeAnyway {
		g.cancel()
		return
	}
	if stop != nil {
		stop()
	}
	g.confirm()
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
