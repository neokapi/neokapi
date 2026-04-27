package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestLoadAuthFromEnvVar(t *testing.T) {
	keyring.MockInit()
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
	keyring.MockInit()
	t.Setenv("BOWRAIN_AUTH_TOKEN", "test-token-123")

	auth, err := LoadAuth()
	require.NoError(t, err)
	assert.Equal(t, "test-token-123", auth.AccessToken)
	assert.Empty(t, auth.ServerURL)
}

func TestLoadAuthEnvVarTakesPrecedence(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", dir)
	t.Setenv("BOWRAIN_AUTH_TOKEN", "env-token")
	t.Setenv("BOWRAIN_SERVER_URL", "https://env-server.example.com")

	require.NoError(t, SaveAuth(StoredAuth{
		ServerURL:   "https://disk-server.example.com",
		AccessToken: "disk-token",
	}))

	auth, err := LoadAuth()
	require.NoError(t, err)
	assert.Equal(t, "env-token", auth.AccessToken)
	assert.Equal(t, "https://env-server.example.com", auth.ServerURL)
}

func TestLoadAuthFromKeychain(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", dir)

	require.NoError(t, SaveAuth(StoredAuth{
		ServerURL:    "https://bowrain.example.com",
		AccessToken:  "keychain-access",
		RefreshToken: "keychain-refresh",
	}))

	auth, err := LoadAuth()
	require.NoError(t, err)
	assert.Equal(t, "https://bowrain.example.com", auth.ServerURL)
	assert.Equal(t, "keychain-access", auth.AccessToken)
	assert.Equal(t, "keychain-refresh", auth.RefreshToken)
}

func TestLoadAuthNoMetadataFile(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", dir)

	_, err := LoadAuth()
	assert.Error(t, err)
}

func TestDeleteAuthClearsKeychainAndFile(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", dir)

	require.NoError(t, SaveAuth(StoredAuth{
		ServerURL:    "https://bowrain.example.com",
		AccessToken:  "a",
		RefreshToken: "r",
	}))

	require.NoError(t, DeleteAuth("https://bowrain.example.com"))

	_, err := LoadAuth()
	assert.Error(t, err, "auth metadata should be gone")

	_, err = keyring.Get(keyringService, keyringAccessKey("https://bowrain.example.com"))
	assert.ErrorIs(t, err, keyring.ErrNotFound)
}
