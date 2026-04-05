package jobs

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// database/sql starts a background goroutine for each DB connection pool
		// that is only cleaned up when DB.Close() is called.
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
	)
}
