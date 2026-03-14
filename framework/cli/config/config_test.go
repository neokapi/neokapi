package config

import (
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
