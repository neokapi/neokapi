package sync

import (
	"context"
	"testing"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
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
	blocks map[string][]*platstore.StoredBlock // itemName → blocks
}

func (s *countingStore) ListItems(_ context.Context, _, _ string) ([]*platstore.Item, error) {
	s.listItemsCalls++
	return s.items, nil
}

func (s *countingStore) GetBlocks(_ context.Context, q platstore.BlockQuery) ([]*platstore.StoredBlock, error) {
	s.getBlocksCalls++
	return s.blocks[q.ItemName], nil
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
	cache.SetItemHashes(ctx, "proj-1", map[string]string{
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
		blocks: map[string][]*platstore.StoredBlock{
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
	cachedItems, ok := cache.GetItemHashes(ctx, "proj-1")
	require.True(t, ok, "item hashes should be cached after a miss")
	assert.Equal(t, hashes, cachedItems)

	cachedBlocks, ok := cache.GetBlockHashes(ctx, "proj-1", "en.json")
	require.True(t, ok, "block hashes should be cached after a miss")
	assert.Equal(t, map[string]string{"b1": "hash-b1", "b2": "hash-b2"}, cachedBlocks)

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

	cache.SetBlockHashes(ctx, "proj-1", "en.json", map[string]string{
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
