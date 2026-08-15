package backend

import (
	"context"
	"errors"
	"log/slog"
	"time"

	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// goOffline transitions the app to offline state and starts the reconnection goroutine.
// It is called when a gRPC call fails due to a network error while connected.
func (a *App) goOffline() {
	a.mu.Lock()
	alreadyOffline := a.connState == StateOffline
	a.connState = StateOffline
	a.mu.Unlock()

	if alreadyOffline {
		return
	}

	slog.Info("bowrain: connection lost, switching to offline mode")
	a.emitConnectionState()
	a.startReconnect()
}

// startReconnect launches a background goroutine that periodically attempts to
// restore the server connection and replay pending changes.
func (a *App) startReconnect() {
	a.stopReconnect()

	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.reconnectCancel = cancel
	a.mu.Unlock()

	go a.reconnectLoop(ctx)
}

func (a *App) reconnectLoop(ctx context.Context) {
	const maxBackoff = 60 * time.Second
	backoff := 2 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		if a.tryReconnect() {
			slog.Info("bowrain: reconnected to server")
			// Drain the offline queue against the reconnect loop's own context so
			// a cancellation (Disconnect) stops replay promptly. The REST editor
			// client threads this ctx through every request.
			a.replayPendingChanges(ctx)
			// Signal the frontend to force a full refresh of every open view.
			// While offline we may have missed any number of external changes
			// (other users, kapi push, connector sync, automations); replaying
			// our own queue is not enough — pull fresh authoritative state.
			a.emit("reconnected", a.GetConnectionState())
			return
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// tryReconnect attempts to re-establish the gRPC connection to the server.
func (a *App) tryReconnect() bool {
	a.mu.RLock()
	serverURL := a.serverURL
	a.mu.RUnlock()

	if serverURL == "" {
		return false
	}

	// Try to connect using stored auth.
	if err := a.ConnectToServer(serverURL); err != nil {
		return false
	}

	a.emitConnectionState()
	return true
}

// replayPendingChanges drains the offline queue by replaying each change to the server.
func (a *App) replayPendingChanges(ctx context.Context) {
	if a.offlineQueue == nil {
		return
	}

	count := a.offlineQueue.PendingCount()
	if count == 0 {
		return
	}

	slog.Info("bowrain: replaying pending changes", "count", count)

	for {
		changes, err := a.offlineQueue.PeekPending(10)
		if err != nil || len(changes) == 0 {
			break
		}

		// A pass makes progress when at least one change leaves the pending set
		// (completed or terminally failed). A pass with no progress means every
		// change failed transiently while the connection stayed up; re-peeking
		// immediately would spin a tight retry loop against the server, so stop
		// and leave the rest for the next reconnect (MarkFailed's attempt cap
		// eventually retires serial offenders).
		progressed := false
		for _, change := range changes {
			if err := a.replayChange(ctx, change); err != nil {
				var statusErr *apiclient.StatusError
				if errors.As(err, &statusErr) && statusErr.Permanent() {
					// The server rejected the change outright (4xx) — e.g. a queued
					// review of a block whose translation no longer exists (422) or a
					// deleted block (404). Retrying the identical request can never
					// succeed, so retire it and keep draining the queue.
					slog.Warn("bowrain: dropping permanently failed change", "change_id", change.ID, "operation", change.Operation, "error", err)
					_ = a.offlineQueue.MarkFailedPermanent(change.ID, err.Error())
					progressed = true
					continue
				}

				slog.Warn("bowrain: replay failed for change", "change_id", change.ID, "operation", change.Operation, "error", err)
				_ = a.offlineQueue.MarkFailed(change.ID, err.Error())

				// If we lost connection again during replay, go offline. This
				// spins up a fresh, independent reconnect loop whose lifetime is
				// deliberately not tied to this replay's context.
				if !a.isConnected() {
					a.goOffline() //nolint:contextcheck // starts an independent reconnect loop with its own root context, by design
					return
				}
				continue
			}
			_ = a.offlineQueue.MarkCompleted(change.ID)
			progressed = true
		}

		if !progressed {
			slog.Warn("bowrain: replay made no progress; leaving remaining changes pending for the next reconnect")
			break
		}
	}

	_ = a.offlineQueue.PurgeCompleted()
	if failed := a.offlineQueue.FailedCount(); failed > 0 {
		slog.Warn("bowrain: some offline changes were permanently rejected by the server", "count", failed)
	}
	slog.Info("bowrain: pending changes replayed")
}

// replayChange replays a single pending change to the server. The persisted
// kind+payload is decoded back into its typed offlineOp, which knows how to
// replay itself; every op goes through the REST/SSE editor client, which threads
// ctx so a cancellation (Disconnect) stops replay promptly.
func (a *App) replayChange(ctx context.Context, change PendingChange) error {
	client, ws := a.editorRemote()
	if client == nil {
		return errNotConnected
	}

	op, err := decodeOp(opKind(change.Operation), change.Payload)
	if err != nil {
		return err
	}
	if op == nil {
		slog.Info("bowrain: unknown pending change operation:", "value", change.Operation)
		return nil // skip unknown operations
	}
	return op.replay(ctx, client, ws)
}

// emitConnectionState sends the current connection state to the frontend (Wails
// runtime and/or the recording event sink).
func (a *App) emitConnectionState() {
	a.emit("connection-state-changed", a.GetConnectionState())
}
