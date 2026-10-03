//go:build unix

package filelock

import (
	"os"

	"golang.org/x/sys/unix"
)

// Supported reports that this platform has a blocking advisory lock.
const Supported = true

// lockFile takes an exclusive flock(2), blocking until it is held.
//
// EINTR is retried: a blocking flock is interruptible by a signal, and Go's
// runtime delivers signals to whichever thread it likes, so a preemption during
// the wait would otherwise surface as a failed lock.
func lockFile(f *os.File) error {
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX)
		if err != unix.EINTR {
			return err
		}
	}
}

// unlockFile drops the advisory lock.
func unlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}

// releaseOnClose does nothing: closing the descriptor releases its lock.
func releaseOnClose(*os.File) {}
