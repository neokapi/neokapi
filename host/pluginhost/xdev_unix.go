//go:build unix

package pluginhost

import (
	"errors"
	"syscall"
)

// isCrossDevice reports a rename refused with EXDEV.
func isCrossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }
