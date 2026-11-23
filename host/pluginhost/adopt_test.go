//go:build !windows && !js

package pluginhost

import (
	"os"
	"path/filepath"
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
