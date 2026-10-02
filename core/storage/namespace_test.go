package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeDB creates a database at path holding one row, and closes it.
func writeDB(t *testing.T, path, value string) {
	t.Helper()
	db, err := storage.Open(path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE IF NOT EXISTS t (v TEXT)`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO t (v) VALUES (?)`, value)
	require.NoError(t, err)
	require.NoError(t, db.Close())
}

// readDB returns the rows a database at path holds.
func readDB(t *testing.T, path string) []string {
	t.Helper()
	db, err := storage.Open(path)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(context.Background(), `SELECT v FROM t ORDER BY v`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		require.NoError(t, rows.Scan(&v))
		out = append(out, v)
	}
	require.NoError(t, rows.Err())
	return out
}

// TestNamespace answers for database files through the driver that holds
// them, whether they are files on disk or databases in the browser module's
// memory.
func TestNamespace(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.db")
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	b := filepath.Join(sub, "b.db")

	held, err := storage.Exists(a)
	require.NoError(t, err)
	assert.False(t, held, "nothing is held before the first open")

	writeDB(t, a, "one")
	held, err = storage.Exists(a)
	require.NoError(t, err)
	assert.True(t, held)
	assert.Equal(t, []string{"one"}, readDB(t, a), "a database outlives the pool that wrote it")

	listed, err := storage.List(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{a}, listed)

	require.NoError(t, storage.Rename(a, b))
	held, err = storage.Exists(a)
	require.NoError(t, err)
	assert.False(t, held, "a renamed database leaves its old name")
	assert.Equal(t, []string{"one"}, readDB(t, b), "a renamed database keeps its rows")
	listed, err = storage.List(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{b}, listed, "List reaches below the directory")

	writeDB(t, a, "two")
	require.NoError(t, storage.Rename(b, a), "a rename replaces the database at its destination")
	assert.Equal(t, []string{"one"}, readDB(t, a))

	require.NoError(t, storage.RemoveAll(dir))
	listed, err = storage.List(dir)
	require.NoError(t, err)
	assert.Empty(t, listed)
	held, err = storage.Exists(a)
	require.NoError(t, err)
	assert.False(t, held)

	require.NoError(t, storage.Remove(a), "removing a database that is not there is not an error")
	listed, err = storage.List(filepath.Join(dir, "missing"))
	require.NoError(t, err)
	assert.Empty(t, listed, "a directory that does not exist holds no databases")
}

// TestNamespace_RelativePathsResolveAgainstTheWorkingDirectory: a database a
// command names relative to where it runs is the same database by either
// spelling.
func TestNamespace_RelativePathsResolveAgainstTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeDB(t, "rel.db", "x")
	held, err := storage.Exists(filepath.Join(dir, "rel.db"))
	require.NoError(t, err)
	assert.True(t, held)
	assert.Equal(t, []string{"x"}, readDB(t, filepath.Join(dir, "rel.db")))
}
