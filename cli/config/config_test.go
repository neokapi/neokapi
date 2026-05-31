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

func TestGlobalConfigFilePath(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", "/tmp/test-kapi-config")
	assert.Equal(t, "/tmp/test-kapi-config/kapi.yaml", GlobalConfigFilePath())
}

func TestGlobalConfigFilePathCustomApp(t *testing.T) {
	t.Setenv("MYAPP_CONFIG_DIR", "/tmp/test-myapp-config")
	assert.Equal(t, "/tmp/test-myapp-config/myapp.yaml", GlobalConfigFilePath("myapp"))
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

// TestNewAppConfigConfigDirPrecedence verifies the KAPI_CONFIG_DIR path is
// searched before the cwd ("."), so an isolated dir wins over a stray
// kapi.yaml in the working directory.
func TestNewAppConfigConfigDirPrecedence(t *testing.T) {
	// Config in the isolated dir.
	isoDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(isoDir, "kapi.yaml"),
		[]byte("language: iso-lang\n"), 0o644))

	// A competing config in the working directory.
	cwdDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(cwdDir, "kapi.yaml"),
		[]byte("language: cwd-lang\n"), 0o644))

	origWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(cwdDir))
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	t.Setenv("KAPI_CONFIG_DIR", isoDir)
	t.Setenv("KAPI_LANGUAGE", "")

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())

	assert.Equal(t, "iso-lang", cfg.Language(),
		"KAPI_CONFIG_DIR should take precedence over the working directory")
}
