package storage_test

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A migration whose own statements violate a UNIQUE constraint is a real fault,
// not a lost race with a concurrent applier. It must surface immediately with
// the failing migration named — not be retried for the bounded lost-race window
// and then reported as a timeout.
func TestMigrate_UniqueViolationInMigrationSQLFailsFast(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer db.Close()

	migrations := []storage.Migration{
		{
			Version:     1,
			Description: "create table",
			SQL:         `CREATE TABLE widget (id INTEGER PRIMARY KEY, sku TEXT NOT NULL UNIQUE)`,
		},
		{
			Version:     2,
			Description: "backfill colliding rows",
			SQL: `INSERT INTO widget (sku) VALUES ('a');
			      INSERT INTO widget (sku) VALUES ('a');`,
		},
	}

	start := time.Now()
	err = storage.Migrate(db, "testns", migrations)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "apply migration 2")
	assert.Contains(t, err.Error(), "UNIQUE constraint failed")
	// The lost-race retry window is 15s; a genuine violation must not wait it out.
	assert.Less(t, elapsed, 5*time.Second, "migration failure was retried instead of surfacing")
}

// Concurrent openers of the same fresh database (converge workers all opening
// the project TM) must each get a migrated DB, with every migration applied
// exactly once.
func TestMigrate_ConcurrentOpenersOfSameDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	migrations := []storage.Migration{
		{Version: 1, Description: "create", SQL: `CREATE TABLE widget (id INTEGER PRIMARY KEY)`},
		{Version: 2, Description: "extend", SQL: `ALTER TABLE widget ADD COLUMN sku TEXT`},
	}

	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := storage.Open(path)
			if err != nil {
				errs[i] = err
				return
			}
			defer db.Close()
			errs[i] = storage.Migrate(db, "testns", migrations)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "worker %d", i)
	}

	db, err := storage.Open(path)
	require.NoError(t, err)
	defer db.Close()

	var applied int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM testns`).Scan(&applied))
	assert.Equal(t, len(migrations), applied, "each migration recorded exactly once")
}

// Open serializes per database file, not process-wide: an opener parked on one
// busy database must not block opens of an unrelated one.
func TestOpen_LocksArePerPath(t *testing.T) {
	dir := t.TempDir()
	a, err := storage.Open(filepath.Join(dir, "a.db"))
	require.NoError(t, err)
	defer a.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 20 {
			db, err := storage.Open(filepath.Join(dir, "b.db"))
			if err != nil {
				return
			}
			db.Close()
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("opens of an unrelated database did not make progress")
	}
}
