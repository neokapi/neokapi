package store_test

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
)

// Initialize a migrated template once per test binary. Database-backed tests
// start from that schema; store constructors find migrations already current.
//
// The external store_test package can import bowrain/migrations without a cycle
// through bowrain/store. Internal test files share the same binary and template.
func TestMain(m *testing.M) {
	pgtest.UseTemplate(func(db *storage.PgDB) error { return migrations.Apply(db, nil) })
	os.Exit(m.Run())
}
