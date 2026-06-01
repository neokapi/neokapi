package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/credentials"
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
		{name: "deepl MT accepted", provider: "deepl"},
		{name: "google MT accepted", provider: "google"},
		{name: "microsoft MT accepted", provider: "microsoft"},
		{name: "modernmt MT accepted", provider: "modernmt"},
		{name: "mymemory MT accepted", provider: "mymemory"},
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
			err := validateProviderType(tc.provider)
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
	known := knownProviderTypes()

	// The list is sorted and deduplicated.
	require.True(t, len(known) > 0, "expected at least one known provider")
	for i := 1; i < len(known); i++ {
		assert.Less(t, known[i-1], known[i], "expected sorted, deduplicated providers")
	}

	// Both AI and MT canonical providers are present.
	for _, want := range []string{"anthropic", "openai", "gemini", "azureopenai", "ollama", "demo", "deepl", "google", "microsoft", "modernmt", "mymemory"} {
		assert.Contains(t, known, want, "expected %q in known providers", want)
	}
}

func TestCredentialsAddCmd_RejectsUnknownProvider(t *testing.T) {
	app := newCredTestApp(t)
	cmd := app.newCredentialsAddCmd()
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
	cmd := app.newCredentialsAddCmd()
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
	cmd := app.newCredentialsAddCmd()
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
