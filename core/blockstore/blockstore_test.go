package blockstore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runStoreSuite exercises the Store / Session contract against any
// provider. Keeps the per-provider test files tiny.
func runStoreSuite(t *testing.T, makeStore func() blockstore.Store) {
	t.Helper()

	t.Run("round-trip a single block", func(t *testing.T) {
		store := makeStore()
		defer store.Close()
		ctx := context.Background()

		sess, err := store.Begin(ctx)
		require.NoError(t, err)

		b := &blockstore.Block{
			ID:           "abc",
			Hash:         "h-abc",
			Translatable: true,
			Type:         klf.BlockTypeJSXElement,
		}
		require.NoError(t, sess.PutBlock("ui", b))
		require.NoError(t, sess.Commit())

		sess2, err := store.Begin(ctx)
		require.NoError(t, err)
		defer sess2.Close()

		got, err := sess2.GetBlock("h-abc")
		require.NoError(t, err)
		assert.Equal(t, "abc", got.ID)
		assert.Equal(t, "h-abc", got.Hash)
	})

	t.Run("GetBlock returns ErrNotFound for unknown hash", func(t *testing.T) {
		store := makeStore()
		defer store.Close()
		sess, err := store.Begin(context.Background())
		require.NoError(t, err)
		defer sess.Close()
		_, err = sess.GetBlock("nope")
		assert.ErrorIs(t, err, blockstore.ErrNotFound)
	})

	t.Run("Blocks iterates everything; filter by collection", func(t *testing.T) {
		store := makeStore()
		defer store.Close()
		ctx := context.Background()

		sess, err := store.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, sess.PutBlock("ui", &blockstore.Block{Hash: "h1", Translatable: true}))
		require.NoError(t, sess.PutBlock("ui", &blockstore.Block{Hash: "h2", Translatable: false}))
		require.NoError(t, sess.PutBlock("api", &blockstore.Block{Hash: "h3", Translatable: true}))
		require.NoError(t, sess.Commit())

		sess2, err := store.Begin(ctx)
		require.NoError(t, err)
		defer sess2.Close()

		// All three.
		var allHashes []string
		for b, err := range sess2.Blocks(blockstore.BlockFilter{}) {
			require.NoError(t, err)
			allHashes = append(allHashes, b.Hash)
		}
		assert.ElementsMatch(t, []string{"h1", "h2", "h3"}, allHashes)

		// Filtered to ui.
		var uiHashes []string
		for b, err := range sess2.Blocks(blockstore.BlockFilter{Collection: "ui"}) {
			require.NoError(t, err)
			uiHashes = append(uiHashes, b.Hash)
		}
		assert.ElementsMatch(t, []string{"h1", "h2"}, uiHashes)

		// Filtered to translatable.
		tr := true
		var trHashes []string
		for b, err := range sess2.Blocks(blockstore.BlockFilter{Translatable: &tr}) {
			require.NoError(t, err)
			trHashes = append(trHashes, b.Hash)
		}
		assert.ElementsMatch(t, []string{"h1", "h3"}, trHashes)
	})

	t.Run("Rollback discards uncommitted writes", func(t *testing.T) {
		store := makeStore()
		defer store.Close()
		ctx := context.Background()

		sess, err := store.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, sess.PutBlock("ui", &blockstore.Block{Hash: "ephemeral"}))
		require.NoError(t, sess.Rollback())

		sess2, err := store.Begin(ctx)
		require.NoError(t, err)
		defer sess2.Close()
		_, err = sess2.GetBlock("ephemeral")
		assert.ErrorIs(t, err, blockstore.ErrNotFound)
	})

	t.Run("Overlay put / get / list", func(t *testing.T) {
		store := makeStore()
		defer store.Close()
		ctx := context.Background()

		sess, err := store.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, sess.PutBlock("ui", &blockstore.Block{Hash: "h1"}))
		require.NoError(t, sess.PutBlock("ui", &blockstore.Block{Hash: "h2"}))
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind: "targets/fr", BlockHash: "h1", Payload: []byte(`"bonjour"`),
		}))
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind: "targets/fr", BlockHash: "h2", Payload: []byte(`"salut"`),
		}))
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind: "annotations/qa", BlockHash: "h1", Payload: []byte(`{"ok":true}`),
		}))
		require.NoError(t, sess.Commit())

		sess2, err := store.Begin(ctx)
		require.NoError(t, err)
		defer sess2.Close()

		got, err := sess2.GetOverlay("targets/fr", "h1")
		require.NoError(t, err)
		assert.Equal(t, `"bonjour"`, string(got.Payload))
		assert.NotZero(t, got.UpdatedAt)

		// List only targets/fr kind.
		var locales []string
		for sc, err := range sess2.ListOverlays("targets/fr") {
			require.NoError(t, err)
			locales = append(locales, sc.BlockHash)
		}
		assert.ElementsMatch(t, []string{"h1", "h2"}, locales)
	})

	t.Run("Overlay update overwrites by (kind, hash)", func(t *testing.T) {
		store := makeStore()
		defer store.Close()
		ctx := context.Background()

		sess, err := store.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind: "targets/fr", BlockHash: "h1", Payload: []byte(`"v1"`),
		}))
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind: "targets/fr", BlockHash: "h1", Payload: []byte(`"v2"`),
		}))
		require.NoError(t, sess.Commit())

		sess2, err := store.Begin(ctx)
		require.NoError(t, err)
		defer sess2.Close()
		got, err := sess2.GetOverlay("targets/fr", "h1")
		require.NoError(t, err)
		assert.Equal(t, `"v2"`, string(got.Payload))
	})

	t.Run("operations on a closed session return ErrClosed", func(t *testing.T) {
		store := makeStore()
		defer store.Close()
		sess, err := store.Begin(context.Background())
		require.NoError(t, err)
		require.NoError(t, sess.Commit())

		err = sess.PutBlock("ui", &blockstore.Block{Hash: "late"})
		assert.ErrorIs(t, err, blockstore.ErrClosed)
	})

	t.Run("empty block hash is rejected", func(t *testing.T) {
		store := makeStore()
		defer store.Close()
		sess, err := store.Begin(context.Background())
		require.NoError(t, err)
		defer sess.Close()
		err = sess.PutBlock("ui", &blockstore.Block{})
		assert.Error(t, err)
	})
}

func TestMemoryStore(t *testing.T) {
	runStoreSuite(t, func() blockstore.Store { return blockstore.NewMemoryStore() })
}

func TestMemoryStore_Capabilities(t *testing.T) {
	s := blockstore.NewMemoryStore()
	caps := s.Capabilities()
	assert.True(t, caps.RandomAccess)
	assert.True(t, caps.Concurrent)
	assert.True(t, caps.Writable)
	assert.False(t, caps.Remote)
}

func TestCacheStore(t *testing.T) {
	runStoreSuite(t, func() blockstore.Store {
		path := filepath.Join(t.TempDir(), "cache.db")
		store, err := sqlitestore.New(path)
		require.NoError(t, err)
		return store
	})
}

func TestCacheStore_PersistenceAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")

	// First process: write a block.
	s1, err := sqlitestore.New(path)
	require.NoError(t, err)
	sess, err := s1.Begin(context.Background())
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("ui", &blockstore.Block{Hash: "persisted"}))
	require.NoError(t, sess.Commit())
	require.NoError(t, s1.Close())

	// Second process: read it back.
	s2, err := sqlitestore.New(path)
	require.NoError(t, err)
	defer s2.Close()
	sess2, err := s2.Begin(context.Background())
	require.NoError(t, err)
	defer sess2.Close()
	got, err := sess2.GetBlock("persisted")
	require.NoError(t, err)
	assert.Equal(t, "persisted", got.Hash)
}

func TestCacheStore_Capabilities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	s, err := sqlitestore.New(path)
	require.NoError(t, err)
	defer s.Close()
	caps := s.Capabilities()
	assert.True(t, caps.RandomAccess)
	assert.True(t, caps.Concurrent)
	assert.True(t, caps.Writable)
}
