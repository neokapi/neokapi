//go:build unix

package host

import (
	"errors"
	"syscall"
)

// processAlive reports whether a process with this id is running. Signal 0
// checks for the process without signalling it: no error means it runs, and
// EPERM means it runs under another user.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
