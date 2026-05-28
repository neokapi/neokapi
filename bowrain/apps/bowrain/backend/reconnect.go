package backend

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
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
			a.replayPendingChanges()
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
func (a *App) replayPendingChanges() {
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

		for _, change := range changes {
			if err := a.replayChange(change); err != nil {
				slog.Warn("bowrain: replay failed for change", "change_id", change.ID, "operation", change.Operation, "error", err)
				_ = a.offlineQueue.MarkFailed(change.ID, err.Error())

				// If we lost connection again during replay, go offline.
				if !a.isConnected() {
					a.goOffline()
					return
				}
				continue
			}
			_ = a.offlineQueue.MarkCompleted(change.ID)
		}
	}

	_ = a.offlineQueue.PurgeCompleted()
	slog.Info("bowrain: pending changes replayed")
}

// replayChange replays a single pending change to the server.
func (a *App) replayChange(change PendingChange) error {
	a.mu.RLock()
	ws := a.activeWS
	client := a.remote
	a.mu.RUnlock()

	if client == nil {
		return errNotConnected
	}

	switch change.Operation {
	case "update_block_target":
		var req UpdateBlockRequest
		if err := json.Unmarshal([]byte(change.Payload), &req); err != nil {
			return err
		}
		// Plain-text replay: emit a single TextRun so the server
		// receives the canonical Run sequence.
		runs := []RunInfo{{Text: &TextRunInfo{Text: req.Text}}}
		return client.UpdateBlockTarget(ws, req.ProjectID, req.BlockID, req.TargetLocale, runs)

	case "update_block_target_runs":
		var req UpdateBlockTargetRunsRequest
		if err := json.Unmarshal([]byte(change.Payload), &req); err != nil {
			return err
		}
		return client.UpdateBlockTarget(ws, req.ProjectID, req.BlockID, req.TargetLocale, req.Runs)

	case "review_block":
		var req reviewBlockPayload
		if err := json.Unmarshal([]byte(change.Payload), &req); err != nil {
			return err
		}
		return client.ReviewBlock(ws, req.ProjectID, req.ItemName, req.BlockID, req.TargetLocale, req.Reviewed)

	case "add_tm_entry":
		var req addTMPayload
		if err := json.Unmarshal([]byte(change.Payload), &req); err != nil {
			return err
		}
		_, err := client.AddTMEntry(ws, req.Source, req.Target, req.SourceLocale, req.TargetLocale)
		return err

	case "update_tm_entry":
		var req TMUpdateRequest
		if err := json.Unmarshal([]byte(change.Payload), &req); err != nil {
			return err
		}
		return client.UpdateTMEntry(ws, req.EntryID, req.Source, req.Target, req.SourceLocale, req.TargetLocale)

	case "delete_tm_entry":
		var req deleteTMPayload
		if err := json.Unmarshal([]byte(change.Payload), &req); err != nil {
			return err
		}
		return client.DeleteTMEntry(ws, req.EntryID)

	case "add_concept":
		var req AddConceptRequest
		if err := json.Unmarshal([]byte(change.Payload), &req); err != nil {
			return err
		}
		_, err := client.AddConcept(ws, req.Domain, req.Definition, req.Terms)
		return err

	case "update_concept":
		var req UpdateConceptRequest
		if err := json.Unmarshal([]byte(change.Payload), &req); err != nil {
			return err
		}
		return client.UpdateConcept(ws, req.ConceptID, req.Domain, req.Definition, req.Terms)

	case "delete_concept":
		var req deleteConceptPayload
		if err := json.Unmarshal([]byte(change.Payload), &req); err != nil {
			return err
		}
		return client.DeleteConcept(ws, req.ConceptID)

	default:
		slog.Info("bowrain: unknown pending change operation:", "value", change.Operation)
		return nil // skip unknown operations
	}
}

// emitConnectionState sends the current connection state to the frontend (Wails
// runtime and/or the recording event sink).
func (a *App) emitConnectionState() {
	a.emit("connection-state-changed", a.GetConnectionState())
}

// Payload types for offline queue serialization.

type reviewBlockPayload struct {
	ProjectID    string `json:"project_id"`
	ItemName     string `json:"item_name"`
	BlockID      string `json:"block_id"`
	TargetLocale string `json:"target_locale"`
	Reviewed     bool   `json:"reviewed"`
}

type addTMPayload struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
}

type deleteTMPayload struct {
	EntryID string `json:"entry_id"`
}

type deleteConceptPayload struct {
	ConceptID string `json:"concept_id"`
}
