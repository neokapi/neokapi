package credentials

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), "providers.json"))
}

// clearProviderEnv unsets every known provider API-key env var for the
// duration of the test so env-var fallback does not interfere with tests that
// exercise the store / error paths. t.Setenv restores the prior value on
// cleanup and also forbids t.Parallel, keeping these tests deterministic.
func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, names := range providerEnvVars {
		for _, name := range names {
			t.Setenv(name, "")
		}
	}
}

func TestResolveCredentials_NoRequirement(t *testing.T) {
	store := newTestStore(t)
	config := map[string]any{"batchSize": 50}

	result, err := ResolveCredentials(store, "translate", []string{"target-language"}, config)
	require.NoError(t, err)
	assert.Equal(t, config, result, "should return unchanged when no credentials requirement")
}

func TestResolveCredentials_ExplicitAPIKey(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t)
	config := map[string]any{
		"provider": "openai",
		"apiKey":   "sk-explicit",
	}

	result, err := ResolveCredentials(store, "translate", []string{"credentials"}, config)
	require.NoError(t, err)
	assert.Equal(t, "sk-explicit", result["apiKey"], "explicit apiKey should win")
}

func TestResolveCredentials_ByName(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t)
	cfg := mustUpsert(t, store, ProviderConfig{
		Name:         "my-openai",
		ProviderType: "openai",
		Model:        "gpt-4o",
	})
	// Can't test SetAPIKey without a real keychain, so test the error path.
	config := map[string]any{"credential": "my-openai"}

	_, err := ResolveCredentials(store, "translate", []string{"credentials"}, config)
	// Will fail because no keychain available in test, but should resolve the config first.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keychain")

	_ = cfg // suppress unused
}

func TestResolveCredentials_NotFound(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t)
	config := map[string]any{"credential": "nonexistent"}

	_, err := ResolveCredentials(store, "translate", []string{"credentials"}, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveCredentials_AutoDetectEmpty(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t)
	config := map[string]any{}

	_, err := ResolveCredentials(store, "", []string{"credentials"}, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no saved credentials")
}

func TestResolveCredentials_AutoDetectMultiple(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t)
	mustUpsert(t, store, ProviderConfig{Name: "A", ProviderType: "openai"})
	mustUpsert(t, store, ProviderConfig{Name: "B", ProviderType: "openai"})

	config := map[string]any{"provider": "openai"}

	_, err := ResolveCredentials(store, "translate", []string{"credentials"}, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple credentials")

	// The error must be the typed AmbiguousCredentialError so GUIs can catch
	// it (and avoid surfacing the CLI's --credential hint), carrying the
	// candidate names and the provider it was filtered by.
	var amb *AmbiguousCredentialError
	require.ErrorAs(t, err, &amb)
	assert.Equal(t, "openai", amb.Provider)
	assert.ElementsMatch(t, []string{"A", "B"}, amb.Candidates)
}

func TestResolveCredentials_DefaultKeyDisambiguates(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t)
	mustUpsert(t, store, ProviderConfig{Name: "A", ProviderType: "openai"})
	b := mustUpsert(t, store, ProviderConfig{Name: "B", ProviderType: "openai"})
	require.NoError(t, store.SetDefault(b.ID))

	_, err := ResolveCredentials(store, "translate", []string{"credentials"}, map[string]any{"provider": "openai"})
	require.Error(t, err)

	// With a default marked, resolution must NOT be ambiguous — it picks "B".
	// (No real keychain in tests, so the only remaining failure is the missing
	// key for the chosen credential.)
	var amb *AmbiguousCredentialError
	assert.NotErrorAs(t, err, &amb, "the default-marked credential should disambiguate")
	assert.Contains(t, err.Error(), "keychain")
	assert.Contains(t, err.Error(), `"B"`)
}

func TestStore_SetDefaultIsExclusivePerProvider(t *testing.T) {
	store := newTestStore(t)
	a := mustUpsert(t, store, ProviderConfig{Name: "A", ProviderType: "openai"})
	b := mustUpsert(t, store, ProviderConfig{Name: "B", ProviderType: "openai"})
	other := mustUpsert(t, store, ProviderConfig{Name: "C", ProviderType: "anthropic"})

	require.NoError(t, store.SetDefault(a.ID))
	require.NoError(t, store.SetDefault(b.ID)) // moves the default within openai

	byID := func(id string) ProviderConfig {
		c, err := store.Get(id)
		require.NoError(t, err)
		return c
	}
	assert.False(t, byID(a.ID).Default, "A should no longer be default")
	assert.True(t, byID(b.ID).Default, "B should be the openai default")
	assert.False(t, byID(other.ID).Default, "a different provider is unaffected")
}

func TestStore_UpsertDedupesByName(t *testing.T) {
	store := newTestStore(t)

	first := mustUpsert(t, store, ProviderConfig{Name: "harness-gemini", ProviderType: "gemini"})
	// Re-adding the same name (no ID) must update the existing record in place,
	// not create a second indistinguishable entry that breaks auto-detect.
	second := mustUpsert(t, store, ProviderConfig{Name: "harness-gemini", ProviderType: "gemini", Model: "gemini-2.0-flash"})

	assert.Equal(t, first.ID, second.ID, "same name should reuse the existing id")
	all := store.FindByType("gemini")
	require.Len(t, all, 1, "must not keep two same-named credentials")
	assert.Equal(t, "gemini-2.0-flash", all[0].Model, "the update should win")
}

func TestResolveCredentials_KeylessLocalProviders(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t) // empty: a remote provider would error here
	for _, provider := range []string{"ollama", "demo"} {
		t.Run(provider, func(t *testing.T) {
			config := map[string]any{"provider": provider}
			got, err := ResolveCredentials(store, "translate", []string{"credentials"}, config)
			require.NoError(t, err, "keyless local provider must not require a credential")
			assert.Equal(t, provider, got["provider"])
			_, hasKey := got["apiKey"]
			assert.False(t, hasKey, "no apiKey injected for keyless provider")
		})
	}
}

func TestMergeCredentials(t *testing.T) {
	config := map[string]any{
		"credential": "my-cred",
		"batchSize":  50,
	}
	cred := &ProviderConfigWithKey{
		ProviderConfig: ProviderConfig{
			ProviderType: "anthropic",
			Model:        "claude-sonnet-4-5-20250514",
		},
		APIKey: "sk-test-key",
	}

	result := mergeCredentials(config, cred)

	// credential key should be removed.
	_, hasCredential := result["credential"]
	assert.False(t, hasCredential, "credential key should be removed")

	// Injected values.
	assert.Equal(t, "anthropic", result["provider"])
	assert.Equal(t, "sk-test-key", result["apiKey"])
	assert.Equal(t, "claude-sonnet-4-5-20250514", result["model"])

	// Original values preserved.
	assert.Equal(t, 50, result["batchSize"])

	// Original config not modified.
	assert.Equal(t, "my-cred", config["credential"], "original config should not be modified")
}

func TestMergeCredentials_PreservesExplicitProvider(t *testing.T) {
	config := map[string]any{
		"provider": "openai",
	}
	cred := &ProviderConfigWithKey{
		ProviderConfig: ProviderConfig{
			ProviderType: "anthropic",
		},
		APIKey: "sk-key",
	}

	result := mergeCredentials(config, cred)
	assert.Equal(t, "openai", result["provider"], "explicit provider should be preserved")
}

// TestProviderInferenceFromCredential covers issue #637: when --credential is
// given without an explicit --provider, the provider must be inferred from the
// credential's provider_type rather than defaulting to "anthropic".
//
// The fix is in the callers (toolcmds.go, flow.go): they drop the schema/flag
// default "anthropic" from the config when a credential name is given and
// --provider was not explicitly changed. These tests verify the resulting
// mergeCredentials behaviour once the default has been stripped.
func TestProviderInferenceFromCredential(t *testing.T) {
	tests := []struct {
		name             string
		config           map[string]any // config as the CLI hands it to mergeCredentials
		credProviderType string
		wantProvider     string
		desc             string
	}{
		{
			name:             "no provider in config uses credential provider",
			config:           map[string]any{"credential": "harness-gemini"},
			credProviderType: "gemini",
			wantProvider:     "gemini",
			desc:             "credential provider_type must win when --provider is absent",
		},
		{
			name:             "explicit provider overrides credential provider",
			config:           map[string]any{"credential": "harness-gemini", "provider": "openai"},
			credProviderType: "gemini",
			wantProvider:     "openai",
			desc:             "explicit --provider must override the credential's provider_type",
		},
		{
			name:             "no credential falls back to default anthropic",
			config:           map[string]any{"provider": "anthropic"},
			credProviderType: "anthropic",
			wantProvider:     "anthropic",
			desc:             "without a credential, provider from config is preserved unchanged",
		},
		{
			name:             "credential with empty provider in config uses credential",
			config:           map[string]any{"credential": "my-cred", "provider": ""},
			credProviderType: "openai",
			wantProvider:     "openai",
			desc:             "empty provider string treated as unset",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cred := &ProviderConfigWithKey{
				ProviderConfig: ProviderConfig{
					ProviderType: tc.credProviderType,
				},
				APIKey: "sk-test",
			}
			result := mergeCredentials(tc.config, cred)
			assert.Equal(t, tc.wantProvider, result["provider"], tc.desc)
		})
	}
}

func TestGetByName(t *testing.T) {
	store := newTestStore(t)
	mustUpsert(t, store, ProviderConfig{Name: "My Key", ProviderType: "anthropic"})

	// Exact match.
	cfg, err := store.GetByName("My Key")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.ProviderType)

	// Case-insensitive.
	cfg, err = store.GetByName("my key")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.ProviderType)

	// Not found.
	_, err = store.GetByName("nonexistent")
	assert.Error(t, err)
}

func TestFindByType(t *testing.T) {
	store := newTestStore(t)
	mustUpsert(t, store, ProviderConfig{Name: "A", ProviderType: "anthropic"})
	mustUpsert(t, store, ProviderConfig{Name: "B", ProviderType: "openai"})
	mustUpsert(t, store, ProviderConfig{Name: "C", ProviderType: "anthropic"})

	// Filter by type.
	anthropic := store.FindByType("anthropic")
	assert.Len(t, anthropic, 2)

	openai := store.FindByType("openai")
	assert.Len(t, openai, 1)

	// Empty type returns all.
	all := store.FindByType("")
	assert.Len(t, all, 3)

	// No match.
	none := store.FindByType("gemini")
	assert.Empty(t, none)
}
