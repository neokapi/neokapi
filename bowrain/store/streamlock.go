package store

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// Writers to a project stream take turns with a push applying to it.
//
// A push applies in one transaction and takes its row locks item by item. A
// write-back, or any other block write, takes the same rows in the order it
// read them. Each can hold a row the other waits on, and Postgres then fails one
// of them with "deadlock detected": the push, or the server run writing back.
//
// So each takes a transaction-scoped advisory lock on the stream before its
// first write. A push, and a removal made directly, hold it exclusively. Block
// writes share it with each other, so they still run side by side, and wait for
// an applying push to commit. A write-back that waited finds the rows the push
// removed or changed, and its guarded update skips them. The lock is released
// when the transaction ends.

const (
	lockStreamExclusive = `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`
	lockStreamShared    = `SELECT pg_advisory_xact_lock_shared(hashtextextended($1, 0))`
)

// lockStream takes the stream's write lock for the rest of tx: exclusively for a
// push or a removal, shared for a block write.
func lockStream(ctx context.Context, tx Runner, projectID, stream string, exclusive bool) error {
	query := lockStreamShared
	if exclusive {
		query = lockStreamExclusive
	}
	if _, err := tx.ExecContext(ctx, query, streamLockKey(projectID, stream)); err != nil {
		return fmt.Errorf("lock stream %q of project %s for writing: %w", storeutil.DefaultStream(stream), projectID, err)
	}
	return nil
}

// streamLockKey names a project stream's write lock. Two streams whose keys hash
// alike take turns they did not need to, and nothing else.
func streamLockKey(projectID, stream string) string {
	return "stream-writes:" + projectID + ":" + storeutil.DefaultStream(stream)
}
