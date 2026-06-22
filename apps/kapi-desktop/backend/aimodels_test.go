package backend

import (
	"path/filepath"
	"testing"

	appconfig "github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/cli/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAIApp builds a minimal App with an isolated kapi config dir and an empty
// credential store, so SetDefaultModel writes to a throwaway kapi.yaml.
func testAIApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	return &App{
		aiConfig:    appconfig.NewAppConfig(),
		credentials: credentials.NewStore(filepath.Join(t.TempDir(), "creds.json")),
	}
}

func TestSetDefaultModel_InfersProviderAndRoundTrips(t *testing.T) {
	app := testAIApp(t)

	require.NoError(t, app.SetDefaultModel("gemma3:4b", ""))
	got := app.GetDefaultModel()
	assert.Equal(t, "ollama", got.Provider, "an ollama tag infers the local provider")
	assert.Equal(t, "gemma3:4b", got.Model)

	require.NoError(t, app.SetDefaultModel("claude-sonnet-4-20250514", ""))
	assert.Equal(t, "anthropic", app.GetDefaultModel().Provider)

	// Explicit provider override (Azure shares gpt-* names with OpenAI).
	require.NoError(t, app.SetDefaultModel("gpt-4o", "azureopenai"))
	got = app.GetDefaultModel()
	assert.Equal(t, "azureopenai", got.Provider)
	assert.Equal(t, "gpt-4o", got.Model)
}

func TestSetDefaultModel_UninferableErrors(t *testing.T) {
	app := testAIApp(t)
	require.Error(t, app.SetDefaultModel("totally-unknown", ""))
}

func TestSetDefaultModel_ClearsWhenEmpty(t *testing.T) {
	app := testAIApp(t)
	require.NoError(t, app.SetDefaultModel("gemma3:4b", ""))
	require.NoError(t, app.SetDefaultModel("", ""))
	got := app.GetDefaultModel()
	assert.Empty(t, got.Provider)
	assert.Empty(t, got.Model)
}

func TestListAIModels_IncludesRecommendedAndMarksDefault(t *testing.T) {
	app := testAIApp(t)
	require.NoError(t, app.SetDefaultModel("llama3.2:3b", "ollama"))

	models := app.ListAIModels()
	require.NotEmpty(t, models)

	var foundDefault, foundCloudNeedsKey bool
	for _, m := range models {
		if m.IsDefault {
			foundDefault = true
			assert.Equal(t, "llama3.2:3b", m.Model)
			assert.True(t, m.Local)
		}
		if !m.Local && m.NeedsKey {
			foundCloudNeedsKey = true
		}
	}
	assert.True(t, foundDefault, "the configured default must be flagged")
	assert.True(t, foundCloudNeedsKey, "cloud models with no saved key must be flagged needs_key")
}
