package host

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// pluginReadApp is an App whose okf_json format is routed to an okapi-bridge
// plugin at version, installed from an archive named for the version, with the parse
// cache of the project at root open.
func pluginReadApp(t *testing.T, root, pluginDir, version string) (*App, *pluginJSON) {
	t.Helper()
	return pluginReadAppAs(t, root, &pluginhost.Plugin{Dir: pluginDir, Archive: "archive-" + version}, version)
}

// pluginReadAppAs is pluginReadApp for a plugin described by p, whose manifest
// it fills in.
func pluginReadAppAs(t *testing.T, root string, p *pluginhost.Plugin, version string) (*App, *pluginJSON) {
	t.Helper()
	a := &App{SourceLang: "en", Quiet: true, Config: config.NewAppConfig()}
	a.InitRegistries()
	reads := registerPluginJSON(t, a.FormatReg)
	p.Manifest = &manifest.Manifest{Plugin: "okapi-bridge", Version: version,
		Capabilities: manifest.Capabilities{Formats: []manifest.Format{{Name: "okf_json"}}}}
	a.PluginHost = pluginhost.NewHost([]*pluginhost.Plugin{p}, nil)
	closeCache, err := a.openParseCache(root)
	require.NoError(t, err)
	t.Cleanup(closeCache)
	t.Cleanup(a.Shutdown)
	return a, reads
}

// A file a plugin reads is served from the parse cache while the plugin is
// unchanged, and parsed again once the plugin's version changes, though the
// file and the configuration are the same. A plugin that declares no version
// is parsed every time.
func TestParseCache_KeysAPluginsFilesByThePlugin(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte("version: v1\nname: pk\n"), 0o644))
	src := filepath.Join(root, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello, world."}`), 0o644))
	pluginDir := t.TempDir()
	read := func(a *App) {
		_, err := a.readBlocksAs(t.Context(), root, src, "okf_json", nil, "en")
		require.NoError(t, err)
	}

	a, p := pluginReadApp(t, root, pluginDir, "1.47.0")
	read(a)
	assert.Equal(t, 1, p.reads(), "the first read parses")
	read(a)
	assert.Equal(t, 0, p.reads(), "the plugin unchanged, the second read is served from the cache")

	b, q := pluginReadApp(t, root, pluginDir, "1.48.0")
	read(b)
	assert.Equal(t, 1, q.reads(), "the plugin upgraded, the same file is parsed again")

	c, r := pluginReadApp(t, root, pluginDir, "")
	read(c)
	read(c)
	assert.Equal(t, 2, r.reads(), "a plugin with no version is parsed every time")
}

// The cache follows the plugin's bytes, not the moment it was installed: the
// same archive installed again on a fresh runner hits, and a development
// build rebuilt at the same version misses.
func TestParseCache_FollowsThePluginsBytes(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte("version: v1\nname: pk\n"), 0o644))
	src := filepath.Join(root, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello, world."}`), 0o644))
	read := func(a *App) {
		_, err := a.readBlocksAs(t.Context(), root, src, "okf_json", nil, "en")
		require.NoError(t, err)
	}

	a, p := pluginReadAppAs(t, root, &pluginhost.Plugin{Dir: t.TempDir(), Archive: "same", BinaryModTime: 1}, "1.47.0")
	read(a)
	require.Equal(t, 1, p.reads())
	b, q := pluginReadAppAs(t, root, &pluginhost.Plugin{Dir: t.TempDir(), Archive: "same", BinaryModTime: 2}, "1.47.0")
	read(b)
	assert.Equal(t, 0, q.reads(), "the same archive installed again, elsewhere and later, hits")

	c, r := pluginReadAppAs(t, root, &pluginhost.Plugin{Dir: t.TempDir(), BinarySize: 10, BinaryModTime: 100}, "1.47.0")
	read(c)
	require.Equal(t, 1, r.reads())
	d, s := pluginReadAppAs(t, root, &pluginhost.Plugin{Dir: t.TempDir(), BinarySize: 10, BinaryModTime: 200}, "1.47.0")
	read(d)
	assert.Equal(t, 1, s.reads(), "a development build rebuilt at the same version misses")
}

// A built-in format has no producer, and a plugin's identity carries its
// name, version and fingerprint.
func TestFormatProducer(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte("version: v1\nname: pk\n"), 0o644))
	a, _ := pluginReadApp(t, root, t.TempDir(), "1.47.0")
	id, ok := a.formatProducer("json")
	assert.True(t, ok)
	assert.Empty(t, id)
	id, ok = a.formatProducer("okf_json")
	assert.True(t, ok)
	assert.Contains(t, id, "okapi-bridge@1.47.0#")
}

// A parse cache that cannot be opened leaves the run uncached and says why,
// where a run summary can report it.
func TestParseCache_OffIsRecorded(t *testing.T) {
	a := &App{}
	a.noteParseCacheOff(errors.New("disk full"))
	assert.Equal(t, "disk full", a.ParseCacheOff())
}
