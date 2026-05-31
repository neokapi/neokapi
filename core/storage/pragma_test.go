package storage_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestForeignKeys_AllPooledConnections opens a file-backed DB (which uses the
// real 25-connection pool, unlike :memory: which is pinned to one connection)
// and asserts that PRAGMA foreign_keys reports ON across many connections.
//
// Regression test for the audit finding that foreign_keys was applied via a
// single startup Exec on one pooled connection, leaving the rest with
// foreign_keys OFF — silently breaking ON DELETE CASCADE.
func TestForeignKeys_AllPooledConnections(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fk.db")
	db, err := storage.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Force the pool to spread across multiple distinct connections. Each
	// goroutine grabs a connection and holds it open (inside Conn) until every
	// goroutine has one, guaranteeing the pool mints `conns` separate
	// connections rather than reusing a single warm one. A per-connection pragma
	// set only via one startup Exec would surface foreign_keys=0 on all but one.
	const conns = 16
	db.SetMaxOpenConns(conns)

	// Seed the DB so the WAL/-shm files exist before we fan out: a brand-new DB
	// file otherwise races on first-write WAL initialization when many fresh
	// connections open at once. Real callers always migrate/write before
	// concurrent reads, so this matches production.
	_, err = db.Exec(`CREATE TABLE seed (id INTEGER PRIMARY KEY)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO seed (id) VALUES (1)`)
	require.NoError(t, err)

	var (
		wg      sync.WaitGroup
		ready   sync.WaitGroup // signals "I hold a connection"
		release = make(chan struct{})
		mu      sync.Mutex
		results []int
		errsAny error
	)
	ready.Add(conns)
	ctx := t.Context()
	for i := 0; i < conns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := db.Conn(ctx)
			if err != nil {
				mu.Lock()
				if errsAny == nil {
					errsAny = err
				}
				mu.Unlock()
				ready.Done()
				return
			}
			defer conn.Close()
			ready.Done()
			// Hold this connection until all peers have one too, so the pool is
			// forced to hand out distinct connections.
			<-release
			var fk int
			if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
				mu.Lock()
				if errsAny == nil {
					errsAny = err
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			results = append(results, fk)
			mu.Unlock()
		}()
	}
	ready.Wait()
	close(release)
	wg.Wait()

	require.NoError(t, errsAny)
	require.Len(t, results, conns)
	for i, fk := range results {
		assert.Equalf(t, 1, fk, "connection %d observed foreign_keys=%d, want 1", i, fk)
	}
}

// TestCascadeDelete_UnderConcurrency proves the user-visible consequence: with
// foreign_keys ON on every pooled connection, ON DELETE CASCADE actually
// deletes child rows even when many deletes run concurrently across the pool.
// Before the fix, a fraction of connections had foreign_keys OFF and left
// orphaned child rows behind.
func TestCascadeDelete_UnderConcurrency(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cascade.db")
	db, err := storage.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	db.SetMaxOpenConns(16)

	_, err = db.Exec(`CREATE TABLE parent (id INTEGER PRIMARY KEY)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE child (
		id     INTEGER PRIMARY KEY,
		parent INTEGER NOT NULL REFERENCES parent(id) ON DELETE CASCADE
	)`)
	require.NoError(t, err)

	const n = 200
	for i := 1; i <= n; i++ {
		_, err = db.Exec(`INSERT INTO parent (id) VALUES (?)`, i)
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO child (id, parent) VALUES (?, ?)`, i, i)
		require.NoError(t, err)
	}

	// Delete every parent concurrently. Each DELETE may land on a different
	// pooled connection; the cascade must fire on all of them.
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, e := db.Exec(`DELETE FROM parent WHERE id = ?`, id)
			errs[id-1] = e
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		require.NoErrorf(t, e, "delete parent %d", i+1)
	}

	// No orphaned children may remain — the whole point of the cascade.
	var orphans int
	err = db.QueryRow(`SELECT COUNT(*) FROM child`).Scan(&orphans)
	require.NoError(t, err)
	assert.Zerof(t, orphans, "%d child rows orphaned — cascade did not fire on some connections", orphans)

	var parents int
	err = db.QueryRow(`SELECT COUNT(*) FROM parent`).Scan(&parents)
	require.NoError(t, err)
	assert.Zero(t, parents)
}

// TestForeignKeys_FileBackedSingleQuery is a focused, deterministic check that a
// freshly opened file-backed DB reports foreign_keys ON — proving the DSN, not
// just the one-shot applyPragmas Exec, carries it. It also exercises a DSN that
// already contains a query string, so the sqliteDSN "&"-separator path is hit.
func TestForeignKeys_FileBackedSingleQuery(t *testing.T) {
	cases := []struct {
		name string
		dsn  func(dir string) string
	}{
		{"plain", func(dir string) string { return filepath.Join(dir, "fk.db") }},
		{"with-existing-query", func(dir string) string {
			return filepath.Join(dir, "fk.db") + "?cache=shared"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := storage.Open(tc.dsn(t.TempDir()))
			require.NoError(t, err)
			defer db.Close()

			var fk int
			require.NoError(t, db.QueryRow("PRAGMA foreign_keys").Scan(&fk))
			assert.Equal(t, 1, fk)
		})
	}
}
