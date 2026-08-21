//go:build integration

package migrations_test

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Renaming a subsystem renames its ledger table, so its baseline replays
// against an empty ledger on a database that already holds the old tables.
// CREATE TABLE IF NOT EXISTS then builds a second, empty table beside the
// populated one and nothing errors — the shape that stranded
// workspaces.brand_voice_profile_id and took production down.
//
// So a baseline that carries a rename is asserted against a database that
// predates it, not only against an empty one. These live here rather than
// beside their subsystem because bowrain/jobs declares a pgtest template: its
// tests are handed a database with every migration already applied, where a
// baseline never replays and this could not be observed. freshDatabase builds
// a real empty one.
func TestContextScanBaselineRenamesTheJobsTable(t *testing.T) {
	db := freshDatabase(t)
	ctx := t.Context()

	_, err := db.ExecContext(ctx, `
		CREATE TABLE brand_scan_jobs (
			id             TEXT PRIMARY KEY,
			workspace_id   TEXT NOT NULL DEFAULT '',
			workspace_slug TEXT NOT NULL DEFAULT '',
			status         TEXT NOT NULL DEFAULT 'queued',
			progress       INTEGER NOT NULL DEFAULT 0,
			message        TEXT NOT NULL DEFAULT '',
			request        JSONB NOT NULL DEFAULT '{}'::jsonb,
			result         JSONB,
			error          TEXT NOT NULL DEFAULT '',
			attempts       INTEGER NOT NULL DEFAULT 0,
			claim_epoch    BIGINT NOT NULL DEFAULT 0,
			tokens_used    INTEGER NOT NULL DEFAULT 0,
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX idx_brand_scan_jobs_workspace ON brand_scan_jobs(workspace_slug, created_at DESC);
		CREATE INDEX idx_brand_scan_jobs_status ON brand_scan_jobs(status);
		INSERT INTO brand_scan_jobs (id, workspace_slug, status) VALUES ('scan-1', 'acme', 'completed');`)
	require.NoError(t, err)

	require.NoError(t, storage.MigratePostgresNS(db, "context_scan_schema_migrations", jobs.ContextScanMigrations))

	var status string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT status FROM context_scan_jobs WHERE id = 'scan-1'`).Scan(&status),
		"the baseline must rename the table an existing database still carries")
	assert.Equal(t, "completed", status, "the rename must carry the rows, not replace them")

	var n int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = 'brand_scan_jobs'`).Scan(&n))
	assert.Zero(t, n, "a renamed table must move, not be duplicated")

	// RENAME TO carries the rows but not the index names, so a second index
	// over the same columns is what silently accumulates.
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT count(*) FROM pg_indexes
		WHERE tablename = 'context_scan_jobs' AND indexname LIKE 'idx_brand_scan%'`).Scan(&n))
	assert.Zero(t, n, "no index may keep the former name")

	// The baseline is re-applied by design, so the guard must make a second
	// pass a no-op rather than an error.
	require.NoError(t, storage.MigratePostgresNS(db, "context_scan_schema_migrations", jobs.ContextScanMigrations))
}

// The same baseline against an empty database: the CREATE names the table
// directly, so the rename must not fire and must not fail.
func TestContextScanBaselineOnAnEmptyDatabase(t *testing.T) {
	db := freshDatabase(t)

	require.NoError(t, storage.MigratePostgresNS(db, "context_scan_schema_migrations", jobs.ContextScanMigrations))

	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = 'context_scan_jobs'`).Scan(&n))
	assert.Equal(t, 1, n)
}
