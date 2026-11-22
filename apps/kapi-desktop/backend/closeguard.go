package backend

import (
	"slices"
	"sync"

	"github.com/neokapi/neokapi/host/desktopclose"
)

// Why a tab holds the window, as host/desktopclose blocker reasons.
const (
	// CloseReasonUnsaved is a tab with unsaved changes.
	CloseReasonUnsaved = "unsaved"
	// CloseReasonRunning is a tab whose flow is running.
	CloseReasonRunning = "running"
)

// closeState is the Kapi Desktop half of the close guard: the tabs the page
// reports as unsaved, beside the shared desktopclose.Guard that holds the
// close. Running flows come from the runner.
type closeState struct {
	mu      sync.Mutex
	unsaved []string
	guard   desktopclose.Guard
	once    sync.Once
}

// SetUnsavedTabs records which project tabs have unsaved changes. The frontend
// calls it whenever that set changes, and with an empty list when the page
// unloads. While it is not empty, or a flow is running, a window close or a
// quit is held and desktopclose.RequestedEvent asks the frontend what to do.
func (a *App) SetUnsavedTabs(tabIDs []string) {
	a.closing.mu.Lock()
	defer a.closing.mu.Unlock()
	a.closing.unsaved = slices.Clone(tabIDs)
}

// AcknowledgeClose tells the guard that the page received the close request
// and is showing its dialog, so the native fallback stays closed.
func (a *App) AcknowledgeClose() {
	a.closeGuard().Acknowledge()
}

// ConfirmClose lets the held close go ahead, after the person saved or
// discarded every listed tab and stopped every running flow. Every later
// close goes ahead too.
func (a *App) ConfirmClose() {
	// The close it starts can call back into the quit hook on the main
	// thread, so the binding call returns first.
	go a.closeGuard().Confirm()
}

// CancelClose drops the held close and keeps the window open.
func (a *App) CancelClose() {
	a.closeGuard().Cancel()
}

// GuardClose is the window-close and quit hook's decision: true lets the close
// by route go ahead, false holds it and asks the frontend. It is a function
// rather than a method so the frontend bindings leave it out.
func GuardClose(a *App, route desktopclose.Route) bool {
	return a.closeGuard().Allow(route)
}

// SetClosers sets how a confirmed close finishes: closeWindow closes the
// window, quit ends the application.
func SetClosers(a *App, closeWindow, quit func()) {
	a.closeGuard().SetClosers(closeWindow, quit)
}

// SetCloseFallback sets the native dialog that asks when the page does not
// acknowledge a held close in time. It lists each tab that holds the window
// with its reasons, and returns true to stop running flows and close without
// saving, false to cancel.
func SetCloseFallback(a *App, ask func(route desktopclose.Route, blockers []desktopclose.Blocker) bool) {
	a.closeGuard().Configure(func(g *desktopclose.Guard) { g.Fallback = ask })
}

// closeGuard returns the app's guard, wiring it to this app on first use.
func (a *App) closeGuard() *desktopclose.Guard {
	a.closing.once.Do(func() {
		a.closing.guard.Configure(func(g *desktopclose.Guard) {
			g.Blockers = a.closeBlockers
			// The quit hook runs on the main thread; the event goes out after
			// it returns.
			g.Emit = func(name string, data any) { go a.emitEvent(name, data) }
			g.OnCloseAnyway = func() { a.cancelRun("") }
		})
	})
	return &a.closing.guard
}

// closeBlockers lists the tabs that hold the window, unsaved ones first, each
// once with every reason it has.
func (a *App) closeBlockers() []desktopclose.Blocker {
	a.closing.mu.Lock()
	unsaved := slices.Clone(a.closing.unsaved)
	a.closing.mu.Unlock()

	var out []desktopclose.Blocker
	index := map[string]int{}
	add := func(id, reason string) {
		if i, ok := index[id]; ok {
			out[i].Reasons = append(out[i].Reasons, reason)
			return
		}
		index[id] = len(out)
		out = append(out, desktopclose.Blocker{ID: id, Name: a.tabDisplayName(id), Reasons: []string{reason}})
	}
	for _, id := range unsaved {
		add(id, CloseReasonUnsaved)
	}
	for _, id := range a.runningTabs() {
		add(id, CloseReasonRunning)
	}
	return out
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
