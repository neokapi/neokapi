package backend

import (
	"context"
	"io"
	"log/slog"
	"time"

	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
)

// ProjectWatcher manages a WatchProject gRPC stream for real-time updates.
type ProjectWatcher struct {
	client pb.EditorServiceClient
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
	ChangeType string         `json:"change_type"`
	User       PresenceUser   `json:"user"`
	AllUsers   []PresenceUser `json:"all_users"`
}

// PresenceUser represents a user's presence info for the frontend.
type PresenceUser struct {
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	AvatarURL string `json:"avatar_url"`
	ItemName  string `json:"item_name"`
	BlockID   string `json:"block_id"`
}

// StartWatching opens a WatchProject stream for the given project.
// Call StopWatching to close the stream when navigating away.
func (a *App) StartWatching(projectID string) {
	a.StopWatching() // close any existing watcher

	if !a.isConnected() {
		return
	}

	a.mu.RLock()
	ws := a.activeWS
	client := a.remote
	a.mu.RUnlock()

	if client == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	watcher := &ProjectWatcher{
		client: client.editor,
		app:    a,
		cancel: cancel,
	}

	a.mu.Lock()
	a.watcher = watcher
	a.mu.Unlock()

	go watcher.run(ctx, ws, projectID)
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

// UpdatePresence reports the user's current position to the server.
func (a *App) UpdatePresence(projectID, itemName, blockID string) {
	if !a.isConnected() {
		return
	}
	a.mu.RLock()
	ws := a.activeWS
	client := a.remote
	a.mu.RUnlock()

	if client == nil {
		return
	}

	_ = client.UpdatePresence(ws, projectID, itemName, blockID)
}

func (w *ProjectWatcher) run(ctx context.Context, wsSlug, projectID string) {
	const maxBackoff = 30 * time.Second
	backoff := time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := w.watchOnce(ctx, wsSlug, projectID)
		if ctx.Err() != nil {
			return // context cancelled, clean shutdown
		}

		slog.Warn("bowrain: WatchProject stream ended, reconnecting", "error", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Exponential backoff.
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (w *ProjectWatcher) watchOnce(ctx context.Context, wsSlug, projectID string) error {
	stream, err := w.client.WatchProject(ctx, &pb.WatchProjectRequest{
		WorkspaceSlug: wsSlug,
		ProjectId:     projectID,
	})
	if err != nil {
		return err
	}

	for {
		event, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		w.handleEvent(event)
	}
}

func (w *ProjectWatcher) handleEvent(event *pb.ProjectEvent) {
	if w.app.app == nil {
		return // no Wails app to emit to
	}

	switch e := event.Event.(type) {
	case *pb.ProjectEvent_BlockChange:
		w.app.emit("blocks-changed", BlockChangedEvent{
			BlockIDs:   []string{e.BlockChange.BlockId},
			ItemName:   e.BlockChange.ItemName,
			ChangeType: e.BlockChange.ChangeType,
			ChangedBy:  e.BlockChange.ChangedBy,
		})

	case *pb.ProjectEvent_PresenceChange:
		var user PresenceUser
		if e.PresenceChange.User != nil {
			user = PresenceUser{
				UserID:    e.PresenceChange.User.UserId,
				UserName:  e.PresenceChange.User.UserName,
				AvatarURL: e.PresenceChange.User.AvatarUrl,
				ItemName:  e.PresenceChange.User.ItemName,
				BlockID:   e.PresenceChange.User.BlockId,
			}
		}
		w.app.emit("presence-changed", PresenceChangedEvent{
			ChangeType: e.PresenceChange.ChangeType,
			User:       user,
		})
	}
}
