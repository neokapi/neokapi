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
// could not be loaded. Callers usually go through RequireSandbox, which
// SkipNow's the test on a missing sandbox; LoadSandbox is exposed so
// tooling and helper functions can introspect without forcing a skip.
func LoadSandbox() (*Sandbox, error) {
	sandboxOnce.Do(func() {
		root := os.Getenv("KAPI_PARITY_SANDBOX")
		if root == "" {
			sandboxErr = errors.New("KAPI_PARITY_SANDBOX is not set — run `make parity-test` from the repo root")
			return
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

// RequireSandbox loads the sandbox and SkipNow's the test if it is not
// available, with a message explaining how to populate it. Tests that
// reach further than the sandbox check (e.g. acquire a daemon) should
// always call this first.
func RequireSandbox(t *testing.T) *Sandbox {
	t.Helper()
	s, err := LoadSandbox()
	if err != nil {
		t.Skipf("parity harness unavailable: %v", err)
	}
	return s
}
