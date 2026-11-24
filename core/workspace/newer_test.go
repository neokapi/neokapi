package workspace_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/workspace"
)

// registryLedgers lists the migration ledgers in a workspace's registry file:
// the tables shaped as the runner writes them.
func registryLedgers(t *testing.T, path string) []string {
	t.Helper()
	db, err := storage.Open(path)
	require.NoError(t, err)
	defer db.Close()
	rows, err := db.QueryContext(t.Context(), `
		SELECT m.name FROM sqlite_master m
		WHERE m.type = 'table'
		  AND EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'version')
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

// advanceLedger records a version one above the newest the ledger holds, as
// a newer kapi's migration would.
func advanceLedger(t *testing.T, path, ledger string) {
	t.Helper()
	db, err := storage.Open(path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.ExecContext(t.Context(),
		"INSERT INTO "+ledger+" (version, description, identity) SELECT COALESCE(MAX(version), 0) + 1, 'from a newer kapi', 'newer' FROM "+ledger)
	require.NoError(t, err)
}

func openAndClose(t *testing.T, root string) {
	t.Helper()
	ws, err := workspace.OpenLocal(t.Context(), root)
	require.NoError(t, err)
	require.NoError(t, ws.Close())
}

// A workspace a newer kapi migrated is refused, whichever ledger of its
// registry moved ahead, and whether it is opened for writing or only for
// reading: every project's terms, voice and decisions live there.
func TestOpen_RefusesAWorkspaceANewerKapiMigrated(t *testing.T) {
	first := t.TempDir()
	openAndClose(t, first)
	openAndClose(t, first) // at this build's versions it opens again
	ledgers := registryLedgers(t, workspace.Local(first).RegistryPath())
	require.NotEmpty(t, ledgers)

	for _, ledger := range ledgers {
		t.Run(ledger, func(t *testing.T) {
			root := t.TempDir()
			openAndClose(t, root)
			advanceLedger(t, workspace.Local(root).RegistryPath(), ledger)

			_, err := workspace.OpenLocal(t.Context(), root)
			require.ErrorIs(t, err, storage.ErrNewerDatabase)
		})
	}

	t.Run("read-only", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("a directory's permissions do not stop the superuser writing")
		}
		root := t.TempDir()
		openAndClose(t, root)
		advanceLedger(t, workspace.Local(root).RegistryPath(), "workspace_registry_migrations")
		require.NoError(t, os.Chmod(root, 0o555))
		t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

		_, err := workspace.OpenLocal(t.Context(), root)
		require.ErrorIs(t, err, storage.ErrNewerDatabase)
	})
}
