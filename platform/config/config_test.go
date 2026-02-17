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

func TestConfigLoadNoFile(t *testing.T) {
	cfg := NewAppConfig()
	err := cfg.Load()
	// No config file is fine — should not error.
	assert.NoError(t, err)
}
