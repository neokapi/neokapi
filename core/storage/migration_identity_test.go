package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
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

// The ledger check adopts a row recorded before identities, refuses a version
// recorded under another name or identity naming both sides, and leaves a
// version the code no longer carries alone.
func TestCheckLedger(t *testing.T) {
	code := []storage.Migration{
		{Version: 1, Description: "a", SQL: "SELECT 1"},
		{Version: 2, Retired: true, Description: "spent"},
		{Version: 3, Description: "c", SQL: "SELECT 3"},
	}
	adopt, err := storage.CheckLedger("t", code, []storage.AppliedMigration{
		{Version: 1, Description: "an older wording of a"},
		{Version: 2, Description: "whatever 2 was", Identity: "x"},
		{Version: 3, Description: "c", Identity: storage.MigrationIdentity(code[2])},
	})
	require.NoError(t, err)
	require.Len(t, adopt, 1)
	assert.Equal(t, 1, adopt[0].Version, "a row without an identity is adopted as it is")

	_, err = storage.CheckLedger("t", code, []storage.AppliedMigration{{Version: 3, Description: "something else", Identity: "y"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `version 3 is recorded as "something else" and the code's version 3 is "c"`)

	_, err = storage.CheckLedger("t", code, []storage.AppliedMigration{{Version: 3, Description: "c", Identity: "y"}})
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
	_, err = db.ExecContext(ctx, `INSERT INTO t_migrations (version, description) VALUES (1, 'an older wording')`)
	require.NoError(t, err)

	code := []storage.Migration{
		{Version: 1, Description: "create a", SQL: "CREATE TABLE IF NOT EXISTS a (x INTEGER)"},
		{Version: 2, Description: "create b", SQL: "CREATE TABLE b (y INTEGER)"},
	}
	require.NoError(t, storage.Migrate(db, "t_migrations", code))
	var desc, identity string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT description, identity FROM t_migrations WHERE version = 1`).Scan(&desc, &identity))
	assert.Equal(t, "create a", desc, "adopted under the code's name")
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

	twice := []storage.Migration{code[0], code[1], {Version: 2, Description: "create c"}}
	err = storage.Migrate(db, "t_migrations", twice)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `version 2 is issued twice: "create b" and "create c"`)
}
