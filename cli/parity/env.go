//go:build parity

package parity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Sandbox describes the locally built kapi + plugin install used by the
// parity harness. It is populated once per test binary by Acquire, by
// reading the KAPI_PARITY_SANDBOX env var that `make parity-test`
// exports.
type Sandbox struct {
	// Root is the absolute path to the sandbox root (e.g. $REPO/.parity).
	Root string

	// KapiBinary is the absolute path to the locally built kapi binary.
	KapiBinary string

	// PluginsDir is the absolute path to the plugin install root that
	// pluginhost.Discover should scan. Same as Root + "/plugins".
	PluginsDir string

	// OkapiBridgeDir is the absolute path to the okapi-bridge plugin
	// install (PluginsDir + "/okapi-bridge").
	OkapiBridgeDir string

	// OkapiBridgeBinary is the absolute path to the daemon launcher
	// inside OkapiBridgeDir, derived from manifest.binary.
	OkapiBridgeBinary string
}

var (
	sandboxOnce sync.Once
	sandbox     *Sandbox
	sandboxErr  error
)

// LoadSandbox returns the parity sandbox or an error explaining why it
// could not be loaded. The lookup order is:
//
//  1. $KAPI_PARITY_SANDBOX (set by `make parity-test` and CI).
//  2. Auto-discovery: walk up from cwd looking for a `.parity/` dir
//     containing `bin/kapi`. Lets `go test -tags parity ./parity/...`
//     work from inside any subdirectory of a repo whose sandbox is
//     already built, with no env var.
//
// Callers usually go through RequireSandbox, which fails the test on a
// missing sandbox by default — silent skips on missing sandbox have
// caused multiple incidents where local agent runs claimed parity-test
// passed while CI failed.
func LoadSandbox() (*Sandbox, error) {
	sandboxOnce.Do(func() {
		root := os.Getenv("KAPI_PARITY_SANDBOX")
		if root == "" {
			discovered, err := discoverSandbox()
			if err != nil {
				sandboxErr = fmt.Errorf("KAPI_PARITY_SANDBOX is not set and no `.parity/` directory was found in any parent of cwd — run `make parity-test` from the repo root: %w", err)
				return
			}
			root = discovered
		}
		root, err := filepath.Abs(root)
		if err != nil {
			sandboxErr = fmt.Errorf("resolve KAPI_PARITY_SANDBOX %q: %w", root, err)
			return
		}
		s := &Sandbox{
			Root:           root,
			KapiBinary:     filepath.Join(root, "bin", "kapi"),
			PluginsDir:     filepath.Join(root, "plugins"),
			OkapiBridgeDir: filepath.Join(root, "plugins", "okapi-bridge"),
		}
		if _, err := os.Stat(s.KapiBinary); err != nil {
			sandboxErr = fmt.Errorf("sandbox kapi binary missing at %s: %w", s.KapiBinary, err)
			return
		}
		manifestPath := filepath.Join(s.OkapiBridgeDir, "manifest.json")
		bin, err := bridgeBinaryFromManifest(manifestPath)
		if err != nil {
			sandboxErr = fmt.Errorf("read bridge manifest %s: %w", manifestPath, err)
			return
		}
		s.OkapiBridgeBinary = filepath.Join(s.OkapiBridgeDir, bin)
		if _, err := os.Stat(s.OkapiBridgeBinary); err != nil {
			sandboxErr = fmt.Errorf("sandbox bridge binary missing at %s: %w", s.OkapiBridgeBinary, err)
			return
		}
		sandbox = s
	})
	return sandbox, sandboxErr
}

// discoverSandbox walks up from cwd looking for a `.parity/bin/kapi`,
// returning the absolute path to the `.parity` dir. Returns an error
// when no candidate is found before hitting the filesystem root. Lets
// `go test -tags parity ./parity/...` find an already-built sandbox
// without requiring callers to set $KAPI_PARITY_SANDBOX.
func discoverSandbox() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	for {
		candidate := filepath.Join(dir, ".parity")
		kapi := filepath.Join(candidate, "bin", "kapi")
		if _, err := os.Stat(kapi); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no .parity/bin/kapi in any ancestor of cwd")
		}
		dir = parent
	}
}

// skipEnv lets developers opt out of the loud failure when they
// genuinely don't have or want a sandbox locally. CI never sets this.
const skipEnv = "KAPI_PARITY_SKIP"

// RequireSandbox loads the sandbox and either returns it, fails the
// test (default), or skips (when KAPI_PARITY_SKIP=1).
//
// Why fail by default: the previous skip-by-default behavior caused
// repeated incidents where local agent runs reported parity-test green
// while CI surfaced real divergences, because the silent skip looked
// indistinguishable from a passing run. Failing forces a build of the
// sandbox (`make parity-test` from the repo root) before anyone — human
// or agent — claims parity is passing locally.
//
// Set KAPI_PARITY_SKIP=1 to opt out (only useful for cross-cutting test
// runs that don't care about parity, e.g. `go test ./...` from a fresh
// checkout before the sandbox exists).
func RequireSandbox(t *testing.T) *Sandbox {
	t.Helper()
	s, err := LoadSandbox()
	if err != nil {
		if os.Getenv(skipEnv) == "1" {
			t.Skipf("parity harness unavailable (KAPI_PARITY_SKIP=1): %v", err)
		}
		t.Fatalf("parity harness unavailable: %v\n\n"+
			"Build the sandbox with `make parity-test` (from the repo root) before running parity tests.\n"+
			"Set KAPI_PARITY_SKIP=1 to skip these tests instead of failing.", err)
	}
	return s
}
