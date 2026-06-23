package config

import (
	"testing"

	"github.com/neokapi/neokapi/core/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A fresh prerelease build with no config and no env override infers the beta
// channel, persists it, and the choice survives a later update to a final
// (non-prerelease) version — so a beta user does not silently fall back to
// stable when the rc becomes the release.
func TestUpdateChannelStickyBeta(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)
	t.Setenv(EnvUpdateChannel, "")

	saved := version.Version
	defer func() { version.Version = saved }()

	// 1. Fresh beta install: prerelease version, no config yet.
	version.Version = "1.2.0-rc.1"
	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "beta", cfg.UpdateChannel(), "fresh prerelease build should infer beta")

	// 2. Pin it (as the CLI/desktop do at startup).
	cfg.EnsureChannelPinned("kapi")

	// 3. Update to the FINAL release: version is no longer a prerelease, so naive
	//    inference would say "stable" — but the persisted preference must win.
	version.Version = "1.2.0"
	after := NewAppConfig()
	require.NoError(t, after.Load())
	assert.True(t, after.Viper().InConfig(KeyUpdateChannel), "channel should be persisted to the config file")
	assert.Equal(t, "beta", after.UpdateChannel(), "beta must stick across the update to a final version")
}

// A stable build pins nothing and defaults to stable.
func TestUpdateChannelStableBuildNoPin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)
	t.Setenv(EnvUpdateChannel, "")

	saved := version.Version
	defer func() { version.Version = saved }()
	version.Version = "1.2.0"

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "stable", cfg.UpdateChannel())
	cfg.EnsureChannelPinned("kapi")

	after := NewAppConfig()
	require.NoError(t, after.Load())
	assert.False(t, after.Viper().InConfig(KeyUpdateChannel), "a stable build must not pin a channel")
	assert.Equal(t, "stable", after.UpdateChannel())
}

// An explicit env override is respected and is NOT persisted (it is ephemeral).
func TestUpdateChannelEnvNotPersisted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)
	t.Setenv(EnvUpdateChannel, "beta")

	saved := version.Version
	defer func() { version.Version = saved }()
	version.Version = "1.2.0-rc.1"

	cfg := NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "beta", cfg.UpdateChannel())
	cfg.EnsureChannelPinned("kapi")

	after := NewAppConfig()
	require.NoError(t, after.Load())
	assert.False(t, after.Viper().InConfig(KeyUpdateChannel), "an env override must not be written to the config file")
}
