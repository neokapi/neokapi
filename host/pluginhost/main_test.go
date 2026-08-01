package pluginhost

import (
	"os"
	"testing"

	"go.uber.org/goleak"
)

// The fake daemon is built once per test binary into a temp dir (see
// sharedFakeDaemon); goleak.Cleanup owns the exit, so removing it there is
// the only hook that runs after the last test.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleak.Cleanup(func(exitCode int) {
		if fakeDaemonDir != "" {
			_ = os.RemoveAll(fakeDaemonDir)
		}
		os.Exit(exitCode)
	}))
}
