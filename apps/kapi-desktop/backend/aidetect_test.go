package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "github.com/neokapi/neokapi/host/config"
)

// stubClaudeBinary points KAPI_CLAUDE_CODE_BIN at a real executable so
// detection reports Claude Code without touching any real CLI.
func stubClaudeBinary(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("KAPI_CLAUDE_CODE_BIN", path)
}

func clearKeyEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "AZURE_OPENAI_API_KEY"} {
		t.Setenv(v, "")
	}
}

func TestDetectAIProviders_ClaudeCodeDetected(t *testing.T) {
	app := testAIApp(t)
	stubClaudeBinary(t)
	clearKeyEnv(t)

	res := app.DetectAIProviders()
	require.NotEmpty(t, res.Detected)
	cc := res.Detected[0]
	assert.Equal(t, "claude-code", cc.Provider)
	assert.Equal(t, "Claude Code", cc.Label)
	assert.Equal(t, "sonnet", cc.Model)
	assert.True(t, cc.Subscription)
	assert.Contains(t, cc.Detail, "Claude subscription")
	assert.False(t, res.Configured, "detection alone is not configuration")
}

func TestDetectAIProviders_NothingDetected(t *testing.T) {
	app := testAIApp(t)
	t.Setenv("KAPI_CLAUDE_CODE_BIN", filepath.Join(t.TempDir(), "missing"))
	clearKeyEnv(t)

	res := app.DetectAIProviders()
	for _, d := range res.Detected {
		assert.NotEqual(t, "claude-code", d.Provider)
	}
	assert.False(t, res.Configured)
}

func TestSelectAIProvider_PersistsDefault(t *testing.T) {
	app := testAIApp(t)

	require.NoError(t, app.SelectAIProvider("claude-code", ""))
	got := app.GetDefaultModel()
	assert.Equal(t, "claude-code", got.Provider)
	assert.Equal(t, "sonnet", got.Model, "empty model falls back to the provider default")

	require.NoError(t, app.SelectAIProvider("ollama", "gemma3:4b"))
	got = app.GetDefaultModel()
	assert.Equal(t, "ollama", got.Provider)
	assert.Equal(t, "gemma3:4b", got.Model)

	// The runner-facing config is configured after selection.
	assert.Equal(t, "ollama", app.aiConfig.GetString(appconfig.KeyAIProvider))

	require.Error(t, app.SelectAIProvider("not-a-provider", ""))
	require.Error(t, app.SelectAIProvider("", ""))
}

func TestListAIModels_ClaudeCodeListedFirstWhenDetected(t *testing.T) {
	app := testAIApp(t)
	stubClaudeBinary(t)

	models := app.ListAIModels()
	require.NotEmpty(t, models)
	first := models[0]
	assert.Equal(t, "claude-code", first.Provider)
	assert.True(t, first.Subscription)
	assert.False(t, first.NeedsKey)

	// claude-code never shows under the needs-key cloud section.
	for _, m := range models[1:] {
		assert.NotEqual(t, "claude-code", m.Provider)
	}
}

func TestListProviderTypes_KeylessFlags(t *testing.T) {
	app := testAIApp(t)
	byName := map[string]ProviderTypeInfo{}
	for _, p := range app.ListProviderTypes() {
		byName[p.Name] = p
	}
	cc, ok := byName["claude-code"]
	require.True(t, ok, "claude-code is a registered provider type")
	assert.True(t, cc.Keyless)
	assert.True(t, cc.Subscription)
	assert.False(t, cc.Local, "claude-code egresses content — not local")
	assert.True(t, byName["ollama"].Keyless)
	assert.False(t, byName["anthropic"].Keyless)
}
