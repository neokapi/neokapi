package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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

// pinNode puts the directory of a Node.js binary at the front of PATH, before
// any test moves XDG_DATA_HOME.
//
// The formatter tests run oxfmt through its node_modules/.bin launcher, which
// runs `node` from PATH. Where `node` is the vite-plus shim (CI, and any
// machine set up with setup-vp), the shim finds its Node.js runtimes under
// XDG_DATA_HOME, which isolateCheckExecution points at a throwaway directory,
// and would download a Node.js release there on every formatter run.
//
// The binary is found by reading, never by running `node`, because the shim
// run under a home with no runtime downloads one (over 200 MB) into it. A real
// node is used where it is; for the shim, the newest runtime it has installed.
// With none, VP_HOME points at this test binary's data root, so a download
// lands there and is removed with it.
func pinNode() {
	path, err := exec.LookPath("node")
	if err != nil {
		return
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return
	}
	if strings.TrimSuffix(filepath.Base(real), ".exe") != "vp" {
		prependPath(filepath.Dir(real))
		return
	}
	for _, root := range vpRuntimeRoots() {
		if dir := newestVPNode(root); dir != "" {
			prependPath(dir)
			return
		}
	}
	_ = os.Setenv("VP_HOME", filepath.Join(DataDir(), "vite-plus"))
}

func prependPath(dir string) {
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// vpRuntimeRoots are the places the vite-plus shim keeps its Node.js
// runtimes: under $VP_HOME when it is set, else under ~/.vite-plus or the
// XDG data directory (~/.local/share/vite-plus).
func vpRuntimeRoots() []string {
	if home := os.Getenv("VP_HOME"); home != "" {
		return []string{filepath.Join(home, "js_runtime", "node")}
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(h, ".local", "share")
	}
	return []string{
		filepath.Join(h, ".vite-plus", "js_runtime", "node"),
		filepath.Join(data, "vite-plus", "js_runtime", "node"),
	}
}

// newestVPNode is the bin directory of the newest Node.js runtime installed
// under root (root/<version>/bin/node), or "" when there is none.
func newestVPNode(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	var best []int
	bestDir := ""
	for _, e := range entries {
		v, ok := parseVersion(e.Name())
		if !ok || !e.IsDir() {
			continue
		}
		bin := filepath.Join(root, e.Name(), "bin")
		if _, err := os.Stat(filepath.Join(bin, "node")); err != nil {
			continue
		}
		if best == nil || slices.Compare(v, best) > 0 {
			best, bestDir = v, bin
		}
	}
	return bestDir
}

// parseVersion reads a dotted numeric version such as "26.13.1".
func parseVersion(s string) ([]int, bool) {
	parts := strings.Split(s, ".")
	v := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		v[i] = n
	}
	return v, true
}
