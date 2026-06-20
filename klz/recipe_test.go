package klz

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// sampleRecipe builds a full project recipe with flows, defaults, and
// side-effecting Extras (server/hooks) plus an inert custom extra.
func sampleRecipe(t *testing.T) *project.KapiProject {
	t.Helper()
	r := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "demo",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
		},
		Flows: map[string]*flow.StepsSpec{
			"translate": {Steps: []flow.FlowStep{{Tool: "translate"}}},
		},
	}
	// Side-effecting Extras that SanitizeRecipe must strip.
	for _, k := range []string{"server", "hooks", "automations"} {
		var n yaml.Node
		require.NoError(t, n.Encode(map[string]any{"url": "https://example.test"}))
		if r.Extras == nil {
			r.Extras = map[string]yaml.Node{}
		}
		r.Extras[k] = n
	}
	// An inert custom extra that must survive.
	var keep yaml.Node
	require.NoError(t, keep.Encode(map[string]any{"flavor": "vanilla"}))
	r.Extras["custom"] = keep
	return r
}

// TestRecipeRoundTrip verifies the full recipe (flows + defaults) survives a
// pack/unpack, while side-effecting Extras are stripped by SanitizeRecipe and
// inert Extras travel.
func TestRecipeRoundTrip(t *testing.T) {
	recipe := SanitizeRecipe(sampleRecipe(t))
	require.NotContains(t, recipe.Extras, "server")
	require.NotContains(t, recipe.Extras, "hooks")
	require.NotContains(t, recipe.Extras, "automations")
	require.Contains(t, recipe.Extras, "custom")

	pkg := &Package{Recipe: recipe, Overlays: sampleOverlays()}
	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)

	require.NotNil(t, got.Recipe)
	assert.Equal(t, "demo", got.Recipe.Name)
	assert.Equal(t, model.LocaleID("en-US"), got.Recipe.Defaults.SourceLanguage)
	assert.Equal(t, []model.LocaleID{"fr-FR", "de-DE"}, got.Recipe.Defaults.TargetLanguages)
	require.Contains(t, got.Recipe.Flows, "translate")
	require.Len(t, got.Recipe.Flows["translate"].Steps, 1)
	assert.Equal(t, "translate", got.Recipe.Flows["translate"].Steps[0].Tool)

	// Inert extra survived; side-effecting ones did not.
	assert.Contains(t, got.Recipe.Extras, "custom")
	assert.NotContains(t, got.Recipe.Extras, "server")
	assert.NotContains(t, got.Recipe.Extras, "hooks")
	assert.NotContains(t, got.Recipe.Extras, "automations")
}

// TestRecipeNotInRootHash verifies the recipe is manifest metadata: a package's
// content RootHash is identical whether the recipe is nil or set.
func TestRecipeNotInRootHash(t *testing.T) {
	withRecipe := &Package{Recipe: SanitizeRecipe(sampleRecipe(t)), Overlays: sampleOverlays()}
	noRecipe := &Package{Overlays: sampleOverlays()}

	r1, err := withRecipe.RootHash()
	require.NoError(t, err)
	r2, err := noRecipe.RootHash()
	require.NoError(t, err)
	assert.Equal(t, r2, r1, "recipe must not affect the content RootHash")
}

// TestSanitizeRecipeNil verifies SanitizeRecipe handles nil.
func TestSanitizeRecipeNil(t *testing.T) {
	assert.Nil(t, SanitizeRecipe(nil))
}

// TestWorkspaceMetaRoundTrip verifies the klz workspace Extras (output layout +
// workspace marker) round-trip through the recipe.
func TestWorkspaceMetaRoundTrip(t *testing.T) {
	r := &project.KapiProject{Version: project.CurrentVersion}
	require.NoError(t, SetRecipeWorkspaceMeta(r, WorkspaceMeta{Out: "l10n/{lang}/{name}.{ext}", Workspace: true}))

	pkg := &Package{Recipe: r, Overlays: sampleOverlays()}
	data, err := pkg.Marshal()
	require.NoError(t, err)
	got, err := Unmarshal(data)
	require.NoError(t, err)

	meta := RecipeWorkspaceMeta(got.Recipe)
	assert.Equal(t, "l10n/{lang}/{name}.{ext}", meta.Out)
	assert.True(t, meta.Workspace)
}
