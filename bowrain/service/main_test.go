package service_test

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// The migrations run once for this binary, not once per test — the stores
	// each test constructs find the schema already current. See
	// bowrain/testutil/pgtest.UseTemplate for what this buys and why the
	// migration is handed over rather than imported by pgtest itself.
	//
	// `package service_test` rather than `package service`: migrations imports
	// jobs, and jobs imports this package, so a file inside it cannot name
	// migrations. An external test package is imported by nothing and may. It
	// holds nothing that needs to be inside — goleak governs the whole binary
	// from either side.
	pgtest.UseTemplate(func(db *storage.PgDB) error { return migrations.Apply(db, nil) })

	goleak.VerifyTestMain(m,
		// The shared pgtest pool lives for the whole test binary; its
		// database/sql and underlying pgx pool background goroutines are only
		// reaped on Close(), which never happens for the process-lifetime pool.
		goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionCleaner"),
		// testcontainers keeps a Ryuk reaper connection alive for the whole test
		// binary to tear down the shared container on exit — same process-lifetime
		// rationale as the pool goroutines above.
		goleak.IgnoreTopFunction("github.com/testcontainers/testcontainers-go.(*Reaper).connect.func1"),
	)
}
