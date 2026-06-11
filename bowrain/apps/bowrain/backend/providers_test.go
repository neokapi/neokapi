package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListProviderConfigs_RequiresConnection(t *testing.T) {
	app := NewApp()

	_, err := app.ListProviderConfigs()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server connection")
}

func TestSaveProviderConfig_RequiresConnection(t *testing.T) {
	app := NewApp()

	_, err := app.SaveProviderConfig(SaveProviderRequest{
		Name:         "Test",
		ProviderType: "anthropic",
		Model:        "claude-sonnet-4-20250514",
		APIKey:       "sk-test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server connection")
}

func TestDeleteProviderConfig_RequiresConnection(t *testing.T) {
	app := NewApp()

	err := app.DeleteProviderConfig("some-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server connection")
}

func TestTestProviderConfig_RequiresConnection(t *testing.T) {
	app := NewApp()

	err := app.TestProviderConfig(SaveProviderRequest{
		Name:         "Test",
		ProviderType: "mock",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server connection")
}
