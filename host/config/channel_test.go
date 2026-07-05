package config

import (
	"testing"

	"github.com/neokapi/neokapi/core/channel"
	"github.com/neokapi/neokapi/core/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An explicit kapi.yaml update.channel wins and is left untouched by the pin.
func TestUpdateChannelExplicitConfigWins(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(channel.Env, "")
	saved := version.Version
	defer func() { version.Version = saved }()
	version.Version = "1.2.0" // stable build

	require.NoError(t, SetGlobalConfig(KeyUpdateChannel, "beta", "kapi"))
	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "beta", cfg.UpdateChannel(), "explicit kapi.yaml update.channel should win")

	cfg.EnsureChannelPinned("kapi")
	assert.Empty(t, channel.Persisted(), "an explicit kapi.yaml choice must not also pin the shared file")
}

// With no kapi.yaml entry, a prerelease build pins the shared channel preference,
// which the CLI then resolves through — sticky across a later final release.
func TestUpdateChannelPinsSharedPreference(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(channel.Env, "")
	saved := version.Version
	defer func() { version.Version = saved }()

	version.Version = "1.2.0-rc.1"
	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "beta", cfg.UpdateChannel(), "fresh prerelease build infers beta")

	cfg.EnsureChannelPinned("kapi")
	assert.Equal(t, "beta", channel.Persisted(), "beta pinned to the shared preference")

	// Update to the final release: kapi.yaml still has no entry, but the shared
	// pin keeps the CLI on beta.
	version.Version = "1.2.0"
	after := NewAppConfig()
	require.NoError(t, after.Load())
	assert.Equal(t, "beta", after.UpdateChannel(), "beta sticks across the update to a final version")
}
