package projectdb_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/storage"
)

// ledgersIn lists the migration ledgers in a SQLite file: the tables shaped
// as the runner writes them (version, description, identity).
func ledgersIn(t *testing.T, path string) []string {
	t.Helper()
	db, err := storage.Open(path)
	require.NoError(t, err)
	defer db.Close()
	rows, err := db.QueryContext(t.Context(), `
		SELECT m.name FROM sqlite_master m
		WHERE m.type = 'table'
		  AND EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'version')
		  AND EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'description')
		  AND EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'identity')
		ORDER BY m.name`)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		out = append(out, name)
	}
	require.NoError(t, rows.Err())
	return out
}

// advance records, in ledger, a version one above the newest it holds, as a
// newer kapi's migration would.
func advance(t *testing.T, path, ledger string) int {
	t.Helper()
	db, err := storage.Open(path)
	require.NoError(t, err)
	defer db.Close()
	var newest int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT COALESCE(MAX(version), 0) FROM "+ledger).Scan(&newest))
	_, err = db.ExecContext(t.Context(), "INSERT INTO "+ledger+" (version, description, identity) VALUES (?, 'from a newer kapi', 'newer')", newest+1)
	require.NoError(t, err)
	return newest + 1
}

// A project store a newer kapi migrated is refused, whichever of its ledgers
// moved ahead: this kapi neither reads nor writes a person's data in a shape
// it does not know. Reopened at its own versions, the store opens.
func TestOpen_RefusesAStoreANewerKapiMigrated(t *testing.T) {
	first := project.LayoutAt(t.TempDir())
	db, err := projectdb.Open(t.Context(), first)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	ledgers := ledgersIn(t, first.StorePath())
	require.NotEmpty(t, ledgers)

	reopened, err := projectdb.Open(t.Context(), first)
	require.NoError(t, err, "a store at this build's versions opens")
	require.NoError(t, reopened.Close())

	for _, ledger := range ledgers {
		t.Run(ledger, func(t *testing.T) {
			layout := project.LayoutAt(t.TempDir())
			db, err := projectdb.Open(t.Context(), layout)
			require.NoError(t, err)
			require.NoError(t, db.Close())
			version := advance(t, layout.StorePath(), ledger)

			_, err = projectdb.Open(t.Context(), layout)
			require.ErrorIs(t, err, storage.ErrNewerDatabase)
			assert.Contains(t, err.Error(), fmt.Sprintf("the database is at version %d", version))
		})
	}
}
