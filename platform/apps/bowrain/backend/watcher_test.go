package backend

import (
	"testing"

	pb "github.com/gokapi/gokapi/bowrain/proto/v1"
	"github.com/stretchr/testify/assert"
)

// mockWailsApp captures emitted events for testing.
type mockWailsApp struct {
	events []emittedEvent
}

type emittedEvent struct {
	name string
	data any
}

func TestHandleBlockChangeEvent(t *testing.T) {
	app := newTestApp(t)

	watcher := &ProjectWatcher{
		app: app,
	}

	event := &pb.ProjectEvent{
		Event: &pb.ProjectEvent_BlockChange{
			BlockChange: &pb.BlockChangeEvent{
				BlockId:    "b1",
				ItemName:   "file.html",
				ChangeType: "updated",
				ChangedBy:  "Alice",
			},
		},
	}

	// Should not panic even without Wails app.
	watcher.handleEvent(event)
}

func TestHandlePresenceChangeEvent(t *testing.T) {
	app := newTestApp(t)

	watcher := &ProjectWatcher{
		app: app,
	}

	event := &pb.ProjectEvent{
		Event: &pb.ProjectEvent_PresenceChange{
			PresenceChange: &pb.PresenceChangeEvent{
				ChangeType: "joined",
				User: &pb.PresenceInfo{
					UserId:   "u1",
					UserName: "Alice",
					ItemName: "file.html",
					BlockId:  "b1",
				},
			},
		},
	}

	// Should not panic even without Wails app.
	watcher.handleEvent(event)
}

func TestHandlePresenceChangeEventNilUser(t *testing.T) {
	app := newTestApp(t)

	watcher := &ProjectWatcher{
		app: app,
	}

	event := &pb.ProjectEvent{
		Event: &pb.ProjectEvent_PresenceChange{
			PresenceChange: &pb.PresenceChangeEvent{
				ChangeType: "left",
				User:       nil,
			},
		},
	}

	// Should not panic with nil user.
	watcher.handleEvent(event)
}

func TestStartStopWatching(t *testing.T) {
	app := newTestApp(t)

	// Starting without connection should be a no-op.
	app.StartWatching("proj-1")
	assert.Nil(t, app.watcher)

	// Stop should not panic when no watcher is active.
	app.StopWatching()
	assert.Nil(t, app.watcher)
}

func TestUpdatePresenceDisconnected(t *testing.T) {
	app := newTestApp(t)

	// Should not panic when disconnected.
	app.UpdatePresence("proj-1", "file.html", "b1")
}

func TestBlockChangedEventJSON(t *testing.T) {
	event := BlockChangedEvent{
		BlockIDs:   []string{"b1", "b2"},
		ItemName:   "file.html",
		ChangeType: "updated",
		ChangedBy:  "Alice",
	}

	assert.Equal(t, []string{"b1", "b2"}, event.BlockIDs)
	assert.Equal(t, "file.html", event.ItemName)
	assert.Equal(t, "updated", event.ChangeType)
	assert.Equal(t, "Alice", event.ChangedBy)
}

func TestPresenceChangedEvent(t *testing.T) {
	event := PresenceChangedEvent{
		ChangeType: "joined",
		User: PresenceUser{
			UserID:   "u1",
			UserName: "Alice",
			ItemName: "file.html",
			BlockID:  "b1",
		},
	}

	assert.Equal(t, "joined", event.ChangeType)
	assert.Equal(t, "u1", event.User.UserID)
	assert.Equal(t, "Alice", event.User.UserName)
}
