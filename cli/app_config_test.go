package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitHonorsCfgFile verifies that an explicit --config / -c file path
// (bound into App.CfgFile) is actually read by Init() rather than silently
// ignored — both as a raw config value and through the real Init codepath
// that applies format-priority overrides to the registry.
func TestInitHonorsCfgFile(t *testing.T) {
	// Isolate from the developer's real ~/.config/kapi and any cwd config so
	// the test only observes the explicit file we pass.
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Chdir(t.TempDir())

	cfgPath := filepath.Join(t.TempDir(), "custom-config.yaml")
	const cfgBody = `language: fr-FR
formats:
  priorities:
    json: 99
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgBody), 0o644))

	a := &App{CfgFile: cfgPath}
	require.NoError(t, a.Init())

	// Raw value from the explicit file is loaded.
	assert.Equal(t, "fr-FR", a.Config.GetString("language"),
		"value from explicit --config file should be loaded")

	// The value flowed through the real Init() codepath: applyFormatPriorities
	// pushed the override into the format registry.
	info := a.FormatReg.FormatInfo(registry.FormatID("json"))
	require.NotNil(t, info, "json format info should exist")
	assert.Equal(t, 99, info.Priority,
		"formats.priorities from explicit --config file should be applied")
}

// TestInitIgnoresSearchPathWhenCfgFileSet verifies that when CfgFile is set,
// the explicit file wins over the fixed-search-path config (KAPI_CONFIG_DIR).
func TestInitIgnoresSearchPathWhenCfgFileSet(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", configDir)
	t.Chdir(t.TempDir())

	// Search-path config (would be picked up without an explicit --config).
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "kapi.yaml"),
		[]byte("language: de-DE\n"), 0o644))

	// Explicit --config file with a different value.
	cfgPath := filepath.Join(t.TempDir(), "explicit.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("language: ja-JP\n"), 0o644))

	a := &App{CfgFile: cfgPath}
	require.NoError(t, a.Init())

	assert.Equal(t, "ja-JP", a.Config.GetString("language"),
		"explicit --config file should win over the search-path config")
}

// TestInitWithoutCfgFileUsesSearchPath verifies the default behavior is
// unchanged when no explicit --config is given: the fixed search path
// (KAPI_CONFIG_DIR) is still honored.
func TestInitWithoutCfgFileUsesSearchPath(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", configDir)
	t.Chdir(t.TempDir())

	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "kapi.yaml"),
		[]byte("language: es-ES\n"), 0o644))

	a := &App{} // no CfgFile
	require.NoError(t, a.Init())

	assert.Equal(t, "es-ES", a.Config.GetString("language"),
		"search-path config should still load when no --config is set")
}
