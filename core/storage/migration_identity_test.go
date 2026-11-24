package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/storage"
)

func noop(context.Context, *sql.Tx) error { return nil }

// A list of migrations is a ledger only when each version is issued once,
// versions climb without a gap nobody declared, a retired version carries no
// work, and a step written in Go declares a revision.
func TestValidateMigrations(t *testing.T) {
	ok := []storage.Migration{
		{Version: 2, Description: "b", SQL: "SELECT 2"},
		{Version: 1, Description: "a", SQL: "SELECT 1"},
		{Version: 3, Retired: true, Description: "spent"},
		{Version: 4, Description: "go", Apply: noop, Revision: "1"},
	}
	ordered, err := storage.ValidateMigrations("t", ok)
	require.NoError(t, err)
	assert.Equal(t, 1, ordered[0].Version, "returned in version order")

	for name, tc := range map[string]struct {
		ms   []storage.Migration
		want string
	}{
		"one version twice":   {[]storage.Migration{{Version: 1, Description: "first"}, {Version: 1, Description: "second"}}, `version 1 is issued twice: "first" and "second"`},
		"a gap":               {[]storage.Migration{{Version: 1, Description: "a"}, {Version: 3, Description: "c"}}, "versions jump from 1 to 3"},
		"retired with work":   {[]storage.Migration{{Version: 1, Retired: true, SQL: "SELECT 1"}}, "is Retired and still carries work"},
		"Go without revision": {[]storage.Migration{{Version: 1, Description: "go", Apply: noop}}, `version 1 ("go") runs Go and declares no Revision`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := storage.ValidateMigrations("t", tc.ms)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// A migration's identity covers its name, its SQL and a Go step's declared
// revision.
func TestMigrationIdentity(t *testing.T) {
	m := storage.Migration{Version: 1, Description: "a", SQL: "SELECT 1"}
	edited := m
	edited.SQL = "SELECT 2"
	renamed := m
	renamed.Description = "b"
	assert.NotEqual(t, storage.MigrationIdentity(m), storage.MigrationIdentity(edited))
	assert.NotEqual(t, storage.MigrationIdentity(m), storage.MigrationIdentity(renamed))

	step := storage.Migration{Version: 2, Description: "go", Apply: noop, Revision: "1"}
	revised := step
	revised.Revision = "2"
	assert.NotEqual(t, storage.MigrationIdentity(step), storage.MigrationIdentity(revised))
}

// The ledger check adopts a row recorded before identities only under a name
// that version has carried, refuses any other name naming both, refuses a
// shipped version whose code no longer has its shipped identity, and leaves a
// version the code no longer carries alone.
func TestCheckLedger(t *testing.T) {
	code := []storage.Migration{
		{Version: 1, Description: "a", SQL: "SELECT 1"},
		{Version: 2, Retired: true, Description: "spent"},
		{Version: 3, Description: "c", SQL: "SELECT 3"},
	}
	shipped := map[int]storage.ShippedMigration{
		1: {Description: "a", Identity: storage.MigrationIdentity(code[0]), Names: []string{"an older wording of a", "a"}},
		3: {Description: "c", Identity: storage.MigrationIdentity(code[2]), Names: []string{"c"}},
	}
	adopt, err := storage.CheckLedger("t", code, []storage.AppliedMigration{
		{Version: 1, Description: "an older wording of a"},
		{Version: 2, Description: "whatever 2 was", Identity: "x"},
		{Version: 3, Description: "c"},
	}, shipped)
	require.NoError(t, err)
	require.Len(t, adopt, 2, "a row under a name its version has carried is adopted")

	// The reviewer's reproduction: the code's version 3 now stands for another
	// change, and the database recorded the old one without an identity.
	collided := []storage.Migration{code[0], code[1], {Version: 3, Description: "the agent host", SQL: "ALTER TABLE x ADD y"}}
	_, err = storage.CheckLedger("t", collided, []storage.AppliedMigration{{Version: 3, Description: "c"}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `version 3 is recorded as "c" and the code's version 3 is "the agent host"`)

	_, err = storage.CheckLedger("t", collided, nil, shipped)
	require.Error(t, err, "a shipped version whose code changed is refused before any row is read")
	assert.Contains(t, err.Error(), `version 3 ("c") shipped with identity`)

	_, err = storage.CheckLedger("t", code, []storage.AppliedMigration{{Version: 3, Description: "c", Identity: "y"}}, shipped)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a shipped migration was edited")
}

// End to end on SQLite: a ledger written before identities gains the column
// and adopts its rows once; afterwards a version reused under another name,
// or a shipped migration edited, refuses to migrate and changes nothing.
func TestMigrate_RefusesALedgerTheCodeDisagreesWith(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	db, err := storage.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()

	// A ledger as a released kapi left it: no identity column.
	_, err = db.ExecContext(ctx, `CREATE TABLE t_migrations (version INTEGER PRIMARY KEY, description TEXT NOT NULL, applied_at TEXT NOT NULL DEFAULT (datetime('now')))`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE a (x INTEGER)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO t_migrations (version, description) VALUES (1, 'create a')`)
	require.NoError(t, err)

	code := []storage.Migration{
		{Version: 1, Description: "create a", SQL: "CREATE TABLE IF NOT EXISTS a (x INTEGER)"},
		{Version: 2, Description: "create b", SQL: "CREATE TABLE b (y INTEGER)"},
	}
	require.NoError(t, storage.Migrate(db, "t_migrations", code))
	var desc, identity string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT description, identity FROM t_migrations WHERE version = 1`).Scan(&desc, &identity))
	assert.Equal(t, "create a", desc, "the recorded name is kept")
	assert.Equal(t, storage.MigrationIdentity(code[0]), identity)
	require.NoError(t, storage.Migrate(db, "t_migrations", code), "a second run agrees")

	collided := []storage.Migration{code[0], {Version: 2, Description: "create c", SQL: "CREATE TABLE c (z INTEGER)"}}
	err = storage.Migrate(db, "t_migrations", collided)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `version 2 is recorded as "create b" and the code's version 2 is "create c"`)

	edited := []storage.Migration{code[0], {Version: 2, Description: "create b", SQL: "CREATE TABLE b (y TEXT)"}}
	err = storage.Migrate(db, "t_migrations", edited)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a shipped migration was edited")

	// A row recorded before identities under a name its version never
	// carried is refused on the first run, and keeps its name.
	_, err = db.ExecContext(ctx, `INSERT INTO t_migrations (version, description) VALUES (3, 'create d')`)
	require.NoError(t, err)
	reused := []storage.Migration{code[0], code[1], {Version: 3, Description: "create e", SQL: "CREATE TABLE e (z INTEGER)"}}
	err = storage.Migrate(db, "t_migrations", reused)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `version 3 is recorded as "create d" and the code's version 3 is "create e"`)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT description, identity FROM t_migrations WHERE version = 3`).Scan(&desc, &identity))
	assert.Equal(t, "create d", desc)
	assert.Empty(t, identity, "nothing is adopted")

	twice := []storage.Migration{code[0], code[1], {Version: 2, Description: "create c"}}
	err = storage.Migrate(db, "t_migrations", twice)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `version 2 is issued twice: "create b" and "create c"`)
}

// In a test binary, a migration of a ledger the committed file holds must be
// listed there before it merges; a ledger the file does not hold (one a test
// writes for itself) is not asked.
func TestShippedCoverage(t *testing.T) {
	// make migration-ledger runs this package while recording; the first
	// assertions are about a run that is not.
	t.Setenv(storage.RecordLedgerEnv, "")
	ms := []storage.Migration{{Version: 1, Description: "a"}, {Version: 2, Retired: true}, {Version: 3, Description: "new"}}
	shipped := map[int]storage.ShippedMigration{1: {Description: "a"}}
	err := storage.ShippedCoverage("t", ms, shipped)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `3 ("new")`)
	assert.Contains(t, err.Error(), "make migration-ledger")
	require.NoError(t, storage.ShippedCoverage("t", ms, nil))
	t.Setenv(storage.RecordLedgerEnv, t.TempDir())
	require.NoError(t, storage.ShippedCoverage("t", ms, shipped), "recording lists it")
}

// A database a newer release migrated is refused before anything is read or
// written: the ledger one version ahead of the code refuses and names both
// versions, a ledger at the code's version opens, and one behind migrates.
// A version the code retired counts as known.
func TestMigrate_RefusesADatabaseANewerReleaseMigrated(t *testing.T) {
	code := []storage.Migration{
		{Version: 1, Description: "create a", SQL: "CREATE TABLE a (x INTEGER)"},
		{Version: 2, Retired: true},
		{Version: 3, Description: "create c", SQL: "CREATE TABLE c (z INTEGER)"},
	}
	open := func(t *testing.T) *storage.DB {
		t.Helper()
		db, err := storage.Open(filepath.Join(t.TempDir(), "s.db"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		return db
	}

	t.Run("behind migrates", func(t *testing.T) {
		db := open(t)
		require.NoError(t, storage.Migrate(db, "t_migrations", code[:1]))
		require.NoError(t, storage.Migrate(db, "t_migrations", code))
		var n int
		require.NoError(t, db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM c`).Scan(&n))
	})
	t.Run("equal opens", func(t *testing.T) {
		db := open(t)
		require.NoError(t, storage.Migrate(db, "t_migrations", code))
		require.NoError(t, storage.Migrate(db, "t_migrations", code))
	})
	t.Run("ahead refuses", func(t *testing.T) {
		db := open(t)
		newer := append(slices.Clone(code), storage.Migration{Version: 4, Description: "create d", SQL: "CREATE TABLE d (w INTEGER)"})
		require.NoError(t, storage.Migrate(db, "t_migrations", newer))

		err := storage.Migrate(db, "t_migrations", code)
		require.ErrorIs(t, err, storage.ErrNewerDatabase)
		assert.Contains(t, err.Error(), "the database is at version 4 and this build knows versions up to 3")
		assert.Contains(t, err.Error(), "at least as new as the database")
	})
}

// Check reads a ledger without migrating it: a missing ledger and one at the
// code's version pass, one a newer release migrated refuses, and nothing is
// written either way.
func TestCheck_RefusesANewerLedgerWithoutMigrating(t *testing.T) {
	code := []storage.Migration{{Version: 1, Description: "create a", SQL: "CREATE TABLE a (x INTEGER)"}}
	db, err := storage.Open(filepath.Join(t.TempDir(), "s.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, storage.Check(t.Context(), db, "t_migrations", code), "no ledger yet")
	var tables int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tables))
	assert.Zero(t, tables, "and nothing was created")

	require.NoError(t, storage.Migrate(db, "t_migrations", code))
	require.NoError(t, storage.Check(t.Context(), db, "t_migrations", code), "at the code's version")

	_, err = db.ExecContext(t.Context(), `INSERT INTO t_migrations (version, description, identity) VALUES (2, 'create b', 'newer')`)
	require.NoError(t, err)
	err = storage.Check(t.Context(), db, "t_migrations", code)
	require.ErrorIs(t, err, storage.ErrNewerDatabase)
	assert.Contains(t, err.Error(), "the database is at version 2 and this build knows versions up to 1")
}
