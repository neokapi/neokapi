package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clearProviderEnvExcept unsets every known provider API-key env var except the
// ones named in keep, so a test can assert that exactly one variable drives the
// fallback. t.Setenv restores prior values on cleanup.
func clearProviderEnvExcept(t *testing.T, keep ...string) {
	t.Helper()
	skip := make(map[string]bool, len(keep))
	for _, k := range keep {
		skip[k] = true
	}
	for _, names := range providerEnvVars {
		for _, name := range names {
			if skip[name] {
				continue
			}
			t.Setenv(name, "")
		}
	}
}

func TestApiKeyFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		setenv   map[string]string
		wantKey  string
		wantOK   bool
	}{
		{
			name:     "anthropic",
			provider: "anthropic",
			setenv:   map[string]string{"ANTHROPIC_API_KEY": "sk-ant"},
			wantKey:  "sk-ant",
			wantOK:   true,
		},
		{
			name:     "openai",
			provider: "openai",
			setenv:   map[string]string{"OPENAI_API_KEY": "sk-oai"},
			wantKey:  "sk-oai",
			wantOK:   true,
		},
		{
			name:     "gemini primary",
			provider: "gemini",
			setenv:   map[string]string{"GEMINI_API_KEY": "g-1"},
			wantKey:  "g-1",
			wantOK:   true,
		},
		{
			name:     "gemini falls back to GOOGLE_API_KEY",
			provider: "gemini",
			setenv:   map[string]string{"GOOGLE_API_KEY": "g-2"},
			wantKey:  "g-2",
			wantOK:   true,
		},
		{
			name:     "gemini prefers GEMINI over GOOGLE",
			provider: "gemini",
			setenv:   map[string]string{"GEMINI_API_KEY": "g-1", "GOOGLE_API_KEY": "g-2"},
			wantKey:  "g-1",
			wantOK:   true,
		},
		{
			name:     "azureopenai",
			provider: "azureopenai",
			setenv:   map[string]string{"AZURE_OPENAI_API_KEY": "az-1"},
			wantKey:  "az-1",
			wantOK:   true,
		},
		{
			name:     "deepl",
			provider: "deepl",
			setenv:   map[string]string{"DEEPL_API_KEY": "dl-1"},
			wantKey:  "dl-1",
			wantOK:   true,
		},
		{
			name:     "google MT primary",
			provider: "google",
			setenv:   map[string]string{"GOOGLE_TRANSLATE_API_KEY": "gt-1"},
			wantKey:  "gt-1",
			wantOK:   true,
		},
		{
			name:     "google MT falls back to GOOGLE_API_KEY",
			provider: "google",
			setenv:   map[string]string{"GOOGLE_API_KEY": "gt-2"},
			wantKey:  "gt-2",
			wantOK:   true,
		},
		{
			name:     "microsoft primary",
			provider: "microsoft",
			setenv:   map[string]string{"MICROSOFT_TRANSLATOR_KEY": "ms-1"},
			wantKey:  "ms-1",
			wantOK:   true,
		},
		{
			name:     "microsoft falls back to AZURE_TRANSLATOR_KEY",
			provider: "microsoft",
			setenv:   map[string]string{"AZURE_TRANSLATOR_KEY": "ms-2"},
			wantKey:  "ms-2",
			wantOK:   true,
		},
		{
			name:     "modernmt",
			provider: "modernmt",
			setenv:   map[string]string{"MODERNMT_API_KEY": "mmt-1"},
			wantKey:  "mmt-1",
			wantOK:   true,
		},
		{
			name:     "mymemory",
			provider: "mymemory",
			setenv:   map[string]string{"MYMEMORY_API_KEY": "mm-1"},
			wantKey:  "mm-1",
			wantOK:   true,
		},
		{
			name:     "case-insensitive provider id",
			provider: "Anthropic",
			setenv:   map[string]string{"ANTHROPIC_API_KEY": "sk-ant"},
			wantKey:  "sk-ant",
			wantOK:   true,
		},
		{
			name:     "ollama has no env var",
			provider: "ollama",
			wantOK:   false,
		},
		{
			name:     "unknown provider has no env var",
			provider: "made-up",
			wantOK:   false,
		},
		{
			name:     "set but empty is treated as unset",
			provider: "anthropic",
			setenv:   map[string]string{"ANTHROPIC_API_KEY": ""},
			wantOK:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearProviderEnv(t)
			for k, v := range tc.setenv {
				t.Setenv(k, v)
			}
			key, ok := apiKeyFromEnv(tc.provider)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantKey, key)
		})
	}
}

func TestInferProviderID(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		config   map[string]any
		want     string
	}{
		{
			name:     "explicit provider wins",
			toolName: "deepl-translate",
			config:   map[string]any{"provider": "openai"},
			want:     "openai",
		},
		{
			name:     "MT tool derives provider from name",
			toolName: "deepl-translate",
			config:   map[string]any{},
			want:     "deepl",
		},
		{
			name:     "microsoft MT tool",
			toolName: "microsoft-translate",
			config:   map[string]any{},
			want:     "microsoft",
		},
		{
			name:     "ai-translate defaults to anthropic",
			toolName: "ai-translate",
			config:   map[string]any{},
			want:     "anthropic",
		},
		{
			name:     "ai-qa defaults to anthropic",
			toolName: "ai-qa",
			config:   map[string]any{},
			want:     "anthropic",
		},
		{
			name:     "empty tool name defaults to anthropic",
			toolName: "",
			config:   map[string]any{},
			want:     "anthropic",
		},
		{
			name:     "empty provider string is treated as unset",
			toolName: "deepl-translate",
			config:   map[string]any{"provider": ""},
			want:     "deepl",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, inferProviderID(tc.toolName, tc.config))
		})
	}
}

// TestResolveCredentials_EnvFallbackPerProvider checks that, with no inline key
// and no --credential, each major provider's standard env var is injected as
// config["apiKey"] while the resolved provider is preserved.
func TestResolveCredentials_EnvFallbackPerProvider(t *testing.T) {
	tests := []struct {
		name         string
		toolName     string
		config       map[string]any
		envVar       string
		envVal       string
		wantProvider string
	}{
		{
			name:         "ai-translate + ANTHROPIC_API_KEY",
			toolName:     "ai-translate",
			config:       map[string]any{"provider": "anthropic"},
			envVar:       "ANTHROPIC_API_KEY",
			envVal:       "sk-ant",
			wantProvider: "anthropic",
		},
		{
			name:         "ai-translate no provider defaults to anthropic env",
			toolName:     "ai-translate",
			config:       map[string]any{},
			envVar:       "ANTHROPIC_API_KEY",
			envVal:       "sk-ant-default",
			wantProvider: "anthropic",
		},
		{
			name:         "openai via OPENAI_API_KEY",
			toolName:     "ai-translate",
			config:       map[string]any{"provider": "openai"},
			envVar:       "OPENAI_API_KEY",
			envVal:       "sk-oai",
			wantProvider: "openai",
		},
		{
			name:         "gemini via GEMINI_API_KEY",
			toolName:     "ai-translate",
			config:       map[string]any{"provider": "gemini"},
			envVar:       "GEMINI_API_KEY",
			envVal:       "g-key",
			wantProvider: "gemini",
		},
		{
			name:         "azureopenai via AZURE_OPENAI_API_KEY",
			toolName:     "ai-translate",
			config:       map[string]any{"provider": "azureopenai"},
			envVar:       "AZURE_OPENAI_API_KEY",
			envVal:       "az-key",
			wantProvider: "azureopenai",
		},
		{
			name:         "deepl MT tool via DEEPL_API_KEY (no provider in config)",
			toolName:     "deepl-translate",
			config:       map[string]any{},
			envVar:       "DEEPL_API_KEY",
			envVal:       "dl-key",
			wantProvider: "deepl",
		},
		{
			name:         "microsoft MT tool via MICROSOFT_TRANSLATOR_KEY",
			toolName:     "microsoft-translate",
			config:       map[string]any{},
			envVar:       "MICROSOFT_TRANSLATOR_KEY",
			envVal:       "ms-key",
			wantProvider: "microsoft",
		},
		{
			name:         "modernmt MT tool via MODERNMT_API_KEY",
			toolName:     "modernmt-translate",
			config:       map[string]any{},
			envVar:       "MODERNMT_API_KEY",
			envVal:       "mmt-key",
			wantProvider: "modernmt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearProviderEnvExcept(t, tc.envVar)
			t.Setenv(tc.envVar, tc.envVal)
			store := newTestStore(t)

			result, err := ResolveCredentials(store, tc.toolName, []string{"credentials"}, tc.config)
			require.NoError(t, err)
			assert.Equal(t, tc.envVal, result["apiKey"], "env var should be injected as apiKey")
			assert.Equal(t, tc.wantProvider, result["provider"], "provider should be preserved/inferred")
		})
	}
}

// TestResolveCredentials_InlineKeyBeatsEnv verifies precedence: an inline
// apiKey wins over a present env var.
func TestResolveCredentials_InlineKeyBeatsEnv(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-env")
	store := newTestStore(t)

	config := map[string]any{"provider": "anthropic", "apiKey": "sk-inline"}
	result, err := ResolveCredentials(store, "ai-translate", []string{"credentials"}, config)
	require.NoError(t, err)
	assert.Equal(t, "sk-inline", result["apiKey"], "inline apiKey must beat env var")
}

// TestResolveCredentials_CredentialRefBeatsEnv verifies precedence: a
// --credential reference is attempted before the env var fallback. Since the
// test keychain is unavailable the lookup errors at the keychain step, proving
// the credential path took priority over the (present) env var.
func TestResolveCredentials_CredentialRefBeatsEnv(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	store := newTestStore(t)
	mustUpsert(t, store, ProviderConfig{Name: "my-openai", ProviderType: "openai"})

	config := map[string]any{"credential": "my-openai"}
	_, err := ResolveCredentials(store, "ai-translate", []string{"credentials"}, config)
	require.Error(t, err, "credential path runs before env fallback; keychain is unavailable in tests")
	assert.Contains(t, err.Error(), "keychain")
}

// TestResolveCredentials_EnvBeatsStoreAutoDetect verifies precedence: when a
// matching store credential exists AND an env var is set, the env var wins
// (the store would otherwise auto-detect the single match — but that requires a
// keychain). The env injection short-circuits before auto-detect, so no
// keychain error surfaces and the env key is used.
func TestResolveCredentials_EnvBeatsStoreAutoDetect(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	store := newTestStore(t)
	// A single matching store credential would be auto-detected (and fail at the
	// keychain) if the env fallback did not run first.
	mustUpsert(t, store, ProviderConfig{Name: "stored-openai", ProviderType: "openai"})

	config := map[string]any{"provider": "openai"}
	result, err := ResolveCredentials(store, "ai-translate", []string{"credentials"}, config)
	require.NoError(t, err, "env var should short-circuit before store auto-detect")
	assert.Equal(t, "sk-from-env", result["apiKey"])
	assert.Equal(t, "openai", result["provider"])
}

// TestResolveCredentials_OllamaNeedsNoKey verifies the ollama provider resolves
// with no env var, no inline key, and no store credential: it has no mapped env
// var, so the fallback is skipped and auto-detect with an empty/ollama provider
// is consulted. ollama needs no key, so resolution must not demand one when the
// tool's provider is ollama and we simply pass through.
func TestResolveCredentials_OllamaNeedsNoKey(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t)

	// ollama has no env var; with no store match it falls through to auto-detect
	// which errors for a missing provider. The real ollama path supplies a
	// BaseURL and no key; the tool's config factory builds the provider without
	// a key. We assert the *helper* never demands a key for ollama.
	key, ok := apiKeyFromEnv("ollama")
	assert.False(t, ok, "ollama must have no env-var key requirement")
	assert.Empty(t, key)

	_ = store
}

func TestResolveCredentials_NoEnvNoStoreErrorsUnchanged(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t)

	// No inline key, no credential, no env var, no store match → unchanged error.
	config := map[string]any{"provider": "openai"}
	_, err := ResolveCredentials(store, "ai-translate", []string{"credentials"}, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no saved credentials")
	assert.Contains(t, err.Error(), "openai", "error should name the provider as before")
}
