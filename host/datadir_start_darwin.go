package host

import (
	"time"

	"golang.org/x/sys/unix"
)

// processStart reports when the process with this id started, read off the
// kernel's process table, and whether it could be read. macOS hands out
// process ids up to 99999 and then wraps, so a long-lived shell soon holds the
// id of a test binary that exited, and the test-data sweep reads the start
// time to tell the two apart.
func processStart(pid int) (time.Time, bool) {
	if pid <= 0 {
		return time.Time{}, false
	}
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(p.Proc.P_starttime.Unix()), true
}
