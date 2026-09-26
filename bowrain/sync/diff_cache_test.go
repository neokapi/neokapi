package sync

import (
	"context"
	"testing"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alicebob/miniredis/v2"
)

// countingStore embeds the full ContentStore interface (so it satisfies the
// type without re-declaring ~70 methods) and overrides only the two methods the
// DiffEngine reads through: ListItems and GetBlocks. Every other method panics
// if reached, which proves the DiffEngine only touches those two — and, in the
// cache-hit case, neither. The embedded nil interface is never dereferenced
// because the overrides shadow the methods the engine calls.
type countingStore struct {
	platstore.ContentStore // nil; method set inherited, never invoked

	listItemsCalls int
	getBlocksCalls int

	items  []*platstore.Item
	blocks map[string][]*venue.StoredBlock // itemName → blocks

	// afterRead runs once, after GetBlocks has read its rows and before the
	// engine sees them: the moment a concurrent writer can commit.
	afterRead func()
}

func (s *countingStore) ListItems(_ context.Context, _, _ string) ([]*platstore.Item, error) {
	s.listItemsCalls++
	return s.items, nil
}

func (s *countingStore) GetBlocks(_ context.Context, q platstore.BlockQuery) ([]*venue.StoredBlock, error) {
	s.getBlocksCalls++
	read := s.blocks[q.ItemName]
	if s.afterRead != nil {
		hook := s.afterRead
		s.afterRead = nil
		hook()
	}
	return read, nil
}

// TestDiffEngine_CacheHit_ServesFromRedis verifies that when a miniredis-backed
// RedisHashCache is pre-populated, CompareItems serves entirely from the cache
// and never queries the content store.
func TestDiffEngine_CacheHit_ServesFromRedis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewRedisHashCache(client, time.Hour)

	store := &countingStore{}
	engine := NewDiffEngine(store, cache)
	ctx := t.Context()

	// Pre-warm the cache with server item hashes.
	cache.View(ctx, "proj-1", "main").SetItemHashes(ctx, map[string]string{
		"en.json":     "h-en",
		"messages.po": "h-po",
	})

	clientHashes := map[string]string{
		"en.json":     "h-en",  // unchanged
		"messages.po": "h-CHG", // changed
		"new.json":    "h-new", // new on client
	}

	result, err := engine.CompareItems(ctx, "proj-1", "main", clientHashes)
	require.NoError(t, err)

	assert.Equal(t, []string{"messages.po"}, result.ChangedItems)
	assert.Equal(t, []string{"new.json"}, result.NewItems)
	assert.Equal(t, 1, result.UnchangedCount)
	assert.Empty(t, result.DeletedItems)

	// The whole point: a cache hit must short-circuit the DB entirely.
	assert.Zero(t, store.listItemsCalls, "ListItems must not be called on cache hit")
	assert.Zero(t, store.getBlocksCalls, "GetBlocks must not be called on cache hit")
}

// TestDiffEngine_CacheMiss_LoadsFromStoreAndPopulatesCache verifies the
// fallback path: on a miss, the engine reads the store, then writes the computed
// hashes into Redis so a subsequent call is served from cache.
func TestDiffEngine_CacheMiss_LoadsFromStoreAndPopulatesCache(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewRedisHashCache(client, time.Hour)

	store := &countingStore{
		items: []*platstore.Item{{Name: "en.json"}},
		blocks: map[string][]*venue.StoredBlock{
			"en.json": {
				{SourceID: "b1", ContentHash: "hash-b1"},
				{SourceID: "b2", ContentHash: "hash-b2"},
			},
		},
	}
	engine := NewDiffEngine(store, cache)
	ctx := t.Context()

	// First call: cache is cold → must read the store.
	hashes, err := engine.ExportItemHashes(ctx, "proj-1", "main")
	require.NoError(t, err)
	require.Len(t, hashes, 1)
	assert.NotEmpty(t, hashes["en.json"])
	assert.Equal(t, 1, store.listItemsCalls)
	assert.Equal(t, 1, store.getBlocksCalls)

	// The engine should have populated both the item-hash and block-hash caches.
	cachedItems, ok := cache.View(ctx, "proj-1", "main").GetItemHashes(ctx)
	require.True(t, ok, "item hashes should be cached after a miss")
	assert.Equal(t, hashes, cachedItems)

	cachedBlocks, ok := cache.View(ctx, "proj-1", "main").GetBlockHashes(ctx, "en.json")
	require.True(t, ok, "block hashes should be cached after a miss")
	// Cached as the transfer hash — both stored halves folded — because that is
	// what a push's block hashes are compared against.
	assert.Equal(t, map[string]string{
		"b1": model.ComputeRecordHash("hash-b1", ""),
		"b2": model.ComputeRecordHash("hash-b2", ""),
	}, cachedBlocks)

	// Second call: now served from the item-hash cache → no further store reads.
	hashes2, err := engine.ExportItemHashes(ctx, "proj-1", "main")
	require.NoError(t, err)
	assert.Equal(t, hashes, hashes2)
	assert.Equal(t, 1, store.listItemsCalls, "second call must not re-list items")
	assert.Equal(t, 1, store.getBlocksCalls, "second call must not re-read blocks")
}

// TestDiffEngine_CompareBlocks_CacheHit verifies the block-level Merkle layer is
// also served from the Redis cache without hitting the store.
func TestDiffEngine_CompareBlocks_CacheHit(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewRedisHashCache(client, time.Hour)

	store := &countingStore{}
	engine := NewDiffEngine(store, cache)
	ctx := t.Context()

	cache.View(ctx, "proj-1", "main").SetBlockHashes(ctx, "en.json", map[string]string{
		"b1": "h1",
		"b2": "h2",
	})

	clientBlocks := map[string]string{
		"b1": "h1",     // unchanged
		"b2": "h2-CHG", // changed
		"b3": "h3-new", // new
	}

	result, err := engine.CompareBlocks(ctx, "proj-1", "main", "en.json", clientBlocks)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"b2", "b3"}, result.Needed)
	assert.Empty(t, result.Deleted)
	assert.Empty(t, result.Conflicts)
	assert.Zero(t, store.getBlocksCalls, "GetBlocks must not be called on block cache hit")
}

// A push negotiation reads the store just before the push worker commits and
// invalidates, then files what it read. The pre-apply hashes must not answer
// the next negotiation: that is how a second push of the same content was
// told diff_computed, and how a push of changed content can be told
// unchanged.
func TestDiffEngine_InvalidateDuringRead_DoesNotCacheStaleHashes(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewRedisHashCache(client, time.Hour)
	ctx := t.Context()

	store := &countingStore{
		items:  []*platstore.Item{{Name: "en.json"}},
		blocks: map[string][]*venue.StoredBlock{"en.json": {{SourceID: "b1", ContentHash: "before"}}},
	}
	store.afterRead = func() {
		// The push worker: its write commits, then it invalidates.
		store.blocks["en.json"] = []*venue.StoredBlock{{SourceID: "b1", ContentHash: "after"}}
		cache.InvalidateProject(ctx, "proj-1")
	}
	engine := NewDiffEngine(store, cache)

	raced, err := engine.ExportItemHashes(ctx, "proj-1", "main")
	require.NoError(t, err)
	assert.Equal(t, venue.ComputeItemHash(map[string]string{"b1": model.ComputeRecordHash("before", "")}),
		raced["en.json"], "the racing read answers with what it read")

	after, err := engine.ExportItemHashes(ctx, "proj-1", "main")
	require.NoError(t, err)
	assert.Equal(t, venue.ComputeItemHash(map[string]string{"b1": model.ComputeRecordHash("after", "")}),
		after["en.json"], "the next read answers for the content the write left")
	assert.Equal(t, 2, store.getBlocksCalls, "the raced read's hashes were never served")
}

// One project's streams hold different content, so one stream's cached hashes
// never answer for another's.
func TestDiffEngine_CacheIsPerStream(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewRedisHashCache(client, time.Hour)
	ctx := t.Context()

	cache.View(ctx, "proj-1", "main").SetItemHashes(ctx, map[string]string{"en.json": "h-main"})
	store := &countingStore{}
	engine := NewDiffEngine(store, cache)

	hashes, err := engine.ExportItemHashes(ctx, "proj-1", "feature")
	require.NoError(t, err)
	assert.Empty(t, hashes, "the feature stream holds nothing")
	assert.Equal(t, 1, store.listItemsCalls, "a miss on the feature stream reads the store")
}

// An item row with no blocks anchors decisions after its file was removed. It
// is not content, so it takes no part in the root hash, as LoadTree leaves it
// out of the tree.
func TestDiffEngine_ItemWithoutBlocks_IsNotHashed(t *testing.T) {
	store := &countingStore{
		items: []*platstore.Item{{Name: "en.json"}, {Name: "removed.json"}},
		blocks: map[string][]*venue.StoredBlock{
			"en.json": {{SourceID: "b1", ContentHash: "h1"}},
		},
	}
	engine := NewDiffEngine(store, nil)

	hashes, err := engine.ExportItemHashes(t.Context(), "proj-1", "main")
	require.NoError(t, err)
	assert.Equal(t, []string{"en.json"}, keysOf(hashes))
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
