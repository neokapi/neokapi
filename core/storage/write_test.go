package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gatedDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.OpenWith(filepath.Join(t.TempDir(), "gated.db"), storage.Options{
		ImmediateTx: true, SerializeWrites: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.ExecContext(t.Context(), `CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`)
	require.NoError(t, err)
	return db
}

// TestGatedDB_TransactionHoldsThePermitUntilItEnds is the session contract in
// miniature: a block-store session is one transaction over a whole
// purge-and-refill, and every other writer in the process waits for it.
func TestGatedDB_TransactionHoldsThePermitUntilItEnds(t *testing.T) {
	db := gatedDB(t)

	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)

	written := make(chan error, 1)
	go func() {
		_, err := db.ExecContext(context.Background(), `INSERT INTO t (v) VALUES ('outside')`)
		written <- err
	}()

	select {
	case err := <-written:
		t.Fatalf("a write landed while a transaction held the permit: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	_, err = tx.ExecContext(t.Context(), `INSERT INTO t (v) VALUES ('inside')`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	select {
	case err := <-written:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the queued write never ran after the transaction ended")
	}

	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM t`).Scan(&n))
	assert.Equal(t, 2, n)
}

// TestGatedDB_ReentrantWriteIsReportedNotHung: writing on the handle from the
// goroutine that holds its transaction is a deadlock. It surfaces as an error
// on the offending call, not as a stopped process.
func TestGatedDB_ReentrantWriteIsReportedNotHung(t *testing.T) {
	db := gatedDB(t)

	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	done := make(chan error, 1)
	go func() {
		_, err := db.ExecContext(context.Background(), `INSERT INTO t (v) VALUES ('nested')`)
		done <- err
	}()
	// The goroutine above is a DIFFERENT goroutine, so it legitimately queues.
	// The reentrant case is this one:
	_, err = db.ExecContext(t.Context(), `INSERT INTO t (v) VALUES ('reentrant')`)
	require.ErrorIs(t, err, storage.ErrWriteGateReentrant)

	require.NoError(t, tx.Rollback())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the queued write never ran")
	}
}

// TestGatedDB_ReadsAreNotGated: under WAL a reader neither blocks nor waits for
// a writer, so gating reads would only add a queue for nothing.
func TestGatedDB_ReadsAreNotGated(t *testing.T) {
	db := gatedDB(t)
	_, err := db.ExecContext(t.Context(), `INSERT INTO t (v) VALUES ('seed')`)
	require.NoError(t, err)

	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	read := make(chan error, 1)
	go func() {
		var n int
		read <- db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM t`).Scan(&n)
	}()
	select {
	case err := <-read:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("a read waited for a write transaction")
	}
}

// TestGatedDB_RollbackAfterCommitReleasesOnce guards the ordinary
// `defer tx.Rollback()` shape: the permit is returned exactly once, or the next
// writer waits forever.
func TestGatedDB_RollbackAfterCommitReleasesOnce(t *testing.T) {
	db := gatedDB(t)

	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(t.Context(), `INSERT INTO t (v) VALUES ('once')`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	_ = tx.Rollback() // the deferred call every caller writes

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, `INSERT INTO t (v) VALUES ('after')`)
	require.NoError(t, err, "the permit was not returned, or was returned twice")
}

// TestUngatedOpen_IsUnchanged: every standalone store — a named content memory,
// a KPZ overlay, the brand database — still opens with SQLite's own discipline
// and no queue.
func TestUngatedOpen_IsUnchanged(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "plain.db"))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.ExecContext(t.Context(), `CREATE TABLE t (id INTEGER PRIMARY KEY)`)
	require.NoError(t, err)

	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	// With no gate this second write is merely a second connection, and lands.
	done := make(chan error, 1)
	go func() {
		_, err := db.ExecContext(context.Background(), `INSERT INTO t (id) VALUES (1)`)
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("an ungated handle queued a write behind a transaction")
	}
}
