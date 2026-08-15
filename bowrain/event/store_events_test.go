package event

import (
	"sync"
	"testing"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEventStore(t *testing.T) (*EventEmittingStore, *ChannelEventBus) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	inner, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)

	bus := NewChannelEventBus()
	t.Cleanup(func() { bus.Close() })

	return NewEventEmittingStore(inner, bus), bus
}

func TestEventEmittingStoreProject(t *testing.T) {
	es, bus := newTestEventStore(t)
	ctx := t.Context()

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
	ctx := t.Context()

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

// The optional capabilities must survive the wrapper. An assertion sees the
// wrapper's method set, not the inner store's, so a capability the wrapper does
// not carry explicitly vanishes exactly where the server holds the wrapper —
// which is everywhere the HTTP handlers run.
func TestEventEmittingStoreCarriesOptionalCapabilities(t *testing.T) {
	es, _ := newTestEventStore(t)

	_, ok := any(es).(store.DecisionStore)
	assert.True(t, ok, "the decision ledger must survive the wrapper")
	_, ok = any(es).(store.BlockAccessStore)
	assert.True(t, ok, "the access ladder must survive the wrapper")

	// The knowledge engine reaches the content store through narrow interfaces
	// and asserts for each one. The server hands it the wrapper, so a method the
	// wrapper does not carry takes the whole capability with it: without
	// StreamBindingStore a pilot silently binds no candidate voice profile and
	// the trial reports none is bound; without CollectionResolver every affected
	// block groups under its item name and the reach split's collection counts
	// collapse to one bucket per file.
	_, ok = any(es).(knowledge.StreamBindingStore)
	assert.True(t, ok, "stream binding must survive the wrapper")
	_, ok = any(es).(knowledge.CollectionResolver)
	assert.True(t, ok, "collection resolution must survive the wrapper")
	_, ok = any(es).(knowledge.BlockSource)
	assert.True(t, ok, "the blast-radius block source must survive the wrapper")

	aliases, ok := any(es).(store.ChannelAliasStore)
	require.True(t, ok, "channel alias proposals must survive the wrapper")

	ctx := t.Context()
	n, err := aliases.UpsertChannelAliasProposals(ctx, []store.ChannelAliasProposal{{
		WorkspaceID: "ws-wrapper", Profile: "acme",
		ProposedChannel: "website", ExistingChannel: "web",
		Evidence: "one slug is a prefix of the other within the same product",
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	judged, err := aliases.JudgeChannelAliasProposal(ctx, store.ChannelAliasJudgement{
		WorkspaceID: "ws-wrapper", Profile: "acme",
		ProposedChannel: "website", ExistingChannel: "web",
		Status: store.ChannelAliasDismissed, JudgedBy: "u1",
	})
	require.NoError(t, err)
	assert.True(t, judged)

	got, err := aliases.ListChannelAliasProposals(ctx, "ws-wrapper", "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, store.ChannelAliasDismissed, got[0].Status)
}

func TestEventEmittingStoreVersion(t *testing.T) {
	es, bus := newTestEventStore(t)
	ctx := t.Context()

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
