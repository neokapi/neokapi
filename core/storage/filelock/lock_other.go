//go:build !unix && !windows

package filelock

import (
	"os"
	"path/filepath"
	"sync"
)

// Supported reports that this platform has no advisory lock between
// processes. The browser build is the only one that lands here. It runs one
// process, but its goroutines write the same files (two commands a page
// started at once), so a Lock still orders them: every Lock on one path takes
// one mutex of this process, as two descriptors' locks exclude each other
// natively.
const Supported = false

var (
	inProcessMu sync.Mutex
	// byPath is the mutex of each lock file, by its absolute path.
	byPath = map[string]*sync.Mutex{}
	// held is the mutex each open lock file holds, so closing the file
	// releases it as closing a descriptor releases its lock.
	held = map[*os.File]*sync.Mutex{}
)

// pathMutex is the mutex every Lock on f's path takes.
func pathMutex(f *os.File) *sync.Mutex {
	key := f.Name()
	if abs, err := filepath.Abs(key); err == nil {
		key = abs
	}
	inProcessMu.Lock()
	defer inProcessMu.Unlock()
	m := byPath[key]
	if m == nil {
		m = new(sync.Mutex)
		byPath[key] = m
	}
	return m
}

// lockFile takes the path's mutex, blocking until it is free.
func lockFile(f *os.File) error {
	m := pathMutex(f)
	m.Lock()
	inProcessMu.Lock()
	held[f] = m
	inProcessMu.Unlock()
	return nil
}

// unlockFile gives the path's mutex back, when f holds it.
func unlockFile(f *os.File) error {
	inProcessMu.Lock()
	m := held[f]
	delete(held, f)
	inProcessMu.Unlock()
	if m != nil {
		m.Unlock()
	}
	return nil
}

// releaseOnClose gives back the mutex a lock file being closed still holds.
func releaseOnClose(f *os.File) { _ = unlockFile(f) }
