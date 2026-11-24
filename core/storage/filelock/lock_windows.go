//go:build windows

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// Supported reports that this platform has a blocking advisory lock.
const Supported = true

// lockFile takes an exclusive LockFileEx over the whole file, blocking until it
// is held. Without LOCKFILE_FAIL_IMMEDIATELY the call waits, which is the
// property a waiting lock holder relies on.
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

// releaseOnClose does nothing: closing the descriptor releases its lock.
func releaseOnClose(*os.File) {}

// tryLockFile takes an exclusive LockFileEx when no one holds one, and
// reports whether it did.
func tryLockFile(f *os.File) (bool, error) {
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, ^uint32(0), ^uint32(0),
		new(windows.Overlapped),
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

// removeHeld gives the lock back, closes the file, and then removes it.
// Windows refuses to remove a file that any process holds open without
// FILE_SHARE_DELETE, which Go's os.OpenFile never asks for. So the removal
// fails, and the file stays, whenever another locker has it open, holding
// the lock or waiting for it.
func (l *Lock) removeHeld() error {
	l.Unlock()
	_ = l.Close()
	return os.Remove(l.path)
}
