package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/project/projecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The positional `kapi config <key> [value]` surface: core recipe fields
// natively, registered extension blocks generically (created on first set),
// unknown keys rejected with an actionable hint.
func TestRecipeConfig_GetSet(t *testing.T) {
	projecttest.ResetExtensions()
	defer projecttest.ResetExtensions()
	project.RegisterExtensionGroup("platform", []project.Extension{
		{Name: "server", Scope: project.ScopeProject},
	})

	root := t.TempDir()
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: app
defaults:
  source_language: en
`), 0o644))

	// Core fields, both spellings.
	v, err := RecipeConfigGet(recipe, "name")
	require.NoError(t, err)
	assert.Equal(t, "app", v)
	v, err = RecipeConfigGet(recipe, "defaults.source_language")
	require.NoError(t, err)
	assert.Equal(t, "en", v)

	require.NoError(t, RecipeConfigSet(recipe, "project.name", "My App"))
	v, err = RecipeConfigGet(recipe, "name")
	require.NoError(t, err)
	assert.Equal(t, "My App", v)

	// Extension key: setting server.url creates the server: block.
	require.NoError(t, RecipeConfigSet(recipe, "server.url", "https://bowrain.example/acme/app"))
	v, err = RecipeConfigGet(recipe, "server.url")
	require.NoError(t, err)
	assert.Equal(t, "https://bowrain.example/acme/app", v)

	// The block survives a reload as a real extras node.
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	_, hasServer := proj.Extras["server"]
	assert.True(t, hasServer)

	// Unknown keys: never invented, hint names the accepted surface.
	_, err = RecipeConfigGet(recipe, "nonsense.key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown recipe key")
	err = RecipeConfigSet(recipe, "bogus", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kapi config get/set")
}
