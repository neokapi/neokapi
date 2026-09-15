package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// ignoredGovernedProject is governedProject with the platform collection at
// platform/docs, a .kapiignore that ignores platform/ignored.md, and both that
// file and a sibling platform/kept.md on disk.
func ignoredGovernedProject(t *testing.T) (recipe, root string, proj *project.KapiProject) {
	t.Helper()
	recipe, root = governedProject(t, "platform/docs")
	for rel, body := range map[string]string{
		".kapiignore":         "platform/ignored.md\n",
		"platform/ignored.md": "# Ignored\n",
		"platform/kept.md":    "# Kept\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	return recipe, root, proj
}

// A run over a file the project's ignore rules match carries the project's
// default bindings, and a flow's inputs group that file with the default point.
// A sibling the rules leave alone carries its collection's.
func TestAnIgnoredInputRunsAtTheDefaultPoint(t *testing.T) {
	recipe, root, proj := ignoredGovernedProject(t)
	ignored := filepath.Join(root, "platform", "ignored.md")
	kept := filepath.Join(root, "platform", "kept.md")

	t.Run("a run's bindings", func(t *testing.T) {
		for input, voice := range map[string]string{ignored: "House Style", kept: "Platform Voice"} {
			bindings := (&App{}).resolveRunBindings(input, bindingsCmd(t, recipe))
			if assert.NotNil(t, bindings, input) && assert.NotNil(t, bindings.profile, input) {
				assert.Equal(t, voice, bindings.profile.Name, input)
			}
		}
	})

	t.Run("a flow's inputs grouped by binding", func(t *testing.T) {
		defaults, err := proj.ResolveGovernanceFor(project.GovernancePoint{})
		require.NoError(t, err)
		groups, err := (&App{}).groupInputsByBinding(bindingsCmd(t, recipe), proj, root, []string{ignored, kept})
		require.NoError(t, err)
		points := map[string]string{}
		for _, g := range groups {
			rc, err := proj.ResolveGovernanceFor(g.Point)
			require.NoError(t, err)
			for _, in := range g.Inputs {
				points[in] = rc.Ref().String()
			}
		}
		assert.Equal(t, map[string]string{ignored: defaults.Ref().String(), kept: "platform/docs"}, points)
	})
}
