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
// plugin at version, with the parse cache of the project at root open.
func pluginReadApp(t *testing.T, root, pluginDir, version string) (*App, *pluginJSON) {
	t.Helper()
	a := &App{SourceLang: "en", Quiet: true, Config: config.NewAppConfig()}
	a.InitRegistries()
	p := registerPluginJSON(t, a.FormatReg)
	a.PluginHost = pluginhost.NewHost([]*pluginhost.Plugin{{
		Dir: pluginDir,
		Manifest: &manifest.Manifest{Plugin: "okapi-bridge", Version: version,
			Capabilities: manifest.Capabilities{Formats: []manifest.Format{{Name: "okf_json"}}}},
	}}, nil)
	closeCache, err := a.openParseCache(root)
	require.NoError(t, err)
	t.Cleanup(closeCache)
	t.Cleanup(a.Shutdown)
	return a, p
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
