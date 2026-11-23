package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCredentialsApp builds an App whose provider store lives in a config
// directory of the test's own. NewApp opens providers.json at its default
// path, and a save or delete rewrites the whole file, so a test that left the
// path to the default once replaced the developer's providers.json with "[]".
func newCredentialsApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)
	return NewApp(), filepath.Join(dir, "providers.json")
}

func TestListProvidersNotNil(t *testing.T) {
	app, _ := newCredentialsApp(t)
	assert.NotNil(t, app.credentials)
	assert.Empty(t, app.ListProviders())
}

func TestSaveAndListProvider(t *testing.T) {
	app, path := newCredentialsApp(t)

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

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "My Anthropic", "the save lands in the test's own providers.json")
}

func TestDeleteProvider(t *testing.T) {
	app, path := newCredentialsApp(t)

	countBefore := len(app.ListProviders())

	result, err := app.SaveProvider(ProviderSaveRequest{
		Name:         "To Delete",
		ProviderType: "openai",
	})
	require.NoError(t, err)
	assert.Len(t, app.ListProviders(), countBefore+1)

	require.NoError(t, app.DeleteProvider(result.ID))
	assert.Len(t, app.ListProviders(), countBefore)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, "[]", string(data), "the delete rewrites the test's own providers.json")
}

func TestDeleteProviderNotFound(t *testing.T) {
	app, _ := newCredentialsApp(t)
	assert.Error(t, app.DeleteProvider("nonexistent"))
}
