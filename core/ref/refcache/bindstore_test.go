package refcache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ref"
)

// The positions in the cache describe what one project store consumed. Bound to
// a different store they describe nothing here, so they are dropped and the next
// transfer replays the feed. The governance identities stay: they identify the
// committed record and what the venue published, neither of which the store
// holds.
func TestBindStore(t *testing.T) {
	cases := []struct {
		name        string
		recorded    string
		bind        string
		wantContent int64
		wantStoreID string
	}{
		{name: "the same store keeps its position", recorded: "store-a", bind: "store-a", wantContent: 42, wantStoreID: "store-a"},
		{name: "a different store drops it", recorded: "store-a", bind: "store-b", wantContent: 0, wantStoreID: "store-b"},
		{name: "a cache that named no store adopts one", recorded: "", bind: "store-b", wantContent: 42, wantStoreID: "store-b"},
		{name: "an unnameable store binds nothing", recorded: "store-a", bind: "", wantContent: 42, wantStoreID: "store-a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := &Cache{StoreID: tc.recorded, Streams: map[string]ref.Ref{}}
			cache.Consume("main", 42)
			cache.Record("main", ref.Ref{Context: "ctx", Terms: "trm", Decisions: "dec"})

			cache.BindStore(tc.bind)

			got := cache.Ref("main")
			assert.Equal(t, tc.wantContent, got.Content)
			assert.Equal(t, tc.wantStoreID, cache.StoreID)
			assert.Equal(t, "ctx", got.Context, "governance identities survive a store")
			assert.Equal(t, "trm", got.Terms)
			assert.Equal(t, "dec", got.Decisions)
		})
	}
}

// The store identity round-trips, so a position written by one command is read
// by the next as belonging to the store that consumed it.
func TestBindStore_RoundTrips(t *testing.T) {
	layout := testLayout(t)

	cache := Load(layout, "https://example.test", "p1")
	cache.Consume("main", 7)
	cache.BindStore("store-a")
	require.NoError(t, cache.Save(layout))

	reloaded := Load(layout, "https://example.test", "p1")
	assert.Equal(t, "store-a", reloaded.StoreID)
	reloaded.BindStore("store-b")
	assert.Zero(t, reloaded.Ref("main").Content)
}
