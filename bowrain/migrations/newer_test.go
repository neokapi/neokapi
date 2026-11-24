//go:build integration

package migrations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
	corestorage "github.com/neokapi/neokapi/core/storage"
)

// --migrate-only refuses before it migrates anything: with only the last
// subsystem's ledger ahead, no earlier subsystem is touched.
func TestApply_RefusesBeforeMigratingAnySubsystem(t *testing.T) {
	db := freshDatabase(t)
	all := migrations.All()
	last := all[len(all)-1]
	known := 0
	for _, m := range last.Migrations {
		known = max(known, m.Version)
	}
	_, err := db.Exec(`CREATE TABLE ` + last.Table + ` (version INTEGER PRIMARY KEY, description TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), identity TEXT NOT NULL DEFAULT '')`)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO "+last.Table+" (version, description, identity) VALUES ($1, 'from a newer server', 'newer')", known+1)
	require.NoError(t, err)

	err = migrations.Apply(db, nil)
	require.ErrorIs(t, err, corestorage.ErrNewerDatabase)
	for _, s := range all[:len(all)-1] {
		var exists bool
		require.NoError(t, db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, s.Table).Scan(&exists))
		assert.False(t, exists, "the %s ledger was not created", s.Name)
	}
	var tables int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema()`).Scan(&tables))
	assert.Equal(t, 1, tables, "nothing but the newer ledger is in the database")
}

// A database a newer server migrated is refused, whichever subsystem moved
// ahead, both by Apply (what --migrate-only runs) and by the check a server
// runs at start for the schemas it migrates later. An empty database
// migrates, and one at this build's versions starts.
func TestApply_RefusesADatabaseANewerServerMigrated(t *testing.T) {
	db := freshDatabase(t)
	require.NoError(t, migrations.Apply(db, nil), "an empty database migrates")
	require.NoError(t, migrations.Apply(db, nil), "a database at this build's versions starts")

	for _, s := range migrations.All() {
		t.Run(s.Name, func(t *testing.T) {
			known := 0
			for _, m := range s.Migrations {
				known = max(known, m.Version)
			}
			_, err := db.Exec("INSERT INTO "+s.Table+" (version, description, identity) VALUES ($1, 'from a newer server', 'newer')", known+1)
			require.NoError(t, err)

			err = migrations.Apply(db, nil)
			require.ErrorIs(t, err, corestorage.ErrNewerDatabase)
			assert.Contains(t, err.Error(), "check the "+s.Name+" ledger")
			assert.Contains(t, err.Error(), "An older version cannot be started against this database")
			err = storage.CheckPostgresLedgerNS(t.Context(), db, s.Table, s.Migrations)
			require.ErrorIs(t, err, corestorage.ErrNewerDatabase)
			err = migrations.RefuseNewer(t.Context(), db)
			require.ErrorIs(t, err, corestorage.ErrNewerDatabase, "the pre-flight a server and a worker run at start")
			assert.Contains(t, err.Error(), "check the "+s.Name+" ledger")

			_, err = db.Exec("DELETE FROM "+s.Table+" WHERE version = $1", known+1)
			require.NoError(t, err)
			require.NoError(t, migrations.RefuseNewer(t.Context(), db))
			require.NoError(t, migrations.Apply(db, nil), "back at this build's versions it starts")
		})
	}
}
