//go:build !darwin

package host

import "time"

// processStart reports no start time off macOS, so the test-data sweep goes by
// the process id alone there. Linux counts a process's start in clock ticks
// since boot, and a wall-clock step moves that against a file's modification
// time.
func processStart(int) (time.Time, bool) { return time.Time{}, false }
