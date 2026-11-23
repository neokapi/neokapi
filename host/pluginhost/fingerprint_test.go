package pluginhost_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
)

const demoManifest = `{"manifest_version": "1", "plugin": "demo", "version": "1.0.0", "binary": "kapi-demo"}`

func discoverDemo(t *testing.T, root string) *pluginhost.Plugin {
	t.Helper()
	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{
		EnvPluginsDir: root, HomeDir: "/nonexistent", SystemDirs: []string{},
	})
	require.Len(t, plugins, 1)
	return plugins[0]
}

// An installed plugin is fingerprinted by the archive it was installed from:
// installing the same bytes again, at another time, keeps the fingerprint.
func TestFingerprint_AReinstallOfTheSameArchiveMatches(t *testing.T) {
	first := t.TempDir()
	dir := writeManifest(t, first, "demo", demoManifest)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi-demo"), []byte("bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "installed.json"), []byte(`{"archive_sha256":"abc123"}`), 0o644))
	a := discoverDemo(t, first)
	assert.Equal(t, "abc123", a.Archive)

	second := t.TempDir()
	dir2 := writeManifest(t, second, "demo", demoManifest)
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "kapi-demo"), []byte("bin"), 0o755))
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir2, "kapi-demo"), later, later))
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "installed.json"), []byte(`{"archive_sha256":"abc123"}`), 0o644))
	b := discoverDemo(t, second)

	require.NotEmpty(t, a.Fingerprint())
	assert.Equal(t, a.Fingerprint(), b.Fingerprint(), "the same archive installed later matches")

	c := *b
	c.Archive = "def456"
	assert.NotEqual(t, a.Fingerprint(), c.Fingerprint(), "another archive at the same version does not")
}

// A plugin with no install record (a development build) is fingerprinted by
// its executable's size and time: a rebuild at the same version does not
// match, and an untouched one does.
func TestFingerprint_ADevelopmentRebuildDoesNotMatch(t *testing.T) {
	root := t.TempDir()
	dir := writeManifest(t, root, "demo", demoManifest)
	bin := filepath.Join(dir, "kapi-demo")
	require.NoError(t, os.WriteFile(bin, []byte("build one"), 0o755))
	before := discoverDemo(t, root)
	require.Empty(t, before.Archive)
	again := discoverDemo(t, root)
	require.NotEmpty(t, before.Fingerprint())
	assert.Equal(t, before.Fingerprint(), again.Fingerprint(), "the same build matches")

	require.NoError(t, os.WriteFile(bin, []byte("build two"), 0o755))
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(bin, later, later))
	after := discoverDemo(t, root)
	assert.NotEqual(t, before.Fingerprint(), after.Fingerprint(), "a rebuild at the same version does not")
}

// A plugin with neither an install record nor an executable kapi could stat
// has no fingerprint, and its files are not cached.
func TestFingerprint_NothingKnownIsEmpty(t *testing.T) {
	p := &pluginhost.Plugin{Manifest: &manifest.Manifest{Plugin: "demo", Version: "1.0.0"}}
	assert.Empty(t, p.Fingerprint())
}
