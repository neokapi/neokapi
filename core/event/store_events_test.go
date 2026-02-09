package event

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEventStore(t *testing.T) (*EventEmittingStore, *ChannelEventBus) {
	t.Helper()
	inner, err := store.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { inner.Close() })

	bus := NewChannelEventBus()
	t.Cleanup(func() { bus.Close() })

	return NewEventEmittingStore(inner, bus), bus
}

func TestEventEmittingStoreProject(t *testing.T) {
	es, bus := newTestEventStore(t)
	ctx := context.Background()

	var events []Event
	var mu sync.Mutex
	bus.SubscribeAll(func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	p := &store.Project{Name: "Test", SourceLocale: model.LocaleEnglish}
	require.NoError(t, es.CreateProject(ctx, p))

	p.Name = "Updated"
	require.NoError(t, es.UpdateProject(ctx, p))

	require.NoError(t, es.DeleteProject(ctx, p.ID))

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, events, 3)
	assert.Equal(t, EventProjectCreated, events[0].Type)
	assert.Equal(t, EventProjectUpdated, events[1].Type)
	assert.Equal(t, EventProjectDeleted, events[2].Type)
}

func TestEventEmittingStoreBlocks(t *testing.T) {
	es, bus := newTestEventStore(t)
	ctx := context.Background()

	var events []Event
	var mu sync.Mutex
	bus.Subscribe(EventBlockUpdated, func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	bus.Subscribe(EventBlockDeleted, func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	p := &store.Project{Name: "Test", SourceLocale: model.LocaleEnglish}
	require.NoError(t, es.CreateProject(ctx, p))

	blocks := []*model.Block{model.NewBlock("b1", "Hello"), model.NewBlock("b2", "World")}
	require.NoError(t, es.StoreBlocks(ctx, p.ID, blocks))

	require.NoError(t, es.DeleteBlock(ctx, p.ID, "b1"))

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, events, 3) // 2 updates + 1 delete
}

func TestEventEmittingStoreVersion(t *testing.T) {
	es, bus := newTestEventStore(t)
	ctx := context.Background()

	var received Event
	var mu sync.Mutex
	bus.Subscribe(EventVersionCreated, func(e Event) {
		mu.Lock()
		received = e
		mu.Unlock()
	})

	p := &store.Project{Name: "Test", SourceLocale: model.LocaleEnglish}
	require.NoError(t, es.CreateProject(ctx, p))
	require.NoError(t, es.StoreBlocks(ctx, p.ID, []*model.Block{model.NewBlock("b1", "Hi")}))

	_, err := es.CreateVersion(ctx, p.ID, "v1", "First")
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, EventVersionCreated, received.Type)
	assert.Equal(t, "v1", received.Data["label"])
}
