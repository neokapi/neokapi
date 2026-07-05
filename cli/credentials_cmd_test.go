package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/host/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// newCredTestApp returns an App with a Credentials store backed by a throwaway
// file under t.TempDir(), so credentials-add tests never touch the developer's
// real ~/.config/kapi/providers.json.
func newCredTestApp(t *testing.T) *App {
	t.Helper()
	// Use an in-memory keyring: SetAPIKey writes to the OS keychain via
	// go-keyring, and CI's headless Linux runner has no D-Bus secret service
	// (org.freedesktop.secrets), so a real write fails there. MockInit keeps the
	// API-key path exercised without depending on a desktop keychain.
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "providers.json")
	return &App{Credentials: credentials.NewStore(path)}
}

func TestValidateProviderType(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		wantErr      bool
		wantContains []string
	}{
		{name: "anthropic accepted", provider: "anthropic"},
		{name: "openai accepted", provider: "openai"},
		{name: "gemini accepted", provider: "gemini"},
		{name: "azureopenai accepted", provider: "azureopenai"},
		{name: "ollama accepted", provider: "ollama"},
		{name: "demo accepted", provider: "demo"},
		{name: "case insensitive", provider: "Anthropic"},
		{name: "surrounding whitespace tolerated", provider: "  openai  "},
		{
			name:         "typo rejected with helpful message",
			provider:     "anthorpic",
			wantErr:      true,
			wantContains: []string{"anthorpic", "valid providers", "anthropic"},
		},
		{
			name:         "empty string rejected",
			provider:     "",
			wantErr:      true,
			wantContains: []string{"valid providers"},
		},
		{
			name:         "arbitrary garbage rejected",
			provider:     "not-a-provider",
			wantErr:      true,
			wantContains: []string{"not-a-provider", "valid providers"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateProviderType(tc.provider)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range tc.wantContains {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestKnownProviderTypes(t *testing.T) {
	known := KnownProviderTypes()

	// The list is sorted and deduplicated.
	require.NotEmpty(t, known, "expected at least one known provider")
	for i := 1; i < len(known); i++ {
		assert.Less(t, known[i-1], known[i], "expected sorted, deduplicated providers")
	}

	// The canonical AI providers are present (classic MT engines are no longer
	// built in; plugin-registered AI providers surface automatically).
	for _, want := range []string{"anthropic", "openai", "gemini", "azureopenai", "ollama", "demo"} {
		assert.Contains(t, known, want, "expected %q in known providers", want)
	}
	for _, gone := range []string{"deepl", "modernmt", "mymemory"} {
		assert.NotContains(t, known, gone, "classic MT engine %q must not be a built-in credential type", gone)
	}
}

func TestCredentialsAddCmd_RejectsUnknownProvider(t *testing.T) {
	app := newCredTestApp(t)
	cmd := newCredentialsAddCmd(app)
	cmd.SetArgs([]string{"my-cred", "--provider", "anthorpic", "--api-key", "sk-test"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
	assert.Contains(t, err.Error(), "anthorpic")

	// Nothing should have been persisted for the typo'd provider.
	_, getErr := app.Credentials.GetByName("my-cred")
	assert.Error(t, getErr, "typo'd provider must not be persisted")
}

func TestCredentialsAddCmd_AcceptsKnownProvider(t *testing.T) {
	app := newCredTestApp(t)
	cmd := newCredentialsAddCmd(app)
	cmd.SetArgs([]string{"my-openai", "--provider", "openai", "--api-key", "sk-test"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	require.NoError(t, cmd.Execute())

	cfg, err := app.Credentials.GetByName("my-openai")
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.ProviderType)
}

func TestCredentialsAddCmd_AcceptsKnownProviderCaseInsensitive(t *testing.T) {
	app := newCredTestApp(t)
	cmd := newCredentialsAddCmd(app)
	// ollama needs no API key.
	cmd.SetArgs([]string{"my-ollama", "--provider", "Ollama"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	require.NoError(t, cmd.Execute())

	cfg, err := app.Credentials.GetByName("my-ollama")
	require.NoError(t, err)
	// The provider string is persisted as supplied; validation is case-insensitive.
	assert.Equal(t, "Ollama", cfg.ProviderType)
}
