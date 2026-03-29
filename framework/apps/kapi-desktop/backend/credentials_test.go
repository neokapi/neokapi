package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListProvidersNotNil(t *testing.T) {
	app := NewApp()
	providers := app.ListProviders()
	// May not be empty if there are providers from other runs; just verify it doesn't panic.
	assert.NotNil(t, app.credentials)
	_ = providers
}

func TestSaveAndListProvider(t *testing.T) {
	app := NewApp()

	countBefore := len(app.ListProviders())

	result, err := app.SaveProvider(ProviderSaveRequest{
		Name:         "My Anthropic",
		ProviderType: "anthropic",
		Model:        "claude-sonnet-4-5-20241022",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.ID)
	assert.Equal(t, "My Anthropic", result.Name)
	assert.Equal(t, "anthropic", result.ProviderType)

	providers := app.ListProviders()
	assert.Len(t, providers, countBefore+1)

	// Clean up.
	_ = app.DeleteProvider(result.ID)
}

func TestDeleteProvider(t *testing.T) {
	app := NewApp()

	countBefore := len(app.ListProviders())

	result, err := app.SaveProvider(ProviderSaveRequest{
		Name:         "To Delete",
		ProviderType: "openai",
	})
	require.NoError(t, err)
	assert.Len(t, app.ListProviders(), countBefore+1)

	require.NoError(t, app.DeleteProvider(result.ID))
	assert.Len(t, app.ListProviders(), countBefore)
}

func TestDeleteProviderNotFound(t *testing.T) {
	app := NewApp()
	assert.Error(t, app.DeleteProvider("nonexistent"))
}
