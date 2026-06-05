//go:build parity

package tools

import (
	"fmt"
	"os"
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
)

// TestMain isolates per-step output artifacts. Several Okapi steps
// (quality-check, char-listing, term-extraction, …) write reports to
// the bridge daemon's CWD. Without a chdir, those files land in the
// package source directory and pollute the worktree. Resolve the
// sandbox first so its path is captured against the original CWD, then
// chdir to a fresh temp dir so the JVM inherits it.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	// Cache the sandbox path against the original CWD so a relative
	// KAPI_PARITY_SANDBOX still resolves correctly after the chdir
	// below. Errors are tolerated — RequireSandbox will SkipNow per
	// test if the sandbox is unavailable.
	_, _ = parity.LoadSandbox()

	tmp, err := os.MkdirTemp("", "parity-tools-cwd-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "parity: mktemp cwd: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmp)
	if err := os.Chdir(tmp); err != nil {
		fmt.Fprintf(os.Stderr, "parity: chdir cwd: %v\n", err)
		return 1
	}

	code := m.Run()
	parity.ShutdownBridgeDaemon()
	if err := parity.FlushReport(); err != nil {
		fmt.Fprintf(os.Stderr, "parity: flush report: %v\n", err)
	}
	return code
}
