//go:build !js

package pluginhost

import (
	"os"
	"testing"

	"go.uber.org/goleak"

	"github.com/neokapi/neokapi/core/devenvtest"
)

// The fake daemon is built once per test binary into a temp dir (see
// sharedFakeDaemon); goleak.Cleanup owns the exit, so removing it there is
// the only hook that runs after the last test.
//
// Discovery tests hand Discover their own roots, so the binary runs without
// the plugin variables a developer's shell may export (see devenvtest).
func TestMain(m *testing.M) {
	devenvtest.Clear()
	goleak.VerifyTestMain(m, goleak.Cleanup(func(exitCode int) {
		if fakeDaemonDir != "" {
			_ = os.RemoveAll(fakeDaemonDir)
		}
		os.Exit(exitCode)
	}))
}
