package event

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
	pgtest.UseTemplate(func(db *storage.PgDB) error { return migrations.Apply(db, nil) })

	goleak.VerifyTestMain(m,
		// PostgreSQL test infrastructure: pgxpool health check, database/sql
		// connection pool, testcontainers Ryuk reaper and Docker log followers.
		goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionCleaner"),
		goleak.IgnoreAnyFunction("github.com/testcontainers/testcontainers-go.(*Reaper).connect.func1"),
	)
}
