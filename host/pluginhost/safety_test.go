//go:build !windows && !js

package pluginhost

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

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

// A plugin under a directory other users can write is not loaded: not when
// its root, its own directory or its binary is group- or world-writable. A
// private one is.
func TestDiscover_RefusesAPluginOthersCanReplace(t *testing.T) {
	for _, tc := range []struct {
		name  string
		chmod func(root, dir string)
	}{
		{"a writable root", func(root, _ string) { require.NoError(t, os.Chmod(root, 0o777)) }},
		{"a writable plugin directory", func(_, dir string) { require.NoError(t, os.Chmod(dir, 0o775)) }},
		{"a writable binary", func(_, dir string) { require.NoError(t, os.Chmod(filepath.Join(dir, "kapi-demo"), 0o777)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := stagePlugin(t, root, "demo")
			tc.chmod(root, dir)
			var warnings []string
			got := Discover(DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true, OnWarn: func(s string) { warnings = append(warnings, s) }})
			assert.Empty(t, got)
			require.NotEmpty(t, warnings)
			assert.Contains(t, warnings[len(warnings)-1], "writable by other users")
		})
	}

	root := t.TempDir()
	stagePlugin(t, root, "demo")
	got := Discover(DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true})
	require.Len(t, got, 1, "a private plugin loads")
}

// A system root the package manager owns may be group-writable (a Homebrew
// prefix often is); kapi did not create it and loads its plugins.
func TestDiscover_LoadsFromASystemRoot(t *testing.T) {
	root := t.TempDir()
	stagePlugin(t, root, "demo")
	require.NoError(t, os.Chmod(root, 0o775))
	got := Discover(DiscoverOptions{SystemDirs: []string{root}, XDGDataHome: t.TempDir(), HomeDir: t.TempDir()})
	require.Len(t, got, 1)
	assert.True(t, got[0].System())
}

// A plugin in the person's own tree that another non-root user owns is not
// loaded; one root owns is, and so is any plugin in a system root.
func TestDiscover_RefusesAPluginAnotherUserOwns(t *testing.T) {
	owner := os.Getuid()
	fileOwnerOf = func(os.FileInfo) (int, bool) { return owner, true }
	t.Cleanup(func() { fileOwnerOf = fileOwner })

	root := t.TempDir()
	stagePlugin(t, root, "demo")
	discover := func() []*Plugin {
		return Discover(DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true})
	}
	require.Len(t, discover(), 1, "the person's own plugin loads")

	owner = os.Getuid() + 1
	var warnings []string
	got := Discover(DiscoverOptions{EnvPluginsDir: root, OnlyEnvDir: true, OnWarn: func(s string) { warnings = append(warnings, s) }})
	assert.Empty(t, got, "another user's plugin is not loaded")
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings[len(warnings)-1], "belongs to another user")

	owner = 0
	assert.Len(t, discover(), 1, "a root-owned plugin loads")

	owner = os.Getuid() + 1
	got = Discover(DiscoverOptions{SystemDirs: []string{root}, XDGDataHome: t.TempDir(), HomeDir: t.TempDir()})
	assert.Len(t, got, 1, "a system root's plugin loads whoever owns it")
}

// The refusal names the path and the command that fixes it.
func TestOwnedAndPrivate_NamesTheFix(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o775))
	err := ownedAndPrivate(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "run chmod go-w "+dir)
}

// A plugin archive that records a group- or world-writable binary is
// unpacked writable by its owner alone, so a plugin kapi installed is never
// refused for its mode.
func TestExtractTarGz_CapsTheModeItWrites(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\n")
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "demo/kapi-demo", Mode: 0o777, Size: int64(len(body)), Typeflag: tar.TypeReg}))
	_, err := tw.Write(body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	target := t.TempDir()
	require.NoError(t, extractTarGz(buf.Bytes(), target, "demo"))
	info, err := os.Stat(filepath.Join(target, "demo", "kapi-demo"))
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0o022, "mode %s", info.Mode().Perm())
}
