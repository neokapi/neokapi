package event

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// pgxpool runs a background health check goroutine per pool.
		goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
		// database/sql connection pool goroutines from the shared test DB.
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionCleaner"),
		// testcontainers goroutines (Ryuk reaper, Docker log followers).
		goleak.IgnoreAnyFunction("github.com/testcontainers/testcontainers-go.(*DockerContainer).startLogProduction.func1"),
		goleak.IgnoreAnyFunction("github.com/testcontainers/testcontainers-go.(*Reaper).Connect.func1"),
		goleak.IgnoreTopFunction("github.com/docker/docker/client.(*Client).Do"),
	)
}
