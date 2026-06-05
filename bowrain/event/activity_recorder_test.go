package event

import (
	"testing"
	"time"

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

func TestActivityRecorder_MapsEvents(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	store := newTestActivityStore(t)
	recorder := NewActivityRecorder(store, bus)
	defer recorder.Close()

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

	ctx := t.Context()
	var result *bstore.ActivityResult
	require.Eventually(t, func() bool {
		var err error
		result, err = store.List(ctx, bstore.ActivityQuery{WorkspaceID: "ws-1"})
		return err == nil && len(result.Activities) == 1
	}, 2*time.Second, 10*time.Millisecond)

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
	defer recorder.Close()

	// Publish an event type that is not mapped.
	bus.Publish(platev.Event{
		Type:      platev.EventBlockCreated,
		ProjectID: "proj-1",
		Data:      map[string]string{"workspace_slug": "ws-1"},
	})

	// Unmapped events should not produce activities. Give the bus time to deliver.
	time.Sleep(50 * time.Millisecond)

	ctx := t.Context()
	result, err := store.List(ctx, bstore.ActivityQuery{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	assert.Empty(t, result.Activities)
}

func TestActivityRecorder_MultipleEventTypes(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	store := newTestActivityStore(t)
	recorder := NewActivityRecorder(store, bus)
	defer recorder.Close()

	events := []platev.Event{
		{Type: platev.EventStreamCreated, ProjectID: "proj-1", Data: map[string]string{"workspace_slug": "ws-1", "stream": "feature/x"}},
		{Type: platev.EventFlowCompleted, ProjectID: "proj-1", Data: map[string]string{"workspace_slug": "ws-1"}},
		{Type: platev.EventQualityGateFail, ProjectID: "proj-1", Data: map[string]string{"workspace_slug": "ws-1"}},
	}
	for _, ev := range events {
		bus.Publish(ev)
	}

	ctx := t.Context()
	require.Eventually(t, func() bool {
		result, err := store.List(ctx, bstore.ActivityQuery{WorkspaceID: "ws-1"})
		return err == nil && len(result.Activities) == 3
	}, 2*time.Second, 10*time.Millisecond)
}

func TestActivityRecorder_Close(t *testing.T) {
	bus := NewChannelEventBus()
	defer bus.Close()

	store := newTestActivityStore(t)
	recorder := NewActivityRecorder(store, bus)
	recorder.Close()

	// Publishing after close should not cause a panic or create activities.
	bus.Publish(platev.Event{
		Type: platev.EventProjectCreated,
		Data: map[string]string{"workspace_slug": "ws-1"},
	})
	time.Sleep(50 * time.Millisecond)

	ctx := t.Context()
	result, err := store.List(ctx, bstore.ActivityQuery{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	assert.Empty(t, result.Activities)
}
