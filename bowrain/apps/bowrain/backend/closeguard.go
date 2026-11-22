package backend

import (
	"sync"

	"github.com/neokapi/neokapi/host/desktopclose"
)

// PendingSavesReason is the reason a blocker gives when the page holds
// edits that have not reached the server yet.
const PendingSavesReason = "pending_saves"

// closeState holds a window close or a quit while the page has saves waiting
// (an edit resting before its delayed save, or a save in flight), so the page
// can flush them before the window goes. The hold, the acknowledgement and
// the native fallback are host/desktopclose's, shared with Kapi Desktop.
type closeState struct {
	mu      sync.Mutex
	pending bool
	guard   desktopclose.Guard
	once    sync.Once
}

// SetPendingSaves records whether the page has saves waiting. The page calls
// it whenever that changes, and with false when it unloads.
func (a *App) SetPendingSaves(pending bool) {
	a.closing.mu.Lock()
	defer a.closing.mu.Unlock()
	a.closing.pending = pending
}

// AcknowledgeClose tells the guard that the page received the close request.
// It returns true when the page should flush and answer, and false when the
// native fallback already asks or nothing is pending, so only one question is
// ever on screen.
func (a *App) AcknowledgeClose() bool {
	return a.closeGuard().Acknowledge()
}

// ConfirmClose lets the held close go ahead, after the page flushed its saves
// or the person chose to close anyway. Every later close goes ahead too.
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
// by route go ahead, false holds it and asks the page to flush. It is a
// function rather than a method so the frontend bindings leave it out.
func GuardClose(a *App, route desktopclose.Route) bool {
	return a.closeGuard().Allow(route)
}

// SetClosers sets how a confirmed close finishes: closeWindow closes the
// window, quit ends the application.
func SetClosers(a *App, closeWindow, quit func()) {
	a.closeGuard().SetClosers(closeWindow, quit)
}

// SetCloseFallback sets the native dialog that asks when the page does not
// acknowledge a held close in time. It returns true to close without the
// waiting saves, false to cancel.
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
			g.Emit = func(name string, data any) { go a.emit(name, data) }
		})
	})
	return &a.closing.guard
}

// closeBlockers reports the page's waiting saves as the one blocker.
func (a *App) closeBlockers() []desktopclose.Blocker {
	a.closing.mu.Lock()
	defer a.closing.mu.Unlock()
	if !a.closing.pending {
		return nil
	}
	return []desktopclose.Blocker{{ID: "saves", Name: "Changes not yet saved", Reasons: []string{PendingSavesReason}}}
}
