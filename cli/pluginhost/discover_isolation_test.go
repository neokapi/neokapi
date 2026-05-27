package pluginhost_test

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OnlyEnvDir (and $KAPI_PLUGINS_DIR_ONLY) restrict discovery to the env dir,
// so an in-repo / dogfood kapi never picks up the developer's (XDG) or the
// machine's (system) globally-installed plugins.
func TestDiscoverRootsOnlyEnvDir(t *testing.T) {
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "") // deterministic regardless of caller env

	opts := pluginhost.DiscoverOptions{
		EnvPluginsDir: "/tmp/devplugins",
		XDGDataHome:   "/tmp/xdg",
		SystemDirs:    []string{"/opt/homebrew/share/kapi/plugins"},
	}

	// Default: env + user (XDG) + system roots.
	full := pluginhost.Roots(opts)
	assert.Greater(t, len(full), 1, "default discovery includes user and system roots")

	// OnlyEnvDir via the option: just the env dir.
	opts.OnlyEnvDir = true
	roots := pluginhost.Roots(opts)
	require.Len(t, roots, 1)
	assert.Equal(t, 1, roots[0].Order)
	assert.Equal(t, filepath.Clean("/tmp/devplugins"), roots[0].Path)

	// OnlyEnvDir via the env var: same effect.
	opts.OnlyEnvDir = false
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	assert.Len(t, pluginhost.Roots(opts), 1)
}
