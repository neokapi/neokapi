package event

import (
	"testing"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestActivityStore(t *testing.T) *bstore.ActivityStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	_, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return bstore.NewActivityStore(db.DB)
}

// These tests read the store after recorder.Close rather than polling it
// against a deadline. Close unsubscribes, and the channel bus returns from
// Unsubscribe only once the handler has finished every event already delivered,
// so the read sees each published event handled however slowly the handler
// ran.

func TestActivityRecorder_MapsEvents(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	store := newTestActivityStore(t)
	recorder := NewActivityRecorder(store, bus)

	bus.Publish(platev.Event{
		Type:      platev.EventProjectCreated,
		ProjectID: "proj-1",
		Actor:     "user-1",
		Data: map[string]string{
			"actor_name":     "Alice",
			"name":           "Test Project",
			"workspace_slug": "ws-1",
		},
	})
	recorder.Close()

	result, err := store.List(t.Context(), bstore.ActivityQuery{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, result.Activities, 1)

	a := result.Activities[0]
	assert.Equal(t, bstore.ActivityProjectCreated, a.Type)
	assert.Equal(t, "user-1", a.ActorID)
	assert.Equal(t, "Alice", a.ActorName)
	assert.Contains(t, a.Summary, "Test Project")
}

func TestActivityRecorder_SkipsUnmappedEvents(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	store := newTestActivityStore(t)
	recorder := NewActivityRecorder(store, bus)

	// Publish an event type that is not mapped.
	bus.Publish(platev.Event{
		Type:      platev.EventBlockCreated,
		ProjectID: "proj-1",
		Data:      map[string]string{"workspace_slug": "ws-1"},
	})
	recorder.Close()

	result, err := store.List(t.Context(), bstore.ActivityQuery{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	assert.Empty(t, result.Activities)
}

func TestActivityRecorder_MultipleEventTypes(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	store := newTestActivityStore(t)
	recorder := NewActivityRecorder(store, bus)

	events := []platev.Event{
		{Type: platev.EventStreamCreated, ProjectID: "proj-1", Data: map[string]string{"workspace_slug": "ws-1", "stream": "feature/x"}},
		{Type: platev.EventFlowCompleted, ProjectID: "proj-1", Data: map[string]string{"workspace_slug": "ws-1"}},
		{Type: platev.EventQualityGateFail, ProjectID: "proj-1", Data: map[string]string{"workspace_slug": "ws-1"}},
	}
	for _, ev := range events {
		bus.Publish(ev)
	}
	recorder.Close()

	result, err := store.List(t.Context(), bstore.ActivityQuery{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	assert.Len(t, result.Activities, 3)
}

func TestActivityRecorder_Close(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	store := newTestActivityStore(t)
	recorder := NewActivityRecorder(store, bus)
	recorder.Close()

	// Publishing after close should not cause a panic or create activities.
	// Unsubscribe removed the subscriber before returning, so the bus has
	// nobody to deliver this event to.
	bus.Publish(platev.Event{
		Type: platev.EventProjectCreated,
		Data: map[string]string{"workspace_slug": "ws-1"},
	})

	result, err := store.List(t.Context(), bstore.ActivityQuery{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	assert.Empty(t, result.Activities)
}
