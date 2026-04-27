package pluginhost_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeManifest(t *testing.T, dir, name, body string) string {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	mp := filepath.Join(pluginDir, "manifest.json")
	require.NoError(t, os.WriteFile(mp, []byte(body), 0o644))
	return pluginDir
}

func TestDiscover_FindsManifest(t *testing.T) {
	tmp := t.TempDir()
	writeManifest(t, tmp, "demo", `{
		"manifest_version": "1",
		"plugin": "demo",
		"version": "0.1.0",
		"binary": "kapi-demo",
		"capabilities": {"commands": [{"name": "hello"}]}
	}`)

	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: tmp,
		// disable XDG + system roots
		XDGDataHome: "",
		HomeDir:     "/nonexistent",
		SystemDirs:  []string{},
	})
	require.Len(t, plugins, 1)
	assert.Equal(t, "demo", plugins[0].Manifest.Plugin)
	assert.Equal(t, "$KAPI_PLUGINS_DIR", plugins[0].Source.Label)
}

func TestDiscover_PrecedenceXDGOverSystem(t *testing.T) {
	tmpHome := t.TempDir()
	xdg := filepath.Join(tmpHome, ".local", "share")
	xdgRoot := filepath.Join(xdg, "kapi", "plugins")
	require.NoError(t, os.MkdirAll(xdgRoot, 0o755))
	writeManifest(t, xdgRoot, "demo", `{
		"manifest_version": "1",
		"plugin": "demo",
		"version": "1.0.0",
		"binary": "kapi-demo"
	}`)

	tmpSystem := t.TempDir()
	writeManifest(t, tmpSystem, "demo", `{
		"manifest_version": "1",
		"plugin": "demo",
		"version": "0.5.0",
		"binary": "kapi-demo"
	}`)

	var conflictMsg string
	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		XDGDataHome: xdg,
		HomeDir:     tmpHome,
		SystemDirs:  []string{tmpSystem},
	})
	require.Len(t, plugins, 2) // both discovered
	host := pluginhost.NewHost(plugins, func(s string) { conflictMsg = s })
	winners := host.Plugins()
	require.Len(t, winners, 1, "duplicate plugin name should dedup")
	assert.Equal(t, "1.0.0", winners[0].Version(), "XDG plugin should win over system")
	assert.Contains(t, conflictMsg, "demo")
}

func TestDiscover_SkipsMissingDirs(t *testing.T) {
	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: "/nonexistent/path",
		HomeDir:       "/nonexistent",
		SystemDirs:    []string{"/another/missing"},
	})
	assert.Empty(t, plugins)
}

func TestDiscover_NameMustMatchDir(t *testing.T) {
	tmp := t.TempDir()
	// manifest declares plugin "alpha" but the dir is named "beta"
	writeManifest(t, tmp, "beta", `{
		"manifest_version": "1",
		"plugin": "alpha",
		"version": "0.1.0",
		"binary": "kapi-alpha"
	}`)

	var warned string
	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: tmp,
		HomeDir:       "/nonexistent",
		SystemDirs:    []string{},
		OnWarn:        func(s string) { warned = s },
	})
	assert.Empty(t, plugins)
	assert.Contains(t, warned, `declares plugin name "alpha"`)
}

func TestDiscover_CommandConflict(t *testing.T) {
	tmp := t.TempDir()
	writeManifest(t, tmp, "alpha", `{
		"manifest_version": "1",
		"plugin": "alpha",
		"version": "0.1.0",
		"binary": "kapi-alpha",
		"capabilities": {"commands": [{"name": "share"}]}
	}`)
	writeManifest(t, tmp, "beta", `{
		"manifest_version": "1",
		"plugin": "beta",
		"version": "0.1.0",
		"binary": "kapi-beta",
		"capabilities": {"commands": [{"name": "share"}]}
	}`)

	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: tmp,
		HomeDir:       "/nonexistent",
		SystemDirs:    []string{},
	})
	require.Len(t, plugins, 2)

	var msg string
	host := pluginhost.NewHost(plugins, func(s string) { msg = s })
	assert.Nil(t, host.CommandRoute("share"), "conflicting commands must not dispatch")
	assert.Contains(t, msg, "share")
}

func TestCache_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	writeManifest(t, tmp, "demo", `{
		"manifest_version": "1",
		"plugin": "demo",
		"version": "0.1.0",
		"binary": "kapi-demo"
	}`)
	opts := pluginhost.DiscoverOptions{
		EnvPluginsDir: tmp,
		HomeDir:       "/nonexistent",
		SystemDirs:    []string{},
	}
	plugins := pluginhost.Discover(opts)
	require.Len(t, plugins, 1)

	cache := pluginhost.BuildCache(opts, plugins)
	cachePath := filepath.Join(t.TempDir(), "cache.json")
	require.NoError(t, pluginhost.SaveCache(cachePath, cache))

	loaded, err := pluginhost.LoadCache(cachePath)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.True(t, pluginhost.IsFresh(loaded, opts))

	rehydrated := pluginhost.PluginsFromCache(loaded)
	require.Len(t, rehydrated, 1)
	assert.Equal(t, "demo", rehydrated[0].Manifest.Plugin)
}

func TestCache_StaleAfterTouch(t *testing.T) {
	tmp := t.TempDir()
	writeManifest(t, tmp, "demo", `{
		"manifest_version": "1",
		"plugin": "demo",
		"version": "0.1.0",
		"binary": "kapi-demo"
	}`)
	opts := pluginhost.DiscoverOptions{
		EnvPluginsDir: tmp,
		HomeDir:       "/nonexistent",
		SystemDirs:    []string{},
	}
	plugins := pluginhost.Discover(opts)
	cache := pluginhost.BuildCache(opts, plugins)
	require.True(t, pluginhost.IsFresh(cache, opts))

	// Touch the dir to bump mtime forward.
	future := time.Unix(0, cache.Roots[0].MtimeUnixNano).Add(2 * time.Second)
	require.NoError(t, os.Chtimes(tmp, future, future))
	assert.False(t, pluginhost.IsFresh(cache, opts))
}
