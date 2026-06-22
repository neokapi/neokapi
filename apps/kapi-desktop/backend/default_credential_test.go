package backend

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAppWithDefault builds a minimal App with a credential store and a
// settings store whose default credential is `defaultID`.
func testAppWithDefault(t *testing.T, defaultID string) *App {
	t.Helper()
	return &App{
		credentials: credentials.NewStore(filepath.Join(t.TempDir(), "creds.json")),
		settings: &settingsStore{
			filePath: filepath.Join(t.TempDir(), "settings.json"),
			settings: AppSettings{DefaultCredentialID: defaultID},
		},
	}
}

func TestApplyDefaultCredential_InjectsWhenUnpinned(t *testing.T) {
	app := testAppWithDefault(t, "")
	cfg, err := app.credentials.Upsert(credentials.ProviderConfig{Name: "My Gemini Key", ProviderType: "gemini"})
	require.NoError(t, err)
	app.SetDefaultCredential(cfg.ID)

	out := app.applyDefaultCredential([]string{"credentials"}, map[string]any{})
	assert.Equal(t, cfg.ID, out["credential"], "default credential should be injected when the step pins nothing")
}

func TestApplyDefaultCredential_LeavesPinnedStepUntouched(t *testing.T) {
	app := testAppWithDefault(t, "")
	cfg, err := app.credentials.Upsert(credentials.ProviderConfig{Name: "My Gemini Key", ProviderType: "gemini"})
	require.NoError(t, err)
	app.SetDefaultCredential(cfg.ID)

	for _, pinned := range []map[string]any{
		{"credential": "some-other"},
		{"provider": "openai"},
		{"apiKey": "sk-inline"},
	} {
		out := app.applyDefaultCredential([]string{"credentials"}, pinned)
		_, injected := out["credential"]
		if _, hadCred := pinned["credential"]; !hadCred {
			assert.False(t, injected, "must not inject default over a pinned provider/apiKey: %v", pinned)
		}
	}
}

func TestApplyDefaultCredential_NoDefaultSet(t *testing.T) {
	app := testAppWithDefault(t, "")
	out := app.applyDefaultCredential([]string{"credentials"}, map[string]any{})
	_, injected := out["credential"]
	assert.False(t, injected, "no default set → config unchanged")
}

func TestApplyDefaultCredential_StaleDefaultFallsThrough(t *testing.T) {
	// Default points at a credential that no longer exists in the store.
	app := testAppWithDefault(t, "deleted-id")
	out := app.applyDefaultCredential([]string{"credentials"}, map[string]any{})
	_, injected := out["credential"]
	assert.False(t, injected, "stale default → fall through to auto-detect, do not inject")
}

func TestApplyDefaultCredential_NonCredentialToolUntouched(t *testing.T) {
	app := testAppWithDefault(t, "")
	cfg, err := app.credentials.Upsert(credentials.ProviderConfig{Name: "My Gemini Key", ProviderType: "gemini"})
	require.NoError(t, err)
	app.SetDefaultCredential(cfg.ID)

	out := app.applyDefaultCredential([]string{"target-language"}, map[string]any{})
	_, injected := out["credential"]
	assert.False(t, injected, "tool that doesn't require credentials → unchanged")
}
