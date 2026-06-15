package pluginhost_test

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolatedOpts scans only the given dir (no XDG / system roots) so tests don't
// pick up the developer's or machine's installed plugins.
func isolatedOpts(dir string) pluginhost.DiscoverOptions {
	return pluginhost.DiscoverOptions{
		EnvPluginsDir: dir,
		XDGDataHome:   "",
		HomeDir:       "/nonexistent",
		SystemDirs:    []string{},
	}
}

const fakeBridgeManifest = `{
	"manifest_version": "1",
	"plugin": "fakebridge",
	"version": "1.0.0",
	"binary": "kapi-fakebridge",
	"daemon": {"idle_timeout_seconds": 1},
	"capabilities": {
		"formats": [
			{"name": "okf_demo", "extensions": [".demo"], "capabilities": ["read", "write"]}
		]
	}
}`

// Rescan builds the host and wires plugin formats into the registry, so a
// plugin's formats become usable without restarting (the install→load path).
func TestRuntime_RescanRegistersPluginFormats(t *testing.T) {
	tmp := t.TempDir()
	writeManifest(t, tmp, "fakebridge", fakeBridgeManifest)

	reg := registry.NewFormatRegistry()
	rt := pluginhost.NewRuntime(pluginhost.RuntimeOptions{
		Discover:  isolatedOpts(tmp),
		FormatReg: reg,
	})
	defer rt.Shutdown()

	host := rt.Rescan()
	require.NotNil(t, host)
	require.Len(t, host.Plugins(), 1)
	assert.Equal(t, "fakebridge", host.Plugins()[0].Name())

	// The plugin's format is now a registered (daemon-backed) reader/writer.
	assert.True(t, reg.HasReader("okf_demo"), "plugin format reader should be registered after Rescan")
	assert.True(t, reg.HasWriter("okf_demo"), "plugin format writer should be registered after Rescan")
}

// Watch detects a plugin installed out-of-band (e.g. by the CLI) and rebuilds
// the host, invoking onChange — the dynamic, event-driven path.
func TestRuntime_WatchDetectsExternalInstall(t *testing.T) {
	tmp := t.TempDir()

	reg := registry.NewFormatRegistry()
	rt := pluginhost.NewRuntime(pluginhost.RuntimeOptions{
		Discover:  isolatedOpts(tmp),
		FormatReg: reg,
	})
	defer rt.Shutdown()

	rt.Rescan()
	require.Empty(t, rt.Host().Plugins(), "no plugins installed yet")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan *pluginhost.Host, 1)
	go rt.Watch(ctx, 50*time.Millisecond, func(h *pluginhost.Host) {
		select {
		case changed <- h:
		default:
		}
	})

	// Simulate `kapi plugins install` in another process.
	writeManifest(t, tmp, "fakebridge", fakeBridgeManifest)

	select {
	case h := <-changed:
		require.Len(t, h.Plugins(), 1)
		assert.Equal(t, "fakebridge", h.Plugins()[0].Name())
		assert.True(t, reg.HasReader("okf_demo"), "format should be wired after a watched install")
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not detect the external install")
	}
}

// Watch stops promptly when its context is cancelled.
func TestRuntime_WatchStopsOnCancel(t *testing.T) {
	tmp := t.TempDir()
	rt := pluginhost.NewRuntime(pluginhost.RuntimeOptions{Discover: isolatedOpts(tmp)})
	defer rt.Shutdown()
	rt.Rescan()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		rt.Watch(ctx, 20*time.Millisecond, func(*pluginhost.Host) {})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after context cancel")
	}
}
