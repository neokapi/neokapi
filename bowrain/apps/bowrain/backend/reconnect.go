package backend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

const (
	// reconnectInitialBackoff is the first wait after the connection drops.
	reconnectInitialBackoff = 2 * time.Second
	// reconnectMaxBackoff caps the wait between attempts. An outage that lasts
	// an afternoon still gets an attempt a minute, so the app comes back within
	// a minute of the network doing so.
	reconnectMaxBackoff = 60 * time.Second
	// reconnectMinInterval is the floor between two attempts, however many
	// nudges arrive. Every queued edit asks for an attempt, and a user typing
	// through an outage would otherwise turn each keystroke into a round trip.
	reconnectMinInterval = 2 * time.Second
)

// nextBackoff doubles cur, stopping at limit.
func nextBackoff(cur, limit time.Duration) time.Duration {
	next := cur * 2
	if next <= 0 || next > limit {
		return limit
	}
	return next
}

// jittered spreads a backoff over [d/2, d], picking a point with frac in [0,1).
// Clients that lost the same server share a schedule without it, and each time
// the backoff fires the server takes the whole fleet at once.
func jittered(d time.Duration, frac float64) time.Duration {
	if d <= 0 {
		return 0
	}
	half := d / 2
	return half + time.Duration(frac*float64(half))
}

// goOffline transitions the app to offline state and starts the reconnection goroutine.
// It is called when a server call fails due to a network error while connected.
func (a *App) goOffline() {
	a.mu.Lock()
	alreadyOffline := a.connState == StateOffline
	a.connState = StateOffline
	a.mu.Unlock()

	if alreadyOffline {
		return
	}

	slog.Info("bowrain: connection lost, switching to offline mode")
	// The change-event stream died with the connection, and it holds a client
	// the reconnect replaces. Close it here; resubscribe opens a fresh one
	// against the project the watcher is still remembered to have been on.
	a.stopWatcher()
	a.emitConnectionState()
	a.startReconnect()
}

// startReconnect launches a background goroutine that periodically attempts to
// restore the server connection and replay pending changes.
func (a *App) startReconnect() {
	a.stopReconnect()

	ctx, cancel := context.WithCancel(context.Background())
	nudge := make(chan struct{}, 1)
	a.mu.Lock()
	a.reconnectCancel = cancel
	a.reconnectNudge = nudge
	a.mu.Unlock()

	go a.reconnectLoop(ctx, nudge)
}

// nudgeReconnect asks a running reconnect loop to attempt now instead of waiting
// out its backoff. It never blocks: the channel holds one pending nudge, and a
// second one arriving before the first is read is the same request.
func (a *App) nudgeReconnect() {
	a.mu.RLock()
	nudge := a.reconnectNudge
	a.mu.RUnlock()
	if nudge == nil {
		return
	}
	select {
	case nudge <- struct{}{}:
	default:
	}
}

// RetryConnection attempts to restore the server connection now rather than at
// the end of the current backoff. The frontend calls it when the webview reports
// the network is back and when the user presses Retry on the offline indicator.
//
// The attempt runs in the background; the returned state is the one that holds
// as the call returns, and the outcome arrives as a connection-state-changed
// event.
func (a *App) RetryConnection() ConnectionInfo {
	if a.isOffline() {
		a.mu.RLock()
		running := a.reconnectNudge != nil
		a.mu.RUnlock()
		if !running {
			a.startReconnect()
		}
		a.nudgeReconnect()
	}
	return a.GetConnectionState()
}

func (a *App) reconnectLoop(ctx context.Context, nudge <-chan struct{}) {
	backoff := reconnectInitialBackoff
	var lastAttempt time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(jittered(backoff, rand.Float64())):
		case <-nudge:
			// A nudge is new information about the network, so the schedule
			// starts again from the bottom. The floor keeps a burst of them
			// from becoming a burst of round trips.
			backoff = reconnectInitialBackoff
			if since := time.Since(lastAttempt); since < reconnectMinInterval {
				select {
				case <-ctx.Done():
					return
				case <-time.After(reconnectMinInterval - since):
				}
			}
		}

		lastAttempt = time.Now()
		err := a.tryReconnect()
		switch {
		case err == nil:
			slog.Info("bowrain: reconnected to server")
			a.emitConnectionState()
			// Drain the offline queue against the reconnect loop's own context so
			// a cancellation (Disconnect) stops replay promptly. The REST editor
			// client threads this ctx through every request.
			a.replayPendingChanges(ctx)
			a.resubscribe()
			// Signal the frontend to force a full refresh of every open view.
			// While offline we may have missed any number of external changes
			// (other users, kapi push, connector sync, automations); replaying
			// our own queue is not enough — pull fresh authoritative state.
			a.emit("reconnected", a.GetConnectionState())
			return

		case errors.Is(err, errAuthRequired):
			// No credentials the server will take. Backing off against that
			// forever leaves the app in a state only a sign-in can leave, with
			// no sign-in offered, so end the loop and show the connect screen.
			slog.Warn("bowrain: reconnect needs a new sign-in", "error", err)
			a.markDisconnected()
			return

		default:
			backoff = nextBackoff(backoff, reconnectMaxBackoff)
			slog.Warn("bowrain: reconnect attempt failed", "error", err, "next_backoff", backoff)
		}
	}
}

// tryReconnect attempts to re-establish the connection to the server, returning
// why it could not.
func (a *App) tryReconnect() error {
	a.mu.RLock()
	serverURL := a.serverURL
	a.mu.RUnlock()

	if serverURL == "" {
		return fmt.Errorf("%w: no server to reconnect to", errAuthRequired)
	}
	return a.ConnectToServer(serverURL)
}

// markDisconnected drops the app to the disconnected state, which the frontend
// gate reads as "show the sign-in screen". Queued offline changes stay in the
// queue and replay after the next successful sign-in.
func (a *App) markDisconnected() {
	a.mu.Lock()
	a.connState = StateDisconnected
	a.remoteHTTP = nil
	a.mu.Unlock()
	a.emitConnectionState()
}

// resubscribe restores what the outage took down but the reconnect did not:
// the project change-event stream, which needs the new client, and this user's
// editing focus, which every other watcher stopped seeing when the stream ended.
func (a *App) resubscribe() {
	a.mu.RLock()
	projectID := a.watchedProject
	focus := a.presence
	a.mu.RUnlock()

	if projectID == "" {
		return
	}
	a.StartWatching(projectID)
	if focus.ProjectID != "" {
		a.UpdatePresence(focus.ProjectID, focus.ItemName, focus.BlockID)
	}
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
