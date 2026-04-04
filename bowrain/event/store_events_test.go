package event

import (
	"context"
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEventStore(t *testing.T) (*EventEmittingStore, *ChannelEventBus) {
	t.Helper()
	inner, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { inner.Close() })

	bus := NewChannelEventBus()
	t.Cleanup(func() { bus.Close() })

	return NewEventEmittingStore(inner, bus), bus
}

func TestEventEmittingStoreProject(t *testing.T) {
	es, bus := newTestEventStore(t)
	ctx := context.Background()

	var events []platev.Event
	var mu sync.Mutex
	bus.SubscribeAll(func(e platev.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	p := &store.Project{Name: "Test", DefaultSourceLanguage: model.LocaleEnglish}
	require.NoError(t, es.CreateProject(ctx, p))

	p.Name = "Updated"
	require.NoError(t, es.UpdateProject(ctx, p))

	require.NoError(t, es.DeleteProject(ctx, p.ID))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) == 3
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, platev.EventProjectCreated, events[0].Type)
	assert.Equal(t, platev.EventProjectUpdated, events[1].Type)
	assert.Equal(t, platev.EventProjectDeleted, events[2].Type)
}

func TestEventEmittingStoreBlocks(t *testing.T) {
	es, bus := newTestEventStore(t)
	ctx := context.Background()

	var events []platev.Event
	var mu sync.Mutex
	bus.Subscribe(platev.EventBlockUpdated, func(e platev.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	bus.Subscribe(platev.EventBlockDeleted, func(e platev.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	p := &store.Project{Name: "Test", DefaultSourceLanguage: model.LocaleEnglish}
	require.NoError(t, es.CreateProject(ctx, p))

	blocks := []*model.Block{model.NewBlock("b1", "Hello"), model.NewBlock("b2", "World")}
	require.NoError(t, es.StoreBlocks(ctx, p.ID, "main", blocks))

	require.NoError(t, es.DeleteBlock(ctx, p.ID, "main", "b1"))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) == 3
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// 2 updates + 1 delete
}

func TestEventEmittingStoreVersion(t *testing.T) {
	es, bus := newTestEventStore(t)
	ctx := context.Background()

	var received platev.Event
	var mu sync.Mutex
	bus.Subscribe(platev.EventVersionCreated, func(e platev.Event) {
		mu.Lock()
		received = e
		mu.Unlock()
	})

	p := &store.Project{Name: "Test", DefaultSourceLanguage: model.LocaleEnglish}
	require.NoError(t, es.CreateProject(ctx, p))
	require.NoError(t, es.StoreBlocks(ctx, p.ID, "main", []*model.Block{model.NewBlock("b1", "Hi")}))

	_, err := es.CreateVersion(ctx, p.ID, "main", "v1", "First")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received.Type == platev.EventVersionCreated
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "v1", received.Data["label"])
}
