package server

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemorySessionStore(t *testing.T) {
	store := NewMemorySessionStore()
	defer store.Close()
	runSessionStoreTests(t, store)
}

// TestRedisSessionStore runs the same test suite against a real Redis instance.
// Skipped unless BOWRAIN_REDIS_URL is set (e.g. in CI or with local Docker).
func TestRedisSessionStore(t *testing.T) {
	redisURL := os.Getenv("BOWRAIN_REDIS_URL")
	if redisURL == "" {
		t.Skip("BOWRAIN_REDIS_URL not set; skipping Redis integration test")
	}

	store, err := NewRedisSessionStore(redisURL, "")
	require.NoError(t, err)
	defer store.Close()
	runSessionStoreTests(t, store)
}

// runSessionStoreTests exercises the SessionStateStore contract.
func runSessionStoreTests(t *testing.T, store SessionStateStore) {
	ctx := context.Background()

	t.Run("set and get", func(t *testing.T) {
		require.NoError(t, store.Set(ctx, "test:key1", []byte("value1"), 1*time.Minute))
		val, err := store.Get(ctx, "test:key1")
		require.NoError(t, err)
		assert.Equal(t, []byte("value1"), val)
	})

	t.Run("get missing key", func(t *testing.T) {
		_, err := store.Get(ctx, "test:nonexistent")
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, store.Set(ctx, "test:key2", []byte("value2"), 1*time.Minute))
		require.NoError(t, store.Delete(ctx, "test:key2"))
		_, err := store.Get(ctx, "test:key2")
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})

	t.Run("delete missing key", func(t *testing.T) {
		// Should not error.
		require.NoError(t, store.Delete(ctx, "test:nonexistent-delete"))
	})

	t.Run("overwrite", func(t *testing.T) {
		require.NoError(t, store.Set(ctx, "test:key3", []byte("original"), 1*time.Minute))
		require.NoError(t, store.Set(ctx, "test:key3", []byte("updated"), 1*time.Minute))
		val, err := store.Get(ctx, "test:key3")
		require.NoError(t, err)
		assert.Equal(t, []byte("updated"), val)
	})

	t.Run("expiry", func(t *testing.T) {
		require.NoError(t, store.Set(ctx, "test:expiring", []byte("temp"), 1*time.Millisecond))
		time.Sleep(10 * time.Millisecond)
		_, err := store.Get(ctx, "test:expiring")
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})

	t.Run("typed helpers", func(t *testing.T) {
		entry := &deviceCodeEntry{
			UserCode:   "abcd-efgh",
			Interval:   5,
			ClientID:   "test-client",
			Authorized: true,
			UserEmail:  "test@example.com",
			UserName:   "Test User",
			OIDCSub:    "sub-123",
		}
		require.NoError(t, sessionSet(ctx, store, prefixDeviceCode, "dc-test", entry, 1*time.Minute))

		got, err := sessionGet[deviceCodeEntry](ctx, store, prefixDeviceCode, "dc-test")
		require.NoError(t, err)
		assert.Equal(t, entry.UserCode, got.UserCode)
		assert.Equal(t, entry.Authorized, got.Authorized)
		assert.Equal(t, entry.UserEmail, got.UserEmail)
		assert.Equal(t, entry.OIDCSub, got.OIDCSub)
	})
}
