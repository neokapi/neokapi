//go:build integration

package migrations_test

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
)

// updateGolden regenerates testdata/schema.golden from the current migration
// set. Run it deliberately, and read the diff — the golden is the record of
// what production's schema actually is, so an unexamined regeneration turns
// the guard into a rubber stamp.
//
//	go test -tags integration ./migrations/ -update
var updateGolden = flag.Bool("update", false, "rewrite testdata/schema.golden from the current migrations")

const goldenPath = "testdata/schema.golden"

// TestBaselineSchemaMatchesGolden is the guard that made consolidating fifteen
// migration histories into fifteen baselines a safe change rather than a hope.
//
// The golden file was generated from the migration CHAINS as they stood before
// consolidation — the exact schema production had been built by, replayed from
// empty. Folding each chain into one baseline had to reproduce that schema
// byte for byte: same tables, columns, types, defaults, nullability, named
// constraints and indexes. Anything a fold silently dropped shows up here as a
// diff.
//
// It keeps earning its place after the fold. The golden is now a plain
// statement of what schema the code builds, so any future change to a baseline
// has to say so in the diff.
func TestBaselineSchemaMatchesGolden(t *testing.T) {
	db := freshDatabase(t)

	require.NoError(t, migrations.Apply(db, nil), "applying every subsystem to an empty database must succeed")

	got := snapshotSchema(t, db)

	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o644))
		t.Logf("wrote %s (%d lines)", goldenPath, strings.Count(got, "\n"))
		return
	}

	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "golden missing — generate it with: go test -tags integration ./migrations/ -update")

	if string(want) != got {
		t.Errorf("schema differs from %s\n%s", goldenPath, unifiedDiff(string(want), got))
	}
}

// TestApplyIsIdempotent proves the property whose absence caused the outage.
//
// A second Apply over a schema that already has everything must be a no-op,
// not a failure. Before consolidation it was neither: the migrations used bare
// CREATE TABLE and ALTER TABLE ADD COLUMN, so replaying them over a live schema
// failed on "already exists" — which is precisely what a truncated bookkeeping
// table caused on 2026-08-01.
//
// This also covers the case the reset cannot: an environment that never ran
// the reset, whose bookkeeping is intact and whose schema is already current.
// Such a database sees a baseline version above its recorded maximum, applies
// it, and must come out unchanged.
func TestApplyIsIdempotent(t *testing.T) {
	db := freshDatabase(t)

	require.NoError(t, migrations.Apply(db, nil))
	first := snapshotSchema(t, db)

	// Clear the bookkeeping, exactly as the broken reset scope did, and
	// migrate again. Every baseline replays over a schema that already has
	// everything in it.
	for _, s := range migrations.All() {
		_, err := db.Exec("TRUNCATE TABLE " + s.Table)
		require.NoError(t, err, "truncating %s", s.Table)
	}

	require.NoError(t, migrations.Apply(db, nil),
		"replaying every baseline over an already-built schema must succeed; this is the 2026-08-01 failure")

	assert.Equal(t, first, snapshotSchema(t, db), "a replay must not change the schema")
}

// TestEverySubsystemIsRegistered fails when a *_schema_migrations table appears
// in a built database that All() does not name.
//
// This is the drift guard for the registry itself. The registry is only useful
// while it is complete, and the way it stops being complete is that someone
// adds a store with its own ledger and does not think about this file.
func TestEverySubsystemIsRegistered(t *testing.T) {
	db := freshDatabase(t)
	require.NoError(t, migrations.Apply(db, nil))

	registered := map[string]bool{}
	for _, s := range migrations.All() {
		registered[s.Table] = true
	}

	rows, err := db.Query(`
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public' AND tablename LIKE '%\_schema\_migrations'
		ORDER BY tablename`)
	require.NoError(t, err)
	defer rows.Close()

	var found []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		found = append(found, name)
		assert.True(t, registered[name],
			"%s exists in the database but is not in migrations.All() — a reset or a migrate-only run would skip it", name)
	}
	require.NoError(t, rows.Err())

	assert.Len(t, found, len(registered),
		"registry names %d ledgers, the built database has %d", len(registered), len(found))
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

// freshDatabase hands back an empty database. It drops and recreates the public
// schema rather than creating a database per test: the migrations are written
// against "public" unqualified, and a search_path trick would test a schema
// production never uses.
func freshDatabase(t *testing.T) *storage.PgDB {
	t.Helper()

	connStr := os.Getenv("BOWRAIN_TEST_POSTGRES_URL")
	if connStr == "" {
		connStr = "postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable"
	}

	db, err := storage.OpenPostgres(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO PUBLIC")
	require.NoError(t, err, "resetting the public schema")

	return db
}

// snapshotSchema renders the public schema as sorted, comparable text.
//
// The column, constraint and index definitions come from Postgres's own
// pg_get_constraintdef and pg_indexes rather than from parsing the migration
// SQL, so the comparison is against what the database actually built — the
// only thing that matters — and normalization is Postgres's problem, not ours.
func snapshotSchema(t *testing.T, db *storage.PgDB) string {
	t.Helper()

	var b strings.Builder

	b.WriteString("# columns: table.column type nullability default\n")
	for _, line := range queryLines(t, db, `
		SELECT format('%s.%s %s %s %s',
		         table_name, column_name, data_type,
		         CASE is_nullable WHEN 'YES' THEN 'NULL' ELSE 'NOT NULL' END,
		         COALESCE('DEFAULT ' || column_default, ''))
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, column_name`) {
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}

	b.WriteString("\n# constraints: table constraint definition\n")
	for _, line := range queryLines(t, db, `
		SELECT format('%s %s %s', conrelid::regclass::text, conname, pg_get_constraintdef(oid))
		FROM pg_constraint
		WHERE connamespace = 'public'::regnamespace
		ORDER BY conrelid::regclass::text, conname`) {
		b.WriteString(line + "\n")
	}

	b.WriteString("\n# indexes\n")
	for _, line := range queryLines(t, db, `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = 'public'
		ORDER BY tablename, indexname`) {
		b.WriteString(line + "\n")
	}

	return b.String()
}

func queryLines(t *testing.T, db *storage.PgDB, query string) []string {
	t.Helper()

	rows, err := db.Query(query)
	require.NoError(t, err, "query: %s", query)
	defer rows.Close()

	var out []string
	for rows.Next() {
		var s sql.NullString
		require.NoError(t, rows.Scan(&s))
		out = append(out, s.String)
	}
	require.NoError(t, rows.Err())
	sort.Strings(out)
	return out
}

// unifiedDiff reports only the lines that differ. A whole-schema dump is
// thousands of lines; printing both in full buries the one column a fold
// dropped.
func unifiedDiff(want, got string) string {
	inWant := map[string]bool{}
	for _, l := range strings.Split(want, "\n") {
		inWant[l] = true
	}
	inGot := map[string]bool{}
	for _, l := range strings.Split(got, "\n") {
		inGot[l] = true
	}

	var b strings.Builder
	for _, l := range strings.Split(want, "\n") {
		if l != "" && !inGot[l] {
			fmt.Fprintf(&b, "- %s\n", l)
		}
	}
	for _, l := range strings.Split(got, "\n") {
		if l != "" && !inWant[l] {
			fmt.Fprintf(&b, "+ %s\n", l)
		}
	}
	if b.Len() == 0 {
		return "(only line ordering differs)\n"
	}
	return b.String()
}
