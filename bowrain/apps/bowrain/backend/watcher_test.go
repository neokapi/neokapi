package backend

import (
	"testing"

	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestHandleEventEmitsTypedFrontendEvents verifies each broadened
// ProjectEvent variant maps to the right typed frontend event so each desktop
// view can refetch on the relevant external change.
func TestHandleEventEmitsTypedFrontendEvents(t *testing.T) {
	tests := []struct {
		name      string
		event     *pb.ProjectEvent
		wantName  string
		wantType  string
		assertExt func(t *testing.T, ce ChangeEvent)
	}{
		{
			name:     "project change",
			event:    &pb.ProjectEvent{Event: &pb.ProjectEvent_ProjectChange{ProjectChange: &pb.ProjectChangeEvent{EventType: "project.updated", ChangeType: "renamed", Actor: "alice"}}},
			wantName: "project-changed",
			wantType: "project.updated",
			assertExt: func(t *testing.T, ce ChangeEvent) {
				assert.Equal(t, "renamed", ce.ChangeType)
				assert.Equal(t, "alice", ce.Actor)
			},
		},
		{
			name:     "item change → project-changed",
			event:    &pb.ProjectEvent{Event: &pb.ProjectEvent_ItemChange{ItemChange: &pb.ItemChangeEvent{EventType: "item.created", ItemName: "about.json", Stream: "main"}}},
			wantName: "project-changed",
			wantType: "item.created",
			assertExt: func(t *testing.T, ce ChangeEvent) {
				assert.Equal(t, "about.json", ce.ItemName)
			},
		},
		{
			name:     "connector sync",
			event:    &pb.ProjectEvent{Event: &pb.ProjectEvent_ConnectorSync{ConnectorSync: &pb.ConnectorSyncEvent{EventType: "connector.sync.completed", Actor: "system"}}},
			wantName: "connector-sync",
			wantType: "connector.sync.completed",
		},
		{
			name:     "flow event",
			event:    &pb.ProjectEvent{Event: &pb.ProjectEvent_FlowEvent{FlowEvent: &pb.FlowEventEvent{EventType: "flow.completed"}}},
			wantName: "flow-changed",
			wantType: "flow.completed",
		},
		{
			name:     "membership change",
			event:    &pb.ProjectEvent{Event: &pb.ProjectEvent_MembershipChange{MembershipChange: &pb.MembershipChangeEvent{EventType: "task.assigned", Actor: "alice"}}},
			wantName: "membership-changed",
			wantType: "task.assigned",
		},
		{
			name:     "brand voice",
			event:    &pb.ProjectEvent{Event: &pb.ProjectEvent_BrandVoice{BrandVoice: &pb.BrandVoiceEvent{EventType: "brand.profile.updated"}}},
			wantName: "brand-voice-changed",
			wantType: "brand.profile.updated",
		},
		{
			name:     "termbase",
			event:    &pb.ProjectEvent{Event: &pb.ProjectEvent_Termbase{Termbase: &pb.TermBaseEvent{EventType: "concept.updated"}}},
			wantName: "termbase-changed",
			wantType: "concept.updated",
		},
		{
			name:     "stream",
			event:    &pb.ProjectEvent{Event: &pb.ProjectEvent_Stream{Stream: &pb.StreamEvent{EventType: "stream.merged", Stream: "feature-x"}}},
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
			app.SetEventSink(func(name string, data any) {
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
	app.SetEventSink(func(name string, data any) {
		got = append(got, emittedEvent{name: name, data: data})
	})
	watcher := &ProjectWatcher{app: app}

	watcher.handleEvent(&pb.ProjectEvent{Event: &pb.ProjectEvent_BlockChange{
		BlockChange: &pb.BlockChangeEvent{BlockId: "b1", ItemName: "home.json", ChangeType: "updated"},
	}})

	assert.Len(t, got, 1)
	assert.Equal(t, "blocks-changed", got[0].name)
	bc, ok := got[0].data.(BlockChangedEvent)
	assert.True(t, ok)
	assert.Equal(t, []string{"b1"}, bc.BlockIDs)
}
