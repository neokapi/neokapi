package event

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// pgxpool runs a background health check goroutine per pool.
		// Test databases use pgxpool for schema isolation; the goroutine
		// is cleaned up when the pool closes, but may outlive goleak's check.
		goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
		// database/sql connection pool goroutines from the shared test DB.
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionCleaner"),
	)
}
