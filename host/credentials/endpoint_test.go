package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// TestResolveCredentials_EndpointIsNotRecipeSettable is the invariant that keeps
// a provider key and the address it is sent to as one decision.
//
// A recipe is committable and is authored by whoever wrote the project
// directory; a credential is per-machine and belongs to the person kapi runs
// as. `schema:"-"` on the MT tool's BaseURL hides the field from the CLI and
// the form but does nothing here — core/schema.ApplyConfig is a json
// round-trip, so a step's config: map reaches anything with a json tag. The
// resolver therefore clears the key itself, on every path out.
func TestResolveCredentials_EndpointIsNotRecipeSettable(t *testing.T) {
	clearProviderEnv(t)
	store := newTestStore(t)

	// One case per distinct exit from ResolveCredentials.
	cases := []struct {
		name     string
		requires []string
		config   map[string]any
	}{
		{
			name:     "tool requires no credentials",
			requires: nil,
			config:   map[string]any{"baseURL": "https://elsewhere.invalid/"},
		},
		{
			name:     "inline api key short-circuits resolution",
			requires: []string{"credentials"},
			config:   map[string]any{"apiKey": "inline", "baseURL": "https://elsewhere.invalid/"},
		},
		{
			name:     "keyless provider needs no credential",
			requires: []string{"credentials"},
			config:   map[string]any{"provider": "ollama", "baseURL": "https://elsewhere.invalid/"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveCredentials(store, "translate", tc.requires, tc.config)
			require.NoError(t, err)
			assert.NotContains(t, got, "baseURL",
				"a config-supplied endpoint must not survive credential resolution")
		})
	}
}

// TestResolveCredentials_OnlyAStoredCredentialCarriesAnEndpoint states the
// consequence of the rule above, which is the part a user meets rather than the
// part a reviewer reads: a custom endpoint reaches a provider through a saved
// credential and through nothing else.
//
// Both of the other ways to supply a key resolve BEFORE any endpoint could be
// injected — an inline key returns immediately, and the environment fallback
// builds a provider config that has no endpoint to give. So a self-hosted
// endpoint plus `--api-key`, or a self-hosted endpoint plus OPENAI_API_KEY,
// silently calls the public host. `kapi credentials add --base-url` with
// `--credential` is the combination that works, and it is what the docs say.
func TestResolveCredentials_OnlyAStoredCredentialCarriesAnEndpoint(t *testing.T) {
	const endpoint = "https://mt.internal.example/v1"

	// The keychain is the developer's, so the key goes to the in-memory
	// keyring rather than the real one — the same guard cli's credential
	// tests use.
	keyring.MockInit()

	newStoreWithEndpoint := func(t *testing.T) *Store {
		t.Helper()
		store := newTestStore(t)
		saved, err := store.Upsert(ProviderConfig{
			Name: "internal", ProviderType: "openai", BaseURL: endpoint,
		})
		require.NoError(t, err)
		require.NoError(t, store.SetAPIKey(saved.ID, "stored-key"))
		return store
	}

	t.Run("an inline key never gets one", func(t *testing.T) {
		clearProviderEnv(t)
		got, err := ResolveCredentials(newStoreWithEndpoint(t), "translate",
			[]string{"credentials"}, map[string]any{"provider": "openai", "apiKey": "inline"})
		require.NoError(t, err)
		assert.NotContains(t, got, "baseURL",
			"an inline key short-circuits before an endpoint could be injected")
	})

	t.Run("an environment key never gets one", func(t *testing.T) {
		clearProviderEnv(t)
		t.Setenv("OPENAI_API_KEY", "from-the-environment")
		got, err := ResolveCredentials(newStoreWithEndpoint(t), "translate",
			[]string{"credentials"}, map[string]any{"provider": "openai"})
		require.NoError(t, err)
		assert.Equal(t, "from-the-environment", got["apiKey"])
		assert.NotContains(t, got, "baseURL",
			"the environment fallback carries a key and nothing else, saved endpoint or not")
	})

	t.Run("the named credential does", func(t *testing.T) {
		clearProviderEnv(t)
		got, err := ResolveCredentials(newStoreWithEndpoint(t), "translate",
			[]string{"credentials"}, map[string]any{"credential": "internal"})
		require.NoError(t, err)
		assert.Equal(t, endpoint, got["baseURL"],
			"the supported way to reach a self-hosted endpoint")
	})
}

// TestMergeCredentials_EndpointComesFromTheCredential: removing the recipe's say
// does not remove the capability. A self-hosted or private-cloud endpoint is
// configured beside the key that authenticates to it (`kapi credentials add
// --base-url`), and is injected from there.
func TestMergeCredentials_EndpointComesFromTheCredential(t *testing.T) {
	got := mergeCredentials(
		map[string]any{},
		&ProviderConfigWithKey{
			ProviderType: "openai",
			BaseURL:      "https://mt.internal.example/v1",
			APIKey:       "k",
		},
	)
	assert.Equal(t, "https://mt.internal.example/v1", got["baseURL"])
	assert.Equal(t, "k", got["apiKey"])
}

// TestMergeCredentials_NoEndpointWhenTheCredentialNamesNone: the ordinary case
// stays clean — no key appears merely because resolution ran.
func TestMergeCredentials_NoEndpointWhenTheCredentialNamesNone(t *testing.T) {
	got := mergeCredentials(
		map[string]any{},
		&ProviderConfigWithKey{
			ProviderType: "openai",
			APIKey:       "k",
		},
	)
	assert.NotContains(t, got, "baseURL")
}

// TestStripRecipeEndpoint_LeavesTheCallersMapAlone: callers still own the
// config they passed in, so the scrub copies rather than edits.
func TestStripRecipeEndpoint_LeavesTheCallersMapAlone(t *testing.T) {
	original := map[string]any{"baseURL": "https://elsewhere.invalid/", "model": "m"}
	got := stripRecipeEndpoint(original)

	assert.NotContains(t, got, "baseURL")
	assert.Equal(t, "m", got["model"], "unrelated keys survive")
	assert.Contains(t, original, "baseURL", "the caller's map is not mutated")

	// A config naming no endpoint is passed straight through, uncopied.
	clean := map[string]any{"model": "m"}
	assert.Equal(t, clean, stripRecipeEndpoint(clean))
}
