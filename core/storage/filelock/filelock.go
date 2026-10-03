// Package filelock is a cross-process advisory lock on one file.
//
// Several kapi processes work on one machine at once: an agent's MCP server, a
// CLI run and Kapi Desktop, each its own process. When two of them write the
// same thing (a SQLite database, a document in a working tree), something has
// to put them in order. A Lock does it with the kernel's advisory lock
// (flock(2) on Unix, LockFileEx on Windows) on a lock file of its own: a
// writer that finds the lock held blocks in the kernel and is woken when the
// holder releases it.
//
// The lock is advisory and same-machine. It orders the processes that take
// it, and nothing else: an editor saving the same file never asks.
//
// The kernel lock belongs to an open file description, so two Locks on one
// path exclude each other even inside one process. One Lock shared by several
// goroutines is ordered by its own mutex. A platform with no lock between
// processes (the browser build, which runs one process) keeps the part that
// holds within one: the Locks on one path take one mutex of the process, so
// two goroutines writing one file still take turns (Supported is false there).
package filelock

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// Lock is an advisory lock on one lock file. A nil *Lock is a lock that does
// nothing.
type Lock struct {
	mu   sync.Mutex
	path string
	file *os.File
}

// Open opens the lock file at path, creating it if absent, and returns a Lock
// on it, not yet held.
//
// A lock file that cannot be created is reported: a caller that asked for
// cross-process ordering and silently did not get it would be told the order
// holds when it does not.
func Open(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		return nil, fmt.Errorf("open the lock file %s: %w", path, err)
	}
	return &Lock{path: path, file: f}, nil
}

// Path is the lock file's path.
func (l *Lock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Lock blocks until this Lock holds the lock, or ctx is done.
//
// The kernel wait cannot be cancelled, so cancellation is honoured before the
// wait begins and again when it ends. A holder keeps the lock for the length
// of one write, so a caller whose context expired during someone else's write
// learns so as soon as that write ends.
func (l *Lock) Lock(ctx context.Context) error {
	if l == nil || l.file == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	if err := lockFile(l.file); err != nil {
		l.mu.Unlock()
		return fmt.Errorf("take the lock on %s: %w", l.path, err)
	}
	if err := ctx.Err(); err != nil {
		_ = unlockFile(l.file)
		l.mu.Unlock()
		return err
	}
	return nil
}

// Unlock gives the lock back. It is safe on a nil Lock, so every caller writes
// it once.
func (l *Lock) Unlock() {
	if l == nil || l.file == nil {
		return
	}
	_ = unlockFile(l.file)
	l.mu.Unlock()
}

// Close releases the lock file's descriptor, and with it any lock still held.
// The file itself stays, so the next Open finds it.
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	f := l.file
	l.file = nil
	releaseOnClose(f)
	return f.Close()
}
