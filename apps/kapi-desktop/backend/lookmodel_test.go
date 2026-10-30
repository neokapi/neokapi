package backend

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// These tests save provider keys, so the keychain is the go-keyring
// in-memory one: no test here reads or writes the developer's keychain.

// countLookups makes the app's deeper-look model resolution count its calls.
// Each call is the real resolution, keychain read included.
func countLookups(app *App) *int {
	n := 0
	real := (&host.App{Credentials: app.credentials, Config: app.aiConfig}).ResolveDeeperLookModel
	app.lookModels.resolve = func(p *project.KapiProject) (string, string, error) {
		n++
		return real(p)
	}
	return &n
}

func lookModelApp(t *testing.T) (app *App, tabID string, lookups *int) {
	t.Helper()
	keyring.MockInit()
	app = deeperLookTestApp(t)
	tabID, _ = harborProject(t, app)
	return app, tabID, countLookups(app)
}

func lookModelOf(t *testing.T, app *App, tabID string) DeeperLookModelDTO {
	t.Helper()
	m, err := app.DeeperLookModel(tabID)
	require.NoError(t, err)
	return *m
}

func saveAnthropicKey(t *testing.T, app *App) string {
	t.Helper()
	return saveAnthropicKeyNamed(t, app, "Anthropic key")
}

func saveAnthropicKeyNamed(t *testing.T, app *App, name string) string {
	t.Helper()
	saved, err := app.SaveProvider(ProviderSaveRequest{
		Name:         name,
		ProviderType: "anthropic",
		APIKey:       "sk-ant-test",
	})
	require.NoError(t, err)
	return saved.ID
}

func TestLookModel_ReadsTheSavedKeyOnceForRepeatedMapViews(t *testing.T) {
	app, tabID, lookups := lookModelApp(t)
	saveAnthropicKey(t, app)

	info, found := aiprovider.ProviderInfoFor(aiprovider.Anthropic)
	require.True(t, found)
	want := DeeperLookModelDTO{Connected: true, Provider: "anthropic", Model: info.DefaultModel}
	for range 5 {
		assert.Equal(t, want, lookModelOf(t, app, tabID))
	}
	assert.Equal(t, 1, *lookups, "five Map views read the saved key once")
}

func TestLookModel_SavingAKeyAsksAgain(t *testing.T) {
	app, tabID, lookups := lookModelApp(t)

	assert.False(t, lookModelOf(t, app, tabID).Connected)
	assert.False(t, lookModelOf(t, app, tabID).Connected)
	assert.Equal(t, 1, *lookups)

	saveAnthropicKey(t, app)
	got := lookModelOf(t, app, tabID)
	assert.True(t, got.Connected, "a key saved in App Settings connects the model without a restart")
	assert.Equal(t, "anthropic", got.Provider)
	assert.Equal(t, 2, *lookups)

	lookModelOf(t, app, tabID)
	assert.Equal(t, 2, *lookups)
}

func TestLookModel_RemovingTheKeyDisconnects(t *testing.T) {
	app, tabID, lookups := lookModelApp(t)
	id := saveAnthropicKey(t, app)
	require.True(t, lookModelOf(t, app, tabID).Connected)

	require.NoError(t, app.DeleteProvider(id))
	assert.Equal(t, DeeperLookModelDTO{}, lookModelOf(t, app, tabID))
	assert.Equal(t, 2, *lookups)
}

func TestLookModel_MarkingADefaultKeyAsksAgain(t *testing.T) {
	app, tabID, lookups := lookModelApp(t)
	first := saveAnthropicKeyNamed(t, app, "Work key")
	saveAnthropicKeyNamed(t, app, "Home key")
	assert.False(t, lookModelOf(t, app, tabID).Connected, "two keys for one provider and no default name no key")

	require.NoError(t, app.SetProviderDefault(first))
	assert.True(t, lookModelOf(t, app, tabID).Connected)
	assert.Equal(t, 2, *lookups)
}

func TestLookModel_ChoosingADefaultModelAsksAgain(t *testing.T) {
	app, tabID, lookups := lookModelApp(t)
	require.False(t, lookModelOf(t, app, tabID).Connected)

	require.NoError(t, app.SelectAIProvider("demo", ""))
	assert.Equal(t,
		DeeperLookModelDTO{Connected: true, Provider: "demo", Model: aiprovider.DemoModelName},
		lookModelOf(t, app, tabID))

	require.NoError(t, app.SetDefaultModel("", ""))
	assert.False(t, lookModelOf(t, app, tabID).Connected)
	assert.Equal(t, 3, *lookups)
}

func TestLookModel_AnOldAnswerIsReadAgain(t *testing.T) {
	app, tabID, lookups := lookModelApp(t)
	saveAnthropicKey(t, app)
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	app.lookModels.now = func() time.Time { return now }

	lookModelOf(t, app, tabID)
	now = now.Add(lookModelTTL - time.Second)
	lookModelOf(t, app, tabID)
	assert.Equal(t, 1, *lookups)

	now = now.Add(2 * time.Second)
	lookModelOf(t, app, tabID)
	assert.Equal(t, 2, *lookups)
}

func TestLookModel_ARefusedDeeperLookAsksAgain(t *testing.T) {
	app, tabID, lookups := lookModelApp(t)
	require.False(t, lookModelOf(t, app, tabID).Connected)

	_, err := app.DeeperLook(tabID)
	require.ErrorIs(t, err, host.ErrNoAIProvider)
	lookModelOf(t, app, tabID)
	assert.Equal(t, 2, *lookups)
}
