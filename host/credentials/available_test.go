package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Available answers from the store's metadata: a saved credential whose key
// the keychain does not hold is available to it, while ResolveCredentials,
// which reads the key, fails on the keychain.
func TestAvailable_ReadsNoKey(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t)
	mustUpsert(t, store, ProviderConfig{Name: "work", ProviderType: "openai"})

	for name, config := range map[string]map[string]any{
		"named":         {"credential": "work"},
		"auto-detected": {},
	} {
		t.Run(name, func(t *testing.T) {
			p, err := Available(store, "translate", []string{"credentials"}, config)
			require.NoError(t, err)
			assert.Equal(t, "openai", p, "the provider comes from the saved credential")

			_, err = ResolveCredentials(store, "translate", []string{"credentials"}, config)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "keychain", "only the run reads the key")
		})
	}
}

// Where ResolveCredentials stops before reading a key, Available returns the
// same error, and names the provider the run would call.
func TestAvailable_SaysWhatTheRunSays(t *testing.T) {
	for _, tc := range []struct {
		name     string
		config   map[string]any
		provider string
	}{
		{name: "no key for the default", config: map[string]any{}, provider: "anthropic"},
		{name: "no key for a named provider", config: map[string]any{"provider": "openai"}, provider: "openai"},
		{name: "a credential that is not saved", config: map[string]any{"credential": "nosuch"}, provider: "anthropic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearProviderEnv(t)
			store := newTestStore(t)
			p, aerr := Available(store, "translate", []string{"credentials"}, tc.config)
			_, rerr := ResolveCredentials(store, "translate", []string{"credentials"}, tc.config)
			require.Error(t, aerr)
			require.Error(t, rerr)
			assert.Equal(t, rerr.Error(), aerr.Error())
			assert.Equal(t, tc.provider, p)
		})
	}
}

// The missing-key message names the provider's environment variable where it
// has one, so the advice fixes the case it reports.
func TestMissingKeyMessage(t *testing.T) {
	assert.Contains(t, MissingKeyMessage("anthropic"), "ANTHROPIC_API_KEY")
	assert.Contains(t, MissingKeyMessage("openai"), "OPENAI_API_KEY")
	assert.Contains(t, MissingKeyMessage("nosuchprovider"), "kapi credentials add")
	assert.NotContains(t, MissingKeyMessage("nosuchprovider"), "_API_KEY")
}
