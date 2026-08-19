package store_test

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
)

// The migrations run once for this binary, not once per test.
//
// Every database-backed test here starts from a fully migrated schema and then
// constructs the stores it needs; those stores call their own Migrate, find it
// current, and return. What used to happen instead was ~279ms of DDL per test
// rebuilding a schema identical to the one the previous test had just dropped.
//
// This file is `package store_test` rather than `package store` on purpose.
// `bowrain/migrations` imports `bowrain/store`, so a file inside the store
// package could not name it — but an external test package is imported by
// nothing, and may. The internal test files in this package share the binary
// and so share the template this sets up, without needing the import
// themselves.
func TestMain(m *testing.M) {
	pgtest.UseTemplate(func(db *storage.PgDB) error { return migrations.Apply(db, nil) })
	os.Exit(m.Run())
}
