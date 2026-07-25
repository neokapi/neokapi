package backend

import (
	"context"
	"log/slog"
	"strings"
	"time"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
)

// ProjectWatcher subscribes to a project's change-event stream over the server's
// /:ws/events SSE relay (via bowrain/core/client) and fans each event out to the
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

// StartWatching opens a change-event subscription for the given project.
// Call StopWatching to close the stream when navigating away.
func (a *App) StartWatching(projectID string) {
	a.StopWatching() // close any existing watcher

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

// StopWatching closes the active project watcher.
func (a *App) StopWatching() {
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
// logged and swallowed (per-cursor presence is carried over Yjs awareness).
func (a *App) UpdatePresence(projectID, itemName, blockID string) {
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

func (w *ProjectWatcher) run(ctx context.Context, client *apiclient.BowrainClient, wsSlug, projectID string) {
	const maxBackoff = 30 * time.Second
	backoff := time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// StreamProjectEvents reads a single SSE connection to exhaustion and
		// returns; we reconnect with backoff. A non-nil error is a hard failure.
		err := client.StreamProjectEvents(ctx, wsSlug, projectID, w.handleEvent)
		if ctx.Err() != nil {
			return // context cancelled, clean shutdown
		}

		slog.Warn("bowrain: change-event stream ended, reconnecting", "error", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// handleEvent maps a relayed change event to the frontend event the matching
// view listens for. It mirrors the server's busEventToProjectEvent routing but
// over the flattened SSE ChangeEvent shape.
func (w *ProjectWatcher) handleEvent(ev apiclient.EditorChangeEvent) {
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

	case strings.HasPrefix(t, "brand."):
		w.app.emit("brand-voice-changed", ChangeEvent{EventType: t})

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
