//go:build unix

package pluginhost_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// demoPlugin lays out a plugin directory with a launcher script and a payload
// directory of jars, as a locally built okapi-bridge has, and returns it.
func demoPlugin(t *testing.T, installed string) *pluginhost.Plugin {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"plugin":"demo"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi-demo"), []byte("#!/bin/sh\nexec java -jar lib/a.jar\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "lib"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lib", "a.jar"), []byte("jar one"), 0o644))
	if installed != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "installed.json"), []byte(installed), 0o644))
	}
	return &pluginhost.Plugin{Dir: dir, Manifest: &manifest.Manifest{Plugin: "demo", Version: "1.0.0"}, BinaryPath: filepath.Join(dir, "kapi-demo")}
}

// An installed plugin is fingerprinted by the archive it was installed from:
// installing the same bytes again, elsewhere and later, keeps the fingerprint,
// and another archive at the same version does not.
func TestFingerprint_FollowsTheInstalledArchive(t *testing.T) {
	a := demoPlugin(t, `{"archive_sha256":"abc123"}`)
	b := demoPlugin(t, `{"archive_sha256":"abc123","installed_at":"later"}`)
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(b.BinaryPath, later, later))
	c := demoPlugin(t, `{"archive_sha256":"def456"}`)

	fa, err := a.Fingerprint()
	require.NoError(t, err)
	fb, err := b.Fingerprint()
	require.NoError(t, err)
	fc, err := c.Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, fa, fb, "the same archive installed again matches")
	assert.NotEqual(t, fa, fc, "another archive at the same version does not")
}

// A plugin with no install record is fingerprinted by every file in its
// directory: a rebuilt payload (a jar beside an unchanged launcher) does not
// match, as a locally rebuilt bridge must not.
func TestFingerprint_ARebuiltPayloadDoesNotMatch(t *testing.T) {
	before := demoPlugin(t, "")
	fp1, err := before.Fingerprint()
	require.NoError(t, err)

	rebuilt := demoPlugin(t, "")
	require.NoError(t, os.WriteFile(filepath.Join(rebuilt.Dir, "lib", "a.jar"), []byte("jar two!"), 0o644))
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(rebuilt.Dir, "lib", "a.jar"), later, later))
	fp2, err := rebuilt.Fingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, fp1, fp2)
}

// A plugin with no install record and more files than the bound is not
// fingerprinted, and says why.
func TestFingerprint_TooManyFilesIsRefused(t *testing.T) {
	p := demoPlugin(t, "")
	for i := 0; i <= pluginhost.MaxFingerprintFiles; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(p.Dir, "lib", fmt.Sprintf("f%05d", i)), nil, 0o644))
	}
	_, err := p.Fingerprint()
	require.ErrorIs(t, err, pluginhost.ErrTooManyFiles)
}

// A plugin with no manifest has no fingerprint.
func TestFingerprint_NoManifest(t *testing.T) {
	_, err := (&pluginhost.Plugin{Dir: t.TempDir()}).Fingerprint()
	require.ErrorIs(t, err, pluginhost.ErrNoFingerprint)
}

// With no install record, touching the executable alone changes the
// fingerprint: nothing in front of it serves an older answer.
func TestFingerprint_TouchingTheExecutableChangesIt(t *testing.T) {
	p := demoPlugin(t, "")
	before, err := p.Fingerprint()
	require.NoError(t, err)
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(p.BinaryPath, later, later))
	after, err := p.Fingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, before, after)
}

// The fingerprint is read from the plugin's directory on every call, not from
// the plugin discovery cache: removing the install record from a plugin that
// discovery cached changes it.
func TestFingerprint_IsNotServedFromTheDiscoveryCache(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"),
		[]byte(`{"manifest_version":"1","plugin":"demo","version":"1.0.0","binary":"kapi-demo"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi-demo"), []byte("bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "installed.json"), []byte(`{"archive_sha256":"abc"}`), 0o644))
	opts := pluginhost.DiscoverOptions{EnvPluginsDir: root, HomeDir: "/nonexistent", SystemDirs: []string{}}
	cache := pluginhost.BuildCache(opts, pluginhost.Discover(opts))

	cached := pluginhost.PluginsFromCache(cache)
	require.Len(t, cached, 1)
	withRecord, err := cached[0].Fingerprint()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "installed.json"), []byte(`{}`), 0o644))
	withoutRecord, err := pluginhost.PluginsFromCache(cache)[0].Fingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, withRecord, withoutRecord)
}

// A link inside a development plugin that is pointed elsewhere changes the
// fingerprint.
func TestFingerprint_ARetargetedLinkChangesIt(t *testing.T) {
	p := demoPlugin(t, "")
	link := filepath.Join(p.Dir, "lib", "current.jar")
	require.NoError(t, os.Symlink("a.jar", link))
	// The first link is dated in the past, so the retargeted one, made now,
	// differs even on a file system with coarse times.
	past := unix.NsecToTimeval(time.Now().Add(-time.Hour).UnixNano())
	require.NoError(t, unix.Lutimes(link, []unix.Timeval{past, past}))
	before, err := p.Fingerprint()
	require.NoError(t, err)
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink("b.jar", link))
	after, err := p.Fingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, before, after)
}
