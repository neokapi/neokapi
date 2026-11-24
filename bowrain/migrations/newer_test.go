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
			assert.Contains(t, err.Error(), "migrate "+s.Name)
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
