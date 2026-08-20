package jobs_test

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"go.uber.org/goleak"
)

// The migrations run once for this binary, not once per test — see the same
// file in bowrain/store for why.
//
// `package jobs_test` rather than `package jobs`, because `bowrain/migrations`
// imports `bowrain/jobs` and a file inside the package could not name it. This
// file holds nothing that needs to be inside: goleak's configuration is about
// goroutines the whole binary shares, and it governs the binary from either
// side of the boundary.
func TestMain(m *testing.M) {
	pgtest.UseTemplate(func(db *storage.PgDB) error { return migrations.Apply(db, nil) })

	goleak.VerifyTestMain(m,
		// The billing/worker Pg suites use the shared pgtest pool, which lives for
		// the whole test binary; its database/sql and underlying pgx pool
		// background goroutines, plus the testcontainers resource reaper, are only
		// reaped when the process exits — never Close()d for the process-lifetime
		// pool. Mirrors the server package's goleak config.
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionCleaner"),
		goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
		goleak.IgnoreTopFunction("github.com/testcontainers/testcontainers-go.(*Reaper).connect.func1"),
	)
}
