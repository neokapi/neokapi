package storage

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/storage/filelock"
)

// The write gate orders the writers of one process. Nothing orders the writers
// of several, and after the context store left the checkout there are several:
// an agent's MCP server, a CLI run, the desktop, each its own process on one
// file.
//
// What SQLite offers there is `busy_timeout`, and its queue is a sleep. A
// writer that loses the race sleeps a fixed step — 1, 2, 5, 10, 15, 20, 25, 25,
// 25, 50, 50, 100 milliseconds and so on — and wakes to try again, so a lock
// that freed a millisecond later is still not taken for the rest of the step.
// Measured with sixteen agent processes writing into one context store, that
// quantization put the 99th percentile of a 0.5 ms write at 66 ms while the
// median stayed at 0.45 ms: the tail was the sleep, not the work.
//
// A cross-process advisory lock replaces the sleep with a wait. A writer blocks
// in the kernel and is woken when the holder releases, so the cost of losing
// the race is the holder's transaction rather than the next rung of a backoff
// ladder. It is taken INSIDE the in-process gate, so a process holds at most
// one, and released by the same Commit or Rollback that releases the permit.
//
// It is advisory and same-machine, which is all it needs to be: it guards a
// SQLite file, and a SQLite file is already unusable across a network
// filesystem. A platform with no implementation leaves the lock a no-op and
// falls back to what SQLite does on its own. The lock itself is package
// filelock, which the change service's file home takes as well.

// fileLock is a cross-process advisory lock on one database file
// (filelock.Lock on a lock file beside it). The nil value is a lock that does
// nothing, which is what a handle opened without Options.CrossProcessWrites
// gets.
type fileLock struct {
	lock *filelock.Lock
}

// lockSuffix names the lock file beside the database. A file of its own rather
// than the database itself: locking the database's own descriptor would sit
// alongside the locks SQLite takes on it, and two advisory schemes on one
// inode is a question nobody should have to answer.
const lockSuffix = ".lock"

// newFileLock opens (creating it if absent) the lock file beside dbPath.
func newFileLock(dbPath string) (*fileLock, error) {
	l, err := filelock.Open(dbPath + lockSuffix)
	if err != nil {
		return nil, fmt.Errorf("open write lock for %s: %w", dbPath, err)
	}
	if l == nil {
		return nil, nil
	}
	return &fileLock{lock: l}, nil
}

// acquire blocks until this process holds the lock, or ctx is done.
func (l *fileLock) acquire(ctx context.Context) error {
	if l == nil {
		return nil
	}
	if err := l.lock.Lock(ctx); err != nil {
		return fmt.Errorf("take the write lock: %w", err)
	}
	return nil
}

// release gives the lock back. Safe on a nil lock, so every call site is
// written once.
func (l *fileLock) release() {
	if l == nil {
		return
	}
	l.lock.Unlock()
}

// close releases the descriptor. The lock itself goes with it.
func (l *fileLock) close() error {
	if l == nil {
		return nil
	}
	return l.lock.Close()
}
