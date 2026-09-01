package backend

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/editorclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type emittedEvent struct {
	name string
	data any
}

func TestHandleBlockChangeEvent(t *testing.T) {
	app := newTestApp(t)
	watcher := &ProjectWatcher{app: app}

	// Should not panic even without Wails app.
	watcher.handleEvent(editorclient.EditorChangeEvent{
		Type:       "editor.block.updated",
		BlockID:    "b1",
		ItemName:   "file.html",
		ChangeType: "updated",
		ChangedBy:  "Alice",
	})
}

func TestHandlePresenceChangeEvent(t *testing.T) {
	app := newTestApp(t)
	watcher := &ProjectWatcher{app: app}

	// Should not panic even without Wails app.
	watcher.handleEvent(editorclient.EditorChangeEvent{
		Type:     "editor.presence.joined",
		UserID:   "u1",
		UserName: "Alice",
		ItemName: "file.html",
		BlockID:  "b1",
	})
}

func TestHandlePresenceChangeEventNoUser(t *testing.T) {
	app := newTestApp(t)
	watcher := &ProjectWatcher{app: app}

	// Should not panic with an unpopulated user.
	watcher.handleEvent(editorclient.EditorChangeEvent{Type: "editor.presence.left"})
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

// TestHandleEventEmitsTypedFrontendEvents verifies each relayed change-event
// type maps to the right typed frontend event so each desktop view can refetch
// on the relevant external change.
func TestHandleEventEmitsTypedFrontendEvents(t *testing.T) {
	tests := []struct {
		name      string
		event     editorclient.EditorChangeEvent
		wantName  string
		wantType  string
		assertExt func(t *testing.T, ce ChangeEvent)
	}{
		{
			name:     "project change",
			event:    editorclient.EditorChangeEvent{Type: "project.updated", ChangeType: "renamed", Actor: "alice"},
			wantName: "project-changed",
			wantType: "project.updated",
			assertExt: func(t *testing.T, ce ChangeEvent) {
				assert.Equal(t, "renamed", ce.ChangeType)
				assert.Equal(t, "alice", ce.Actor)
			},
		},
		{
			name:     "item change → project-changed",
			event:    editorclient.EditorChangeEvent{Type: "item.created", ItemName: "about.json", Stream: "main"},
			wantName: "project-changed",
			wantType: "item.created",
			assertExt: func(t *testing.T, ce ChangeEvent) {
				assert.Equal(t, "about.json", ce.ItemName)
			},
		},
		{
			name:     "connector sync",
			event:    editorclient.EditorChangeEvent{Type: "connector.sync.completed", Actor: "system"},
			wantName: "connector-sync",
			wantType: "connector.sync.completed",
		},
		{
			name:     "flow event",
			event:    editorclient.EditorChangeEvent{Type: "flow.completed"},
			wantName: "flow-changed",
			wantType: "flow.completed",
		},
		{
			name:     "membership change",
			event:    editorclient.EditorChangeEvent{Type: "task.assigned", Actor: "alice"},
			wantName: "membership-changed",
			wantType: "task.assigned",
		},
		{
			name:     "voice",
			event:    editorclient.EditorChangeEvent{Type: "voice.profile.updated"},
			wantName: "voice-changed",
			wantType: "voice.profile.updated",
		},
		{
			name:     "terms",
			event:    editorclient.EditorChangeEvent{Type: "concept.updated"},
			wantName: "terms-changed",
			wantType: "concept.updated",
		},
		{
			name:     "stream",
			event:    editorclient.EditorChangeEvent{Type: "stream.merged", Stream: "feature-x"},
			wantName: "stream-changed",
			wantType: "stream.merged",
			assertExt: func(t *testing.T, ce ChangeEvent) {
				assert.Equal(t, "feature-x", ce.Stream)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(t)
			var got []emittedEvent
			InjectEventSink(app, func(name string, data any) {
				got = append(got, emittedEvent{name: name, data: data})
			})
			watcher := &ProjectWatcher{app: app}

			watcher.handleEvent(tc.event)

			require.Len(t, got, 1)
			assert.Equal(t, tc.wantName, got[0].name)
			ce, ok := got[0].data.(ChangeEvent)
			require.True(t, ok, "expected ChangeEvent payload")
			assert.Equal(t, tc.wantType, ce.EventType)
			if tc.assertExt != nil {
				tc.assertExt(t, ce)
			}
		})
	}
}

// TestHandleBlockChangeEmitsBlocksChanged confirms block events still emit the
// original "blocks-changed" event for wire-compat with the existing editor.
func TestHandleBlockChangeEmitsBlocksChanged(t *testing.T) {
	app := newTestApp(t)
	var got []emittedEvent
	InjectEventSink(app, func(name string, data any) {
		got = append(got, emittedEvent{name: name, data: data})
	})
	watcher := &ProjectWatcher{app: app}

	watcher.handleEvent(editorclient.EditorChangeEvent{
		Type: "editor.block.updated", BlockID: "b1", ItemName: "home.json", ChangeType: "updated",
	})

	assert.Len(t, got, 1)
	assert.Equal(t, "blocks-changed", got[0].name)
	bc, ok := got[0].data.(BlockChangedEvent)
	assert.True(t, ok)
	assert.Equal(t, []string{"b1"}, bc.BlockIDs)
}

// TestHandlePresenceEmitsPresenceChanged confirms a presence relay event maps
// to the frontend presence-changed event with identity fields.
func TestHandlePresenceEmitsPresenceChanged(t *testing.T) {
	app := newTestApp(t)
	var got []emittedEvent
	InjectEventSink(app, func(name string, data any) {
		got = append(got, emittedEvent{name: name, data: data})
	})
	watcher := &ProjectWatcher{app: app}

	watcher.handleEvent(editorclient.EditorChangeEvent{
		Type: "editor.presence.moved", UserID: "u1", UserName: "Alice",
		AvatarURL: "https://x/a.png", ItemName: "home.json", BlockID: "b1",
	})

	require.Len(t, got, 1)
	assert.Equal(t, "presence-changed", got[0].name)
	pc, ok := got[0].data.(PresenceChangedEvent)
	require.True(t, ok)
	assert.Equal(t, "moved", pc.ChangeType)
	assert.Equal(t, "u1", pc.User.UserID)
	assert.Equal(t, "Alice", pc.User.UserName)
	assert.Equal(t, "https://x/a.png", pc.User.AvatarURL)
}
