// Package devenvtest keeps a test binary independent of the kapi variables a
// developer exports to isolate an in-repo kapi (CLAUDE.md, "the in-repo
// isolation contract").
//
// Those variables select what a kapi process reads: whether it discovers a
// project, which plugins it loads, and where its workspace lives. A test that
// builds a project in a temporary directory and relies on discovery to find
// it, or one that hands plugin discovery its own roots, fails when the shell
// running `go test` has switched discovery off. A shared KAPI_DATA_DIR is
// worse: every test in the binary reads and writes one workspace, so one
// test's terms and memory leak into the next, and the directory outlives the
// run. A package whose tests depend on any of this calls Main from its
// TestMain, and a test that wants one of the variables sets it with t.Setenv.
//
// KAPI_CONFIG_DIR and the XDG directories stay as the shell set them. They
// point a developer's kapi at throwaway directories, and clearing them would
// send a test to the real ones.
package devenvtest

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/project"
)

// Vars are the variables Clear unsets.
var Vars = []string{
	project.ProjectEnvVar,
	project.NoProjectEnvVar,
	"KAPI_PLUGINS_DIR",
	"KAPI_PLUGINS_DIR_ONLY",
	"KAPI_DATA_DIR",
}

// Clear unsets Vars in this process.
func Clear() {
	for _, v := range Vars {
		_ = os.Unsetenv(v)
	}
}

// Main clears Vars, runs the package's tests, and then runs each of after
// before the process exits. Call it from TestMain:
//
//	func TestMain(m *testing.M) { devenvtest.Main(m, host.RemoveTestDataDir) }
//
// after is where a package removes what its tests left behind for the whole
// process, such as the data root a test binary gets (host.RemoveTestDataDir).
func Main(m *testing.M, after ...func()) {
	Clear()
	code := m.Run()
	for _, f := range after {
		f()
	}
	os.Exit(code)
}
