package jobs

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
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
