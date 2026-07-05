package server

import (
	"context"
	"testing"
	"time"

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
