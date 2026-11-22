package mcptools

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/devenvtest"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// These tests open workspaces through host, which gives this test binary a
// data root of its own under the temporary directory. It is removed once they
// have run, and the tests run without the kapi variables a developer's shell
// may export (see devenvtest).
//
// Discovery is then switched off: a call that names no project would
// otherwise find the repository's own recipe by the upward walk from the
// package directory and run inside it. A test that wants discovery sets the
// variable empty with t.Setenv.
func TestMain(m *testing.M) {
	devenvtest.Clear()
	if err := os.Setenv(project.NoProjectEnvVar, "1"); err != nil {
		panic(err)
	}
	code := m.Run()
	host.RemoveTestDataDir()
	os.Exit(code)
}
