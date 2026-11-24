//go:build !windows && !js

package pluginhost

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A plugin directory that passes the install checks moves into the root, and
// its files end with the install mode cap (no setuid bit).
func TestAdoptPluginDir_MovesACleanPlugin(t *testing.T) {
	from := privateRoot(t)
	src := stagePlugin(t, from, "demo")
	require.NoError(t, os.Chmod(filepath.Join(src, "kapi-demo"), os.ModeSetuid|0o755))
	target := filepath.Join(privateRoot(t), "plugins")

	require.NoError(t, AdoptPluginDir(src, target))
	_, err := os.Stat(src)
	assert.True(t, os.IsNotExist(err), "the original is gone")
	info, err := os.Stat(filepath.Join(target, "demo", "kapi-demo"))
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSetuid, "mode %s", info.Mode())
	got := Discover(DiscoverOptions{EnvPluginsDir: target, OnlyEnvDir: true})
	require.Len(t, got, 1, "the moved plugin is discovered")
}

// What an install would refuse stays where it is: a link inside the plugin, a
// directory others can write, or a plugin the root already holds.
func TestAdoptPluginDir_RefusesWhatAnInstallWould(t *testing.T) {
	for _, tc := range []struct {
		name, refuse string
		prepare      func(src, target string)
	}{
		{"a link inside the plugin", "is a link", func(src, _ string) {
			require.NoError(t, os.Symlink("/bin/sh", filepath.Join(src, "helper")))
		}},
		{"a plugin directory others can write", "writable by every user", func(src, _ string) {
			require.NoError(t, os.Chmod(src, 0o777))
		}},
		{"a plugin the root already holds", "already installed", func(_, target string) {
			stagePlugin(t, target, "demo")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := stagePlugin(t, privateRoot(t), "demo")
			target := privateRoot(t)
			tc.prepare(src, target)
			err := AdoptPluginDir(src, target)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.refuse)
			_, serr := os.Stat(src)
			assert.NoError(t, serr, "the original stays")
		})
	}
}

// acrossFileSystems makes the move of src fail as a rename across file
// systems does, so AdoptPluginDir takes its copy path.
func acrossFileSystems(t *testing.T) {
	t.Helper()
	real := moveDir
	moveDir = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
	}
	t.Cleanup(func() { moveDir = real })
}

// hiddenEntries lists the hidden entries of dir.
func hiddenEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

// Across file systems the copy lands complete or not at all: it is made in a
// hidden directory and renamed into place, and the original goes only after.
func TestAdoptPluginDir_CopiesAcrossFileSystems(t *testing.T) {
	acrossFileSystems(t)
	src := stagePlugin(t, privateRoot(t), "demo")
	target := privateRoot(t)

	require.NoError(t, AdoptPluginDir(src, target))
	assert.FileExists(t, filepath.Join(target, "demo", "kapi-demo"))
	assert.NoDirExists(t, src, "the original goes once the copy is in place")
	assert.Empty(t, hiddenEntries(t, target), "no hidden directory is left")
	assert.Len(t, Discover(DiscoverOptions{EnvPluginsDir: target, OnlyEnvDir: true}), 1)
}

// OpenRoot failing after the hidden directory exists leaves neither it nor a
// destination behind, and the original where it was.
func TestAdoptPluginDir_OpenRootFailingLeavesNothing(t *testing.T) {
	acrossFileSystems(t)
	src := stagePlugin(t, privateRoot(t), "demo")
	target := privateRoot(t)
	real := openRoot
	calls := 0
	openRoot = func(name string) (*os.Root, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("simulated failure")
		}
		return real(name)
	}
	t.Cleanup(func() { openRoot = real })

	require.Error(t, AdoptPluginDir(src, target))
	assert.NoDirExists(t, filepath.Join(target, "demo"), "no destination appears")
	assert.Empty(t, hiddenEntries(t, target), "the hidden directory is removed")
	assert.FileExists(t, filepath.Join(src, "kapi-demo"), "the original stays")
}

// A run killed between the copy and the rename leaves a hidden directory.
// Discovery does not take it for a plugin, and the next adoption removes it
// before anything else and completes.
func TestAdoptPluginDir_ALeftoverFromAKilledRunIsRemoved(t *testing.T) {
	src := stagePlugin(t, privateRoot(t), "demo")
	target := privateRoot(t)
	leftover := filepath.Join(target, adoptTempPrefix+"demo-123")
	require.NoError(t, os.Mkdir(leftover, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(leftover, "manifest.json"), []byte(`{"plugin":"demo"`), 0o644))
	assert.Empty(t, Discover(DiscoverOptions{EnvPluginsDir: target, OnlyEnvDir: true}), "a hidden directory is no plugin")

	require.NoError(t, AdoptPluginDir(src, target))
	assert.NoDirExists(t, leftover)
	assert.FileExists(t, filepath.Join(target, "demo", "kapi-demo"))
}

// A usable plugin already in place wins; an empty directory in its place does
// not, and is replaced by the working plugin.
func TestAdoptPluginDir_OnlyAUsablePluginWins(t *testing.T) {
	target := privateRoot(t)
	stagePlugin(t, target, "demo")
	err := AdoptPluginDir(stagePlugin(t, privateRoot(t), "demo"), target)
	require.ErrorIs(t, err, ErrAlreadyInstalled)

	empty := privateRoot(t)
	require.NoError(t, os.Mkdir(filepath.Join(empty, "demo"), 0o700))
	require.NoError(t, AdoptPluginDir(stagePlugin(t, privateRoot(t), "demo"), empty))
	assert.FileExists(t, filepath.Join(empty, "demo", "kapi-demo"), "the empty directory gave way")
}
