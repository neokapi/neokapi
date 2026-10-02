//go:build !unix && !windows

package host

// processAlive counts every process as running on a platform with no process
// table to ask, so the test-data sweep removes nothing there.
func processAlive(int) bool { return true }
