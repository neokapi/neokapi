package storage_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_InMemory(t *testing.T) {
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Verify pragmas were applied.
	var journalMode string
	err = db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	// In-memory databases use "memory" journal mode regardless.
	assert.Contains(t, []string{"wal", "memory"}, journalMode)

	// Verify foreign keys are on.
	var fk int
	err = db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk)
	require.NoError(t, err)
	assert.Equal(t, 1, fk)
}

// TestOpen_ForeignKeysAcrossPooledConnections opens a file-backed DB (which uses
// a multi-connection pool) and asserts that foreign_keys is ON on many
// connections, not just the one applyPragmas happened to configure. Before the
// fix, the per-connection pragma rode a single startup Exec and most pooled
// connections defaulted to foreign_keys=OFF.
func TestOpen_ForeignKeysAcrossPooledConnections(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fk.db")

	db, err := storage.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Force several distinct connections to be live at once by holding open
	// transactions in parallel, then check foreign_keys on each.
	const conns = 8
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		gate = make(chan struct{})
	)
	results := make([]int, conns)
	errs := make([]error, conns)

	for i := range conns {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				mu.Lock()
				errs[idx] = err
				mu.Unlock()
				return
			}
			defer func() { _ = tx.Rollback() }()

			var fk int
			if err := tx.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
				mu.Lock()
				errs[idx] = err
				mu.Unlock()
				return
			}
			results[idx] = fk
			// Block so all connections stay checked out simultaneously,
			// guaranteeing distinct underlying pool connections.
			<-gate
		}(i)
	}

	// Give goroutines time to each grab a connection, then release them.
	// A small busy loop keeps the test deterministic without a fixed sleep.
	require.Eventually(t, func() bool {
		return db.Stats().InUse >= conns
	}, 5*time.Second, 5*time.Millisecond)
	close(gate)
	wg.Wait()

	for i := range conns {
		require.NoErrorf(t, errs[i], "connection %d errored", i)
		assert.Equalf(t, 1, results[i], "foreign_keys must be ON on pooled connection %d", i)
	}
}

// TestOpen_CascadeUnderConcurrency exercises ON DELETE CASCADE across many
// concurrent connections on a file-backed DB. If foreign_keys were OFF on any
// pooled connection, the cascade would silently no-op and leave orphan rows.
func TestOpen_CascadeUnderConcurrency(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cascade.db")

	db, err := storage.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	_, err = db.ExecContext(ctx, `CREATE TABLE parent (id INTEGER PRIMARY KEY)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE child (
		id INTEGER PRIMARY KEY,
		parent_id INTEGER NOT NULL REFERENCES parent(id) ON DELETE CASCADE
	)`)
	require.NoError(t, err)

	const n = 50
	for i := 1; i <= n; i++ {
		_, err = db.ExecContext(ctx, "INSERT INTO parent (id) VALUES (?)", i)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "INSERT INTO child (id, parent_id) VALUES (?, ?)", i, i)
		require.NoError(t, err)
	}

	// Delete every parent concurrently to spread work across pool connections.
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := db.ExecContext(ctx, "DELETE FROM parent WHERE id = ?", id)
			errs[id-1] = err
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		require.NoErrorf(t, e, "delete parent %d", i+1)
	}

	// All children must have cascaded away. A surviving child proves a
	// foreign_keys=OFF connection silently skipped the cascade.
	var childCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM child").Scan(&childCount)
	require.NoError(t, err)
	assert.Equal(t, 0, childCount, "ON DELETE CASCADE left orphan child rows — foreign_keys was OFF on some pooled connection")
}

func TestMigrate(t *testing.T) {
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	migrations := []storage.Migration{
		{
			Version:     1,
			Description: "create users table",
			SQL:         "CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT NOT NULL)",
		},
		{
			Version:     2,
			Description: "add email column",
			SQL:         "ALTER TABLE users ADD COLUMN email TEXT",
		},
	}

	// Apply all migrations.
	err = storage.Migrate(db, migrations)
	require.NoError(t, err)

	ctx := context.Background()

	// Insert a row to verify schema.
	_, err = db.ExecContext(ctx, "INSERT INTO users (id, name, email) VALUES ('u1', 'Alice', 'alice@example.com')")
	require.NoError(t, err)

	// Re-running migrations should be idempotent.
	err = storage.Migrate(db, migrations)
	require.NoError(t, err)

	// Verify migration was recorded.
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestMigrate_Incremental(t *testing.T) {
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Apply first migration.
	migrations1 := []storage.Migration{
		{Version: 1, Description: "v1", SQL: "CREATE TABLE t1 (id TEXT PRIMARY KEY)"},
	}
	err = storage.Migrate(db, migrations1)
	require.NoError(t, err)

	// Add a second migration.
	migrations2 := []storage.Migration{
		{Version: 1, Description: "v1", SQL: "CREATE TABLE t1 (id TEXT PRIMARY KEY)"},
		{Version: 2, Description: "v2", SQL: "CREATE TABLE t2 (id TEXT PRIMARY KEY)"},
	}
	err = storage.Migrate(db, migrations2)
	require.NoError(t, err)

	// Both tables should exist.
	ctx := context.Background()
	_, err = db.ExecContext(ctx, "INSERT INTO t1 (id) VALUES ('a')")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO t2 (id) VALUES ('b')")
	require.NoError(t, err)
}
