package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestMergeCredentials_EndpointComesFromTheCredential: removing the recipe's say
// does not remove the capability. A self-hosted or private-cloud endpoint is
// configured beside the key that authenticates to it (`kapi credentials add
// --base-url`), and is injected from there.
func TestMergeCredentials_EndpointComesFromTheCredential(t *testing.T) {
	got := mergeCredentials(
		map[string]any{},
		&ProviderConfigWithKey{
			ProviderConfig: ProviderConfig{
				ProviderType: "openai",
				BaseURL:      "https://mt.internal.example/v1",
			},
			APIKey: "k",
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
			ProviderConfig: ProviderConfig{ProviderType: "openai"},
			APIKey:         "k",
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
