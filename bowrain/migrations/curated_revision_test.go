//go:build integration

package migrations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
)

// databaseBefore is a fresh database with every subsystem migrated, except
// the named subsystem's migrations from version on.
func databaseBefore(t *testing.T, subsystem string, version int) *storage.PgDB {
	t.Helper()
	db := freshDatabase(t)
	for _, system := range migrations.All() {
		versions := system.Migrations
		if system.Name == subsystem {
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

func distinctRevisions(t *testing.T, db *storage.PgDB, table string) (rows, distinct, missing int) {
	t.Helper()
	require.NoError(t, db.QueryRowContext(t.Context(),
		`SELECT count(*), count(DISTINCT revision), count(*) FILTER (WHERE revision IS NULL OR revision = '')
		 FROM `+table).Scan(&rows, &distinct, &missing))
	return rows, distinct, missing
}

// Content-memory entries stored before the revision column each get a token
// of their own, and adding it never rewrites the table.
func TestUpgradeGivesEveryMemoryEntryItsOwnRevision(t *testing.T) {
	db := databaseBefore(t, "memory", 8)
	ctx := t.Context()
	for _, id := range []string{"a", "b", "c"} {
		_, err := db.ExecContext(ctx, `INSERT INTO tm_entries (workspace_id, id) VALUES ('ws', $1)`, id)
		require.NoError(t, err)
	}
	var before uint32
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT relfilenode FROM pg_class WHERE relname = 'tm_entries'`).Scan(&before))

	require.NoError(t, migrations.Apply(db, nil))

	var after uint32
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT relfilenode FROM pg_class WHERE relname = 'tm_entries'`).Scan(&after))
	assert.Equal(t, before, after, "adding the revision rewrote tm_entries")
	rows, distinct, missing := distinctRevisions(t, db, "tm_entries")
	assert.Equal(t, 3, rows)
	assert.Equal(t, 3, distinct, "two stored entries share a revision")
	assert.Zero(t, missing)
}

// Markets stored before the knowledge baseline carried a revision each get
// one of their own when the baseline is applied again. The table is a few rows
// per workspace, so the baseline adds the column with its default in one step.
func TestUpgradeGivesEveryMarketItsOwnRevision(t *testing.T) {
	db := freshDatabase(t)
	require.NoError(t, migrations.Apply(db, nil))
	ctx := t.Context()
	// The schema an earlier baseline built: no revision column, and the
	// ledger at the version before this one.
	_, err := db.ExecContext(ctx, `ALTER TABLE kg_markets DROP COLUMN revision`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM knowledge_schema_migrations WHERE version = 6`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO knowledge_schema_migrations (version, description) VALUES (5, 'knowledge graph baseline')`)
	require.NoError(t, err)
	for _, id := range []string{"a", "b", "c"} {
		_, err := db.ExecContext(ctx,
			`INSERT INTO kg_markets (workspace_id, id, name) VALUES ('ws', $1, $1)`, id)
		require.NoError(t, err)
	}

	require.NoError(t, migrations.Apply(db, nil))

	rows, distinct, missing := distinctRevisions(t, db, "kg_markets")
	assert.Equal(t, 3, rows)
	assert.Equal(t, 3, distinct, "two stored markets share a revision")
	assert.Zero(t, missing)
}

// Voice profiles stored before the revision column each get one of their own.
// Profiles are a few rows per workspace, so the column arrives with its default
// in one step.
func TestUpgradeGivesEveryVoiceProfileItsOwnRevision(t *testing.T) {
	db := databaseBefore(t, "voice", 8)
	ctx := t.Context()
	for _, id := range []string{"a", "b", "c"} {
		_, err := db.ExecContext(ctx,
			`INSERT INTO voice_profiles (id, workspace_id, name) VALUES ($1, 'ws', $1)`, id)
		require.NoError(t, err)
	}

	require.NoError(t, migrations.Apply(db, nil))

	rows, distinct, missing := distinctRevisions(t, db, "voice_profiles")
	assert.Equal(t, 3, rows)
	assert.Equal(t, 3, distinct, "two stored profiles share a revision")
	assert.Zero(t, missing)
}
