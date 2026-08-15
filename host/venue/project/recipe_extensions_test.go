package project

import (
	"reflect"
	"strings"
	"testing"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/venue/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recipeExtensionFields returns the YAML keys of Recipe's own top-level
// fields — the bowrain extension blocks it adds beside the inline
// KapiProject (whose yaml tag is ",inline" and therefore skipped).
func recipeExtensionFields(t *testing.T) []string {
	t.Helper()
	rt := reflect.TypeFor[Recipe]()
	var names []string
	for f := range rt.Fields() {
		tag := f.Tag.Get("yaml")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			continue // the inline KapiProject embed
		}
		names = append(names, name)
	}
	return names
}

// TestRecipe_CoversAllRegisteredProjectExtensions is the schema-extension
// drift guard: every project-scope extension registered by
// host/venue/schema must have a matching typed field on Recipe (and
// vice versa). When host/venue/schema registers a new top-level block,
// this fails until Recipe grows the field — and the probe test below fails
// until Recipe.Validate handles it — so a new block can't be silently
// missed by the hand-written fan-out.
func TestRecipe_CoversAllRegisteredProjectExtensions(t *testing.T) {
	registered := coreproj.RegisteredExtensions(coreproj.ScopeProject)
	require.NotEmpty(t, registered, "host/venue/schema should have registered project-scope extensions")

	// This module only links the bowrain schema, so every project-scope
	// registration must belong to the bowrain group.
	for _, name := range registered {
		group, ok := coreproj.ExtensionRegistered(coreproj.ScopeProject, name)
		require.True(t, ok)
		require.Equal(t, schema.Group, group, "unexpected extension group for %q", name)
	}

	assert.ElementsMatch(t, registered, recipeExtensionFields(t),
		"Recipe's typed extension fields must match the project-scope extensions registered by host/venue/schema — add the missing field AND its Recipe.Validate fan-out")
}

// TestRecipeValidate_FanOutPerExtensionBlock pins Recipe.Validate's
// hand-coded fan-out to the registered extension set: the probe table must
// name every registered project-scope block, and each probe (an invalid
// value for that block) must make Validate fail with the block name in the
// error. A nil probe records the conscious decision that the block's spec
// Validate is a no-op today; replace it with a real probe when the spec
// grows validation rules.
func TestRecipeValidate_FanOutPerExtensionBlock(t *testing.T) {
	probes := map[string]func(*Recipe){
		schema.VenueKey: func(r *Recipe) {
			r.Server = &ServerSpec{URL: "ftp://bowrain.example.com/team/proj"}
		},
		"hooks": func(r *Recipe) {
			r.Hooks = HooksSpec{"not-a-trigger": {"qa"}}
		},
		"automations": func(r *Recipe) {
			r.Automations = []AutomationSpec{{ /* missing name */ }}
		},
		"assets":      nil, // AssetsSpec.Validate is a no-op today
		"brand_voice": nil, // BrandVoiceSpec.Validate is a no-op today
	}

	registered := coreproj.RegisteredExtensions(coreproj.ScopeProject)
	probeKeys := make([]string, 0, len(probes))
	for k := range probes {
		probeKeys = append(probeKeys, k)
	}
	require.ElementsMatch(t, registered, probeKeys,
		"add a Validate probe (or a documented nil) for every registered project-scope extension block")

	baseRecipe := func() *Recipe {
		r := &Recipe{}
		r.Version = coreproj.CurrentVersion
		r.Name = "probe"
		return r
	}
	require.NoError(t, baseRecipe().Validate(), "the base recipe must be valid so probe failures are attributable")

	for name, probe := range probes {
		if probe == nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			r := baseRecipe()
			probe(r)
			err := r.Validate()
			require.Error(t, err, "Recipe.Validate must fan out to the %s block", name)
			assert.Contains(t, err.Error(), name,
				"the %s block's validation error should name the block", name)
		})
	}
}
