package service

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
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
