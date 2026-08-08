package host

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInstallPluginFromRegistry_RefusesTombstoned is finding 6: the desktop
// install path did not check the tombstone registry, so it could install a
// plugin kapi has retired. The shared install path refuses it — before any
// network call — pointing at the replacement.
func TestInstallPluginFromRegistry_RefusesTombstoned(t *testing.T) {
	_, err := InstallPluginFromRegistry(context.Background(), pluginhost.InstallOptions{PluginName: "llm"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retired")
	assert.Contains(t, err.Error(), "llm")
}

// TestUpdatePlugin_RefusesTombstoned confirms update goes through the same
// tombstone gate — updating a retired plugin is refused, not reinstalled.
func TestUpdatePlugin_RefusesTombstoned(t *testing.T) {
	_, _, err := UpdatePlugin(context.Background(), "llm", PluginUpdateOverrides{TargetDir: t.TempDir()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retired")
}
