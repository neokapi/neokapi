package backend

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/credentials/providerenv"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	appconfig "github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/credentials"
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

func TestConvergenceModelChoiceUsesResolvedRecipe(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	// A provider key in the developer's environment resolves credentials, and
	// so would answer the question before the recipe does.
	for _, names := range providerenv.Vars {
		for _, n := range names {
			t.Setenv(n, "")
		}
	}
	t.Setenv("KAPI_AI_PROVIDER", "")
	app := NewApp()
	tab, _ := newConvergenceProject(t, app)
	proj := app.getOpenProject(tab.ID).Project
	kind := func() host.AISetupKind { return app.CheckAISetup(tab.ID, "", RunAIOptions{}).Kind }
	assert.Equal(t, host.AISetupNone, kind())
	proj.Defaults.Flow = ""
	assert.Equal(t, host.AISetupChooseModel, kind())
	proj.Defaults.Locales = map[string]project.LocaleDefaults{
		"de-DE": {Tools: map[string]map[string]any{"translate": {"provider": "demo"}}},
	}
	assert.Equal(t, host.AISetupChooseModel, kind())
	proj.Defaults.Locales["fr-FR"] = proj.Defaults.Locales["de-DE"]
	assert.Equal(t, host.AISetupNone, kind())
	proj.Defaults.Locales = nil
	proj.Defaults.TargetLanguages = nil
	assert.Equal(t, host.AISetupNone, kind())
}

// The reviewer's scenario: the recipe names anthropic, no key is saved, and
// the stored default is another provider. A model choice cannot fix that, so
// the desktop is told the key is missing for anthropic, and asking changes no
// stored default.
func TestCheckAISetup_NamedProviderWithoutKey(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	for _, names := range providerenv.Vars {
		for _, n := range names {
			t.Setenv(n, "")
		}
	}
	t.Setenv("KAPI_AI_PROVIDER", "")
	t.Setenv("KAPI_AI_MODEL", "")
	app := NewApp()
	app.aiConfig.Set(appconfig.KeyAIProvider, "ollama")
	app.aiConfig.Set(appconfig.KeyAIModel, "llama3.2:3b")
	tab, _ := newConvergenceProject(t, app)
	proj := app.getOpenProject(tab.ID).Project
	proj.Defaults.Flow = ""
	proj.Defaults.Tools = map[string]map[string]any{"translate": {"provider": "anthropic"}}

	need := app.CheckAISetup(tab.ID, "", RunAIOptions{})
	assert.Equal(t, host.AISetupMissingCredential, need.Kind)
	assert.Equal(t, "anthropic", need.Provider)
	// The desktop says where in the app the key goes, not the CLI's advice.
	assert.Equal(t, "No key for anthropic. Add one in Settings › AI Models, then try again.", need.Message)
	assert.Equal(t, DefaultModelInfo{Provider: "ollama", Model: "llama3.2:3b"}, app.GetDefaultModel())
}

// A desktop run started with an explicit provider is judged with it, as
// `kapi up --provider` is: demo over a preset whose provider has no key needs
// nothing, and the run starts with demo.
func TestCheckAISetup_ExplicitProvider(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	for _, names := range providerenv.Vars {
		for _, n := range names {
			t.Setenv(n, "")
		}
	}
	t.Setenv("KAPI_AI_PROVIDER", "")
	t.Setenv("KAPI_AI_MODEL", "")
	app := NewApp()
	tab, _ := newConvergenceProject(t, app)
	proj := app.getOpenProject(tab.ID).Project
	proj.Defaults.Flow = ""
	proj.Defaults.Tools = map[string]map[string]any{"translate": {"provider": "anthropic"}}

	assert.Equal(t, host.AISetupMissingCredential, app.CheckAISetup(tab.ID, "", RunAIOptions{}).Kind)
	assert.Equal(t, host.AISetupNone, app.CheckAISetup(tab.ID, "", RunAIOptions{Provider: "demo"}).Kind)
	assert.Equal(t, map[string]any{"provider": "demo"}, RunAIOptions{Provider: "demo"}.toolConfig())
	assert.Nil(t, RunAIOptions{}.toolConfig())
}
