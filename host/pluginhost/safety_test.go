//go:build !windows && !js

package pluginhost

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stagePlugin writes a minimal plugin named name under root.
func stagePlugin(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"),
		[]byte(`{"manifest_version":"1","plugin":"`+name+`","version":"1.0.0","binary":"kapi-`+name+`"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi-"+name), []byte("#!/bin/sh\n"), 0o755))
	return dir
}

// privateRoot is a temporary plugin root no other user can write, owned by
// the person's primary group. A file created on macOS takes the group of its
// directory, and a temporary directory under /tmp belongs to wheel, an
// administrators' group whose write the rule accepts; the person's own group
// makes a group-writable mode mean what the tests below say it means.
func privateRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o700))
	require.NoError(t, os.Chown(root, -1, os.Getgid()))
	require.False(t, isAdminGroup(os.Getgid()), "the tests need a primary group that is not an administrators' group")
	return root
}

// A plugin under a path other users can write is not loaded: not when its
// root, its own directory or its binary is writable by everyone or by an
// ordinary group. A private one is, and each refusal is returned with why.
func TestDiscover_RefusesAPluginOthersCanReplace(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		chmod        func(root, dir string)
	}{
		{"a root everyone can write", "writable by every user", func(root, _ string) { require.NoError(t, os.Chmod(root, 0o777)) }},
		{"a plugin directory its group can write", "writable by its group", func(_, dir string) { require.NoError(t, os.Chmod(dir, 0o775)) }},
		{"a binary everyone can write", "writable by every user", func(_, dir string) { require.NoError(t, os.Chmod(filepath.Join(dir, "kapi-demo"), 0o777)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := privateRoot(t)
			dir := stagePlugin(t, root, "demo")
			tc.chmod(root, dir)
			var warnings []string
			got, refused := DiscoverAll(DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true, OnWarn: func(s string) { warnings = append(warnings, s) }})
			assert.Empty(t, got)
			require.Len(t, refused, 1)
			assert.Equal(t, "demo", refused[0].Name)
			assert.Contains(t, refused[0].Reason, tc.reason)
			require.NotEmpty(t, warnings)
			assert.Contains(t, warnings[len(warnings)-1], tc.reason)
		})
	}

	root := privateRoot(t)
	stagePlugin(t, root, "demo")
	got, refused := DiscoverAll(DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true})
	require.Len(t, got, 1, "a private plugin loads")
	assert.Empty(t, refused)
}

// A system root is checked like any other: one an ordinary group can write
// is refused; one owned by the person with mode 755, as Homebrew's is, loads.
func TestDiscover_ChecksASystemRoot(t *testing.T) {
	root := privateRoot(t)
	stagePlugin(t, root, "demo")
	opts := DiscoverOptions{SystemDirs: []string{root}, XDGDataHome: t.TempDir(), HomeDir: t.TempDir()}

	require.NoError(t, os.Chmod(root, 0o755))
	got := Discover(opts)
	require.Len(t, got, 1)
	assert.True(t, got[0].System())

	require.NoError(t, os.Chmod(root, 0o775))
	got, refused := DiscoverAll(opts)
	assert.Empty(t, got)
	require.Len(t, refused, 1)
	assert.Contains(t, refused[0].Reason, "writable by its group")
}

// withOwner makes every path report uid as its owner for one test.
func withOwner(t *testing.T, uid *int) {
	t.Helper()
	real := statFacts
	statFacts = func(path string) (fileFacts, bool) {
		f, ok := real(path)
		f.uid = *uid
		return f, ok
	}
	t.Cleanup(func() { statFacts = real })
}

// A plugin another non-root user owns is not loaded, in any root; one the
// person or root owns is.
func TestDiscover_RefusesAPluginAnotherUserOwns(t *testing.T) {
	owner := os.Getuid()
	withOwner(t, &owner)
	root := privateRoot(t)
	stagePlugin(t, root, "demo")
	discover := func(opts DiscoverOptions) ([]*Plugin, []Refusal) { return DiscoverAll(opts) }
	env := DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true}
	sys := DiscoverOptions{SystemDirs: []string{root}, XDGDataHome: t.TempDir(), HomeDir: t.TempDir()}

	got, _ := discover(env)
	require.Len(t, got, 1, "the person's own plugin loads")

	owner = os.Getuid() + 1
	for _, opts := range []DiscoverOptions{env, sys} {
		got, refused := discover(opts)
		assert.Empty(t, got, "another user's plugin is not loaded")
		require.Len(t, refused, 1)
		assert.Contains(t, refused[0].Reason, "belongs to another user")
	}

	owner = 0
	got, _ = discover(env)
	assert.Len(t, got, 1, "a root-owned plugin loads")
}

// The refusal names the path and the command that fixes it.
func TestCheckPath_NamesTheFix(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o777))
	err := checkPath(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "run chmod o-w "+dir)
}

// CheckLaunch refuses at launch a plugin whose binary was made writable after
// discovery.
func TestCheckLaunch_RefusesAChangedBinary(t *testing.T) {
	root := privateRoot(t)
	stagePlugin(t, root, "demo")
	got := Discover(DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true})
	require.Len(t, got, 1)
	require.NoError(t, CheckLaunch(got[0]))

	require.NoError(t, os.Chmod(got[0].BinaryPath, 0o777))
	err := CheckLaunch(got[0])
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin demo is not run")
}

// saveCacheFile writes cache to a private cache file and returns its path.
func saveCacheFile(t *testing.T, cache *Cache) string {
	t.Helper()
	dir := filepath.Join(privateRoot(t), "kapi")
	path := filepath.Join(dir, "plugins-cache.json")
	require.NoError(t, SaveCache(path, cache))
	return path
}

// A cache entry cannot carry a plugin past the rule: the order it claims is
// ignored, a directory outside every current root is dropped, and the binary
// is the manifest's, inside the plugin's directory.
func TestPluginsFromCache_TrustsTheRootsNotTheFile(t *testing.T) {
	root := privateRoot(t)
	stagePlugin(t, root, "demo")
	opts := DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true}
	plugins, refused := DiscoverAll(opts)
	cache := BuildCache(opts, plugins, refused)

	// A directory planted outside the roots, everyone-writable, as a reviewer
	// did with an entry claiming the system order.
	elsewhere := t.TempDir()
	planted := stagePlugin(t, elsewhere, "planted")
	require.NoError(t, os.Chmod(planted, 0o777))
	cache.Plugins = append(cache.Plugins, CachedPlugin{Dir: planted, Manifest: cache.Plugins[0].Manifest})

	got := PluginsFromCache(cache, opts)
	require.Len(t, got, 1)
	assert.Equal(t, filepath.Join(root, "demo"), got[0].Dir)
	assert.Equal(t, filepath.Join(root, "demo", "kapi-demo"), got[0].BinaryPath)
	assert.Equal(t, 1, got[0].Source.Order, "the order is the env root's, from the current roots")

	// A manifest whose binary climbs out of the directory is dropped.
	escaping := *cache.Plugins[0].Manifest
	escaping.Binary = "../../bin/sh"
	cache.Plugins = []CachedPlugin{{Dir: filepath.Join(root, "demo"), Manifest: &escaping}}
	assert.Empty(t, PluginsFromCache(cache, opts))
}

// The cache file is read only when no other user can have written it.
func TestLoadCache_RefusesAFileOthersCanWrite(t *testing.T) {
	root := privateRoot(t)
	stagePlugin(t, root, "demo")
	opts := DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true}
	plugins, refused := DiscoverAll(opts)
	path := saveCacheFile(t, BuildCache(opts, plugins, refused))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "kapi writes its cache for the person alone")
	_, err = LoadCache(path)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(path, 0o666))
	_, err = LoadCache(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not read")

	require.NoError(t, os.Chmod(path, 0o600))
	require.NoError(t, os.Chmod(filepath.Dir(path), 0o777))
	_, err = LoadCache(path)
	require.Error(t, err, "a directory others can write is refused too")
}

// The fix a refusal names takes effect on the next command: chmod changes the
// change time and the mode, not the modification time, and the cache reads
// stale. The refusal is in the cache until then.
func TestIsFresh_StaleAfterTheChmodARefusalNames(t *testing.T) {
	root := privateRoot(t)
	dir := stagePlugin(t, root, "demo")
	require.NoError(t, os.Chmod(dir, 0o777))
	opts := DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true}
	plugins, refused := DiscoverAll(opts)
	require.Empty(t, plugins)
	require.Len(t, refused, 1)

	cache := BuildCache(opts, plugins, refused)
	data, err := json.Marshal(cache)
	require.NoError(t, err)
	var reread Cache
	require.NoError(t, json.Unmarshal(data, &reread))
	require.True(t, IsFresh(&reread, opts))
	require.Len(t, reread.Refused, 1, "the refusal is kept with its reason")
	assert.Contains(t, reread.Refused[0].Reason, "chmod o-w")

	require.NoError(t, os.Chmod(dir, 0o755))
	assert.False(t, IsFresh(&reread, opts), "the chmod makes the cache stale")
	plugins, refused = DiscoverAll(opts)
	assert.Len(t, plugins, 1)
	assert.Empty(t, refused)
}

// tarball builds a gzipped tar from headers and bodies.
func tarball(t *testing.T, entries ...struct {
	hdr  tar.Header
	body string
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := e.hdr
		h.Size = int64(len(e.body))
		require.NoError(t, tw.WriteHeader(&h))
		_, err := tw.Write([]byte(e.body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

type entry = struct {
	hdr  tar.Header
	body string
}

// A plugin archive that records a writable or setuid binary is unpacked
// writable by its owner alone and without the setuid bit, from a tarball and
// from a zip, so a plugin kapi installed is never refused for its mode.
func TestExtract_CapsTheModeItWrites(t *testing.T) {
	target := t.TempDir()
	body := tarball(t, entry{tar.Header{Name: "demo/kapi-demo", Mode: 0o4777, Typeflag: tar.TypeReg}, "#!/bin/sh\n"})
	require.NoError(t, extractTarGz(body, target, "demo"))
	info, err := os.Stat(filepath.Join(target, "demo", "kapi-demo"))
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&(os.ModeSetuid|0o022), "tar: mode %s", info.Mode())

	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	h := &zip.FileHeader{Name: "demo/kapi-demo", Method: zip.Store}
	h.SetMode(os.ModeSetuid | 0o777)
	w, err := zw.CreateHeader(h)
	require.NoError(t, err)
	_, err = w.Write([]byte("#!/bin/sh\n"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	ztarget := t.TempDir()
	require.NoError(t, extractZip(zbuf.Bytes(), ztarget, "demo"))
	info, err = os.Stat(filepath.Join(ztarget, "demo", "kapi-demo"))
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&(os.ModeSetuid|0o022), "zip: mode %s", info.Mode())
}

// A chain of links an earlier entry plants (a -> ., then a/x -> ..) cannot
// lead a later write out of the plugin's directory.
func TestExtractTarGz_RefusesALinkChain(t *testing.T) {
	target := t.TempDir()
	body := tarball(t,
		entry{tar.Header{Name: "demo/a", Linkname: ".", Typeflag: tar.TypeSymlink}, ""},
		entry{tar.Header{Name: "demo/a/x", Linkname: "..", Typeflag: tar.TypeSymlink}, ""},
		entry{tar.Header{Name: "demo/a/x/escaped", Mode: 0o644, Typeflag: tar.TypeReg}, "owned"},
	)
	require.Error(t, extractTarGz(body, target, "demo"))
	_, err := os.Stat(filepath.Join(target, "escaped"))
	assert.True(t, os.IsNotExist(err), "nothing is written outside the plugin's directory")
}

// The macOS prewarm runs the freshly installed binary only after the launch
// check passes, like every other launch. Elsewhere it runs nothing.
func TestPrewarmFirstExec_ChecksTheLaunch(t *testing.T) {
	stage := func(t *testing.T) (*Plugin, string) {
		t.Helper()
		root := privateRoot(t)
		dir := stagePlugin(t, root, "demo")
		marker := filepath.Join(t.TempDir(), "ran")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi-demo"), []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755))
		m := &manifest.Manifest{ManifestVersion: "1", Plugin: "demo", Version: "1.0.0", Binary: "kapi-demo"}
		return &Plugin{Dir: dir, Source: Source{Path: root}, Manifest: m, BinaryPath: filepath.Join(dir, "kapi-demo")}, marker
	}

	var warnings []string
	refused, marker := stage(t)
	require.NoError(t, os.Chmod(refused.Dir, 0o777))
	prewarmFirstExec(t.Context(), refused, func(s string) { warnings = append(warnings, s) })
	assert.NoFileExists(t, marker, "a binary others can replace is not run")

	allowed, marker := stage(t)
	prewarmFirstExec(t.Context(), allowed, func(string) {})
	if runtime.GOOS == "darwin" {
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "plugin demo is not run")
		assert.FileExists(t, marker, "a binary that passes the check is warmed")
	} else {
		assert.NoFileExists(t, marker, "only macOS warms a new binary")
	}
}
