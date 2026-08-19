package blockstore_test

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
)

// The migrations run once for this binary, not once per test. Every
// database-backed test here starts from a fully migrated schema; the stores it
// constructs call their own Migrate, find it current, and return. See
// bowrain/testutil/pgtest.UseTemplate.
//
// `package blockstore_test` rather than the package under test, because
// `bowrain/migrations` imports it and a file inside could not name migrations.
// An external test package is imported by nothing and may.
func TestMain(m *testing.M) {
	pgtest.UseTemplate(func(db *storage.PgDB) error { return migrations.Apply(db, nil) })
	os.Exit(m.Run())
}
