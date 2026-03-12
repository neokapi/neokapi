package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAuthFromEnvVar(t *testing.T) {
	t.Setenv("BOWRAIN_AUTH_TOKEN", "test-token-123")
	t.Setenv("BOWRAIN_SERVER_URL", "https://bowrain.example.com")

	auth, err := LoadAuth()
	require.NoError(t, err)
	assert.Equal(t, "test-token-123", auth.AccessToken)
	assert.Equal(t, "https://bowrain.example.com", auth.ServerURL)
	assert.Empty(t, auth.RefreshToken)
	assert.True(t, auth.Expiry.IsZero())
}

func TestLoadAuthFromEnvVarWithoutServerURL(t *testing.T) {
	t.Setenv("BOWRAIN_AUTH_TOKEN", "test-token-123")

	auth, err := LoadAuth()
	require.NoError(t, err)
	assert.Equal(t, "test-token-123", auth.AccessToken)
	assert.Empty(t, auth.ServerURL)
}

func TestLoadAuthEnvVarTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", dir)
	t.Setenv("BOWRAIN_AUTH_TOKEN", "env-token")
	t.Setenv("BOWRAIN_SERVER_URL", "https://env-server.example.com")

	// Save auth to disk.
	require.NoError(t, SaveAuth(StoredAuth{
		ServerURL:   "https://disk-server.example.com",
		AccessToken: "disk-token",
	}))

	// Env var should take precedence.
	auth, err := LoadAuth()
	require.NoError(t, err)
	assert.Equal(t, "env-token", auth.AccessToken)
	assert.Equal(t, "https://env-server.example.com", auth.ServerURL)
}

func TestLoadAuthFromDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", dir)

	require.NoError(t, SaveAuth(StoredAuth{
		ServerURL:   "https://bowrain.example.com",
		AccessToken: "disk-token",
	}))

	auth, err := LoadAuth()
	require.NoError(t, err)
	assert.Equal(t, "disk-token", auth.AccessToken)
	assert.Equal(t, "https://bowrain.example.com", auth.ServerURL)
}

func TestLoadAuthNoDiskFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", dir)

	_, err := LoadAuth()
	assert.Error(t, err)
}
