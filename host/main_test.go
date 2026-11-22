package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/devenvtest"
)

// These tests build projects in temporary directories, rely on discovery to
// find them, and expect a workspace of their own, so they run without the kapi
// variables a developer's shell may export (see devenvtest). The data root
// this binary got, and the plugin binary it built, are removed once they have
// run.
func TestMain(m *testing.M) {
	pinNode()
	devenvtest.Main(m, RemoveTestDataDir, removeSourcecodePlugin)
}

// pinNode puts the directory of the Node.js binary that `node` runs at the
// front of PATH, before any test moves XDG_DATA_HOME.
//
// The formatter tests run oxfmt through its node_modules/.bin launcher, which
// runs `node` from PATH. Where `node` is the vite-plus shim (CI, and any
// machine set up with setup-vp), the shim finds its Node.js runtimes under
// XDG_DATA_HOME. isolateCheckExecution points XDG_DATA_HOME at a throwaway
// directory. Without this, the shim downloads and unpacks a Node.js release
// from nodejs.org into it on every formatter run. That is over 200 MB and
// about ten seconds on a fast link, and past the formatter's one-minute limit
// on a slow one. The binary resolved here is the runtime the shim has already
// installed.
//
// Without a node on PATH nothing changes, and a test that needs one reports it.
func pinNode() {
	out, err := exec.Command("node", "-p", "process.execPath").Output()
	if err != nil {
		return
	}
	dir := filepath.Dir(strings.TrimSpace(string(out)))
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
