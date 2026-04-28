//go:build parity

package parity

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/cli/pluginhost"
)

// daemonState owns the singleton DaemonPool + okapi-bridge plugin
// metadata used by every parity test in one binary invocation.
type daemonState struct {
	once     sync.Once
	pool     *pluginhost.DaemonPool
	plugin   *pluginhost.Plugin
	initErr  error
	shutdown sync.Once
}

var bridgeDaemon = &daemonState{}

// AcquireBridgeDaemon returns a ready-to-use DaemonClient for the
// okapi-bridge plugin. The pool is shared across all tests in the
// binary; the daemon is spawned the first time AcquireBridgeDaemon is
// called and reused thereafter.
func AcquireBridgeDaemon(t *testing.T) *pluginhost.DaemonClient {
	t.Helper()
	s := RequireSandbox(t)

	bridgeDaemon.once.Do(func() {
		host, err := loadHost(s)
		if err != nil {
			bridgeDaemon.initErr = err
			return
		}
		plugin := host.Plugin("okapi-bridge")
		if plugin == nil {
			bridgeDaemon.initErr = fmt.Errorf("okapi-bridge plugin not present in sandbox %s", s.OkapiBridgeDir)
			return
		}
		bridgeDaemon.plugin = plugin
		bridgeDaemon.pool = pluginhost.NewDaemonPool(pluginhost.DaemonPoolOptions{
			MaxDaemons: 1,
			Logger: func(format string, args ...any) {
				if testing.Verbose() {
					fmt.Fprintf(os.Stderr, "[parity-daemon] "+format+"\n", args...)
				}
			},
		})
	})
	if bridgeDaemon.initErr != nil {
		t.Fatalf("acquire bridge daemon: %v", bridgeDaemon.initErr)
	}

	client, err := bridgeDaemon.pool.Acquire(context.Background(), bridgeDaemon.plugin)
	if err != nil {
		t.Fatalf("daemon pool: acquire okapi-bridge: %v", err)
	}
	return client
}

// ShutdownBridgeDaemon stops the shared daemon. Tests do not call this
// directly — TestMain in the per-package test files invokes it via
// runtime.AddCleanup-style teardown so the JVM is released between
// `go test` invocations.
func ShutdownBridgeDaemon() {
	bridgeDaemon.shutdown.Do(func() {
		if bridgeDaemon.pool != nil {
			bridgeDaemon.pool.Shutdown()
		}
	})
}

// loadHost runs plugin discovery against a single explicit plugins dir
// (the sandbox), with no fallback to XDG/system paths. We pass empty
// SystemDirs to suppress the platform defaults — the parity harness
// must never measure pre-installed plugins.
func loadHost(s *Sandbox) (*pluginhost.Host, error) {
	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: s.PluginsDir,
		XDGDataHome:   s.Root + "/__nonexistent_xdg",
		HomeDir:       s.Root + "/__nonexistent_home",
		SystemDirs:    []string{},
	})
	if len(plugins) == 0 {
		return nil, fmt.Errorf("no plugins discovered in sandbox %s", s.PluginsDir)
	}
	return pluginhost.NewHost(plugins, nil), nil
}
