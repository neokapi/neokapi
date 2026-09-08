package backend

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/editorclient"
)

// ProjectWatcher subscribes to a project's change-event stream over the server's
// /:ws/events SSE relay (via host/venue/client) and fans each event out to the
// frontend. It replaces the former gRPC WatchProject stream; the same relay
// backs the web app's useWorkspaceEvents.
type ProjectWatcher struct {
	app    *App
	cancel context.CancelFunc
}

// BlockChangedEvent is emitted to the frontend when blocks change.
type BlockChangedEvent struct {
	BlockIDs   []string `json:"block_ids"`
	ItemName   string   `json:"item_name"`
	ChangeType string   `json:"change_type"`
	ChangedBy  string   `json:"changed_by"`
}

// PresenceChangedEvent is emitted to the frontend when user presence changes.
type PresenceChangedEvent struct {
	ChangeType string       `json:"change_type"`
	User       PresenceUser `json:"user"`
}

// PresenceUser represents a user's presence info for the frontend.
type PresenceUser struct {
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	AvatarURL string `json:"avatar_url"`
	ItemName  string `json:"item_name"`
	BlockID   string `json:"block_id"`
}

// ChangeEvent is emitted to the frontend for non-block, non-presence external
// changes (project/item/connector/flow/membership/brand/terms/stream) so
// any open view can refresh itself. EventType carries the raw platform event
// type (e.g. "connector.sync.completed", "flow.completed").
type ChangeEvent struct {
	EventType  string `json:"event_type"`
	ChangeType string `json:"change_type,omitempty"`
	ItemName   string `json:"item_name,omitempty"`
	Stream     string `json:"stream,omitempty"`
	Actor      string `json:"actor,omitempty"`
}

const (
	// streamInitialBackoff is the first wait after the change-event stream ends.
	streamInitialBackoff = time.Second
	// streamMaxBackoff caps the wait between subscription attempts.
	streamMaxBackoff = 30 * time.Second
	// streamHealthyFor is how long a stream has to stay up to count as a
	// working subscription rather than an attempt that failed slowly.
	streamHealthyFor = 30 * time.Second
)

// streamBackoffAfter returns the wait before the next subscription attempt,
// given the current backoff and how long the stream that just ended lived for.
// A stream that stayed up long enough to have been a working subscription is
// evidence this path is healthy, so its next drop retries from the bottom.
// Carrying one bad afternoon's backoff across a whole session costs 30 seconds
// of staleness on every drop for the rest of the day.
func streamBackoffAfter(cur, lived time.Duration) time.Duration {
	if lived >= streamHealthyFor {
		return streamInitialBackoff
	}
	return cur
}

// presenceFocus is what this user was last reported to be editing. A reconnect
// re-reports it, because every other watcher's view of this user ended with the
// stream that carried it.
type presenceFocus struct {
	ProjectID string
	ItemName  string
	BlockID   string
}

// StartWatching opens a change-event subscription for the given project.
// Call StopWatching to close the stream when navigating away.
func (a *App) StartWatching(projectID string) {
	a.stopWatcher() // close any existing watcher

	a.mu.Lock()
	a.watchedProject = projectID
	a.mu.Unlock()

	if !a.isConnected() {
		return
	}

	client, ws := a.editorRemote()
	if client == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	watcher := &ProjectWatcher{app: a, cancel: cancel}

	a.mu.Lock()
	a.watcher = watcher
	a.mu.Unlock()

	go watcher.run(ctx, client, ws, projectID)
}

// StopWatching closes the active project watcher and forgets the project, so a
// later reconnect does not resurrect a subscription the user navigated away from.
func (a *App) StopWatching() {
	a.mu.Lock()
	a.watchedProject = ""
	a.presence = presenceFocus{}
	a.mu.Unlock()
	a.stopWatcher()
}

// stopWatcher closes the active subscription but keeps the project it was on,
// so an outage can be followed by a reconnect that restores it.
func (a *App) stopWatcher() {
	a.mu.Lock()
	w := a.watcher
	a.watcher = nil
	a.mu.Unlock()

	if w != nil && w.cancel != nil {
		w.cancel()
	}
}

// UpdatePresence reports the user's current editing focus to the server, which
// fans it out to other watchers over the SSE relay. Best-effort — a failure is
// logged and swallowed (per-cursor presence is carried over Yjs awareness). The
// focus is remembered either way, so a reconnect can report it again.
func (a *App) UpdatePresence(projectID, itemName, blockID string) {
	a.mu.Lock()
	a.presence = presenceFocus{ProjectID: projectID, ItemName: itemName, BlockID: blockID}
	a.mu.Unlock()

	if !a.isConnected() {
		return
	}
	client, ws := a.editorRemote()
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.ReportPresence(ctx, ws, projectID, itemName, blockID); err != nil {
		slog.Debug("bowrain: report presence failed", "error", err)
	}
}

// checkStillReachable decides what the end of a change-event stream meant. A
// server restart, a proxy idle timeout and a severed network all end it the same
// way, so the stream alone says nothing; one probe separates them. An
// unreachable server moves the app to offline mode now, which is where the user
// finds out about the outage anyway, minutes earlier than their next write.
func (a *App) checkStillReachable(ctx context.Context) {
	if !a.isConnected() {
		return
	}
	client, _ := a.editorRemote()
	if client == nil {
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, connectProbeTimeout)
	defer cancel()
	err := client.Ping(probeCtx)
	if err == nil || ctx.Err() != nil {
		return
	}

	if rejectedSession(err) {
		slog.Warn("bowrain: the server rejected the session", "error", err)
		a.markDisconnected()
		return
	}
	slog.Warn("bowrain: server unreachable after the change-event stream ended", "error", err)
	a.goOffline() //nolint:contextcheck // starts a reconnect loop with its own root context, by design
}

func (w *ProjectWatcher) run(ctx context.Context, client *editorclient.EditorClient, wsSlug, projectID string) {
	backoff := streamInitialBackoff

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// StreamProjectEvents reads a single SSE connection to exhaustion and
		// returns; we reconnect with backoff. A non-nil error is a hard failure.
		started := time.Now()
		err := client.StreamProjectEvents(ctx, wsSlug, projectID, w.handleEvent)
		if ctx.Err() != nil {
			return // context cancelled, clean shutdown
		}
		lived := time.Since(started)

		backoff = streamBackoffAfter(backoff, lived)

		slog.Warn("bowrain: change-event stream ended, reconnecting",
			"error", err, "lived", lived.Round(time.Millisecond), "backoff", backoff)

		// The stream is the earliest evidence the desktop gets of an outage.
		// A failed probe cancels this context, so the loop ends here and
		// resubscribe opens a fresh stream on the new client.
		w.app.checkStillReachable(ctx)

		select {
		case <-ctx.Done():
			return
		case <-time.After(jittered(backoff, rand.Float64())):
		}

		backoff = nextBackoff(backoff, streamMaxBackoff)
	}
}

// handleEvent maps a relayed change event to the frontend event the matching
// view listens for. It mirrors the server's busEventToProjectEvent routing but
// over the flattened SSE ChangeEvent shape.
func (w *ProjectWatcher) handleEvent(ev editorclient.EditorChangeEvent) {
	// emit() is safe when both the Wails app and the event sink are absent, and
	// the recording wbridge relies on the sink even without a Wails runtime, so
	// don't bail early here — let emit fan out to whichever sinks exist.
	t := ev.Type

	switch {
	case strings.HasPrefix(t, "editor.presence."):
		changeType := "moved"
		switch {
		case strings.Contains(t, "joined"):
			changeType = "joined"
		case strings.Contains(t, "left"):
			changeType = "left"
		}
		w.app.emit("presence-changed", PresenceChangedEvent{
			ChangeType: changeType,
			User: PresenceUser{
				UserID:    ev.UserID,
				UserName:  ev.UserName,
				AvatarURL: ev.AvatarURL,
				ItemName:  ev.ItemName,
				BlockID:   ev.BlockID,
			},
		})

	case strings.HasPrefix(t, "editor.block."), strings.HasPrefix(t, "block."):
		w.app.emit("blocks-changed", BlockChangedEvent{
			BlockIDs:   []string{ev.BlockID},
			ItemName:   ev.ItemName,
			ChangeType: ev.ChangeType,
			ChangedBy:  ev.ChangedBy,
		})

	case strings.HasPrefix(t, "item."):
		w.app.emit("project-changed", ChangeEvent{
			EventType: t,
			ItemName:  ev.ItemName,
			Stream:    ev.Stream,
		})

	case strings.HasPrefix(t, "connector."):
		w.app.emit("connector-sync", ChangeEvent{EventType: t, Actor: ev.Actor})

	case strings.HasPrefix(t, "flow."):
		w.app.emit("flow-changed", ChangeEvent{EventType: t})

	case strings.HasPrefix(t, "member."), strings.HasPrefix(t, "task."):
		w.app.emit("membership-changed", ChangeEvent{EventType: t, Actor: ev.Actor})

	case strings.HasPrefix(t, "voice."):
		w.app.emit("voice-changed", ChangeEvent{EventType: t})

	case strings.HasPrefix(t, "term."), strings.HasPrefix(t, "concept."):
		w.app.emit("terms-changed", ChangeEvent{EventType: t})

	case strings.HasPrefix(t, "stream."):
		w.app.emit("stream-changed", ChangeEvent{EventType: t, Stream: ev.Stream})

	case t == "":
		// Ignore empty/keepalive frames.

	default:
		// Project lifecycle, collections, extraction, quality gates, versions,
		// and any other state-changing event → generic project refresh.
		w.app.emit("project-changed", ChangeEvent{
			EventType:  t,
			ChangeType: ev.ChangeType,
			Actor:      ev.Actor,
		})
	}
}
