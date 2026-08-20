//go:build integration

package migrations_test

import (
	"database/sql"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

// freshDatabase hands back an empty database of its very own.
//
// It has to be a whole database, not a schema. The golden records what
// Postgres built, and Postgres writes the schema name into what it reports —
// every index line reads `ON public.tb_terms`. Run these migrations under a
// schema called anything else and the golden differs on every one of them, so
// the only ways to use a schema are to rename it back out of the snapshot
// afterwards (a substitution that can hide a real difference) or to accept a
// golden that says something production does not. In its own database `public`
// is genuinely public and the snapshot needs no help.
//
// It also must not be the database everyone else is using, which is what it
// was. The integration lane runs five packages against one server, four of
// them through testutil/pgtest — which gives each test its own schema and
// anchors the extensions those schemas share INTO public, because an extension
// belongs to a database but installs into a schema. This package then reset
// itself by dropping public CASCADE, taking the extensions with it, while
// those packages were still running: `go test` runs packages in parallel, so
// whether terms and memory found pg_trgm depended on where this package
// happened to be. Nothing said so; the failures read as unrelated flakes.
func freshDatabase(t *testing.T) *storage.PgDB {
	t.Helper()

	connStr := os.Getenv("BOWRAIN_TEST_POSTGRES_URL")
	if connStr == "" {
		connStr = "postgres://bowrain:bowrain@localhost:5432/bowrain_test?sslmode=disable"
	}

	// The connection the database is created and dropped from. It is not the
	// one handed back: a database cannot be dropped from inside itself.
	admin, err := storage.OpenPostgres(connStr)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	name := scratchDatabaseName()
	if _, err := admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		require.NoError(t, err, "creating a scratch database for the schema snapshot")
	}
	t.Cleanup(func() {
		// FORCE detaches anything still connected (PostgreSQL 13+). A server
		// too old for it drops the database the ordinary way, which is enough
		// once the pool above is closed — this cleanup runs after it.
		if _, err := admin.Exec(`DROP DATABASE IF EXISTS "` + name + `" WITH (FORCE)`); err != nil {
			_, _ = admin.Exec(`DROP DATABASE IF EXISTS "` + name + `"`)
		}
	})

	scratchStr, err := withDatabase(connStr, name)
	require.NoError(t, err)

	db, err := storage.OpenPostgres(scratchStr)
	require.NoError(t, err, "connecting to the scratch database")
	t.Cleanup(func() { _ = db.Close() })

	return db
}

// scratchDatabaseName is unique per database created, and unique per process:
// two `go test` runs against one server (the framework module and this one, a
// developer and CI) must not name the same database.
func scratchDatabaseName() string {
	scratchMu.Lock()
	defer scratchMu.Unlock()
	scratchSeq++
	return fmt.Sprintf("bowrain_schema_%d_%d", os.Getpid(), scratchSeq)
}

var (
	scratchMu  sync.Mutex
	scratchSeq int
)

// withDatabase points a connection string at another database on the same
// server.
//
// A failure here is a misconfiguration, not an absence — the URL is either the
// package default or something the operator set — so it is reported rather
// than skipped. A skip would read as "PostgreSQL is not available" and the
// golden would silently stop being checked, which is the shape of failure this
// guard exists to prevent in the first place.
func withDatabase(connStr, name string) (string, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", fmt.Errorf("BOWRAIN_TEST_POSTGRES_URL is not a URL: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("BOWRAIN_TEST_POSTGRES_URL must be a postgres:// URL, got %q", u.Scheme)
	}
	u.Path = "/" + name
	return u.String(), nil
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
