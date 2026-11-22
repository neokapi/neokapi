//go:build integration

package migrations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
)

// databaseBeforeStoreVersion is a fresh database with every subsystem
// migrated, except the store's migrations from version on.
func databaseBeforeStoreVersion(t *testing.T, version int) *storage.PgDB {
	t.Helper()
	db := freshDatabase(t)
	for _, system := range migrations.All() {
		versions := system.Migrations
		if system.Name == "store" {
			for i, m := range versions {
				if m.Version >= version {
					versions = versions[:i]
					break
				}
			}
		}
		require.NoError(t, storage.MigratePostgresNS(db, system.Table, versions))
	}
	return db
}

// Flows stored before the revision column each get a token of their own, and
// adding it never rewrites the table: the table keeps its storage file.
func TestUpgradeGivesEveryStoredFlowItsOwnRevision(t *testing.T) {
	db := databaseBeforeStoreVersion(t, 58)
	ctx := t.Context()
	_, err := db.ExecContext(ctx, `INSERT INTO projects (id, name) VALUES ('p-1', 'Flows')`)
	require.NoError(t, err)
	for _, id := range []string{"a", "b", "c"} {
		_, err := db.ExecContext(ctx,
			`INSERT INTO flow_definitions (id, project_id, name) VALUES ($1, 'p-1', $1)`, id)
		require.NoError(t, err)
	}
	var before uint32
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT relfilenode FROM pg_class WHERE relname = 'flow_definitions'`).Scan(&before))

	require.NoError(t, migrations.Apply(db, nil))

	var after uint32
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT relfilenode FROM pg_class WHERE relname = 'flow_definitions'`).Scan(&after))
	assert.Equal(t, before, after, "adding the revision rewrote flow_definitions")

	var rows, distinct, missing int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*), count(DISTINCT revision), count(*) FILTER (WHERE revision IS NULL OR revision = '')
		 FROM flow_definitions`).Scan(&rows, &distinct, &missing))
	assert.Equal(t, 3, rows)
	assert.Equal(t, 3, distinct, "two stored flows share a revision")
	assert.Zero(t, missing)

	_, err = db.ExecContext(ctx, `INSERT INTO flow_definitions (id, project_id, name) VALUES ('d', 'p-1', 'd')`)
	require.NoError(t, err, "a row inserted without a revision gets one by default")
}
