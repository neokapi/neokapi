package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errStub is returned by stubConnector, which is never asked to connect.
var errStub = errors.New("stub connector")

func TestDSNSetsPoolMaxConns(t *testing.T) {
	tests := []struct {
		name    string
		connStr string
		want    bool
	}{
		{"url without pool size", "postgres://u:p@h:5432/db?sslmode=disable", false},
		{"url with pool size", "postgres://u:p@h:5432/db?sslmode=disable&pool_max_conns=8", true},
		{"postgresql scheme with pool size", "postgresql://h/db?pool_max_conns=1", true},
		{"keyword form without pool size", "host=h user=u dbname=db sslmode=disable", false},
		{"keyword form with pool size", "host=h user=u dbname=db pool_max_conns=8", true},
		{"substring of another key is not a match", "postgres://h/db?xpool_max_connsy=8", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, dsnSetsPoolMaxConns(tt.connStr))
		})
	}
}

func TestConfigureSQLOverPoolMatchesCeilings(t *testing.T) {
	tests := []struct {
		name         string
		poolMaxConns int32
	}{
		{"single connection", 1},
		{"small pool", 3},
		{"default", DefaultMaxConns},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := sql.OpenDB(stubConnector{})
			t.Cleanup(func() { _ = db.Close() })

			ConfigureSQLOverPool(db, tt.poolMaxConns)

			stats := db.Stats()
			assert.Equal(t, int(tt.poolMaxConns), stats.MaxOpenConnections,
				"database/sql must not believe it may open more connections than the pool can supply")
		})
	}
}

// TestConcurrentQueriesBeyondPoolSizeAllComplete is the regression for the
// production hang: a burst larger than the pool must queue, not strand.
//
// Before the two ceilings were made one number, database/sql would answer the
// burst by opening driver connections the pgxpool could not supply. Those
// openers blocked inside pgxpool.Acquire — outside database/sql's waiter queue
// — so every connection returned afterwards went to an idle list they no longer
// consulted, and they waited until their context was canceled while later
// requests were served from that same idle list in a millisecond.
func TestConcurrentQueriesBeyondPoolSizeAllComplete(t *testing.T) {
	dsn := os.Getenv("BOWRAIN_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("set BOWRAIN_TEST_POSTGRES_URL to run the pool contention regression")
	}

	tests := []struct {
		name        string
		poolSize    string
		concurrency int
	}{
		{"burst of ten over a pool of three", "3", 10},
		{"burst of eight over a pool of one", "1", 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := OpenPostgres(dsn + "&pool_max_conns=" + tt.poolSize)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = db.Close()
				db.Pool().Close()
			})

			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()

			var wg sync.WaitGroup
			errs := make([]error, tt.concurrency)
			start := make(chan struct{})
			for i := range tt.concurrency {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					var one int
					errs[i] = db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
				}(i)
			}
			close(start)
			wg.Wait()

			for i, err := range errs {
				assert.NoError(t, err, "query %d never got a connection", i)
			}
		})
	}
}

// stubConnector satisfies driver.Connector without a database. The pool
// configuration under test is settled before any connection is opened, so no
// method on it is ever called.
type stubConnector struct{}

func (stubConnector) Connect(context.Context) (driver.Conn, error) { return nil, errStub }
func (stubConnector) Driver() driver.Driver                        { return nil }
