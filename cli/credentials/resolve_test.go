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

func TestResolveCredentials_NoRequirement(t *testing.T) {
	store := newTestStore(t)
	config := map[string]any{"batchSize": 50}

	result, err := ResolveCredentials(store, []string{"target-language"}, config)
	require.NoError(t, err)
	assert.Equal(t, config, result, "should return unchanged when no credentials requirement")
}

func TestResolveCredentials_ExplicitAPIKey(t *testing.T) {
	store := newTestStore(t)
	config := map[string]any{
		"provider": "openai",
		"apiKey":   "sk-explicit",
	}

	result, err := ResolveCredentials(store, []string{"credentials"}, config)
	require.NoError(t, err)
	assert.Equal(t, "sk-explicit", result["apiKey"], "explicit apiKey should win")
}

func TestResolveCredentials_ByName(t *testing.T) {
	store := newTestStore(t)
	cfg := store.Upsert(ProviderConfig{
		Name:         "my-openai",
		ProviderType: "openai",
		Model:        "gpt-4o",
	})
	// Can't test SetAPIKey without a real keychain, so test the error path.
	config := map[string]any{"credential": "my-openai"}

	_, err := ResolveCredentials(store, []string{"credentials"}, config)
	// Will fail because no keychain available in test, but should resolve the config first.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keychain")

	_ = cfg // suppress unused
}

func TestResolveCredentials_NotFound(t *testing.T) {
	store := newTestStore(t)
	config := map[string]any{"credential": "nonexistent"}

	_, err := ResolveCredentials(store, []string{"credentials"}, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveCredentials_AutoDetectEmpty(t *testing.T) {
	store := newTestStore(t)
	config := map[string]any{}

	_, err := ResolveCredentials(store, []string{"credentials"}, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no saved credentials")
}

func TestResolveCredentials_AutoDetectMultiple(t *testing.T) {
	store := newTestStore(t)
	store.Upsert(ProviderConfig{Name: "A", ProviderType: "openai"})
	store.Upsert(ProviderConfig{Name: "B", ProviderType: "openai"})

	config := map[string]any{"provider": "openai"}

	_, err := ResolveCredentials(store, []string{"credentials"}, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple credentials")
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

func TestGetByName(t *testing.T) {
	store := newTestStore(t)
	store.Upsert(ProviderConfig{Name: "My Key", ProviderType: "anthropic"})

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
	store.Upsert(ProviderConfig{Name: "A", ProviderType: "anthropic"})
	store.Upsert(ProviderConfig{Name: "B", ProviderType: "openai"})
	store.Upsert(ProviderConfig{Name: "C", ProviderType: "anthropic"})

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
