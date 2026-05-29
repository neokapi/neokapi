package event

import (
	"context"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResolver maps project IDs to workspace IDs for relay tests.
type fakeResolver map[string]string

func (f fakeResolver) WorkspaceForProject(_ context.Context, projectID string) (string, error) {
	if ws, ok := f[projectID]; ok {
		return ws, nil
	}
	return "", nil
}

func recvOne(t *testing.T, ch <-chan ChangeEvent) (ChangeEvent, bool) {
	t.Helper()
	select {
	case ce, ok := <-ch:
		return ce, ok
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for relay event")
		return ChangeEvent{}, false
	}
}

func TestChangeRelay_ProjectScopedDelivery(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()
	relay := NewChangeRelay(bus, fakeResolver{"proj-1": "ws-1"})
	defer relay.Close()

	_, ch := relay.Subscribe("ws-1", "proj-1")

	bus.Publish(platev.Event{
		Type:      platev.EventBlockUpdated,
		ProjectID: "proj-1",
		Data:      map[string]string{"block_id": "b1", "item_name": "home.json"},
	})

	ce, ok := recvOne(t, ch)
	require.True(t, ok)
	assert.Equal(t, "block.updated", ce.Type)
	assert.Equal(t, "proj-1", ce.ProjectID)
	assert.Equal(t, "b1", ce.BlockID)
	assert.Equal(t, "home.json", ce.ItemName)
}

func TestChangeRelay_FiltersByProject(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()
	relay := NewChangeRelay(bus, fakeResolver{"proj-1": "ws-1", "proj-2": "ws-1"})
	defer relay.Close()

	_, ch := relay.Subscribe("ws-1", "proj-1")

	// Event for a different project in the same workspace must NOT reach a
	// project-scoped client.
	bus.Publish(platev.Event{Type: platev.EventBlockUpdated, ProjectID: "proj-2", Data: map[string]string{}})
	// Event for the subscribed project must reach it.
	bus.Publish(platev.Event{Type: platev.EventBlockUpdated, ProjectID: "proj-1", Data: map[string]string{"block_id": "b9"}})

	ce, ok := recvOne(t, ch)
	require.True(t, ok)
	assert.Equal(t, "proj-1", ce.ProjectID)
	assert.Equal(t, "b9", ce.BlockID)

	// No further events should be queued (proj-2 was filtered out).
	select {
	case extra := <-ch:
		t.Fatalf("unexpected extra event: %+v", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestChangeRelay_WorkspaceScopedAcrossProjects(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()
	relay := NewChangeRelay(bus, fakeResolver{"proj-1": "ws-1", "proj-2": "ws-2"})
	defer relay.Close()

	// Workspace-scoped client (no project filter): receives events for any
	// project belonging to ws-1, but not ws-2.
	_, ch := relay.Subscribe("ws-1", "")

	bus.Publish(platev.Event{Type: platev.EventProjectUpdated, ProjectID: "proj-2", Data: map[string]string{}}) // ws-2: filtered
	bus.Publish(platev.Event{Type: platev.EventProjectUpdated, ProjectID: "proj-1", Data: map[string]string{}}) // ws-1: delivered

	ce, ok := recvOne(t, ch)
	require.True(t, ok)
	assert.Equal(t, "proj-1", ce.ProjectID)
	assert.Equal(t, "project.updated", ce.Type)

	select {
	case extra := <-ch:
		t.Fatalf("unexpected cross-workspace event: %+v", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestChangeRelay_WorkspaceFromEventData(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()
	// No resolver — workspace must come from event Data.
	relay := NewChangeRelay(bus, nil)
	defer relay.Close()

	_, ch := relay.Subscribe("ws-7", "")

	bus.Publish(platev.Event{
		Type:      platev.EventBrandProfileUpdated,
		ProjectID: "",
		Data:      map[string]string{"workspace_id": "ws-7"},
	})

	ce, ok := recvOne(t, ch)
	require.True(t, ok)
	assert.Equal(t, "brand.profile.updated", ce.Type)
}

func TestChangeRelay_SkipsPresenceAndAgent(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()
	relay := NewChangeRelay(bus, fakeResolver{"proj-1": "ws-1"})
	defer relay.Close()

	_, ch := relay.Subscribe("ws-1", "proj-1")

	bus.Publish(platev.Event{Type: "editor.presence.joined", ProjectID: "proj-1", Data: map[string]string{"event_kind": "presence"}})
	bus.Publish(platev.Event{Type: platev.EventAgentMessageSent, ProjectID: "proj-1", Data: map[string]string{}})
	// A real content event should still come through.
	bus.Publish(platev.Event{Type: platev.EventBlockUpdated, ProjectID: "proj-1", Data: map[string]string{"block_id": "x"}})

	ce, ok := recvOne(t, ch)
	require.True(t, ok)
	assert.Equal(t, "block.updated", ce.Type, "presence/agent events must be filtered out")
}

func TestChangeRelay_Teardown(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()
	relay := NewChangeRelay(bus, fakeResolver{"proj-1": "ws-1"})

	clientID, ch := relay.Subscribe("ws-1", "proj-1")
	require.Equal(t, 1, relay.ClientCount())

	relay.Unsubscribe(clientID)
	assert.Equal(t, 0, relay.ClientCount())

	// Channel must be closed after unsubscribe.
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed on unsubscribe")

	// Publishing after unsubscribe must not panic or deliver.
	bus.Publish(platev.Event{Type: platev.EventBlockUpdated, ProjectID: "proj-1", Data: map[string]string{}})

	// Close is idempotent-safe alongside an already-unsubscribed client.
	relay.Close()
}

func TestChangeRelay_NonBlockingOnFullClient(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()
	relay := NewChangeRelay(bus, fakeResolver{"proj-1": "ws-1"})
	defer relay.Close()

	relay.Subscribe("ws-1", "proj-1") // never drained

	// Publish more than the client buffer (64) — must not block the bus.
	done := make(chan struct{})
	go func() {
		for range 500 {
			bus.Publish(platev.Event{Type: platev.EventBlockUpdated, ProjectID: "proj-1", Data: map[string]string{}})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("relay blocked the event bus on a full client")
	}
}
