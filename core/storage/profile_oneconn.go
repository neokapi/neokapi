//go:build storage_oneconn

package storage

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"time"
)

// singleConnection holds every pool to one connection: the storage_oneconn
// test mode (see DriverProfile).
const singleConnection = true

// poolWait is how long a call may wait for a pool's only connection in this
// mode before it is reported. A goroutine that holds a transaction or open rows
// and then asks the same pool for a second session waits forever; reporting the
// wait names the offender instead of leaving the test binary to its timeout.
const poolWait = 20 * time.Second

// watchPool reports a call still waiting for the pool after poolWait, with
// every goroutine's stack, and returns the function that ends the watch.
func watchPool(what string) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-time.After(poolWait):
			buf := make([]byte, 4<<20)
			n := runtime.Stack(buf, true)
			panic(fmt.Sprintf("storage_oneconn: %s waited %s for the pool's only connection. "+
				"A goroutine holds a transaction or open rows on this pool and asked it for a second session.\n\n%s",
				what, poolWait, buf[:n]))
		}
	}()
	return func() { close(done) }
}

// The reads below shadow the promoted *sql.DB methods in this mode only, so a
// read that waits on the pool is watched like a write is (write.go).

func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	defer watchPool("a query")()
	return db.DB.QueryContext(ctx, query, args...)
}

func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return db.QueryContext(context.Background(), query, args...)
}

func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	defer watchPool("a query")()
	return db.DB.QueryRowContext(ctx, query, args...)
}

func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	return db.QueryRowContext(context.Background(), query, args...)
}

func (db *DB) Conn(ctx context.Context) (*sql.Conn, error) {
	defer watchPool("a dedicated connection")()
	return db.DB.Conn(ctx)
}
