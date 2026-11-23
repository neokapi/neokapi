package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// pluginInstall lays out a plugin directory: a manifest, an executable, and,
// when archive is not empty, the install record `kapi plugins install` writes.
func pluginInstall(t *testing.T, name, archive string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"plugin":"`+name+`"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi-"+name), []byte("bin"), 0o755))
	if archive != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "installed.json"), []byte(`{"archive_sha256":"`+archive+`"}`), 0o644))
	}
	return dir
}

// routedPlugin is one plugin whose format the test routes to it.
type routedPlugin struct {
	name, version, dir, format string
}

// pluginReadApp is an App whose okf_json and okf_html formats are routed to
// the given plugins, read with the built-in JSON and HTML readers, with the
// parse cache of the project at root open. reads counts okf_json readers made,
// subReads okf_html ones.
func pluginReadApp(t *testing.T, root string, plugins ...routedPlugin) (a *App, reads, subReads *int32) {
	t.Helper()
	a = &App{SourceLang: "en", Quiet: true, Config: config.NewAppConfig()}
	a.InitRegistries()
	var r, s int32
	for _, p := range []struct {
		id, base string
		n        *int32
	}{{"okf_json", "json", &r}, {"okf_html", "html", &s}} {
		factory := a.FormatReg.ReaderFactory(registry.FormatID(p.base))
		require.NotNil(t, factory)
		n := p.n
		a.FormatReg.RegisterReader(registry.FormatID(p.id), func() format.DataFormatReader {
			atomic.AddInt32(n, 1)
			return factory()
		}, format.FormatSignature{}, p.id)
	}
	var hosted []*pluginhost.Plugin
	for _, p := range plugins {
		hosted = append(hosted, &pluginhost.Plugin{Dir: p.dir, Manifest: &manifest.Manifest{Plugin: p.name, Version: p.version,
			Capabilities: manifest.Capabilities{Formats: []manifest.Format{{Name: p.format}}}}})
	}
	a.PluginHost = pluginhost.NewHost(hosted, nil)
	closeCache, err := a.openParseCache(root)
	require.NoError(t, err)
	t.Cleanup(closeCache)
	t.Cleanup(a.Shutdown)
	return a, &r, &s
}

func parseCacheTestProject(t *testing.T, content string) (root, src string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, project.RecipeFileName), []byte("version: v1\nname: pk\n"), 0o644))
	src = filepath.Join(root, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(content), 0o644))
	return root, src
}

// A file a plugin reads is served from the parse cache while the plugin is
// unchanged, and parsed again once the plugin's version changes, though the
// file and the configuration are the same. A plugin that declares no version
// is parsed every time.
func TestParseCache_KeysAPluginsFilesByThePlugin(t *testing.T) {
	root, src := parseCacheTestProject(t, `{"greeting":"Hello, world."}`)
	read := func(a *App) {
		_, err := a.readBlocksAs(t.Context(), root, src, "okf_json", nil, "en")
		require.NoError(t, err)
	}
	bridge := func(version, archive string) routedPlugin {
		return routedPlugin{"okapi-bridge", version, pluginInstall(t, "okapi-bridge", archive), "okf_json"}
	}

	a, n, _ := pluginReadApp(t, root, bridge("1.47.0", "a1"))
	read(a)
	read(a)
	assert.Equal(t, int32(1), atomic.LoadInt32(n), "the plugin unchanged, the second read is served from the cache")

	b, m, _ := pluginReadApp(t, root, bridge("1.48.0", "a2"))
	read(b)
	assert.Equal(t, int32(1), atomic.LoadInt32(m), "the plugin upgraded, the same file is parsed again")

	c, k, _ := pluginReadApp(t, root, bridge("", "a3"))
	read(c)
	read(c)
	assert.Equal(t, int32(2), atomic.LoadInt32(k), "a plugin with no version is parsed every time")
}

// The cache follows the plugin's bytes, not the moment it was installed: the
// same archive installed again on a fresh runner hits, and a development
// build whose payload was rebuilt at the same version misses.
func TestParseCache_FollowsThePluginsBytes(t *testing.T) {
	root, src := parseCacheTestProject(t, `{"greeting":"Hello, world."}`)
	read := func(a *App) {
		_, err := a.readBlocksAs(t.Context(), root, src, "okf_json", nil, "en")
		require.NoError(t, err)
	}

	a, n, _ := pluginReadApp(t, root, routedPlugin{"okapi-bridge", "1.47.0", pluginInstall(t, "okapi-bridge", "same"), "okf_json"})
	read(a)
	require.Equal(t, int32(1), atomic.LoadInt32(n))
	b, m, _ := pluginReadApp(t, root, routedPlugin{"okapi-bridge", "1.47.0", pluginInstall(t, "okapi-bridge", "same"), "okf_json"})
	read(b)
	assert.Equal(t, int32(0), atomic.LoadInt32(m), "the same archive installed again, elsewhere and later, hits")

	dev := pluginInstall(t, "okapi-bridge", "")
	c, k, _ := pluginReadApp(t, root, routedPlugin{"okapi-bridge", "1.47.0", dev, "okf_json"})
	read(c)
	require.Equal(t, int32(1), atomic.LoadInt32(k))
	rebuilt := pluginInstall(t, "okapi-bridge", "")
	jar := filepath.Join(rebuilt, "lib", "bridge.jar")
	require.NoError(t, os.MkdirAll(filepath.Dir(jar), 0o755))
	require.NoError(t, os.WriteFile(jar, []byte("rebuilt"), 0o644))
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(jar, later, later))
	d, j, _ := pluginReadApp(t, root, routedPlugin{"okapi-bridge", "1.47.0", rebuilt, "okf_json"})
	read(d)
	assert.Equal(t, int32(1), atomic.LoadInt32(j), "a development build with a rebuilt payload misses")
}

// containerReader reads a file as JSON and, like a container format with an
// embedded part, hands a part of it to the okf_html reader through its
// subfilter resolver, the way a real container delegates embedded content.
type containerReader struct {
	format.DataFormatReader
	res format.SubfilterResolver
}

func (c *containerReader) SetSubfilterResolver(r format.SubfilterResolver) { c.res = r }

func (c *containerReader) Open(ctx context.Context, doc *model.RawDocument) error {
	if c.res != nil {
		sub, err := c.res.ResolveReader("okf_html")
		if err != nil {
			return err
		}
		_ = sub.Close()
	}
	return c.DataFormatReader.Open(ctx, doc)
}

// Embedded content a second plugin reads keys the parse too: upgrading only
// that plugin parses the file again, and leaving it alone keeps the entry.
func TestParseCache_KeysEveryPluginThatReadPartOfTheParse(t *testing.T) {
	root, src := parseCacheTestProject(t, `{"greeting":"Hello, world."}`)
	read := func(a *App) {
		_, err := a.readBlocksAs(t.Context(), root, src, "container", nil, "en")
		require.NoError(t, err)
	}
	app := func(archive string) (*App, *int32) {
		a, _, sub := pluginReadApp(t, root, routedPlugin{"second", "1.0.0", pluginInstall(t, "second", archive), "okf_html"})
		jsonReader := a.FormatReg.ReaderFactory("json")
		a.FormatReg.RegisterReader("container", func() format.DataFormatReader {
			return &containerReader{DataFormatReader: jsonReader()}
		}, format.FormatSignature{}, "container")
		return a, sub
	}

	a, sub := app("s1")
	read(a)
	require.Equal(t, int32(1), atomic.LoadInt32(sub), "the embedded part is read by the second plugin")
	b, sub2 := app("s1")
	read(b)
	assert.Equal(t, int32(0), atomic.LoadInt32(sub2), "the second plugin unchanged, the parse is served from the cache")

	c, sub3 := app("s2")
	read(c)
	assert.Equal(t, int32(1), atomic.LoadInt32(sub3), "only the second plugin upgraded, the file is parsed again")
}

// A built-in format has no producer, and a plugin's identity carries its
// name, version and fingerprint.
func TestFormatProducer(t *testing.T) {
	root, _ := parseCacheTestProject(t, `{}`)
	a, _, _ := pluginReadApp(t, root, routedPlugin{"okapi-bridge", "1.47.0", pluginInstall(t, "okapi-bridge", "x"), "okf_json"})
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

// The open cache asks for each format's producer once, so a development
// plugin's directory is walked once per run rather than once per file.
func TestMemoProducer_AsksOncePerFormat(t *testing.T) {
	calls := 0
	p := memoProducer(func(f string) (string, bool) { calls++; return f + "-id", true })
	for range 3 {
		id, ok := p("okf_json")
		assert.True(t, ok)
		assert.Equal(t, "okf_json-id", id)
	}
	assert.Equal(t, 1, calls)
}
