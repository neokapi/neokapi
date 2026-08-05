//go:build !wasm

package sqlitestore_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
)

func sharedDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestNewFromDB_MigratesAndRoundTrips(t *testing.T) {
	db := sharedDB(t)

	store, err := sqlitestore.NewFromDB(db, false)
	require.NoError(t, err)

	var ledgered int
	require.NoError(t, db.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM cache_migrations`).Scan(&ledgered))
	assert.Positive(t, ledgered, "the shared database carries the cache ledger under its own name")

	sess, err := store.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("docs", &blockstore.Block{
		ID: "b1", Hash: "h1", Source: []model.Run{model.TextR("Hello")},
	}))
	require.NoError(t, sess.Commit())

	read, err := store.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = read.Rollback() }()
	got, err := read.GetBlock("h1")
	require.NoError(t, err)
	assert.Equal(t, "b1", got.ID)
}

// Adopting a database twice must be idempotent: the merged store binds both a
// transactional and an autocommit handle to the same pool.
func TestNewFromDB_AdoptTwiceShareOnePool(t *testing.T) {
	db := sharedDB(t)

	transactional, err := sqlitestore.NewFromDB(db, false)
	require.NoError(t, err)
	autocommit, err := sqlitestore.NewFromDB(db, true)
	require.NoError(t, err)

	sess, err := autocommit.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("docs", &blockstore.Block{
		Hash: "h1", Source: []model.Run{model.TextR("Hello")},
	}))
	// No Commit: an autocommit write is durable when it returns.

	other, err := transactional.Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = other.Rollback() }()
	_, err = other.GetBlock("h1")
	assert.NoError(t, err, "both handles see the same rows")
}

// Closing an adopted store must not close the pool three other subsystems are
// still using. Closing one it opened itself must.
func TestNewFromDB_CloseDoesNotCloseAdoptedPool(t *testing.T) {
	db := sharedDB(t)

	store, err := sqlitestore.NewFromDB(db, false)
	require.NoError(t, err)
	require.NoError(t, store.Close())
	require.NoError(t, store.Close(), "Close is idempotent")

	var n int
	assert.NoError(t, db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM blocks`).Scan(&n),
		"the pool is still usable after the adopting store closed")

	owned, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	require.NoError(t, owned.Close())
	_, err = owned.Begin(t.Context())
	assert.ErrorIs(t, err, blockstore.ErrClosed, "a store that owns its file really closes")
}

func TestNewFromDB_RejectsNilDatabase(t *testing.T) {
	_, err := sqlitestore.NewFromDB(nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil database")
}
