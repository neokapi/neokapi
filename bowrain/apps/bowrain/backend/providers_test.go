package backend

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/gokapi/gokapi/bowrain/credentials"
)

func testApp(t *testing.T) *App {
	t.Helper()
	keyring.MockInit()
	app := NewApp()
	storePath := filepath.Join(t.TempDir(), "providers.json")
	app.credentials = credentials.NewStore(storePath)
	return app
}

func TestSaveProviderConfig(t *testing.T) {
	app := testApp(t)

	saved, err := app.SaveProviderConfig(SaveProviderRequest{
		Name:         "Test Anthropic",
		ProviderType: "anthropic",
		Model:        "claude-sonnet-4-20250514",
		APIKey:       "sk-ant-test-123",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, saved.ID)
	assert.Equal(t, "Test Anthropic", saved.Name)

	// Verify in list
	configs := app.ListProviderConfigs()
	require.Len(t, configs, 1)
	assert.Equal(t, saved.ID, configs[0].ID)

	// Verify API key stored
	key, err := app.credentials.GetAPIKey(saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-test-123", key)
}

func TestSaveProviderConfigEmptyKey(t *testing.T) {
	app := testApp(t)

	// First save with key
	saved, err := app.SaveProviderConfig(SaveProviderRequest{
		Name:         "OpenAI",
		ProviderType: "openai",
		Model:        "gpt-4o",
		APIKey:       "sk-original",
	})
	require.NoError(t, err)

	// Update without key (should keep existing)
	_, err = app.SaveProviderConfig(SaveProviderRequest{
		ID:           saved.ID,
		Name:         "OpenAI Updated",
		ProviderType: "openai",
		Model:        "gpt-4o-mini",
	})
	require.NoError(t, err)

	// API key should still be the original
	key, err := app.credentials.GetAPIKey(saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "sk-original", key)

	// Name should be updated
	configs := app.ListProviderConfigs()
	require.Len(t, configs, 1)
	assert.Equal(t, "OpenAI Updated", configs[0].Name)
}

func TestDeleteProviderConfig(t *testing.T) {
	app := testApp(t)

	saved, err := app.SaveProviderConfig(SaveProviderRequest{
		Name:         "ToDelete",
		ProviderType: "ollama",
		APIKey:       "key-to-delete",
	})
	require.NoError(t, err)

	err = app.DeleteProviderConfig(saved.ID)
	require.NoError(t, err)

	assert.Empty(t, app.ListProviderConfigs())

	// Deleting non-existent returns error
	err = app.DeleteProviderConfig("no-such-id")
	assert.Error(t, err)
}

func TestTestProviderConfig(t *testing.T) {
	app := testApp(t)

	// Mock provider will succeed
	err := app.TestProviderConfig(SaveProviderRequest{
		Name:         "Mock",
		ProviderType: "mock",
	})
	require.NoError(t, err)
}
