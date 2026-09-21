//go:build windows

package storage

import (
	"os"

	"golang.org/x/sys/windows"
)

// fileLockSupported reports that this platform has a blocking advisory lock.
const fileLockSupported = true

// lockFile takes an exclusive LockFileEx over the whole file, blocking until it
// is held. Without LOCKFILE_FAIL_IMMEDIATELY the call waits, which is the
// property the gate is after.
func lockFile(f *os.File) error {
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0, ^uint32(0), ^uint32(0),
		new(windows.Overlapped),
	)
}

// unlockFile drops the lock over the whole file.
func unlockFile(f *os.File) error {
	return windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0, ^uint32(0), ^uint32(0),
		new(windows.Overlapped),
	)
}
