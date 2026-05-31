package sync

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRedisCache spins up an in-process miniredis server and returns a
// RedisHashCache wired to it, plus the raw client and the miniredis handle for
// assertions on keys, TTLs, and time travel.
func newTestRedisCache(t *testing.T, ttl time.Duration) (*RedisHashCache, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisHashCache(client, ttl), client, mr
}

func TestRedisHashCache_ItemHashes_RoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		hashes map[string]string
	}{
		{
			name: "single entry",
			hashes: map[string]string{
				"en.json": "abc123",
			},
		},
		{
			name: "multiple entries",
			hashes: map[string]string{
				"en.json":     "abc123",
				"messages.po": "def456",
				"de.json":     "ghi789",
			},
		},
		{
			name: "values with special characters",
			hashes: map[string]string{
				"a:b:c.json": "sha256:deadbeef==",
				"empty":      "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, _, _ := newTestRedisCache(t, time.Hour)
			ctx := t.Context()

			cache.SetItemHashes(ctx, "proj-1", tt.hashes)

			got, ok := cache.GetItemHashes(ctx, "proj-1")
			require.True(t, ok, "expected cache hit after Set")
			assert.Equal(t, tt.hashes, got)
		})
	}
}

func TestRedisHashCache_BlockHashes_RoundTrip(t *testing.T) {
	cache, _, _ := newTestRedisCache(t, time.Hour)
	ctx := t.Context()

	hashes := map[string]string{
		"block-1": "h1",
		"block-2": "h2",
		"block-3": "h3",
	}

	cache.SetBlockHashes(ctx, "proj-1", "en.json", hashes)

	got, ok := cache.GetBlockHashes(ctx, "proj-1", "en.json")
	require.True(t, ok)
	assert.Equal(t, hashes, got)
}

func TestRedisHashCache_GetMiss(t *testing.T) {
	cache, _, _ := newTestRedisCache(t, time.Hour)
	ctx := t.Context()

	// Nothing stored yet → miss for both item and block hashes.
	items, ok := cache.GetItemHashes(ctx, "proj-unknown")
	assert.False(t, ok)
	assert.Nil(t, items)

	blocks, ok := cache.GetBlockHashes(ctx, "proj-unknown", "missing.json")
	assert.False(t, ok)
	assert.Nil(t, blocks)
}

func TestRedisHashCache_SetEmptyHashes_IsMiss(t *testing.T) {
	cache, client, _ := newTestRedisCache(t, time.Hour)
	ctx := t.Context()

	// Seed a value first so we can prove an empty Set clears it.
	cache.SetItemHashes(ctx, "proj-1", map[string]string{"a": "1"})
	cache.SetBlockHashes(ctx, "proj-1", "en.json", map[string]string{"b": "2"})

	// Setting an empty map deletes the key and writes nothing (len(hashes)==0).
	cache.SetItemHashes(ctx, "proj-1", map[string]string{})
	cache.SetBlockHashes(ctx, "proj-1", "en.json", map[string]string{})

	_, ok := cache.GetItemHashes(ctx, "proj-1")
	assert.False(t, ok, "empty set must clear item hashes → miss")

	_, ok = cache.GetBlockHashes(ctx, "proj-1", "en.json")
	assert.False(t, ok, "empty set must clear block hashes → miss")

	// The underlying keys should not exist at all.
	exists, err := client.Exists(ctx, "sync:items:proj-1", "sync:blocks:proj-1:en.json").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

func TestRedisHashCache_KeyNamespacing(t *testing.T) {
	cache, client, _ := newTestRedisCache(t, time.Hour)
	ctx := t.Context()

	cache.SetItemHashes(ctx, "proj-1", map[string]string{"a": "1"})
	cache.SetBlockHashes(ctx, "proj-1", "en.json", map[string]string{"b": "2"})

	// Item hashes live under sync:items:{projectID}.
	itemExists, err := client.Exists(ctx, "sync:items:proj-1").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), itemExists)

	// Block hashes live under sync:blocks:{projectID}:{itemName}.
	blockExists, err := client.Exists(ctx, "sync:blocks:proj-1:en.json").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), blockExists)

	// Different project must not collide.
	_, ok := cache.GetItemHashes(ctx, "proj-2")
	assert.False(t, ok)
}

func TestRedisHashCache_Isolation_BetweenItemsAndProjects(t *testing.T) {
	cache, _, _ := newTestRedisCache(t, time.Hour)
	ctx := t.Context()

	cache.SetBlockHashes(ctx, "proj-1", "en.json", map[string]string{"b1": "h1"})
	cache.SetBlockHashes(ctx, "proj-1", "de.json", map[string]string{"b2": "h2"})
	cache.SetBlockHashes(ctx, "proj-2", "en.json", map[string]string{"b3": "h3"})

	en1, ok := cache.GetBlockHashes(ctx, "proj-1", "en.json")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"b1": "h1"}, en1)

	de1, ok := cache.GetBlockHashes(ctx, "proj-1", "de.json")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"b2": "h2"}, de1)

	en2, ok := cache.GetBlockHashes(ctx, "proj-2", "en.json")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"b3": "h3"}, en2)
}

func TestRedisHashCache_TTLExpiry(t *testing.T) {
	ttl := 5 * time.Minute
	cache, client, mr := newTestRedisCache(t, ttl)
	ctx := t.Context()

	cache.SetItemHashes(ctx, "proj-1", map[string]string{"a": "1"})
	cache.SetBlockHashes(ctx, "proj-1", "en.json", map[string]string{"b": "2"})

	// The TTL must be applied to both key types.
	itemTTL, err := client.TTL(ctx, "sync:items:proj-1").Result()
	require.NoError(t, err)
	assert.Greater(t, itemTTL, time.Duration(0))
	assert.LessOrEqual(t, itemTTL, ttl)

	blockTTL, err := client.TTL(ctx, "sync:blocks:proj-1:en.json").Result()
	require.NoError(t, err)
	assert.Greater(t, blockTTL, time.Duration(0))
	assert.LessOrEqual(t, blockTTL, ttl)

	// Advance miniredis time past the TTL → both keys expire → cache miss.
	mr.FastForward(ttl + time.Second)

	_, ok := cache.GetItemHashes(ctx, "proj-1")
	assert.False(t, ok, "item hashes should expire after TTL")

	_, ok = cache.GetBlockHashes(ctx, "proj-1", "en.json")
	assert.False(t, ok, "block hashes should expire after TTL")
}

func TestRedisHashCache_Overwrite_ReplacesEntireSet(t *testing.T) {
	cache, _, _ := newTestRedisCache(t, time.Hour)
	ctx := t.Context()

	cache.SetItemHashes(ctx, "proj-1", map[string]string{
		"a.json": "1",
		"b.json": "2",
		"c.json": "3",
	})

	// Overwrite with a smaller set — stale keys (b, c) must be gone, not merged.
	cache.SetItemHashes(ctx, "proj-1", map[string]string{
		"a.json": "1-updated",
	})

	got, ok := cache.GetItemHashes(ctx, "proj-1")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"a.json": "1-updated"}, got)
}

func TestRedisHashCache_InvalidateProject(t *testing.T) {
	cache, client, _ := newTestRedisCache(t, time.Hour)
	ctx := t.Context()

	// Seed two projects with item + several block keys each.
	cache.SetItemHashes(ctx, "proj-1", map[string]string{"x": "1"})
	cache.SetBlockHashes(ctx, "proj-1", "en.json", map[string]string{"b1": "h1"})
	cache.SetBlockHashes(ctx, "proj-1", "de.json", map[string]string{"b2": "h2"})
	cache.SetBlockHashes(ctx, "proj-1", "fr.json", map[string]string{"b3": "h3"})

	cache.SetItemHashes(ctx, "proj-2", map[string]string{"y": "1"})
	cache.SetBlockHashes(ctx, "proj-2", "en.json", map[string]string{"b4": "h4"})

	cache.InvalidateProject(ctx, "proj-1")

	// proj-1 item + all block keys gone.
	_, ok := cache.GetItemHashes(ctx, "proj-1")
	assert.False(t, ok)
	for _, item := range []string{"en.json", "de.json", "fr.json"} {
		_, ok := cache.GetBlockHashes(ctx, "proj-1", item)
		assert.Falsef(t, ok, "block hashes for %s should be invalidated", item)
	}

	// No stray proj-1 keys remain.
	remaining, err := client.Keys(ctx, "sync:*:proj-1*").Result()
	require.NoError(t, err)
	assert.Empty(t, remaining)

	// proj-2 untouched.
	items2, ok := cache.GetItemHashes(ctx, "proj-2")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"y": "1"}, items2)
	blocks2, ok := cache.GetBlockHashes(ctx, "proj-2", "en.json")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"b4": "h4"}, blocks2)
}

func TestRedisHashCache_InvalidateProject_NoBlocks(t *testing.T) {
	cache, _, _ := newTestRedisCache(t, time.Hour)
	ctx := t.Context()

	cache.SetItemHashes(ctx, "proj-1", map[string]string{"x": "1"})

	// Invalidate when there are no block keys to scan — must not error/panic.
	cache.InvalidateProject(ctx, "proj-1")

	_, ok := cache.GetItemHashes(ctx, "proj-1")
	assert.False(t, ok)
}

func TestRedisHashCache_GetError_OnClosedClient(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := NewRedisHashCache(client, time.Hour)
	ctx := context.Background()

	cache.SetItemHashes(ctx, "proj-1", map[string]string{"a": "1"})

	// Close the client so the next HGetAll returns an error; the cache must
	// translate that into a clean miss (nil, false), not a panic.
	require.NoError(t, client.Close())

	items, ok := cache.GetItemHashes(ctx, "proj-1")
	assert.False(t, ok)
	assert.Nil(t, items)

	blocks, ok := cache.GetBlockHashes(ctx, "proj-1", "en.json")
	assert.False(t, ok)
	assert.Nil(t, blocks)
}
