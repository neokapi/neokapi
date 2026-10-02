package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommit_LeavesThePoolUsable: whatever Commit returns, the transaction is
// over and the pool's connection can begin the next one. A second pool reading
// the file holds a read lock through its open cursor. With WAL the commit
// lands beside it; without WAL (the browser driver) COMMIT fails with
// "database is locked", which leaves SQLite inside the transaction until the
// driver rolls it back. On a one-connection pool a transaction left open
// would fail every later Begin.
func TestCommit_LeavesThePoolUsable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commit.db")
	ctx := t.Context()
	a, err := storage.Open(path)
	require.NoError(t, err)
	defer func() { _ = a.Close() }()
	b, err := storage.Open(path)
	require.NoError(t, err)
	defer func() { _ = b.Close() }()

	// More rows than one batch of the browser driver, so b's cursor is still
	// open after its first row.
	_, err = a.ExecContext(ctx, `CREATE TABLE t (n INTEGER)`)
	require.NoError(t, err)
	_, err = a.ExecContext(ctx, `WITH RECURSIVE s(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM s WHERE n < 600)
		INSERT INTO t SELECT n FROM s`)
	require.NoError(t, err)

	rows, err := b.QueryContext(ctx, `SELECT n FROM t`)
	require.NoError(t, err)
	require.True(t, rows.Next())

	tx, err := a.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `INSERT INTO t VALUES (0)`)
	require.NoError(t, err)
	commitErr := tx.Commit()
	require.NoError(t, rows.Close())

	want := 601
	if storage.DriverProfile().WAL {
		require.NoError(t, commitErr, "a WAL reader does not block a commit")
		want++
	} else {
		require.ErrorContains(t, commitErr, "database is locked")
	}

	tx, err = a.BeginTx(ctx, nil)
	require.NoError(t, err, "the failed transaction is over")
	_, err = tx.ExecContext(ctx, `INSERT INTO t VALUES (-1)`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var n int
	require.NoError(t, b.QueryRowContext(ctx, `SELECT COUNT(*) FROM t`).Scan(&n))
	assert.Equal(t, want, n, "a failed commit's row is discarded, the next transaction's row lands")
}
