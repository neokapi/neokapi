package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/pluginhost"
)

// A plugin is installed only where no other user can replace it: a target
// directory kapi creates is the user's alone, and one that group or others
// can write is refused.
func TestInstallFromRegistry_InstallsOnlyIntoAPrivateDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	withIsolatedXDG(t)
	server, _ := servePluginRegistry(t, "demo", map[string]string{"1.0.0": "stable"})

	created := filepath.Join(t.TempDir(), "plugins")
	_, err := InstallPluginFromRegistry(context.Background(), pluginhost.InstallOptions{
		IndexURL: server.URL + "/plugins.json", PluginName: "demo", TargetDir: created, Unsafe: true,
	})
	require.NoError(t, err)
	info, err := os.Stat(created)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "kapi creates the plugin root for the user alone")

	shared := filepath.Join(t.TempDir(), "shared")
	require.NoError(t, os.Mkdir(shared, 0o777))
	require.NoError(t, os.Chmod(shared, 0o777))
	_, err = InstallPluginFromRegistry(context.Background(), pluginhost.InstallOptions{
		IndexURL: server.URL + "/plugins.json", PluginName: "demo", TargetDir: shared, Unsafe: true,
	})
	require.ErrorContains(t, err, "writable by every user")
	assert.NoDirExists(t, filepath.Join(shared, "demo"), "nothing is unpacked there")
}

// With no per-user data directory there is no install target, and the
// refusal names the variable that sets one; nothing falls back to a shared
// temporary directory.
func TestInstallTarget_RefusesWithoutAPerUserDirectory(t *testing.T) {
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")
	_, err := pluginhost.InstallTarget()
	require.ErrorContains(t, err, "set XDG_DATA_HOME")
}
