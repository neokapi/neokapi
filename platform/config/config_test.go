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

func TestServerURLDefault(t *testing.T) {
	cfg := NewAppConfig()
	assert.Equal(t, "http://localhost:8080", cfg.ServerURL())
}

func TestServerURLOverride(t *testing.T) {
	cfg := NewAppConfig()
	cfg.Set("server.url", "https://bowrain.example.com")
	assert.Equal(t, "https://bowrain.example.com", cfg.ServerURL())
}

func TestGlobalConfigFilePath(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", "/tmp/test-kapi-config")
	assert.Equal(t, "/tmp/test-kapi-config/kapi.yaml", GlobalConfigFilePath())
}

func TestSetGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)

	err := SetGlobalConfig("server.url", "https://bowrain.example.com")
	require.NoError(t, err)

	// Verify by reading back.
	cfg := NewAppConfig()
	cfg.v.SetConfigFile(GlobalConfigFilePath())
	require.NoError(t, cfg.v.ReadInConfig())
	assert.Equal(t, "https://bowrain.example.com", cfg.v.GetString("server.url"))
}

func TestConfigLoadNoFile(t *testing.T) {
	cfg := NewAppConfig()
	err := cfg.Load()
	// No config file is fine — should not error.
	assert.NoError(t, err)
}
