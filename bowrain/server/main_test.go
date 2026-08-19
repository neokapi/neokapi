package server

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/migrations"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// Every test in this package works against a fully migrated database, so
	// the migrations run once here instead of once per test — 279ms of DDL
	// each, against 37ms to copy a template. The stores a test constructs still
	// call their own Migrate and simply find the schema current.
	//
	// This package can name migrations.Apply because nothing in
	// bowrain/migrations imports it. The subsystems that ARE imported there
	// cannot, and hand pgtest their own migration instead.
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

// shutdownOnCleanup registers a full Server.Shutdown for test cleanup so
// server-lifetime goroutines (session store cleanup, event bus subscribers,
// background workers) are reaped before the goleak gate runs. Use it around
// every NewServer call in tests:
//
//	srv := shutdownOnCleanup(t, NewServer(cfg))
func shutdownOnCleanup(t testing.TB, srv *Server) *Server {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return srv
}
