package service

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// database/sql starts background pool goroutines per DB handle that
		// are only cleaned up when DB.Close() is called.
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionOpener"),
		goleak.IgnoreTopFunction("database/sql.(*DB).connectionCleaner"),
	)
}
