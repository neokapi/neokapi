package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatPrioritiesEmpty(t *testing.T) {
	cfg := NewAppConfig()
	priorities := cfg.FormatPriorities()
	assert.Empty(t, priorities)
}

func TestFormatPrioritiesFromViper(t *testing.T) {
	cfg := NewAppConfig()
	cfg.Set("formats.priorities.html", 200)
	cfg.Set("formats.priorities.okapi-html", 50)

	priorities := cfg.FormatPriorities()
	require.Len(t, priorities, 2)
	assert.Equal(t, 200, priorities["html"])
	assert.Equal(t, 50, priorities["okapi-html"])
}

func TestFormatPrioritiesHandlesFloat64(t *testing.T) {
	cfg := NewAppConfig()
	// Viper often stores YAML numbers as float64 internally.
	cfg.v.Set("formats.priorities.html", float64(150))

	priorities := cfg.FormatPriorities()
	require.Len(t, priorities, 1)
	assert.Equal(t, 150, priorities["html"])
}

func TestFormatPrioritiesIgnoresNonNumeric(t *testing.T) {
	cfg := NewAppConfig()
	cfg.v.Set("formats.priorities.html", "not-a-number")
	cfg.v.Set("formats.priorities.json", 100)

	priorities := cfg.FormatPriorities()
	// "html" should be ignored since its value is a string.
	require.Len(t, priorities, 1)
	assert.Equal(t, 100, priorities["json"])
}

func TestChannelBufferDefault(t *testing.T) {
	cfg := NewAppConfig()
	assert.Equal(t, 64, cfg.ChannelBuffer())
}

func TestUpdateChannelDefault(t *testing.T) {
	// Without an isolated config dir this reads the developer's real
	// ~/.config/kapi, so the assertion depends on whoever ran it last having
	// stayed on the stable channel.
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())

	cfg := NewAppConfig()
	assert.Equal(t, "stable", cfg.UpdateChannel())
}

func TestUpdateChannelEnvOverride(t *testing.T) {
	t.Setenv("KAPI_UPDATE_CHANNEL", "beta")
	cfg := NewAppConfig()
	assert.Equal(t, "beta", cfg.UpdateChannel())
}

func TestGlobalConfigFilePath(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", "/tmp/test-kapi-config")
	assert.Equal(t, "/tmp/test-kapi-config/kapi.yaml", GlobalConfigFilePath())
}

func TestGlobalConfigFilePathCustomApp(t *testing.T) {
	t.Setenv("MYAPP_CONFIG_DIR", "/tmp/test-myapp-config")
	assert.Equal(t, "/tmp/test-myapp-config/myapp.yaml", GlobalConfigFilePath("myapp"))
}

func TestGlobalConfigFilePathHyphenatedApp(t *testing.T) {
	// Hyphens map to underscores in the env key so hyphenated app names
	// (kapi-desktop) get a legal environment variable.
	t.Setenv("KAPI_DESKTOP_CONFIG_DIR", "/tmp/test-desktop-config")
	assert.Equal(t, "/tmp/test-desktop-config/kapi-desktop.yaml", GlobalConfigFilePath("kapi-desktop"))
}

func TestSetGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)

	err := SetGlobalConfig("flow.channelBuffer", "128")
	require.NoError(t, err)

	// Verify by reading back.
	cfg := NewAppConfig()
	cfg.v.SetConfigFile(GlobalConfigFilePath())
	require.NoError(t, cfg.v.ReadInConfig())
	assert.Equal(t, "128", cfg.v.GetString("flow.channelBuffer"))
}

func TestSetGlobalConfigCustomApp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MYAPP_CONFIG_DIR", dir)

	err := SetGlobalConfig("server.url", "https://example.com", "myapp")
	require.NoError(t, err)

	// Verify by reading back.
	cfg := NewAppConfig()
	cfg.v.SetConfigFile(GlobalConfigFilePath("myapp"))
	require.NoError(t, cfg.v.ReadInConfig())
	assert.Equal(t, "https://example.com", cfg.v.GetString("server.url"))
}

func TestOverlayAppConfig(t *testing.T) {
	cfg := NewOverlayAppConfig("testapp", func(c *AppConfig) {
		c.Set("custom.key", "custom-value")
	})
	assert.Equal(t, "custom-value", cfg.GetString("custom.key"))
}

func TestConfigLoadNoFile(t *testing.T) {
	cfg := NewAppConfig()
	err := cfg.Load()
	// No config file is fine — should not error.
	assert.NoError(t, err)
}

// TestNewAppConfigHonorsConfigDir verifies that NewAppConfig adds
// KAPI_CONFIG_DIR to its search path, so an isolated config dir takes
// precedence over the developer's real ~/.config/kapi. This upholds the
// dogfood isolation contract for the app-config layer.
func TestNewAppConfigHonorsConfigDir(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kapi.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("language: qps-ploc\n"), 0o644))

	t.Setenv("KAPI_CONFIG_DIR", dir)
	// Ensure the env-var prefix doesn't leak a real-home language value.
	t.Setenv("KAPI_LANGUAGE", "")

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	assert.Equal(t, "qps-ploc", cfg.Language(),
		"NewAppConfig should read language from KAPI_CONFIG_DIR, not real home")
}

// TestNewAppConfigIgnoresTheWorkingDirectory is the regression guard for the bug
// behind `kapi models default` being silently ignored: the app config used to be
// resolved through a search path whose first entry was "." — and a kapi
// project's recipe is named kapi.yaml, exactly the name being searched for. So
// inside any project the *recipe* was loaded as the app config, and every stored
// ai.provider / ai.model read as empty.
//
// The working directory is not a config location. A recipe is project
// configuration, committed and per-project; app config is per-machine.
func TestNewAppConfigIgnoresTheWorkingDirectory(t *testing.T) {
	isoDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(isoDir, "kapi.yaml"),
		[]byte("language: iso-lang\n"), 0o644))

	// A project recipe in the working directory — the exact filename the old
	// search path would have picked up first.
	cwdDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(cwdDir, "kapi.yaml"),
		[]byte("version: v1\nname: a-recipe-not-app-config\nlanguage: cwd-lang\n"), 0o644))
	t.Chdir(cwdDir)

	t.Setenv("KAPI_CONFIG_DIR", isoDir)
	t.Setenv("KAPI_LANGUAGE", "")

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	assert.Equal(t, "iso-lang", cfg.Language(),
		"the pinned global config must win; a cwd kapi.yaml is a recipe, not app config")
}

// TestAppConfigRoundTripsAStoredDefault is the founder-reported symptom in its
// smallest form: whatever a writer stores must be readable by a *fresh* reader,
// including from inside a project directory.
func TestAppConfigRoundTripsAStoredDefault(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_AI_PROVIDER", "")
	t.Setenv("KAPI_AI_MODEL", "")

	require.NoError(t, SetGlobalConfig(KeyAIProvider, "ollama"))
	require.NoError(t, SetGlobalConfig(KeyAIModel, "gemma4:e2b"))

	// Stand inside a project — the case that used to lose the value.
	projDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projDir, "kapi.yaml"),
		[]byte("version: v1\nname: proj\n"), 0o644))
	t.Chdir(projDir)

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "ollama", cfg.GetString(KeyAIProvider))
	assert.Equal(t, "gemma4:e2b", cfg.GetString(KeyAIModel))
}

// TestAppConfigEnvStillOverridesTheFile keeps the precedence order: env beats
// the stored file, which beats the built-in default.
func TestAppConfigEnvStillOverridesTheFile(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	require.NoError(t, SetGlobalConfig(KeyAIProvider, "ollama"))
	t.Setenv("KAPI_AI_PROVIDER", "anthropic")

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "anthropic", cfg.GetString(KeyAIProvider))
}

// TestGlobalConfigFilePathIsolatesPluginNamespaces asserts KAPI_CONFIG_DIR
// contains plugin config files too. `kapi config set bowrain.…` writes a
// plugin's file, so an isolated run (the in-repo dogfood contract) must keep
// that write inside the pinned root rather than reaching into the real home.
func TestGlobalConfigFilePathIsolatesPluginNamespaces(t *testing.T) {
	iso := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", iso)
	t.Setenv("BOWRAIN_CONFIG_DIR", "")

	assert.Equal(t, filepath.Join(iso, "kapi.yaml"), GlobalConfigFilePath())
	assert.Equal(t, filepath.Join(iso, "bowrain", "bowrain.yaml"), GlobalConfigFilePath("bowrain"),
		"a plugin namespace must nest under the pinned kapi config root")
}

// TestGlobalConfigFilePathAppOverrideWins keeps the per-app override ahead of
// the kapi root, so an app can still be pointed anywhere explicitly.
func TestGlobalConfigFilePathAppOverrideWins(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	appDir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", appDir)
	assert.Equal(t, filepath.Join(appDir, "bowrain.yaml"), GlobalConfigFilePath("bowrain"))
}

// TestUnsetGlobalConfigPrunesEmptyParents: removing the last leaf of a nested
// block must not leave a hollow `ai: {}` behind.
func TestUnsetGlobalConfigPrunesEmptyParents(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())

	require.NoError(t, SetGlobalConfig(KeyAIProvider, "ollama"))
	require.NoError(t, SetGlobalConfig(KeyAIModel, "gemma4:e2b"))

	values, err := GlobalConfigValues()
	require.NoError(t, err)
	assert.Equal(t, "ollama", values[KeyAIProvider])
	assert.Equal(t, "gemma4:e2b", values[KeyAIModel])

	changed, err := UnsetGlobalConfig(KeyAIModel)
	require.NoError(t, err)
	assert.True(t, changed)

	values, err = GlobalConfigValues()
	require.NoError(t, err)
	assert.Equal(t, "ollama", values[KeyAIProvider], "the sibling key survives")
	assert.NotContains(t, values, KeyAIModel)

	changed, err = UnsetGlobalConfig(KeyAIProvider)
	require.NoError(t, err)
	assert.True(t, changed)

	raw, err := os.ReadFile(GlobalConfigFilePath())
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "ai:", "the empty parent block is pruned")
}

// TestUnsetGlobalConfigMissingKeyIsNotAnError: unsetting what was never set is
// a no-op that reports itself, not a failure.
func TestUnsetGlobalConfigMissingKeyIsNotAnError(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	changed, err := UnsetGlobalConfig("ai.model")
	require.NoError(t, err)
	assert.False(t, changed)

	require.NoError(t, SetGlobalConfig(KeyAIProvider, "ollama"))
	changed, err = UnsetGlobalConfig("ai.model")
	require.NoError(t, err)
	assert.False(t, changed)
}
