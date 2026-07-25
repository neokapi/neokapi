package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// appWithNamespace builds an App whose plugin host declares one config
// namespace, so the routing can be exercised without a real plugin binary.
func appWithNamespace(t *testing.T) *App {
	t.Helper()
	p := &pluginhost.Plugin{
		Manifest: &manifest.Manifest{
			Plugin: "bowrain",
			Capabilities: manifest.Capabilities{
				ConfigNamespaces: []manifest.ConfigNamespace{{
					Prefix:      "bowrain",
					App:         "bowrain",
					Description: "Per-machine Bowrain defaults",
					Keys: []manifest.ConfigKey{
						{Key: "server.url", Description: "Default Bowrain server URL"},
					},
				}},
			},
		},
	}
	return &App{
		Config:     config.NewAppConfig(),
		PluginHost: pluginhost.NewHost([]*pluginhost.Plugin{p}, nil),
	}
}

// TestConfigSetRoutesNamespacedKeyToThePlugin is the core of the one-config-verb
// model: a namespaced key is written to the plugin's own file (with the prefix
// stripped), which is exactly where the plugin already reads it from — so
// retiring the per-plugin config command loses nothing.
func TestConfigSetRoutesNamespacedKeyToThePlugin(t *testing.T) {
	iso := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", iso)
	t.Setenv("BOWRAIN_CONFIG_DIR", "")
	a := appWithNamespace(t)

	res, err := a.ConfigSet("bowrain.server.url", "https://app.bowrain.cloud")
	require.NoError(t, err)
	assert.Equal(t, "bowrain", res.Entry.Scope)
	assert.Equal(t, filepath.Join(iso, "bowrain", "bowrain.yaml"), res.Entry.File)

	// The plugin's own reader must see it under the un-prefixed key.
	stored, err := config.GlobalConfigValues("bowrain")
	require.NoError(t, err)
	assert.Equal(t, "https://app.bowrain.cloud", stored["server.url"])
	assert.NotContains(t, stored, "bowrain.server.url", "the prefix is a routing token, not part of the key")

	got, err := a.ConfigGet("bowrain.server.url")
	require.NoError(t, err)
	assert.Equal(t, "https://app.bowrain.cloud", got.Entry.Value)
	assert.Equal(t, "Default Bowrain server URL", got.Entry.Description,
		"a manifest-documented key carries its description")
}

// TestConfigUnsetRoutesNamespacedKey: unset crosses the namespace boundary too.
func TestConfigUnsetRoutesNamespacedKey(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("BOWRAIN_CONFIG_DIR", "")
	a := appWithNamespace(t)

	_, err := a.ConfigSet("bowrain.server.url", "https://app.bowrain.cloud")
	require.NoError(t, err)

	res, err := a.ConfigUnset("bowrain.server.url")
	require.NoError(t, err)
	assert.True(t, res.Changed)

	got, err := a.ConfigGet("bowrain.server.url")
	require.NoError(t, err)
	assert.Empty(t, got.Entry.Value)
}

// TestConfigListSpansEveryNamespace: one listing covers kapi's own keys and
// each installed plugin's, each spelled the way `kapi config set` takes it back.
func TestConfigListSpansEveryNamespace(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("BOWRAIN_CONFIG_DIR", "")
	a := appWithNamespace(t)

	require.NoError(t, config.SetGlobalConfig(config.KeyAIProvider, "ollama"))
	_, err := a.ConfigSet("bowrain.server.url", "https://app.bowrain.cloud")
	require.NoError(t, err)

	out, err := a.ConfigList()
	require.NoError(t, err)

	byKey := map[string]ConfigEntry{}
	for _, e := range out.Entries {
		byKey[e.Key] = e
	}
	require.Contains(t, byKey, "ai.provider")
	assert.Equal(t, "app", byKey["ai.provider"].Scope)
	require.Contains(t, byKey, "bowrain.server.url")
	assert.Equal(t, "bowrain", byKey["bowrain.server.url"].Scope)
	assert.Equal(t, "https://app.bowrain.cloud", byKey["bowrain.server.url"].Value)
}

// TestConfigUnknownPrefixStaysAppScoped: a dotted key whose first segment names
// no installed plugin is kapi's own, not an error — `ai.provider` must not be
// mistaken for a namespace.
func TestConfigUnknownPrefixStaysAppScoped(t *testing.T) {
	iso := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", iso)
	a := appWithNamespace(t)

	res, err := a.ConfigSet("ai.provider", "ollama")
	require.NoError(t, err)
	assert.Equal(t, "app", res.Entry.Scope)
	assert.Equal(t, filepath.Join(iso, "kapi.yaml"), res.Entry.File)
}

// TestConfigPathNamespace: `kapi config path bowrain` names the plugin's file.
func TestConfigPathNamespace(t *testing.T) {
	iso := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", iso)
	t.Setenv("BOWRAIN_CONFIG_DIR", "")
	a := appWithNamespace(t)

	assert.Equal(t, filepath.Join(iso, "kapi.yaml"), a.ConfigPath("").Entry.File)
	assert.Equal(t, filepath.Join(iso, "bowrain", "bowrain.yaml"), a.ConfigPath("bowrain").Entry.File)
}

// TestConfigNamespaceHint gives an error message something true to point at.
func TestConfigNamespaceHint(t *testing.T) {
	assert.Equal(t, "bowrain.*", appWithNamespace(t).ConfigNamespaceHint())
	assert.Empty(t, (&App{}).ConfigNamespaceHint())
}
